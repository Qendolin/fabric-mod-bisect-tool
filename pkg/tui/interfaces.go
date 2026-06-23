package tui

import (
	"github.com/Qendolin/fabric-mod-bisect-tool/pkg/logging"
	"github.com/Qendolin/fabric-mod-bisect-tool/pkg/ui"
	"github.com/rivo/tview"
)

// TUIApp defines methods the TUI components need to access from the TUI app struct.
type TUIApp interface {
	ui.Controller // Embed the core controller

	// --- UI methods & Managers ---
	QueueUpdateDraw(f func())
	Stop()
	Navigation() *NavigationManager
	Dialogs() *DialogManager
	Layout() *LayoutManager
	GetLogger() *logging.Logger
	GetFocus() tview.Primitive
	SetFocus(tview.Primitive)
}

// SearchStateObserver defines an interface for pages that need to be updated
// when the conflict searcher's state changes.
type SearchStateObserver interface {
	// RefreshSearchState is called to update the page with the latest searcher data.
	RefreshSearchState()
}

// Focusable is an interface for any primitive that contains child elements
// which can be focused. It's used by the FocusManager to build a dynamic focus chain.
type Focusable interface {
	// GetFocusablePrimitives returns a slice of the immediate child primitives
	// that can receive focus.
	GetFocusablePrimitives() []tview.Primitive
}
