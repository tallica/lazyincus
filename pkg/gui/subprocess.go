package gui

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"strings"

	"github.com/fatih/color"
	"github.com/tallica/lazyincus/pkg/utils"
)

func (gui *Gui) runSubprocess(cmd *exec.Cmd) error {
	return gui.runSubprocessWithMessage(cmd, "")
}

func (gui *Gui) runSubprocessWithMessage(cmd *exec.Cmd, msg string) error {
	gui.SubprocessMutex.Lock()
	defer gui.SubprocessMutex.Unlock()

	if err := gui.g.Suspend(); err != nil {
		return gui.createErrorPanel(err.Error())
	}

	gui.PauseBackgroundThreads.Store(true)

	gui.runCommand(cmd, msg)

	if err := gui.g.Resume(); err != nil {
		return gui.createErrorPanel(err.Error())
	}

	gui.PauseBackgroundThreads.Store(false)

	return nil
}

func (gui *Gui) runCommand(cmd *exec.Cmd, msg string) {
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stdout
	cmd.Stdin = os.Stdin

	interrupt := make(chan os.Signal, 1)
	signal.Notify(interrupt, os.Interrupt)

	done := make(chan struct{})
	defer func() {
		signal.Stop(interrupt)
		close(done)
	}()

	go func() {
		select {
		case <-interrupt:
			if err := gui.OSCommand.Kill(cmd); err != nil {
				gui.Log.Error(err)
			}
		case <-done:
		}
	}()

	fmt.Fprintf(os.Stdout, "\n%s\n\n", utils.ColoredString("+ "+strings.Join(cmd.Args, " "), color.FgBlue))
	if msg != "" {
		fmt.Fprintf(os.Stdout, "\n%s\n\n", utils.ColoredString(msg, color.FgGreen))
	}
	if err := cmd.Run(); err != nil {
		gui.Log.Error(err)
	}

	cmd.Stdin = nil
	cmd.Stdout = io.Discard
	cmd.Stderr = io.Discard

	gui.promptToReturn()
}
