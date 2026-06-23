package app

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/Qendolin/fabric-mod-bisect-tool/pkg/core/bisect"
	"github.com/Qendolin/fabric-mod-bisect-tool/pkg/core/imcs"
	"github.com/Qendolin/fabric-mod-bisect-tool/pkg/core/mods"
	"github.com/Qendolin/fabric-mod-bisect-tool/pkg/core/sets"
	"github.com/Qendolin/fabric-mod-bisect-tool/pkg/embeds"
	"github.com/Qendolin/fabric-mod-bisect-tool/pkg/logging"
	"github.com/Qendolin/fabric-mod-bisect-tool/pkg/ui"
)

// App orchestrates the bisection application, managing the lifecycle and core services.
type App struct {
	view   ui.View
	logger *logging.Logger

	// Core Service (only initialized after successful loading)
	bisectSvc *bisect.Service

	cliArgs CLIArgs
}

// NewApp creates and initializes the application logic.
func NewApp(logger *logging.Logger, cliArgs *CLIArgs) *App {
	a := &App{
		logger:  logger,
		cliArgs: *cliArgs,
	}
	return a
}

func (a *App) SetView(view ui.View) {
	a.view = view
}

func (a *App) StartLoadingProcess(modsPath string, quiltSupport, neoForgeSupport bool) {
	a.view.SwitchToLoadingPage()

	go func() {
		defer logging.HandlePanic()
		overrides := a.loadAndMergeOverrides(modsPath)

		loader := mods.ModLoader{ModParser: mods.ModParser{QuiltParsing: quiltSupport, NeoForgeParsing: neoForgeSupport}}
		logging.Infof("App: Loading mods from '%s', Quilt Support: %v", modsPath, a.cliArgs.QuiltSupport)
		allMods, providers, _, loadErr := loader.LoadMods(modsPath, overrides, a.view.UpdateLoadingProgress)

		// Signal the main thread to handle the result.
		a.view.QueueUpdateDraw(func() {
			a.onLoadingComplete(modsPath, allMods, providers, loadErr)
		})
	}()
}

func (a *App) onLoadingComplete(modsPath string, allMods map[string]*mods.Mod, providers mods.PotentialProvidersMap, err error) {
	if err != nil {
		logging.Errorf("App: Failed to load mods: %v", err)
		a.view.ShowErrorDialog("Loading Error", "Failed to load mods!", err, func() {
			a.view.SwitchToSetupPage()
		})
		return
	}
	if len(allMods) == 0 {
		logging.Errorf("App: No mods were found in '%s'.", modsPath)
		a.view.ShowErrorDialog("Information", fmt.Sprintf("No mods were found in '%s'.\nPlease ensure that you've entered the path correctly.", modsPath), nil, func() {
			a.view.SwitchToSetupPage()
		})
		return
	}

	// Loading was successful, now create the runtime services.
	stateMgr := mods.NewStateManager(allMods, providers)
	activator := mods.NewModActivator(modsPath, allMods)

	svc, err := bisect.NewService(stateMgr, activator)
	if err != nil {
		logging.Errorf("App: Failed to initialize the bisection service: %v", err)
		a.view.ShowErrorDialog("Initialization Error", "Failed to initialize the bisection!", err, func() {
			a.view.SwitchToSetupPage()
		})
		return
	}

	a.bisectSvc = svc
	a.bisectSvc.OnStateChange = a.handleCoreStateChange
	a.bisectSvc.ResetSearch()
	a.view.SwitchToMainPage()
	a.handleCoreStateChange()
}

func (a *App) handleCoreStateChange() {
	if a.view != nil {
		a.view.RefreshSearchState()
	}
}

func (a *App) Reconcile(callback func()) {
	if a.bisectSvc.NeedsReconciliation() {
		logging.Debugf("App: Reconciliation triggered and needed.")
		report := a.bisectSvc.ReconcileState()
		if report.HasChanges {
			a.showReconciliationReport(&report, callback)
			return
		}
	}
	if callback != nil {
		callback()
	}
}

// Step orchestrates the next bisection test.
func (a *App) Step() {
	if !a.IsBisectionReady() {
		return
	}
	plan, changes, err := a.bisectSvc.PlanAndApplyNextTest()
	if err != nil {
		a.bisectSvc.Engine().InvalidateActivePlan()
		a.handleStepError(err)
		return
	}

	onSuccess := func() { a.bisectSvc.SubmitTestResult(imcs.TestResultGood, changes) }
	onFailure := func() { a.bisectSvc.SubmitTestResult(imcs.TestResultFail, changes) }
	onCancel := func() { a.bisectSvc.CancelTest(changes) }

	a.view.ShowTestModal(plan.IsVerificationStep, onSuccess, onFailure, onCancel)
}

func (a *App) ContinueSearch() {
	if !a.IsBisectionReady() {
		return
	}
	logging.Debugf("App: ContinueSearch action triggered.")

	report, err := a.bisectSvc.ContinueSearch()
	if err != nil {
		a.view.ShowErrorDialog("Unexpected Error", "Cannot continue the search!", err, nil)
		return
	}

	if len(report.ModsSetUnresolvable) > 0 {
		a.view.ShowInfoDialog(
			"Unresolvable Mods Disabled",
			"To continue the search, the following mods were automatically disabled because their dependencies can no longer be met:",
			sets.FormatSet(report.ModsSetUnresolvable).String(),
			nil,
		)
	}
}

func (a *App) Undo() bool {
	logging.Debugf("App: Undo action triggered.")
	err := a.bisectSvc.UndoLastStep()
	if errors.Is(err, bisect.ErrUndoStackEmpty) {
		a.view.ShowInfoDialog("Cannot Undo", "Nothing left to undo.", "", nil)
		return false
	}
	if err != nil {
		logging.Errorf("App: Undo failed: %v", err)
		a.view.ShowInfoDialog("Cannot Undo", "The undo operation failed or there were no more steps to undo.", "", nil)
		return false
	}
	return true
}

func (a *App) ResetSearch() {
	logging.Debugf("App: ResetSearch faction triggered.")
	a.bisectSvc.ResetSearch()
	a.Reconcile(nil)
}

func (a *App) IsBisectionReady() bool {
	return a.bisectSvc != nil
}

func (a *App) displayResults() {
	if !a.IsBisectionReady() {
		return
	}
	state := a.bisectSvc.GetCurrentState()
	if state.IsComplete || a.bisectSvc.Engine().WasLastTestVerification() {
		a.view.SwitchToResultPage()
	}
}

// loadAndMergeOverrides handles the layered loading and merging of dependency overrides.
func (a *App) loadAndMergeOverrides(modsPath string) *mods.DependencyOverrides {
	var allOverrides []*mods.DependencyOverrides

	cwd, _ := os.Getwd()
	cwdPath := filepath.Join(cwd, "fabric_loader_dependencies.json")
	if cwdOverrides, err := mods.LoadDependencyOverridesFromPath(cwdPath, mods.OverrideSourceUserProvided); err != nil {
		if !os.IsNotExist(err) {
			logging.Warnf("App: Could not load dependency overrides from '%s': %v", cwdPath, err)
		}
	} else {
		logging.Infof("App: Loaded dependency overrides from current directory.")
		allOverrides = append(allOverrides, cwdOverrides)
	}

	configPath := filepath.Join(modsPath, "..", "config", "fabric_loader_dependencies.json")
	if configOverrides, err := mods.LoadDependencyOverridesFromPath(configPath, mods.OverrideSourceUserProvided); err != nil {
		if !os.IsNotExist(err) {
			logging.Warnf("App: Could not load dependency overrides from '%s': %v", configPath, err)
		}
	} else {
		logging.Infof("App: Loaded dependency overrides from config directory.")
		allOverrides = append(allOverrides, configOverrides)
	}

	if !a.cliArgs.NoEmbeddedOverrides {
		if embedded, err := mods.LoadDependencyOverrides(bytes.NewReader(embeds.GetEmbeddedOverrides()), mods.OverrideSourceBuiltin); err != nil {
			logging.Errorf("App: Failed to load embedded dependency overrides: %v", err)
		} else {
			logging.Infof("App: Loaded embedded dependency overrides.")
			allOverrides = append(allOverrides, embedded)
		}
	}

	return mods.MergeDependencyOverrides(allOverrides...)
}

func (a *App) handleStepError(err error) {
	if errors.Is(err, imcs.ErrSearchComplete) {
		logging.Infof("App: Step error, bisection complete: %s", err)
		a.displayResults()
		return
	}

	if missingErr, ok := err.(*mods.MissingFilesError); ok {
		logging.Warnf("App: Step error, missing files: %v", missingErr)

		vm := a.GetViewModel()

		allKnownConflicts := sets.Copy(vm.CurrentConflictSet)
		for _, s := range vm.AllConflictSets {
			allKnownConflicts = sets.Union(allKnownConflicts, s)
		}

		unexpectedDeletions := make(sets.Set)
		expectedDeletions := make(sets.Set)
		var missingIDs []string

		for _, e := range missingErr.Errors {
			missingIDs = append(missingIDs, e.ModID)
			if _, isProblem := allKnownConflicts[e.ModID]; isProblem {
				expectedDeletions[e.ModID] = struct{}{}
			} else {
				unexpectedDeletions[e.ModID] = struct{}{}
			}
		}

		if len(unexpectedDeletions) > 0 {
			a.view.ShowQuestionDialog(
				"Missing Mod Files Detected",
				"The following mod files were unexpectedly missing. Do you want to continue the search without them?",
				sets.FormatSet(unexpectedDeletions).String(),
				func() {
					logging.Infof("App: Disabling %d mods that are undexpectedly missing: %v", len(missingIDs), missingIDs)
					a.bisectSvc.StateManager().SetMissingBatch(missingIDs, true)
					a.Step()
				},
				nil,
			)
		} else {
			a.view.ShowInfoDialog(
				"Known Problematic Mod(s) Removed",
				"The following mod(s), which were part of a known conflict set, have been detected as missing. This is expected. The search will now proceed with the updated mod list.",
				sets.FormatSet(expectedDeletions).String(),
				func() {
					logging.Infof("App: Disabling %d mods that are expectedly missing: %v", len(missingIDs), missingIDs)
					a.bisectSvc.StateManager().SetMissingBatch(missingIDs, true)
					a.Step()
				},
			)
		}
		return
	}

	if errors.Is(err, bisect.ErrNeedsReconciliation) {
		report := a.bisectSvc.ReconcileState()
		if report.HasChanges {
			a.showReconciliationReport(&report, a.Step)
		} else {
			logging.Error("App: Reconciliation triggered by ErrNeedsReconciliation but reconciliation yielded no changes.")
			a.Step()
		}
		return
	}

	logging.Errorf("App: Step error: %v", err)

	a.view.ShowErrorDialog("Bisection Error", "An error occurred and the next step could not be prepared.\nIf another program, like Minecraft, is currently acessing your mods, please close it.\n\nPlease check the application log for details.", nil, nil)
}

func (a *App) showReconciliationReport(report *bisect.ActionReport, callback func()) {
	if len(report.ModsSetUnresolvable) > 0 {
		a.view.ShowInfoDialog(
			"Disabled Mods",
			"The following mods were automatically disabled due to unmet dependencies:",
			sets.FormatSet(report.ModsSetUnresolvable).String(),
			callback,
		)
		return
	}
	if callback != nil {
		logging.Info("App: Reconciliation report has no 'Unresolvable Mods' changes. This is odd. Calling callback directly.")
		callback()
	}
}

func (a *App) GetLogger() *logging.Logger { return a.logger }

func (a *App) GetViewModel() ui.BisectionViewModel {
	vm := ui.BisectionViewModel{
		IsReady:         false,
		QuiltSupport:    a.cliArgs.QuiltSupport,
		NeoForgeSupport: a.cliArgs.NeoForgeSupport,
	}
	if !a.IsBisectionReady() {
		return vm
	}

	engine := a.bisectSvc.Engine()
	enumState := a.bisectSvc.EnumerationState()
	state := engine.GetCurrentState()
	currentPlan, _ := engine.GetCurrentTestPlan()

	isVerification := currentPlan != nil && currentPlan.IsVerificationStep

	vm.IsReady = true
	vm.IsComplete = state.IsComplete
	vm.IsVerificationStep = isVerification
	vm.StepCount = engine.GetStepCount()
	vm.Iteration = state.Iteration
	vm.Round = state.Round
	vm.EstimatedMaxTests = engine.GetEstimatedMaxTests()
	vm.LastTestResult = state.LastTestResult
	vm.AllConflictSets = enumState.FoundConflictSets
	vm.CurrentConflictSet = state.ConflictSet
	vm.LastFoundElement = state.LastFoundElement
	vm.AllModIDs = state.AllModIDs
	vm.CandidateSet = state.GetCandidateSet()
	vm.ClearedSet = state.GetClearedSet()
	vm.PendingAdditions = engine.GetPendingAdditions()
	vm.CurrentTestPlan = currentPlan
	vm.ExecutionLog = a.bisectSvc.GetCombinedExecutionLog()
	vm.CanUndo = a.bisectSvc.Engine().UndoCount() > 0

	return vm
}

func (a *App) GetStateManager() *mods.StateManager { return a.bisectSvc.StateManager() }
