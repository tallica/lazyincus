package tui

import (
	"fmt"
	"io"
	"os/exec"
	"sort"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/fatih/color"
	"github.com/tallica/lazyincus/pkg/commands"
	"github.com/tallica/lazyincus/pkg/gui/presentation"
	"github.com/tallica/lazyincus/pkg/utils"
)

type instancesMsg struct {
	seq       int
	instances []*commands.Instance
	err       error
}

type execDoneMsg struct{ err error }

func (m *Model) fetchInstances() tea.Cmd {
	m.fetchSeq++
	seq := m.fetchSeq

	return func() tea.Msg {
		instances, err := m.incus.GetInstances()

		return instancesMsg{seq: seq, instances: instances, err: err}
	}
}

func (m *Model) selectedInstance() *commands.Instance {
	p := m.panelNamed("instances")
	if p.selected < 0 || p.selected >= len(m.instances) {
		return nil
	}

	return m.instances[p.selected]
}

// setInstances keeps the selection on the same instance across a refresh.
func (m *Model) setInstances(instances []*commands.Instance) {
	key := ""
	if selected := m.selectedInstance(); selected != nil {
		key = selected.Key()
	}

	sort.SliceStable(instances, func(a, b int) bool { return lessInstance(instances[a], instances[b]) })
	m.instances = instances

	projects := map[string]bool{}
	for _, instance := range instances {
		projects[instance.Project] = true
	}
	m.showProject = len(projects) > 1

	p := m.panelNamed("instances")
	p.setCells(m.instanceCells())
	for i, instance := range instances {
		if instance.Key() == key {
			p.selected = i
			break
		}
	}
	p.selectRow(p.selected, m.listRows(p))
}

func lessInstance(a, b *commands.Instance) bool {
	if a.IsRunning() != b.IsRunning() {
		return a.IsRunning()
	}

	return a.Name < b.Name
}

func (m *Model) instanceCells() [][]string {
	cells := make([][]string, len(m.instances))
	for i, instance := range m.instances {
		cells[i] = presentation.GetInstanceDisplayStrings(&m.cfg.UserConfig.Gui, instance, m.showProject, nil)
	}

	return cells
}

func (m *Model) instanceFlex() []utils.FlexColumn {
	return []utils.FlexColumn{{
		Index:    presentation.InstanceImageColumn(&m.cfg.UserConfig.Gui, m.showProject),
		MinWidth: presentation.MinImageAliasWidth,
	}}
}

func (m *Model) instanceMainTabs() []mainTab {
	return []mainTab{
		{title: m.tr.InfoTitle, content: m.instanceInfo},
		{title: m.tr.ConfigTitle, content: m.instanceConfig},
	}
}

func (m *Model) instanceConfig() string {
	instance := m.selectedInstance()
	if instance == nil {
		return ""
	}

	data, err := utils.MarshalIntoYaml(instance.Instance)
	if err != nil {
		return err.Error()
	}

	return utils.ColoredYamlString(string(data))
}

// instanceInfo is a cut-down Info tab; the full one still lives on *gui.Gui.
func (m *Model) instanceInfo() string {
	instance := m.selectedInstance()
	if instance == nil {
		return ""
	}

	label := func(name, value string) string {
		if value == "" {
			return ""
		}
		return utils.ColoredString(fmt.Sprintf("%-10s", name+":"), color.FgCyan) + " " + value + "\n"
	}

	info := label("Name", instance.Name) +
		label("Type", presentation.InstanceType(instance)) +
		label("Status", instance.Instance.Status) +
		label("Project", instance.Project) +
		label("Remote", instance.Remote) +
		label("Image", instance.ComposeImage()) +
		label("Created", instance.Instance.CreatedAt.Format(time.DateTime))

	if state := instance.Instance.State; state != nil {
		info += label("PID", fmt.Sprint(state.Pid)) +
			label("Processes", fmt.Sprint(state.Processes))
	}

	return info
}

// execShell is gui.instanceExecShell, refusing as guardReadOnly does.
func (m *Model) execShell() tea.Cmd {
	instance := m.selectedInstance()
	if instance == nil {
		return nil
	}

	remote := instance.Remote
	if remote == "" {
		remote = m.incus.RemoteName()
	}

	switch {
	case m.cfg.ReadOnly:
		m.openError(m.tr.ReadOnlySession)
		return nil
	case m.cfg.UserConfig.Remotes[remote].ReadOnly:
		m.openError(fmt.Sprintf(m.tr.ReadOnlyRemote, remote))
		return nil
	case !instance.IsRunning():
		m.openError(m.tr.CannotExecStoppedInstanceError)
		return nil
	}

	args := []string{}
	if instance.Project != "" {
		args = append(args, "--project", instance.Project)
	}
	args = append(args, "exec", instance.Name, "--", "sh", "-c",
		"exec $(command -v bash || command -v ash || command -v sh)")

	cmd := commands.WithRemote(m.os.NewCmd("incus", args...), instance.Remote)

	return tea.Exec(&announcedCmd{cmd: cmd, message: m.tr.ExitShellToReturn}, func(err error) tea.Msg {
		return execDoneMsg{err: err}
	})
}

// announcedCmd prints its message first, after a reset: the last frame's
// attributes are still in force on the released terminal.
type announcedCmd struct {
	cmd     *exec.Cmd
	message string
}

func (c *announcedCmd) Run() error {
	if c.cmd.Stdout != nil {
		fmt.Fprintln(c.cmd.Stdout, reset+utils.ColoredString(c.message, color.FgBlue))
	}

	return c.cmd.Run()
}

func (c *announcedCmd) SetStdin(r io.Reader)  { c.cmd.Stdin = r }
func (c *announcedCmd) SetStdout(w io.Writer) { c.cmd.Stdout = w }
func (c *announcedCmd) SetStderr(w io.Writer) { c.cmd.Stderr = w }
