//go:build (linux && !android) || freebsd || openbsd

package guiapp

import gioapp "gioui.org/app"

// setViewHandle captures the platform window handle used to attach native
// zenity dialogs to the main window. X11 has a window id; Wayland has no
// usable id that zenity accepts, so those dialogs stay unattached.
func (a *App) setViewHandle(e gioapp.ViewEvent) {
	switch ev := e.(type) {
	case gioapp.X11ViewEvent:
		a.attachID = int(ev.Window)
	case gioapp.WaylandViewEvent:
		a.attachID = nil
	}
}
