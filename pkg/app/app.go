package app

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

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
	adapter   *mods.FileAdapter
	// loader is the mod loader the search was actually started with (set once
	// loading begins). It may differ from the preferred loader requested via
	// the command line.
	loader mods.RunLoader

	// Staged, not yet committed, per-mod overrides for the manage mods page.
	// stagedMu guards stagedOverrides: staging is done on the UI event loop,
	// but Commit may run on a worker goroutine.
	stagedOverrides map[string]ui.ModStatusOverride
	stagedMu        sync.Mutex

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

func (a *App) StartLoadingProcess(modsPath string, loader mods.RunLoader) {
	a.view.OnLoadingStarted()
	a.loader = loader

	a.adapter = &mods.FileAdapter{BaseDirectory: modsPath}

	go func() {
		defer logging.HandlePanic()
		overrides := a.loadAndMergeOverrides(modsPath)

		modLoader := mods.ModLoader{ModParser: mods.ModParser{RunLoader: loader}, Adapter: a.adapter}
		logging.Infof("App: Loading mods from '%s', Loader: %s", modsPath, loader.String())
		allMods, providers, _, loadErr := modLoader.LoadMods(modsPath, overrides, a.view.OnLoadingProgress)

		a.onLoadingComplete(modsPath, allMods, providers, loadErr)
	}()
}

func (a *App) onLoadingComplete(modsPath string, allMods map[string]*mods.Mod, providers mods.PotentialProvidersMap, err error) {
	if err != nil {
		logging.Errorf("App: Failed to load mods: %v", err)
		a.view.ShowDialogErrorModLoadingGeneric(modsPath, err)
		return
	}
	if len(allMods) == 0 {
		logging.Errorf("App: No mods were found in '%s'.", modsPath)
		a.view.ShowDialogErrorModLoadingNoMods(modsPath)
		return
	}

	// Loading was successful, now create the runtime services.
	stateMgr := mods.NewStateManager(allMods, providers)
	activator := mods.NewModActivator(a.adapter, allMods)

	svc, err := bisect.NewService(stateMgr, activator)
	if err != nil {
		logging.Errorf("App: Failed to initialize the bisection service: %v", err)
		a.view.ShowDialogErrorBisectionInitialization(err)
		return
	}

	a.bisectSvc = svc
	a.bisectSvc.ResetSearch()

	// Initial reconciliation scans the full mod set for unresolvable mods and
	// reports the directly-unresolvable roots so the UI can ask the user what
	// to do with each of them.
	report := a.bisectSvc.ReconcileState()
	if len(report.ModsUnresolvable) > 0 {
		a.view.OnUnresolvableMods(a.buildUnresolvableModInfos(report.ModsUnresolvable))
		return
	}
	a.view.OnBisectionReady()
}

// buildUnresolvableModInfos converts the reconcile report's directly-unresolvable
// mods (mod id -> failing dependencies) into a deterministic list for the UI.
func (a *App) buildUnresolvableModInfos(mods map[string][]string) []ui.UnresolvableModInfo {
	allMods := a.bisectSvc.StateManager().GetAllMods()
	ids := make([]string, 0, len(mods))
	for id := range mods {
		ids = append(ids, id)
	}
	sort.Strings(ids)

	infos := make([]ui.UnresolvableModInfo, 0, len(ids))
	for _, id := range ids {
		infos = append(infos, ui.UnresolvableModInfo{
			Mod:         makeModVM(id, allMods),
			DepsDisplay: formatDependencyRefs(allMods[id], mods[id]),
		})
	}
	return infos
}

// formatDependencyRefs renders each failing dependency id together with its
// version predicates, e.g. "nonexistent (>=1.0)", one entry per dependency.
func formatDependencyRefs(mod *mods.Mod, depIDs []string) []string {
	refs := make([]string, 0, len(depIDs))
	for _, depID := range depIDs {
		ref := depID
		if mod != nil {
			if predicates := mod.Metadata.Depends[depID]; len(predicates) > 0 {
				parts := make([]string, 0, len(predicates))
				for _, p := range predicates {
					parts = append(parts, p.String())
				}
				ref = fmt.Sprintf("%s (%s)", depID, strings.Join(parts, ", "))
			}
		}
		refs = append(refs, ref)
	}
	return refs
}

// ResolveUnresolvableMods applies the user's decisions from the unresolvable
// mods screen. Mods marked UnresolvableModActionIgnore have their failing
// dependencies dropped and stay active; everything else stays disabled. The
// state is reconciled afterwards.
func (a *App) ResolveUnresolvableMods(decisions map[string]ui.UnresolvableModAction) {
	if !a.IsBisectionReady() {
		return
	}
	details := a.bisectSvc.DirectlyUnresolvableMods()
	for modID, action := range decisions {
		if action == ui.UnresolvableModActionIgnore {
			a.bisectSvc.StateManager().RemoveDependencies(modID, details[modID])
		}
	}
	a.bisectSvc.ReconcileState()
}

// CompleteLoading finishes the loading phase. After the unresolvable mods
// screen's decisions have been applied, it merges any re-added mods so they
// participate in the search immediately, and signals the UI that loading is
// done.
func (a *App) CompleteLoading() {
	if !a.IsBisectionReady() {
		return
	}
	a.bisectSvc.Engine().MergePendingAdditions()
	a.view.OnBisectionReady()
}

func (a *App) Reconcile() {
	logging.Debugf("App: Reconciliation triggered.")
	report := a.bisectSvc.ReconcileState()
	if report.HasChanges {
		a.showReconciliationReport(&report)
	}
	a.view.Update()
}

// Step orchestrates the next bisection test.
func (a *App) Step() {
	if !a.IsBisectionReady() {
		return
	}
	err := a.bisectSvc.PlanAndApplyNextTest()
	if err != nil {
		a.bisectSvc.Engine().InvalidateActivePlan()
		a.handleStepError(err)
		a.view.Update()
		return
	}

	a.view.OnTestReady()
	a.view.Update()
}

func (a *App) SubmitTestResult(result imcs.TestResult) {
	a.bisectSvc.SubmitTestResult(result)
	state := a.bisectSvc.GetCurrentState()
	if state.IsHalted {
		a.showHaltedPage()
	} else {
		a.displayResults()
	}
	a.view.Update()
}

// showHaltedPage shows the halt page with the two candidate halves that
// were being tested when the search halted, mirroring the split the bisection
// algorithm performs on the current candidate set.
func (a *App) showHaltedPage() {
	candidateSlice := sets.MakeSlice(a.bisectSvc.GetCurrentState().GetCandidateSet())
	groupA, groupB := sets.Split(candidateSlice)
	a.view.OnBisectionHalted(sets.MakeSet(groupA), sets.MakeSet(groupB))
}

func (a *App) CancelTest() {
	a.bisectSvc.CancelTest()
	a.view.Update()
}

func (a *App) GetBisectionController() ui.BisectionController {
	return a
}

func (a *App) GetModStatusController() ui.ModStatusController {
	return a
}

func (a *App) ResolveEffectiveSet(targetSet sets.Set) (effectiveSet sets.Set) {
	if !a.IsBisectionReady() {
		return sets.Set{}
	}
	return a.bisectSvc.StateManager().ResolveEffectiveSet(targetSet).EffectiveSet
}

// RestoreInitialModState restores the on-disk mod files to the state they were
// in when the bisection first loaded them. It is best-effort: mods that cannot
// be restored (e.g. missing files) are logged and skipped. Safe to call even if
// no mods were loaded. It is intended to be called when the application exits,
// so user mod files are left as they were found.
func (a *App) RestoreInitialModState() {
	if !a.IsBisectionReady() {
		return
	}
	a.bisectSvc.Activator().RestoreInitialState()
}

// GetModStatuses returns a serializable snapshot of every mod's status, merged
// with any staged (not yet committed) overrides.
func (a *App) GetModStatuses() map[string]ui.ModStatusViewModel {
	if !a.IsBisectionReady() {
		return map[string]ui.ModStatusViewModel{}
	}

	allMods := a.bisectSvc.StateManager().GetAllMods()
	result := make(map[string]ui.ModStatusViewModel, len(allMods))

	a.stagedMu.Lock()
	staged := a.stagedOverrides
	a.stagedMu.Unlock()

	for id, status := range a.bisectSvc.StateManager().GetModStatusesSnapshot() {
		vm := ui.ModStatusViewModel{
			ModViewModel:   makeModVM(id, allMods),
			IsMissing:      status.IsMissing,
			IsProblematic:  status.IsProblematic,
			IsUnresolvable: status.IsUnresolvable,
			IsUserEditable: !status.IsMissing,
		}
		if override, ok := staged[id]; ok {
			vm.Override = override
		} else {
			vm.Override = overrideFromStatus(status)
		}
		result[id] = vm
	}
	return result
}

// overrideFromStatus maps a committed mod status to its override enum.
func overrideFromStatus(status mods.ModStatus) ui.ModStatusOverride {
	switch {
	case status.ForceEnabled:
		return ui.ModOverrideForceEnabled
	case status.ForceDisabled:
		return ui.ModOverrideForceDisabled
	case status.Omitted:
		return ui.ModOverrideOmitted
	default:
		return ui.ModOverrideNone
	}
}

// SetOverride stages a new override for a single mod. It does not touch the
// underlying state until Commit is called.
func (a *App) SetOverride(id string, override ui.ModStatusOverride) {
	if !a.IsBisectionReady() {
		return
	}
	a.stagedMu.Lock()
	defer a.stagedMu.Unlock()
	if a.stagedOverrides == nil {
		a.stagedOverrides = make(map[string]ui.ModStatusOverride)
	}
	a.stagedOverrides[id] = override
}

// Commit applies all staged overrides to the state manager and triggers a
// reconciliation. Pending additions (mods that will re-enter the search pool)
// are available via GetViewModel().PendingAdditions afterwards.
func (a *App) Commit() {
	if !a.IsBisectionReady() {
		a.Discard()
		return
	}

	a.stagedMu.Lock()
	overrides := a.stagedOverrides
	a.stagedOverrides = nil
	a.stagedMu.Unlock()

	for id, override := range overrides {
		switch override {
		case ui.ModOverrideNone:
			a.bisectSvc.StateManager().SetForceEnabled(id, false)
			a.bisectSvc.StateManager().SetForceDisabled(id, false)
			a.bisectSvc.StateManager().SetOmitted(id, false)
		case ui.ModOverrideForceEnabled:
			// Unresolvable mods cannot be force-enabled; they are dealt with on
			// the unresolvable mods screen instead.
			if status, ok := a.bisectSvc.StateManager().GetModStatus(id); ok && status.IsUnresolvable {
				continue
			}
			a.bisectSvc.StateManager().SetForceEnabled(id, true)
			a.bisectSvc.StateManager().SetOmitted(id, false)
		case ui.ModOverrideForceDisabled:
			a.bisectSvc.StateManager().SetForceDisabled(id, true)
			a.bisectSvc.StateManager().SetOmitted(id, false)
		case ui.ModOverrideOmitted:
			a.bisectSvc.StateManager().SetOmitted(id, true)
			a.bisectSvc.StateManager().SetForceEnabled(id, false)
			a.bisectSvc.StateManager().SetForceDisabled(id, false)
		}
	}
	a.Reconcile()
}

// Discard drops all staged overrides without applying them.
func (a *App) Discard() {
	a.stagedMu.Lock()
	a.stagedOverrides = nil
	a.stagedMu.Unlock()
}

func (a *App) ContinueSearch() {
	if !a.IsBisectionReady() {
		return
	}
	logging.Debugf("App: ContinueSearch action triggered.")

	report, err := a.bisectSvc.ContinueSearch()
	if err != nil {
		a.view.ShowDialogErrorBisectionCannotContinue(err)
		a.view.Update()
		return
	}

	if len(report.ModsUnresolvable) > 0 {
		disabled := make(sets.Set, len(report.ModsUnresolvable))
		for id := range report.ModsUnresolvable {
			disabled[id] = struct{}{}
		}
		a.view.ShowDialogInfoBisectionUnresolvableModsDisabled(disabled)
	}
	a.view.Update()
}

func (a *App) Undo() error {
	err := a.bisectSvc.UndoLastStep()
	if err != nil {
		logging.Errorf("App: Undo failed: %v", err)
	} else {
		logging.Debugf("App: Undo successful.")
		a.Reconcile()
	}
	return err
}

func (a *App) ResetSearch() {
	logging.Debugf("App: ResetSearch faction triggered.")
	a.bisectSvc.ResetSearch()
	a.Reconcile()
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
		a.view.OnIterationComplete()
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

	if errors.Is(err, imcs.ErrSearchHalted) {
		logging.Warnf("App: Step error, bisection halted: %s", err)
		a.showHaltedPage()
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
			ok := a.view.ShowDialogQuestionBisectionContinueWithMissingMods(unexpectedDeletions)
			if ok {
				logging.Infof("App: Disabling %d mods that are unexpectedly missing: %v", len(missingIDs), missingIDs)
				a.bisectSvc.StateManager().SetMissingBatch(missingIDs, true)
				a.Reconcile()
				a.Step()
			}
		} else {
			a.view.ShowDialogInfoBisectionModsMissingExpected(expectedDeletions)
			logging.Infof("App: Disabling %d mods that are expectedly missing: %v", len(missingIDs), missingIDs)
			a.bisectSvc.StateManager().SetMissingBatch(missingIDs, true)
			a.Reconcile()
			a.Step()
		}
		return
	}

	if errors.Is(err, bisect.ErrNeedsReconciliation) {
		report := a.bisectSvc.ReconcileState()
		if report.HasChanges {
			a.showReconciliationReport(&report)
			a.Step()
		} else {
			logging.Error("App: Reconciliation triggered by ErrNeedsReconciliation but reconciliation yielded no changes.")
			a.Step()
		}
		return
	}

	logging.Errorf("App: Step error: %v", err)

	a.view.ShowDialogErrorBisectionPrepare(err)
}

func (a *App) showReconciliationReport(report *bisect.ActionReport) {
	if len(report.ModsUnresolvable) > 0 {
		disabled := make(sets.Set, len(report.ModsUnresolvable))
		for id := range report.ModsUnresolvable {
			disabled[id] = struct{}{}
		}
		a.view.ShowDialogInfoBisectionUnresolvableModsDisabled(disabled)
		return
	}
	logging.Info("App: Reconciliation report has no 'Unresolvable Mods' changes. This is odd.")
}

func (a *App) GetLogger() *logging.Logger { return a.logger }
