package gui

import (
	"github.com/samber/lo"
	"github.com/tallica/lazyincus/pkg/commands"
	"github.com/tallica/lazyincus/pkg/utils"
)

// location is where an item lives, broadest first, for the lines that head
// its Info or Config tab: with several remotes, projects and stacks about,
// the list it came from no longer says.
type location struct {
	remote  string
	project string
	// stack and service are a compose instance's, empty for anything else:
	// the stack is where it's listed from, its name being the project's.
	stack   string
	service string
}

// locationStr is loc's lines, those it has no value for left out, and
// those omit names too: a tab whose heading has already said them.
func (gui *Gui) locationStr(loc location, omit ...string) string {
	output := ""

	for _, line := range [][2]string{
		{"Remote", loc.remote},
		{"Project", loc.project},
		{"Stack", loc.stack},
		{"Service", loc.service},
	} {
		if line[1] != "" && !lo.Contains(omit, line[0]) {
			output += utils.WithPadding(line[0]+": ", identityPadding) + line[1] + "\n"
		}
	}

	return output
}

// sessionLocation is where an item the session's panels list lives: the
// resources, which only ever show its remote.
func (gui *Gui) sessionLocation(project string) location {
	return location{remote: gui.IncusCommand.RemoteName(), project: project}
}

// instanceLocation is an instance's, its stack and service named when
// incus-compose made it.
func (gui *Gui) instanceLocation(instance *commands.Instance) location {
	loc := location{remote: instance.Remote, project: instance.Project}
	if loc.remote == "" {
		loc.remote = gui.IncusCommand.RemoteName()
	}

	service := instance.ComposeService()
	if service == "" {
		return loc
	}

	loc.stack = gui.stackLabel(loc.remote, instance.Project)
	loc.service = service

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

// setStackDirs records where each listed stack's directory is, for the
// tabs, which render off the main loop and so can't read the Stacks panel.
func (gui *Gui) setStackDirs(stacks []*commands.ComposeStack) {
	dirs := map[string]string{}

	for _, stack := range stacks {
		if stack.Name == "" {
			continue
		}

		key := stackDirKey(gui.stackRemote(stack), stack.Name)
		if _, ok := dirs[key]; !ok {
			dirs[key] = stack.Dir
		}
	}

	gui.stackDirs.Store(&dirs)
}
