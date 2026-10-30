package tui

import (
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/fatih/color"
	"github.com/tallica/lazyincus/pkg/commands"
	"github.com/tallica/lazyincus/pkg/utils"
)

type copyValue struct {
	label string
	value string
}

type copiedMsg struct {
	value string
	err   error
}

// copyMenu: each value beside its label, empty ones left out.
func (m *Model) copyMenu(values ...copyValue) {
	var items []*menuItem
	for _, value := range values {
		if value.value == "" {
			continue
		}

		items = append(items, &menuItem{
			labelColumns: []string{value.label, utils.ColoredString(value.value, color.FgCyan)},
			onPress:      func(m *Model) tea.Cmd { return m.copyToClipboard(value.value) },
		})
	}

	if len(items) == 0 {
		m.openError(m.tr.NothingToCopy)
		return
	}

	m.openMenu(menuOptions{title: m.tr.CopyMenuTitle, items: items})
}

func (m *Model) copyToClipboard(value string) tea.Cmd {
	osCommand := m.os

	return func() tea.Msg { return copiedMsg{value: value, err: osCommand.CopyToClipboard(value)} }
}

func (m *Model) onCopied(msg copiedMsg) tea.Cmd {
	if msg.err != nil {
		m.openError(msg.err.Error())
		return nil
	}

	return m.withTransientStatus(fmt.Sprintf("%s %s", m.tr.CopiedToClipboard, msg.value), 2*time.Second)
}

func (m *Model) instanceCopy(instance *commands.Instance) tea.Cmd {
	ipv4, ipv6 := instance.Addresses("inet"), instance.Addresses("inet6")

	all := ""
	if len(ipv4)+len(ipv6) > 1 {
		all = strings.Join(append(ipv4, ipv6...), " ")
	}

	m.copyMenu(
		copyValue{m.tr.CopyName, instance.Name},
		copyValue{"IPv4", first(ipv4)},
		copyValue{"IPv6", first(ipv6)},
		copyValue{m.tr.CopyAllAddresses, all},
	)

	return nil
}

func first(values []string) string {
	if len(values) == 0 {
		return ""
	}

	return values[0]
}
