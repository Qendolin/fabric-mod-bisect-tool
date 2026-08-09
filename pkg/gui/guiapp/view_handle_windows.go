//go:build windows

package guiapp

import gioapp "gioui.org/app"

// setViewHandle captures the platform window handle used to attach native
// zenity dialogs to the main window.
func (a *App) setViewHandle(e gioapp.ViewEvent) {
	if ev, ok := e.(gioapp.Win32ViewEvent); ok {
		a.attachID = ev.HWND
	}
}
