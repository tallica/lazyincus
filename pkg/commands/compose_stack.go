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
	// that follows the session's: the local stack, or a saved entry with
	// none, which the next start pins.
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

	// Hashes are the config hashes those instances were created from.
	Hashes map[string][]string

	// StatusErr is why the stack's remote gave no statuses, and
	// StatusPending that it hasn't yet: it's still being connected to.
	StatusErr     error
	StatusPending bool
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

// Changed names the services with an instance from an older config.
func (s *ComposeStack) Changed() []string {
	var changed []string

	for _, service := range s.Services {
		for _, hash := range s.Hashes[service.Name] {
			if configChanged(service.Hash, hash) {
				changed = append(changed, service.Name)
				break
			}
		}
	}

	sort.Strings(changed)

	return changed
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

// ComposeListing is compose instances' statuses and hashes, by project.
type ComposeListing struct {
	Statuses map[string]map[string][]string
	Hashes   map[string]map[string][]string
}

// GetComposeStatuses lists every compose-labelled instance on c's remote:
// one listing for every stack there.
func (c *IncusCommand) GetComposeStatuses() (ComposeListing, error) {
	instances, err := c.Client().GetInstancesAllProjects(api.InstanceTypeAny)
	if err != nil {
		return ComposeListing{}, err
	}

	listing := ComposeListing{Statuses: map[string]map[string][]string{}, Hashes: map[string]map[string][]string{}}

	add := func(byProject map[string]map[string][]string, project, service, value string) {
		if byProject[project] == nil {
			byProject[project] = map[string][]string{}
		}

		byProject[project][service] = append(byProject[project][service], value)
	}

	for _, instance := range instances {
		service := instance.ExpandedConfig[composeServiceKey]
		if service == "" {
			continue
		}

		add(listing.Statuses, instance.Project, service, instance.Status)

		if hash := instance.ExpandedConfig[composeHashKey]; hash != "" {
			add(listing.Hashes, instance.Project, service, hash)
		}
	}

	return listing, nil
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

// composeFileNames are compose-go's default compose files, which
// incus-compose looks for, in its order of preference.
var composeFileNames = []string{"compose.yaml", "compose.yml", "docker-compose.yml", "docker-compose.yaml"}

// ComposeFile is the file incus-compose reads for the stack in dir: the
// first of composeFileNames there, or in the nearest directory above that
// has one, as compose-go looks.
func ComposeFile(dir string) (string, error) {
	for search := dir; ; search = filepath.Dir(search) {
		for _, name := range composeFileNames {
			file := filepath.Join(search, name)
			if info, err := os.Stat(file); err == nil && !info.IsDir() {
				return file, nil
			}
		}

		if filepath.Dir(search) == search {
			return "", fmt.Errorf("%s: no compose file", dir)
		}
	}
}

// StackFilesStamp changes with a stack's compose file or its .env.
func StackFilesStamp(dir string) string {
	stamp := ""

	files := []string{filepath.Join(dir, ".env")}
	if file, err := ComposeFile(dir); err == nil {
		files = append(files, file)
	}

	for _, file := range files {
		if info, err := os.Stat(file); err == nil {
			stamp += fmt.Sprintf("%s:%d:%d;", file, info.ModTime().UnixNano(), info.Size())
		}
	}

	return stamp
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
