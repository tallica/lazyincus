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
	_, err := gui.runSubprocessReturning(cmd, msg, nil)

	return err
}

// runSubprocessReturning is runSubprocessWithMessage that also hands back
// how the command itself ended, which only the terminal has shown. after
// runs before the screen comes back, so waiting for what trails the command
// doesn't hold the TUI.
func (gui *Gui) runSubprocessReturning(cmd *exec.Cmd, msg string, after func(runErr error)) (runErr, err error) {
	gui.SubprocessMutex.Lock()
	defer gui.SubprocessMutex.Unlock()

	if err := gui.g.Suspend(); err != nil {
		return nil, gui.createErrorPanel(err.Error())
	}

	gui.PauseBackgroundThreads.Store(true)
	defer gui.PauseBackgroundThreads.Store(false)

	runErr = gui.runCommand(cmd, msg)

	if after != nil {
		after(runErr)
	}

	if err := gui.g.Resume(); err != nil {
		return runErr, gui.createErrorPanel(err.Error())
	}

	return runErr, nil
}

func (gui *Gui) runCommand(cmd *exec.Cmd, msg string) error {
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
				gui.Log.Error().Err(err).Send()
			}
		case <-done:
		}
	}()

	fmt.Fprintf(os.Stdout, "\n%s\n\n", utils.ColoredString("+ "+strings.Join(cmd.Args, " "), color.FgBlue))
	if msg != "" {
		fmt.Fprintf(os.Stdout, "\n%s\n\n", utils.ColoredString(msg, color.FgGreen))
	}
	err := cmd.Run()
	if err != nil {
		gui.Log.Error().Err(err).Send()
	}

	cmd.Stdin = nil
	cmd.Stdout = io.Discard
	cmd.Stderr = io.Discard

	gui.promptToReturn()

	return err
}
