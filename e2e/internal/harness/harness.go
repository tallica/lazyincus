// Package harness is the e2e suite's guard, connection and per-run project.
package harness

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	incus "github.com/lxc/incus/v7/client"
	"github.com/lxc/incus/v7/shared/api"
	"github.com/rs/zerolog"
	"github.com/tallica/lazyincus/pkg/commands"
	"github.com/tallica/lazyincus/pkg/config"
)

const (
	remoteEnv     = "LAZYINCUS_E2E_REMOTE"
	projectPrefix = "e2e-"
	createdKey    = "user.e2e.created"

	// staleAfter leaves a run still going on the same daemon alone.
	staleAfter = 2 * time.Hour
)

// Run is one test binary's connection and project.
type Run struct {
	Command *commands.IncusCommand
	Config  *config.AppConfig
	OS      *commands.OSCommand
	Remote  string
	Project string

	// Client is scoped to Project, Default to the default project.
	Client  incus.InstanceServer
	Default incus.InstanceServer

	// Pool and Network come from the default project's default profile.
	Pool    string
	Network string
}

var (
	current *Run
	skipped string
)

// Main is a TestMain: see CLAUDE.md, "End-to-end".
func Main(m *testing.M) int {
	remote := os.Getenv(remoteEnv)
	if remote == "" {
		skipped = remoteEnv + " not set"
		return m.Run()
	}

	run, err := setUp(remote)
	if err != nil {
		fmt.Fprintln(os.Stderr, "e2e:", err)
		return 1
	}

	current = run
	code := m.Run()

	if err := run.Default.DeleteProjectForce(run.Project); err != nil {
		fmt.Fprintf(os.Stderr, "e2e: deleting project %s: %v\n", run.Project, err)
		code = 1
	}

	return code
}

// Get is the run, skipping t when there isn't one.
func Get(t *testing.T) *Run {
	t.Helper()

	if current == nil {
		t.Skip(skipped)
	}

	return current
}

// AllowlistPath names the remotes the suite may run on, one a line.
func AllowlistPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "lazyincus", "e2e-remotes")
}

func allowed(remote string) error {
	path := AllowlistPath()

	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("%w; list %q there to run against it", err, remote)
	}

	for line := range strings.Lines(string(data)) {
		if strings.TrimSpace(line) == remote {
			return nil
		}
	}

	return fmt.Errorf("%q isn't listed in %s", remote, path)
}

func setUp(remote string) (*Run, error) {
	if err := allowed(remote); err != nil {
		return nil, err
	}

	run, err := connect(remote)
	if err != nil {
		return nil, err
	}

	run.Default = run.Command.Client().UseProject("default")
	sweep(run.Default)

	if err := run.createProject(); err != nil {
		return nil, err
	}

	return run, nil
}

func connect(remote string) (*Run, error) {
	dir, err := os.MkdirTemp("", "lazyincus-e2e-")
	if err != nil {
		return nil, err
	}

	// As main does for --remote: the incus CLI the app shells out to reads it too.
	_ = os.Setenv("INCUS_REMOTE", remote)
	_ = os.Setenv("CONFIG_DIR", dir)

	cfg, err := config.NewAppConfig("lazyincus", "e2e", "", "", "", false)
	if err != nil {
		return nil, err
	}

	log := zerolog.Nop()

	osCommand := commands.NewOSCommand(&log, cfg)

	command, err := commands.NewIncusCommand(&log, osCommand, cfg)
	if err != nil {
		return nil, err
	}

	if command.RemoteName() != remote {
		return nil, fmt.Errorf("connected to %q, %s says %q", command.RemoteName(), remoteEnv, remote)
	}

	return &Run{Command: command, Config: cfg, OS: osCommand, Remote: remote}, nil
}

// sweep deletes the projects of runs that died before their own cleanup.
func sweep(defaults incus.InstanceServer) {
	projects, err := defaults.GetProjects()
	if err != nil {
		return
	}

	for _, project := range projects {
		created, err := time.Parse(time.RFC3339, project.Config[createdKey])
		if !strings.HasPrefix(project.Name, projectPrefix) || err != nil || time.Since(created) < staleAfter {
			continue
		}

		if err := defaults.DeleteProjectForce(project.Name); err != nil {
			fmt.Fprintf(os.Stderr, "e2e: sweeping stale project %s: %v\n", project.Name, err)
		}
	}
}

// A new project lacks a root disk and NIC (CLAUDE.md, "Verifying in a live TUI").
func (r *Run) createProject() error {
	profile, _, err := r.Default.GetProfile("default")
	if err != nil {
		return err
	}

	devices := map[string]map[string]string{}

	for name, device := range profile.Devices {
		switch {
		case device["type"] == "disk" && device["path"] == "/":
			r.Pool = device["pool"]
		case device["type"] == "nic":
			r.Network = device["network"]
		default:
			continue
		}

		devices[name] = device
	}

	if r.Pool == "" {
		return errors.New("the default profile has no root disk to copy")
	}

	r.Project = projectPrefix + Suffix()

	err = r.Default.CreateProject(api.ProjectsPost{
		Name: r.Project,
		ProjectPut: api.ProjectPut{
			Description: "lazyincus e2e run, deleted when it ends",
			Config: map[string]string{
				createdKey:                 time.Now().UTC().Format(time.RFC3339),
				"features.images":          "true",
				"features.profiles":        "true",
				"features.storage.volumes": "true",
			},
		},
	})
	if err != nil {
		return err
	}

	r.Client = r.Default.UseProject(r.Project)

	_, etag, err := r.Client.GetProfile("default")
	if err == nil {
		err = r.Client.UpdateProfile("default", api.ProfilePut{Devices: devices}, etag)
	}

	if err != nil {
		_ = r.Default.DeleteProjectForce(r.Project)
		return err
	}

	return nil
}

func Suffix() string {
	b := make([]byte, 3)
	_, _ = rand.Read(b)

	return hex.EncodeToString(b)
}

// WaitFor polls until cond holds, failing with what it last saw once within has passed.
func WaitFor(t *testing.T, within time.Duration, what string, cond func() (bool, string)) {
	t.Helper()

	deadline := time.Now().Add(within)

	for {
		ok, seen := cond()
		if ok {
			return
		}

		if time.Now().After(deadline) {
			t.Fatalf("%s: not within %s, last saw %q", what, within, seen)
		}

		time.Sleep(250 * time.Millisecond)
	}
}
