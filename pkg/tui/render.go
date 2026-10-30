package tui

import (
	"fmt"
	"slices"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/jesseduffield/lazycore/pkg/boxlayout"
)

// render layers each window at its rectangle, ID'd by name for hit-testing.
func (m *Model) render() string {
	if m.width == 0 || m.height == 0 {
		return ""
	}

	dims := m.dims()
	layers := []*lipgloss.Layer{}
	place := func(id string, content string) {
		d := dims[id]
		layers = append(layers, lipgloss.NewLayer(content).ID(id).X(d.X0).Y(d.Y0))
	}

	if _, ok := dims[limitView]; ok {
		w, h := size(dims[limitView])
		place(limitView, m.theme.frame(frameSpec{title: m.tr.NotEnoughSpace}, w, h))

		return m.compose(layers)
	}

	for index, window := range m.windowNames() {
		if d, ok := dims[window]; ok {
			place(window, m.windowFrame(window, index, d))
		}
	}

	if d, ok := dims[mainView]; ok {
		place(mainView, m.mainFrame(d))
	}

	if d, ok := dims[optionsView]; ok {
		w, _ := size(d)
		place(optionsView, fit(tint(m.options(), m.theme.options), w))
	}

	if d, ok := dims[appStatusView]; ok {
		w, _ := size(d)
		place(appStatusView, fit(tint(m.appStatus, "\x1b[36m"), w))
	}

	if _, ok := dims[informationView]; ok {
		place(informationView, tint(m.information(), "\x1b[32m"))
	}

	if _, ok := dims[filterPrefixView]; ok {
		place(filterPrefixView, tint(m.filterPrompt(), promptColour))
	}

	if d, ok := dims[filterView]; ok {
		w, _ := size(d)
		place(filterView, m.filterLine(w))
	}

	for i, p := range m.popups {
		x0, y0, x1, y1, lines := m.popupLayout(p)
		spec := frameSpec{title: p.title, subtitle: p.fittedSubtitle, body: lines, focused: i == len(m.popups)-1, total: len(lines)}
		if p.menu != nil {
			// gocui's menu highlights its selection focused or not.
			spec.body = m.rowsBody(p.menu.list, x1-x0-1, y1-y0-1, m.theme.selectedBg)
			spec.origin = p.menu.list.origin
		}
		id := p.view
		if p.snapshot != nil {
			id, spec.focused = confirmationView, spec.focused && p.view == confirmationView
			layers = append(layers, lipgloss.NewLayer(m.snapshotOptionsFrame(p, x0, x1)).ID(snapshotOptionsView).X(x0).Y(y1+1).Z(i+1))
		}
		frame := m.theme.frame(spec, x1-x0+1, y1-y0+1)
		layers = append(layers, lipgloss.NewLayer(frame).ID(id).X(x0).Y(y0).Z(i+1))
	}

	return m.compose(layers)
}

func (m *Model) compose(layers []*lipgloss.Layer) string {
	canvas := lipgloss.NewCanvas(m.width, m.height)
	canvas.Compose(lipgloss.NewCompositor(layers...))

	return canvas.Render()
}

func (m *Model) dims() map[string]boxlayout.Dimensions {
	return m.dimensions(m.width, m.height, m.information(), m.appStatus)
}

func size(d boxlayout.Dimensions) (int, int) {
	return d.X1 - d.X0 + 1, d.Y1 - d.Y0 + 1
}

func (m *Model) windowFrame(window string, index int, d boxlayout.Dimensions) string {
	w, h := size(d)
	p := m.activeInWindow(window)

	spec := frameSpec{
		prefix:  titlePrefix(index),
		title:   m.sideTitle(p, window),
		tabs:    m.windowTabs(window, index, w),
		tab:     slices.Index(m.windowPanels(window), p),
		body:    m.listBody(p, w-2, h-2),
		focused: m.currentView() == p.name,
		colour:  remoteColour(m.cfg.UserConfig, m.incus.RemoteName()),
		total:   max(len(p.rows), 1),
		origin:  p.origin,
	}

	return m.theme.frame(spec, w, h)
}

// sideTitle is pkg/gui's titleStacks: collapsed, Stacks and Instances count.
func (m *Model) sideTitle(p *panel, window string) string {
	if !m.stacksCollapsed(window, m.windowOf(m.currentSideView())) {
		return p.title()
	}

	switch p.name {
	case "stacks":
		return fmt.Sprintf(m.tr.TitleCount, p.title(), len(p.cells))
	case "instances":
		return fmt.Sprintf(m.tr.TitleCount, p.title(), p.count())
	case "services":
		return m.tr.ServicesTitle
	}

	return p.title()
}

// listBody is a list's visible rows, highlighted as gocui's onFocusChange.
func (m *Model) listBody(p *panel, width, rows int) []string {
	bg := ""
	switch {
	case m.currentView() == p.name:
		bg = m.theme.selectedBg
	case m.currentSideView() == p.name:
		bg = m.theme.inactiveSelected
	}

	return m.rowsBody(p, width, rows, bg)
}

func (m *Model) rowsBody(p *panel, width, rows int, bg string) []string {
	all := p.rows
	end := min(p.origin+rows, len(all))
	body := make([]string, 0, max(end-p.origin, 0))
	for i := p.origin; i < end; i++ {
		if i == p.selected {
			body = append(body, highlight(all[i], width, bg))
			continue
		}
		body = append(body, all[i])
	}

	return body
}

func (m *Model) mainFrame(d boxlayout.Dimensions) string {
	w, h := size(d)
	p := m.currentSidePanel()

	spec := frameSpec{
		focused: m.currentView() == mainView,
		colour:  remoteColour(m.cfg.UserConfig, m.incus.RemoteName()),
	}

	if p.mainTabs != nil && p.count() > 0 {
		tabs := p.mainTabs()
		for _, tab := range tabs {
			spec.tabs = append(spec.tabs, tab.title)
		}
		spec.tab = p.mainTab
	}
	spec.body = m.main.visible(h - 2)
	spec.total, spec.origin = len(m.main.lines), m.main.oy

	return m.theme.frame(spec, w, h)
}

// tint is a gocui view's FgColor: a reset in s returns to it, not default.
func tint(s, colour string) string {
	if colour == "" {
		return s
	}

	return colour + strings.ReplaceAll(s, reset, reset+colour) + reset
}

// options are the footer's key hints, as pkg/gui's optionsMapToString.
func (m *Model) options() string {
	options := map[string]string{
		"PgUp/PgDn": m.tr.Scroll,
		"← → ↑ ↓":   m.tr.Navigate,
		"q":         m.tr.Quit,
		"x":         m.tr.Menu,
	}

	if len(m.popups) > 0 {
		top := m.popups[len(m.popups)-1]
		switch {
		case top.menu != nil:
			options = m.menuOptionsMap(top.menu)
		case top.view == snapshotOptionsView:
			options = m.snapshotOptionsKeys()
		case top.input != nil:
			options = map[string]string{"esc": m.tr.Close, "enter": m.tr.Submit}
		case top.asks:
			options = map[string]string{"n/esc": m.tr.No, "y/enter": m.tr.Yes}
			if top.skipID != "" {
				options["a"] = m.tr.YesAlways
			}
		default:
			options = map[string]string{"esc/enter": m.tr.Close}
		}
	}

	pairs := make([]string, 0, len(options))
	for key, description := range options {
		pairs = append(pairs, key+": "+description)
	}
	slices.Sort(pairs)

	return strings.Join(pairs, ", ")
}

// information is pkg/gui's getInformationContent.
func (m *Model) information() string {
	return infoSectionPadding + m.operationsStatus() + m.warningsStatus() + m.incusStatus() + m.cfg.Version
}

func (m *Model) incusStatus() string {
	remote := m.incus.RemoteName()
	version := m.incus.ServerVersion()
	if remote == "" && version == "" {
		return ""
	}

	label := "Incus"
	if version != "" {
		label += " v" + version
	}

	scope := remote
	if colour := remoteColour(m.cfg.UserConfig, remote); colour != "" && remote != "" {
		scope = colour + remote + reset
	}

	project := m.incus.ProjectName()
	if m.incus.IsAllProjects() {
		project = m.tr.AllProjects
	}
	if project != "" {
		scope = strings.TrimPrefix(scope+"/"+project, "/")
	}
	if scope != "" {
		label += " (" + scope + ")"
	}

	if m.cfg.ReadOnly || m.cfg.UserConfig.Remotes[remote].ReadOnly {
		label += " \x1b[33m" + m.tr.ReadOnly + reset
	}

	if m.incus.IsConnected() {
		label += " \x1b[32m●" + reset
	} else {
		label += " \x1b[31m✗" + reset
	}

	return label + "  "
}
