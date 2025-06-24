package ui

import (
	"context"

	"github.com/Qendolin/fabric-mod-bisect-tool/app/core/conflict"
	"github.com/Qendolin/fabric-mod-bisect-tool/app/core/mods"
	"github.com/Qendolin/fabric-mod-bisect-tool/app/core/systemrunner"
	"github.com/rivo/tview"
)

// AppInterface defines methods the UI layer needs to access from the main App struct.
// It acts as a facade for UI components to interact with the application's core.
type AppInterface interface {
	QueueUpdateDraw(f func()) *tview.Application
	SetFocus(p tview.Primitive) *tview.Application
	GetFocus() tview.Primitive
	GetApplicationContext() context.Context
	GetFocusManager() *FocusManager
	Navigation() *NavigationManager
	Dialogs() *DialogManager
	Layout() *LayoutManager
	GetModLoader() mods.ModLoaderService
	OnModsLoaded(modsPath string, allMods map[string]*mods.Mod, potentialProviders mods.PotentialProvidersMap, sortedModIDs []string)
	StartModLoad(path string)
	Stop()
	GetLogTextView() *tview.TextView
	GetSearcher() *conflict.Searcher
	Step()
	Undo()
	ResetSearch()
	SubmitTestResult(result systemrunner.Result, changes []systemrunner.BatchStateChange)
	CancelTest(changes []systemrunner.BatchStateChange)
	ShowResultPage(title, message, explanation string)
}

// Page is an interface that all UI pages must implement.
type Page interface {
	// Primitive returns the tview.Primitive that represents the page.
	Primitive() tview.Primitive
	// GetActionPrompts returns a map of key-to-description for the footer.
	GetActionPrompts() map[string]string
	// GetStatusPrimitive returns the page specific text view that displays the page's status
	GetStatusPrimitive() *tview.TextView
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
