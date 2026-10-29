package gui

import (
	"slices"

	"github.com/tallica/lazyincus/pkg/commands"
	"github.com/tallica/lazyincus/pkg/gui/presentation"
)

type location = presentation.Location

// sessionLocation is where an item the session's panels list lives: the
// resources, which only ever show its remote.
func (gui *Gui) sessionLocation(project string) location {
	return location{Remote: gui.IncusCommand.RemoteName(), Project: project}
}

// instanceLocation is an instance's, its stack and service named when
// incus-compose made it.
func (gui *Gui) instanceLocation(instance *commands.Instance) location {
	loc := location{Remote: instance.Remote, Project: instance.Project}
	if loc.Remote == "" {
		loc.Remote = gui.IncusCommand.RemoteName()
	}

	service := instance.ComposeService()
	if service == "" {
		return loc
	}

	loc.Stack = gui.stackLabel(loc.Remote, instance.Project)
	loc.Service = service

	return loc
}

// stackLabel is where the stack in project is listed from, or that it
// isn't, its instances being on the daemon whether or not a compose file
// here says so. Not its name: incus-compose names the project for the
// stack, and the Project line has said it.
func (gui *Gui) stackLabel(remote, project string) string {
	if dirs := gui.stackDirs.Load(); dirs != nil {
		if dir, ok := (*dirs)[stackDirKey(remote, project)]; ok {
			return commands.ShortenHome(dir, gui.home)
		}
	}

	return gui.Tr.StackNotListed
}

// stackDirKey is how stackDirs finds a listed stack: by the remote it's on
// and its compose project, which is also its instances' Incus project.
func stackDirKey(remote, project string) string {
	return remote + "\x00" + project
}

// setStackDirs records where each listed stack's directory is, and which
// remotes they're on, for the tabs and the image check, which run off the
// main loop and so can't read the Stacks panel.
func (gui *Gui) setStackDirs(stacks []*commands.ComposeStack) {
	dirs := map[string]string{}
	remotes := []string{}

	for _, stack := range stacks {
		if remote := gui.stackRemote(stack); !slices.Contains(remotes, remote) {
			remotes = append(remotes, remote)
		}

		if stack.Name == "" {
			continue
		}

		key := stackDirKey(gui.stackRemote(stack), stack.Name)
		if _, ok := dirs[key]; !ok {
			dirs[key] = stack.Dir
		}
	}

	gui.stackDirs.Store(&dirs)
	gui.stackRemotes.Store(&remotes)
}
