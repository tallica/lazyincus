package commands

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/go-errors/errors"

	"github.com/jesseduffield/kill"
	"github.com/mgutz/str"
	"github.com/sirupsen/logrus"
	"github.com/tallica/lazyincus/pkg/config"
	"github.com/tallica/lazyincus/pkg/utils"
)

// Platform stores the os state
type Platform struct {
	os              string
	shell           string
	shellArg        string
	openCommand     string
	openLinkCommand string
}

// OSCommand holds all the os commands
type OSCommand struct {
	Log      *logrus.Entry
	Platform *Platform
	Config   *config.AppConfig
	command  func(string, ...string) *exec.Cmd
	getenv   func(string) string
}

// NewOSCommand os command runner
func NewOSCommand(log *logrus.Entry, config *config.AppConfig) *OSCommand {
	return &OSCommand{
		Log:      log,
		Platform: getPlatform(),
		Config:   config,
		command:  exec.Command,
		getenv:   os.Getenv,
	}
}

// SetCommand sets the command function used by the struct.
// To be used for testing only
func (c *OSCommand) SetCommand(cmd func(string, ...string) *exec.Cmd) {
	c.command = cmd
}

// RunCommandWithOutput wrapper around commands returning their output and error
func (c *OSCommand) RunCommandWithOutput(command string) (string, error) {
	cmd := c.ExecutableFromString(command)
	before := time.Now()
	output, err := sanitisedCommandOutput(cmd.Output())
	c.Log.Warn(fmt.Sprintf("'%s': %s", command, time.Since(before)))
	return output, err
}

// RunCommandWithOutputContext wrapper around commands returning their output and error
func (c *OSCommand) RunCommandWithOutputContext(ctx context.Context, command string) (string, error) {
	cmd := c.ExecutableFromStringContext(ctx, command)
	before := time.Now()
	output, err := sanitisedCommandOutput(cmd.Output())
	c.Log.Warn(fmt.Sprintf("'%s': %s", command, time.Since(before)))
	return output, err
}

// RunExecutableWithOutput runs an executable file and returns its output
func (c *OSCommand) RunExecutableWithOutput(cmd *exec.Cmd) (string, error) {
	return sanitisedCommandOutput(cmd.CombinedOutput())
}

// RunExecutable runs an executable file and returns an error if there was one
func (c *OSCommand) RunExecutable(cmd *exec.Cmd) error {
	_, err := c.RunExecutableWithOutput(cmd)
	return err
}

// ExecutableFromString takes a string like `incus list` and returns an executable command for it
func (c *OSCommand) ExecutableFromString(commandStr string) *exec.Cmd {
	splitCmd := str.ToArgv(commandStr)
	return c.NewCmd(splitCmd[0], splitCmd[1:]...)
}

// Same as ExecutableFromString but cancellable via a context
func (c *OSCommand) ExecutableFromStringContext(ctx context.Context, commandStr string) *exec.Cmd {
	splitCmd := str.ToArgv(commandStr)
	return exec.CommandContext(ctx, splitCmd[0], splitCmd[1:]...)
}

func (c *OSCommand) NewCmd(cmdName string, commandArgs ...string) *exec.Cmd {
	cmd := c.command(cmdName, commandArgs...)
	cmd.Env = os.Environ()
	return cmd
}

// RunCommand runs a command and just returns the error
func (c *OSCommand) RunCommand(command string) error {
	_, err := c.RunCommandWithOutput(command)
	return err
}

func sanitisedCommandOutput(output []byte, err error) (string, error) {
	outputString := string(output)
	if err != nil {
		exitError, ok := err.(*exec.ExitError)
		if ok {
			return outputString, errors.New(string(exitError.Stderr))
		}
		return "", WrapError(err)
	}
	return outputString, nil
}

// OpenFile opens a file with the configured open command
func (c *OSCommand) OpenFile(filename string) error {
	commandTemplate := c.Config.UserConfig.OS.OpenCommand
	templateValues := map[string]string{
		"filename": c.Quote(filename),
	}

	command := utils.ResolvePlaceholderString(commandTemplate, templateValues)
	err := c.RunCommand(command)
	return err
}

// OpenLink opens a link with the configured open-link command
func (c *OSCommand) OpenLink(link string) error {
	commandTemplate := c.Config.UserConfig.OS.OpenLinkCommand
	templateValues := map[string]string{
		"link": c.Quote(link),
	}

	command := utils.ResolvePlaceholderString(commandTemplate, templateValues)
	err := c.RunCommand(command)
	return err
}

// clipboardCommandCandidates are the clipboard tools we look for on PATH when
// the user hasn't configured a copyToClipboardCommand of their own, in
// priority order: macOS's pbcopy, then Wayland's wl-copy, then the two X11
// ones.
var clipboardCommandCandidates = []string{
	"pbcopy",
	"wl-copy",
	"xclip -selection clipboard -in",
	"xsel --clipboard --input",
}

// CopyToClipboard copies text to the system clipboard by piping it into the
// configured copy-to-clipboard command, or - when none is configured - the
// first of clipboardCommandCandidates found on PATH.
func (c *OSCommand) CopyToClipboard(text string) error {
	commandStr := c.Config.UserConfig.OS.CopyToClipboardCommand
	if commandStr == "" {
		commandStr = c.defaultClipboardCommand()
	}

	if commandStr == "" {
		return errors.New("No clipboard command found. Install one of pbcopy, wl-copy, xclip or xsel, or set os.copyToClipboardCommand in your config")
	}

	cmd := c.ExecutableFromString(commandStr)
	cmd.Stdin = strings.NewReader(text)

	return c.RunPreparedCommand(cmd)
}

// defaultClipboardCommand returns the first clipboard tool available on PATH,
// or an empty string if none of them are.
func (c *OSCommand) defaultClipboardCommand() string {
	for _, candidate := range clipboardCommandCandidates {
		if _, err := exec.LookPath(str.ToArgv(candidate)[0]); err == nil {
			return candidate
		}
	}

	return ""
}

// EditFile opens a file in a subprocess using whatever editor is available,
// falling back to core.editor, VISUAL, EDITOR, then vi
func (c *OSCommand) EditFile(filename string) (*exec.Cmd, error) {
	editor := c.getenv("VISUAL")
	if editor == "" {
		editor = c.getenv("EDITOR")
	}
	if editor == "" {
		if err := c.RunCommand("which vi"); err == nil {
			editor = "vi"
		}
	}
	if editor == "" {
		return nil, errors.New("No editor defined in $VISUAL or $EDITOR")
	}

	return c.NewCmd(editor, filename), nil
}

// Quote wraps a message in platform-specific quotation marks
func (c *OSCommand) Quote(message string) string {
	quote := `"`
	message = strings.NewReplacer(
		`\`, `\\`,
		`"`, `\"`,
		`$`, `\$`,
		"`", "\\`",
	).Replace(message)
	return quote + message + quote
}

// RunPreparedCommand takes a pointer to an exec.Cmd and runs it
func (c *OSCommand) RunPreparedCommand(cmd *exec.Cmd) error {
	out, err := cmd.CombinedOutput()
	outString := string(out)
	c.Log.Info(outString)
	if err != nil {
		if len(outString) == 0 {
			return err
		}
		return errors.New(outString)
	}
	return nil
}

// GetLazyincusPath returns the path of the currently executed file
func (c *OSCommand) GetLazyincusPath() string {
	ex, err := os.Executable()
	if err != nil {
		ex = os.Args[0]
	}
	return filepath.ToSlash(ex)
}

// PipeCommands runs a heap of commands and pipes their inputs/outputs together like A | B | C
func (c *OSCommand) PipeCommands(commandStrings ...string) error {
	cmds := make([]*exec.Cmd, len(commandStrings))

	for i, str := range commandStrings {
		cmds[i] = c.ExecutableFromString(str)
	}

	for i := 0; i < len(cmds)-1; i++ {
		stdout, err := cmds[i].StdoutPipe()
		if err != nil {
			return err
		}

		cmds[i+1].Stdin = stdout
	}

	finalErrors := []string{}

	wg := sync.WaitGroup{}
	wg.Add(len(cmds))

	for _, cmd := range cmds {
		currentCmd := cmd
		go func() {
			stderr, err := currentCmd.StderrPipe()
			if err != nil {
				c.Log.Error(err)
			}

			if err := currentCmd.Start(); err != nil {
				c.Log.Error(err)
			}

			if b, err := io.ReadAll(stderr); err == nil {
				if len(b) > 0 {
					finalErrors = append(finalErrors, string(b))
				}
			}

			if err := currentCmd.Wait(); err != nil {
				c.Log.Error(err)
			}

			wg.Done()
		}()
	}

	wg.Wait()

	if len(finalErrors) > 0 {
		return errors.New(strings.Join(finalErrors, "\n"))
	}
	return nil
}

// Kill kills a process.
func (c *OSCommand) Kill(cmd *exec.Cmd) error {
	return kill.Kill(cmd)
}

// PrepareForChildren sets Setpgid to true on the cmd, so that when we run it as a subprocess, we can kill its group rather than the process itself.
func (c *OSCommand) PrepareForChildren(cmd *exec.Cmd) {
	kill.PrepareForChildren(cmd)
}
