package tui

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/fatih/color"
	"github.com/rs/zerolog"
	"github.com/tallica/lazyincus/pkg/commands"
	"github.com/tallica/lazyincus/pkg/utils"
)

type execDoneMsg struct {
	err  error
	then func(error) tea.Cmd
}

func (m *Model) runSubprocess(cmd *exec.Cmd, message string, then func() tea.Cmd) tea.Cmd {
	if then == nil {
		return m.runSubprocessReturning(cmd, message, nil, nil)
	}

	return m.runSubprocessReturning(cmd, message, nil, func(error) tea.Cmd { return then() })
}

// runSubprocessReturning: after runs before the screen comes back, so its wait doesn't hold the TUI.
func (m *Model) runSubprocessReturning(cmd *exec.Cmd, message string, after func(error), then func(error) tea.Cmd) tea.Cmd {
	s := &subprocess{
		cmd: cmd, message: message, os: m.os, log: m.incus.Log, pressEnter: m.tr.PressEnterToReturn,
		returnImmediately: m.cfg.UserConfig.Gui.ReturnImmediately, after: after,
	}

	return tea.Exec(s, func(err error) tea.Msg { return execDoneMsg{err: err, then: then} })
}

// onExecDone only logs the command's failure: the terminal has shown it.
func (m *Model) onExecDone(msg execDoneMsg) tea.Cmd {
	if msg.err != nil {
		m.incus.Log.Error().Err(msg.err).Send()
	}

	if msg.then == nil {
		return nil
	}

	return msg.then(msg.err)
}

type subprocess struct {
	cmd               *exec.Cmd
	message           string
	returnImmediately bool
	os                *commands.OSCommand
	log               *zerolog.Logger
	pressEnter        string
	after             func(error)
	stdin             io.Reader
	stdout            io.Writer
}

func (s *subprocess) Run() error {
	s.cmd.Stdin, s.cmd.Stdout, s.cmd.Stderr = s.stdin, s.stdout, s.stdout

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
			if err := s.os.Kill(s.cmd); err != nil {
				s.log.Error().Err(err).Send()
			}
		case <-done:
		}
	}()

	// The last frame's attributes are still in force on the released terminal.
	fmt.Fprintf(s.stdout, "%s\n%s\n\n", reset, utils.ColoredString("+ "+strings.Join(s.cmd.Args, " "), color.FgBlue))
	if s.message != "" {
		fmt.Fprintf(s.stdout, "\n%s\n\n", utils.ColoredString(s.message, color.FgGreen))
	}

	err := s.cmd.Run()

	if s.after != nil {
		s.after(err)
	}

	if !s.returnImmediately {
		fmt.Fprintf(s.stdout, "\n\n%s", utils.ColoredString(s.pressEnter, color.FgGreen))
		if _, scanErr := fmt.Fscanln(s.stdin); scanErr != nil {
			s.log.Error().Err(scanErr).Send()
		}
	}

	return err
}

func (s *subprocess) SetStdin(r io.Reader)  { s.stdin = r }
func (s *subprocess) SetStdout(w io.Writer) { s.stdout = w }
func (s *subprocess) SetStderr(io.Writer)   {}
