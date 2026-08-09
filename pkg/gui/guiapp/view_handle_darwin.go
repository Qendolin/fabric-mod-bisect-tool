//go:build darwin && !ios

package guiapp

import (
	"os"

	gioapp "gioui.org/app"
)

// setViewHandle captures the platform window handle used to attach native
// zenity dialogs to the main window. zenity attaches by process id on macOS.
func (a *App) setViewHandle(e gioapp.ViewEvent) {
	if _, ok := e.(gioapp.AppKitViewEvent); ok {
		a.attachID = os.Getpid()
	}
}
