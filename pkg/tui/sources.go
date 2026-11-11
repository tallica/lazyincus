package tui

import (
	"fmt"
	"slices"

	"github.com/samber/lo"
	"github.com/tallica/lazyincus/pkg/commands"
	"github.com/tallica/lazyincus/pkg/i18n"
)

// sources is what a fetch reads off the loop, each set once in NewModel or self-synchronising.
type sources struct {
	incus              *commands.IncusCommand
	tr                 *i18n.TranslationSet
	remotes            *remoteCommands
	stacks             *stackCache
	localStackDir      string
	localStackExplicit bool
	loadStack          func(dir string) *commands.ComposeStack
	loadBackups        func(command *commands.IncusCommand, dir string) ([]*commands.ComposeBackup, error)
	notifyRemotes      func()
}

func (m *Model) sources() sources {
	return sources{
		incus: m.incus, tr: m.tr, remotes: m.remotes, stacks: m.stackCache,
		localStackDir: m.localStackDir, localStackExplicit: m.localStackExplicit,
		loadStack: m.loadStack, loadBackups: m.loadBackups, notifyRemotes: m.notifyRemotes,
	}
}

func (m *Model) commandFor(remote string) (*commands.IncusCommand, error) {
	return m.sources().commandFor(remote)
}

func (m *Model) onSessionRemote(remote string) bool { return m.sources().onSessionRemote(remote) }

func (m *Model) unknownRemote(remote string) error { return m.sources().unknownRemote(remote) }

// commandFor is the command a stack pinned to remote goes through: the
// session's own for no remote, or for the session's remote by name. The
// first call for another remote starts connecting to it, and answers
// errConnecting until that's done.
func (s sources) commandFor(remote string) (*commands.IncusCommand, error) {
	if s.onSessionRemote(remote) {
		return s.incus, nil
	}

	if !s.remotes.known(remote) {
		return nil, s.unknownRemote(remote)
	}

	return s.remotes.get(remote)
}

// unknownRemote is a stack's remote gone from the CLI's config, renamed or
// removed since the stack was saved, saying how to point the stack at
// another.
func (s sources) unknownRemote(remote string) error {
	return fmt.Errorf(s.tr.UnknownRemote, remote)
}

// onSessionRemote is whether a stack pinned to remote is on the remote the
// rest of the panels show.
func (s sources) onSessionRemote(remote string) bool {
	return remote == "" || remote == s.incus.RemoteName()
}

// upstreamTargets are the session's remote's images to check, and those of
// every other remote a listed stack is on that's connected; one that isn't
// yet is left for the next check.
func (s sources) upstreamTargets(remotes []string) ([]commands.UpstreamTarget, error) {
	targets, err := s.incus.UpstreamTargets()
	if err != nil {
		return nil, err
	}

	seen := map[string]bool{}
	for _, target := range targets {
		seen[target.Fingerprint] = true
	}

	for _, remote := range remotes {
		if remote == s.incus.RemoteName() {
			continue
		}

		command, err := s.remoteCommand(remote)
		if err == nil {
			var others []commands.UpstreamTarget
			if others, err = command.UpstreamTargets(); err == nil {
				for _, target := range others {
					if !seen[target.Fingerprint] {
						seen[target.Fingerprint] = true
						targets = append(targets, target)
					}
				}
			}
		}

		if err != nil {
			s.incus.Log.Warn().Err(err).Str("remote", remote).Msg("no image check")
		}
	}

	return targets, nil
}

// remoteCommand is commandFor for a remote other than the session's, which a fetch can ask off the loop.
func (s sources) remoteCommand(remote string) (*commands.IncusCommand, error) {
	if !s.remotes.known(remote) {
		return nil, s.unknownRemote(remote)
	}

	return s.remotes.get(remote)
}

// listStacks: every stack with its config, each a copy of the cached one.
func (s sources) listStacks(saved []string) []*commands.ComposeStack {
	stacks := []*commands.ComposeStack{}
	byRef := map[string]*commands.ComposeStack{}

	// Keyed by the remote each is on now, so the local stack and the same
	// directory saved for the session's remote are one row.
	add := func(remote, dir string, local bool) {
		on := remote
		if on == "" {
			on = s.incus.RemoteName()
		}

		ref := commands.StackRef(on, dir)
		if existing, ok := byRef[ref]; ok {
			existing.Local = existing.Local || local
			existing.Saved = existing.Saved || !local

			if !local {
				existing.Remote = remote
			}

			return
		}

		cached := s.stacks.get(dir, !local || s.localStackExplicit, s.loadStack)
		if local && !s.localStackExplicit && cached.Err != nil {
			return
		}

		stack := *cached
		stack.Remote = remote
		stack.Local, stack.Saved = local, !local
		byRef[ref] = &stack
		stacks = append(stacks, &stack)
	}

	if s.localStackDir != "" {
		add("", s.localStackDir, true)
	}

	for _, ref := range saved {
		remote, dir := commands.ParseStackRef(ref)
		add(remote, dir, false)
	}

	return stacks
}

func (s sources) stacksHereIn(stacks []*commands.ComposeStack) bool {
	return slices.ContainsFunc(stacks, func(stack *commands.ComposeStack) bool {
		return s.onSessionRemote(stack.Remote)
	})
}

// readStackStatuses: the session's remote asked, every other one's as last read.
func (s sources) readStackStatuses(stacks []*commands.ComposeStack, l *commands.Listing) error {
	byRemote := lo.GroupBy(stacks, func(stack *commands.ComposeStack) string {
		if stack.Remote == s.incus.RemoteName() {
			return ""
		}

		return stack.Remote
	})

	var sessionErr error

	for remote, remoteStacks := range byRemote {
		var statuses remoteStatuses

		pending := false

		if remote == "" {
			statuses.byProject, statuses.err = s.incus.GetComposeStatuses(l)
			sessionErr = statuses.err
		} else {
			var read bool

			statuses, read = s.remoteStatuses(remote)
			pending = !read
		}

		for _, stack := range remoteStacks {
			stack.StatusPending = pending
			stack.StatusErr = statuses.err

			if stack.Name != "" {
				project := statuses.byProject[stack.Name]
				stack.Statuses, stack.Images, stack.Hashes = project.Statuses, project.Images, project.Hashes
				stack.Beacon = project.Beacon
			}
		}
	}

	return sessionErr
}

// remoteStatuses is another remote's compose statuses as last read, false
// before any has been, asking again in the background.
func (s sources) remoteStatuses(remote string) (remoteStatuses, bool) {
	read := func() (map[string]commands.ComposeProjectState, error) {
		command, err := s.commandFor(remote)
		if err != nil {
			return nil, err
		}

		return command.GetComposeStatuses(nil)
	}

	return s.remotes.cachedStatuses(remote, read, s.notifyRemotes)
}
