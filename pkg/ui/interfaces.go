package ui

import (
	"github.com/Qendolin/fabric-mod-bisect-tool/pkg/core/imcs"
	"github.com/Qendolin/fabric-mod-bisect-tool/pkg/core/sets"
)

// AppController is the top-level controller handed to the UI. It exposes the
// lifecycle and read models, plus accessors for the narrower role controllers.
type AppController interface {
	StartLoadingProcess(modsPath string, quiltSupport, neoForgeSupport bool)

	GetViewModel() BisectionViewModel
	GetResultViewModel() ResultViewModel

	GetBisectionController() BisectionController
	GetModStatusController() ModStatusController
}

// BisectionController defines the operations that drive the bisection search.
type BisectionController interface {
	Step()
	Undo() error
	ResetSearch()
	ContinueSearch()
	Reconcile()
	IsBisectionReady() bool

	CancelTest()
	SubmitTestResult(result imcs.TestResult)
}

// ModStatusController defines the operations to inspect and change the
// per-mod status. Changes are staged via SetOverride and applied atomically
// by Commit, which also triggers a reconciliation.
type ModStatusController interface {
	// GetModStatuses returns the current status of every mod, merged with any
	// staged (not yet committed) overrides.
	GetModStatuses() map[string]ModStatusViewModel

	// SetOverride stages a new override for a single mod. It is not applied
	// until Commit is called.
	SetOverride(id string, override ModStatusOverride)

	// Commit applies all staged overrides to the underlying state manager and
	// triggers a reconciliation.
	Commit()

	// Discard drops all staged overrides without applying them.
	Discard()

	ResolveEffectiveSet(targetSet sets.Set) (effectiveSet sets.Set)
}

// View defines the operations that the business logic can request from the UI.
type View interface {
	Start() error
	Stop()
	Update()

	// Dialogs (Blocking)
	ShowDialogErrorModLoadingGeneric(path string, err error)
	ShowDialogErrorModLoadingNoMods(path string)
	ShowDialogErrorBisectionInitialization(err error)
	ShowDialogErrorBisectionCannotContinue(err error)
	ShowDialogErrorBisectionPrepare(err error)

	ShowDialogInfoBisectionModsMissingExpected(missingMods sets.Set)
	ShowDialogInfoBisectionUnresolvableModsDisabled(disabledMods sets.Set)

	ShowDialogQuestionBisectionContinueWithMissingMods(missingMods sets.Set) bool

	OnLoadingStarted()
	OnLoadingProgress(fileName string, i, count int)
	OnBisectionReady()
	OnTestReady()
	OnIterationComplete()
}
