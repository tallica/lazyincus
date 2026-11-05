package commands

import (
	"errors"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/kballard/go-shellquote"
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
// executable command for it.
func (c *OSCommand) ExecutableFromString(commandStr string) (*exec.Cmd, error) {
	args, err := toArgv(commandStr)
	if err != nil {
		return nil, err
	}

	return c.NewCmd(args[0], args[1:]...), nil
}

// toArgv splits line by shell quoting rules, which Quote escapes for.
func toArgv(line string) ([]string, error) {
	args, err := shellquote.Split(line)
	if err != nil {
		return nil, err
	}
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
		return "", err
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
func (c *OSCommand) OpenLink(commandTemplate, link string) error {
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

var ErrNoClipboardCommand = errors.New("no clipboard command found")

// CopyToClipboard pipes text into commandStr, else the first of clipboardCommandCandidates on PATH.
func (c *OSCommand) CopyToClipboard(commandStr, text string) error {
	if commandStr == "" {
		commandStr = c.defaultClipboardCommand()
	}

	if commandStr == "" {
		return ErrNoClipboardCommand
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
		return nil, errors.New("no editor defined in $VISUAL or $EDITOR")
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
	if cmd.Process == nil {
		return nil
	}
	return cmd.Process.Kill()
}
