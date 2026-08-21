package gui

// excludes popups
func (gui *Gui) currentStaticWindowName() string {
	return gui.currentStaticViewName()
}

func (gui *Gui) currentSideWindowName() string {
	return gui.currentSideViewName()
}
