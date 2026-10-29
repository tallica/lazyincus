package tui

import (
	"slices"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/tallica/lazyincus/pkg/utils"
)

// spinnerInterval is pkg/gui's: utils.Loader turns once per 50ms.
const spinnerInterval = 50 * time.Millisecond

// appStatus is one of pkg/gui's statusManager's, the newest first.
type appStatus struct {
	name    string
	waiting bool
}

type waitingDoneMsg struct {
	name string
	err  error
	then func(error) tea.Cmd
}

// heldStatusMsg is what a waiting status's then led to, the status on until it lands.
type heldStatusMsg struct {
	name string
	msg  tea.Msg
}

type transientDoneMsg struct{ name string }

type spinnerMsg struct{}

// withWaitingStatus is pkg/gui's WithWaitingStatus, f run off the loop.
func (m *Model) withWaitingStatus(name string, f func() error) tea.Cmd {
	return m.withWaitingStatusThen(name, f, nil)
}

// withWaitingStatusThen runs then on the loop with f's error; the status waits for its Cmd too.
func (m *Model) withWaitingStatusThen(name string, f func() error, then func(error) tea.Cmd) tea.Cmd {
	m.addStatus(appStatus{name: name, waiting: true})

	return tea.Batch(m.spin(), func() tea.Msg { return waitingDoneMsg{name: name, err: f(), then: then} })
}

// withTransientStatus is pkg/gui's WithTransientStatus.
func (m *Model) withTransientStatus(name string, duration time.Duration) tea.Cmd {
	m.addStatus(appStatus{name: name})

	return m.tick(duration, transientDoneMsg{name: name})
}

func (m *Model) addStatus(status appStatus) {
	m.removeStatus(status.name)
	m.statuses = append([]appStatus{status}, m.statuses...)
}

func (m *Model) removeStatus(name string) {
	m.statuses = slices.DeleteFunc(m.statuses, func(status appStatus) bool { return status.name == name })
}

func (m *Model) spin() tea.Cmd {
	if m.spinning {
		return nil
	}

	m.spinning = true

	return m.tick(spinnerInterval, spinnerMsg{})
}

func (m *Model) onSpinner() tea.Cmd {
	m.spinning = false
	if !slices.ContainsFunc(m.statuses, func(status appStatus) bool { return status.waiting }) {
		return nil
	}

	return m.spin()
}

func (m *Model) onWaitingDone(msg waitingDoneMsg) tea.Cmd {
	if msg.then == nil {
		m.removeStatus(msg.name)
		if msg.err != nil {
			m.openError(msg.err.Error())
		}
		return nil
	}

	next := msg.then(msg.err)
	if next == nil {
		m.removeStatus(msg.name)
		return nil
	}

	return func() tea.Msg { return heldStatusMsg{name: msg.name, msg: next()} }
}

func (m *Model) onHeldStatus(msg heldStatusMsg) tea.Cmd {
	cmd := m.update(msg.msg)
	m.removeStatus(msg.name)

	return cmd
}

// fitStatus is pkg/gui's getStatusString.
func (m *Model) fitStatus() {
	if len(m.statuses) == 0 {
		m.appStatus = ""
		return
	}

	top := m.statuses[0]
	if top.waiting {
		m.appStatus = top.name + " " + utils.Loader()
		return
	}
	m.appStatus = top.name
}
