package ui

import (
	"github.com/Qendolin/fabric-mod-bisect-tool/app/core/imcs"
	"github.com/Qendolin/fabric-mod-bisect-tool/app/core/mods"
	"github.com/Qendolin/fabric-mod-bisect-tool/app/core/sets"
	"github.com/Qendolin/fabric-mod-bisect-tool/app/logging"
	"github.com/rivo/tview"
)

// BisectionViewModel provides a snapshot of the current bisection state,
// tailored for UI consumption. It decouples the UI from the underlying engine's implementation.
type BisectionViewModel struct {
	IsReady            bool
	IsComplete         bool
	IsVerificationStep bool
	StepCount          int
	Iteration          int
	EstimatedMaxTests  int
	LastTestResult     imcs.TestResult
	AllModIDs          []string
	ConflictSet        sets.Set
	CandidateSet       sets.Set
	ClearedSet         sets.Set
	ActiveTestPlan     *imcs.TestPlan
	NextTestPlan       *imcs.TestPlan
	ExecutionLog       []imcs.CompletedTest
}

// AppInterface defines methods the UI layer needs to access from the main App struct.
// It acts as a facade for UI components to interact with the application's core.
type AppInterface interface {
	// --- UI methods & Managers ---
	QueueUpdateDraw(f func()) *tview.Application
	Stop()
	Navigation() *NavigationManager
	Dialogs() *DialogManager
	Layout() *LayoutManager
	GetLogger() *logging.Logger
	GetFocus() tview.Primitive
	SetFocus(tview.Primitive)

	// --- Core Logic ---
	StartLoadingProcess(modsPath string)
	GetViewModel() BisectionViewModel
	GetStateManager() *mods.StateManager // StateManager is still needed for detailed mod info.

	// --- Actions ---
	Step()
	Undo()
	ResetSearch()
}

// Page is an interface that all UI pages must implement.
type Page interface {
	tview.Primitive

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

// PageActivator defines an interface for pages that need to perform an action
// (like refreshing their content) when they become the active page.
type PageActivator interface {
	// OnPageActivated is called by the NavigationManager when the page is switched to.
	OnPageActivated()
}
