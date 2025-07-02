package ui

import (
	"github.com/Qendolin/fabric-mod-bisect-tool/pkg/core/imcs"
	"github.com/Qendolin/fabric-mod-bisect-tool/pkg/core/mods"
	"github.com/Qendolin/fabric-mod-bisect-tool/pkg/core/sets"
	"github.com/Qendolin/fabric-mod-bisect-tool/pkg/logging"
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
	Round              int
	EstimatedMaxTests  int
	LastTestResult     imcs.TestResult
	LastFoundElement   string
	AllModIDs          []string
	AllConflictSets    []sets.Set
	CurrentConflictSet sets.Set
	CandidateSet       sets.Set
	ClearedSet         sets.Set
	PendingAdditions   sets.Set
	CurrentTestPlan    *imcs.TestPlan
	ExecutionLog       []imcs.CompletedTest
	QuiltSupport       bool
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
	StartLoadingProcess(modsPath string, quiltSupport bool)
	GetViewModel() BisectionViewModel
	GetStateManager() *mods.StateManager // StateManager is still needed for detailed mod info.

	// --- Actions ---
	Step()
	Undo() bool
	ResetSearch()
	ContinueSearch()
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
