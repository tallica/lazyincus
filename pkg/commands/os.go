package commands

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/go-errors/errors"

	"github.com/jesseduffield/kill"
	"github.com/mgutz/str"
	"github.com/rs/zerolog"
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
	Log      *zerolog.Logger
	Platform *Platform
	Config   *config.AppConfig
	command  func(string, ...string) *exec.Cmd
	getenv   func(string) string
}

// NewOSCommand os command runner
func NewOSCommand(log *zerolog.Logger, config *config.AppConfig) *OSCommand {
	return &OSCommand{
		Log:      log,
		Platform: getPlatform(),
		Config:   config,
		command:  exec.Command,
		getenv:   os.Getenv,
	}
}

// RunCommandWithOutput wrapper around commands returning their output and error
func (c *OSCommand) RunCommandWithOutput(command string) (string, error) {
	cmd, err := c.ExecutableFromString(command)
	if err != nil {
		return "", err
	}

	before := time.Now()
	output, err := sanitisedCommandOutput(cmd.Output())
	c.Log.Warn().Msgf("'%s': %s", command, time.Since(before))

	return output, err
}

// RunExecutableWithOutput runs an executable file and returns its output
func (c *OSCommand) RunExecutableWithOutput(cmd *exec.Cmd) (string, error) {
	return sanitisedCommandOutput(cmd.CombinedOutput())
}

// ExecutableFromString takes a string like `incus list` and returns an
// executable command for it. The line is the user's own - an open command, a
// clipboard command - so one str.ToArgv can't read is an error to report
// rather than the panic it answers with.
func (c *OSCommand) ExecutableFromString(commandStr string) (*exec.Cmd, error) {
	args, err := toArgv(commandStr)
	if err != nil {
		return nil, err
	}

	return c.NewCmd(args[0], args[1:]...), nil
}

// toArgv is str.ToArgv with the two ways a line leaves it with nothing to run
// turned into errors: the panic it raises on a quote it can't close or a
// backslash it ends on, and the nothing it returns for a line of whitespace.
// Quote escapes for this parser, so its output goes through it untouched.
func toArgv(line string) (args []string, err error) {
	defer func() {
		if reason := recover(); reason != nil {
			err = errors.New(fmt.Sprint(reason))
		}
	}()

	args = str.ToArgv(line)
	if len(args) == 0 {
		return nil, errors.New("no command to run")
	}

	return args, nil
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
			// CombinedOutput leaves Stderr empty, having put it in output.
			message := string(exitError.Stderr)
			if message == "" {
				message = strings.TrimSpace(outputString)
			}

			return outputString, errors.New(message)
		}
		return "", WrapError(err)
	}
	return outputString, nil
}

// OpenFile opens filename with commandTemplate, which the caller reads from the config.
func (c *OSCommand) OpenFile(commandTemplate, filename string) error {
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

// CopyToClipboard pipes text into commandStr, else the first of clipboardCommandCandidates on PATH.
func (c *OSCommand) CopyToClipboard(commandStr, text string) error {
	if commandStr == "" {
		commandStr = c.defaultClipboardCommand()
	}

	if commandStr == "" {
		return errors.New("No clipboard command found. Install one of pbcopy, wl-copy, xclip or xsel, or set os.copyToClipboardCommand in your config")
	}

	cmd, err := c.ExecutableFromString(commandStr)
	if err != nil {
		return err
	}

	cmd.Stdin = strings.NewReader(text)

	return c.RunPreparedCommand(cmd)
}

// defaultClipboardCommand returns the first clipboard tool available on PATH,
// or an empty string if none of them are.
func (c *OSCommand) defaultClipboardCommand() string {
	for _, candidate := range clipboardCommandCandidates {
		args, err := toArgv(candidate)
		if err != nil {
			continue
		}

		if _, err := exec.LookPath(args[0]); err == nil {
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
	c.Log.Info().Msg(outString)
	if err != nil {
		if len(outString) == 0 {
			return err
		}
		return errors.New(outString)
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
