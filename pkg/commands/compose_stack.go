package commands

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/lxc/incus/v7/shared/api"
)

// ComposeStack is a compose project directory: what its compose file
// declares, and how its instances are doing.
type ComposeStack struct {
	Dir string

	// Remote is the CLI remote the stack is pinned to, or empty for one
	// that follows the session's: the local stack, or one saved before
	// stacks had remotes.
	Remote string

	// Name is the compose project name, which is also the Incus project
	// incus-compose puts the stack in.
	Name     string
	Services []ComposeService

	// Err is why the stack's config couldn't be read - no directory, no
	// compose file - leaving Name and Services empty.
	Err error

	// Local is the stack lazyincus was started in, or pointed at with -P.
	// Saved is one listed in the state file. A stack can be both.
	Local bool
	Saved bool

	// Statuses are the daemon's statuses of the stack's compose instances,
	// by service.
	Statuses map[string][]string

	// StatusErr is why the stack's remote gave no statuses.
	StatusErr error
}

// Ref is how the stack is saved and shown: its directory, prefixed with
// the remote it's pinned to, the way the incus CLI writes `remote:name`.
func (s *ComposeStack) Ref() string {
	return StackRef(s.Remote, s.Dir)
}

// StackRef is remote and dir as one string, dir alone for no remote.
func StackRef(remote, dir string) string {
	if remote == "" {
		return dir
	}

	return remote + ":" + dir
}

// ParseStackRef splits a saved stack: its directory is absolute, so
// anything before a path's leading separator is the remote.
func ParseStackRef(ref string) (remote, dir string) {
	if strings.HasPrefix(ref, string(filepath.Separator)) {
		return "", ref
	}

	remote, dir, ok := strings.Cut(ref, ":")
	if !ok {
		return "", ref
	}

	return remote, dir
}

// SplitStackInput splits what was typed for a stack into a remote and a
// path. A prefix counts as a remote only when isRemote says it names one,
// so a directory with a colon in it still reads as a path.
func SplitStackInput(input string, isRemote func(string) bool) (remote, path string) {
	input = strings.TrimSpace(input)

	prefix, rest, ok := strings.Cut(input, ":")
	if !ok || strings.ContainsRune(prefix, filepath.Separator) || !isRemote(prefix) {
		return "", input
	}

	return prefix, rest
}

// Title is the stack's name, or its directory's while it has none.
func (s *ComposeStack) Title() string {
	if s.Name != "" {
		return s.Name
	}

	return filepath.Base(s.Dir)
}

// RemoteTitle is Title prefixed with the remote the stack is pinned to.
func (s *ComposeStack) RemoteTitle() string {
	return StackRef(s.Remote, s.Title())
}

// Status rolls every compose instance in the stack's project up the way a
// service rolls up its replicas.
func (s *ComposeStack) Status() string {
	var statuses []string

	for _, service := range s.Statuses {
		statuses = append(statuses, service...)
	}

	return RollUpStatus(statuses)
}

// ServiceStatuses is each declared service's own rolled-up status, in
// name order.
func (s *ComposeStack) ServiceStatuses() []string {
	names := s.serviceNames()
	statuses := make([]string, 0, len(names))

	for _, name := range names {
		statuses = append(statuses, RollUpStatus(s.Statuses[name]))
	}

	return statuses
}

func (s *ComposeStack) serviceNames() []string {
	names := make([]string, 0, len(s.Services))
	for _, service := range s.Services {
		names = append(names, service.Name)
	}

	sort.Strings(names)

	return names
}

// RollUpStatus is several instances' statuses as one: theirs when they
// agree, ServicePartial when they don't, ServiceNone for no instances.
func RollUpStatus(statuses []string) string {
	if len(statuses) == 0 {
		return ServiceNone
	}

	for _, status := range statuses[1:] {
		if !strings.EqualFold(status, statuses[0]) {
			return ServicePartial
		}
	}

	return statuses[0]
}

// GetComposeStatuses is the status of every compose-labelled instance on the
// server, by project and then service: one listing for every stack.
func (c *IncusCommand) GetComposeStatuses() (map[string]map[string][]string, error) {
	instances, err := c.Client().GetInstancesAllProjects(api.InstanceTypeAny)
	if err != nil {
		return nil, err
	}

	statuses := map[string]map[string][]string{}

	for _, instance := range instances {
		service := instance.ExpandedConfig[composeServiceKey]
		if service == "" {
			continue
		}

		if statuses[instance.Project] == nil {
			statuses[instance.Project] = map[string][]string{}
		}

		statuses[instance.Project][service] = append(statuses[instance.Project][service], instance.Status)
	}

	return statuses, nil
}

// ResolveStackDir turns what was typed for a stack into the absolute,
// cleaned directory it names: `~` is home, and a relative path is taken
// from cwd.
func ResolveStackDir(input, cwd, home string) (string, error) {
	path := strings.TrimSpace(input)
	if path == "" {
		return "", errors.New("no directory given")
	}

	if path == "~" || strings.HasPrefix(path, "~/") {
		path = filepath.Join(home, path[1:])
	}

	if !filepath.IsAbs(path) {
		path = filepath.Join(cwd, path)
	}

	return filepath.Clean(path), nil
}

// CheckStackDir refuses a path that isn't an existing directory; whether it
// holds a compose file is incus-compose's to say.
func CheckStackDir(dir string) error {
	info, err := os.Stat(dir)
	if err != nil {
		return err
	}

	if !info.IsDir() {
		return fmt.Errorf("%s: not a directory", dir)
	}

	return nil
}

// ShortenHome writes a path under home with `~` in its place.
func ShortenHome(path, home string) string {
	if home == "" || home == "/" {
		return path
	}

	if path == home {
		return "~"
	}

	if rest, ok := strings.CutPrefix(path, home+string(filepath.Separator)); ok {
		return filepath.Join("~", rest)
	}

	return path
}
