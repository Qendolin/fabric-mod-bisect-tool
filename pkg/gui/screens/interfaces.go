package screens

import (
	"fyne.io/fyne/v2"
	"github.com/Qendolin/fabric-mod-bisect-tool/pkg/ui"
)

// App defines the interface that screens use to communicate with the GUI App.
type App interface {
	ui.Controller
	ui.View

	// GetWindow returns the main Fyne window.
	GetWindow() fyne.Window
	// NewWindow returns a new Fyne window.
	NewWindow(title string) fyne.Window
}
