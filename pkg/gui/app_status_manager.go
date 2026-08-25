package gui

import (
	"time"

	"github.com/jesseduffield/gocui"
	"github.com/tallica/lazyincus/pkg/utils"
)

type appStatus struct {
	name       string
	statusType string
	duration   int
}

type statusManager struct {
	statuses []appStatus
}

func (m *statusManager) removeStatus(name string) {
	newStatuses := []appStatus{}
	for _, status := range m.statuses {
		if status.name != name {
			newStatuses = append(newStatuses, status)
		}
	}
	m.statuses = newStatuses
}

func (m *statusManager) addWaitingStatus(name string) {
	m.removeStatus(name)
	newStatus := appStatus{
		name:       name,
		statusType: "waiting",
		duration:   0,
	}
	m.statuses = append([]appStatus{newStatus}, m.statuses...)
}

func (m *statusManager) addTransientStatus(name string) {
	m.removeStatus(name)
	newStatus := appStatus{
		name:       name,
		statusType: "transient",
		duration:   0,
	}
	m.statuses = append([]appStatus{newStatus}, m.statuses...)
}

func (m *statusManager) getStatusString() string {
	if len(m.statuses) == 0 {
		return ""
	}
	topStatus := m.statuses[0]
	if topStatus.statusType == "waiting" {
		return topStatus.name + " " + utils.Loader()
	}
	return topStatus.name
}

// WithWaitingStatus wraps a function and shows a waiting status while the function is still executing
func (gui *Gui) WithWaitingStatus(name string, f func() error) error {
	go func() {
		gui.statusManager.addWaitingStatus(name)

		defer func() {
			gui.statusManager.removeStatus(name)
		}()

		go func() {
			ticker := time.NewTicker(time.Millisecond * 50)
			defer ticker.Stop()
			for range ticker.C {
				appStatus := gui.statusManager.getStatusString()
				if appStatus == "" {
					return
				}
				if err := gui.renderString(gui.g, "appStatus", appStatus); err != nil {
					gui.Log.Warn(err)
				}
			}
		}()

		if err := f(); err != nil {
			gui.g.Update(func(g *gocui.Gui) error {
				return gui.createErrorPanel(err.Error())
			})
		}
	}()

	return nil
}

// WithTransientStatus flashes a message in the app status view for the given
// duration, for actions that complete instantly and so have no waiting status
// of their own to show.
func (gui *Gui) WithTransientStatus(name string, duration time.Duration) {
	go func() {
		gui.statusManager.addTransientStatus(name)
		gui.renderAppStatus()

		time.Sleep(duration)

		gui.statusManager.removeStatus(name)
		gui.renderAppStatus()
	}()
}

func (gui *Gui) renderAppStatus() {
	gui.g.Update(func(g *gocui.Gui) error {
		return gui.renderString(g, "appStatus", gui.statusManager.getStatusString())
	})
}
