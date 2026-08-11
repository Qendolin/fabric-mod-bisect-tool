package app

import (
	"sort"

	"github.com/Qendolin/fabric-mod-bisect-tool/pkg/core/mods"
	"github.com/Qendolin/fabric-mod-bisect-tool/pkg/core/sets"
	"github.com/Qendolin/fabric-mod-bisect-tool/pkg/ui"
)

func makeModVM(id string, mods map[string]*mods.Mod) ui.ModViewModel {
	if mod, ok := mods[id]; ok {
		return ui.ModViewModel{
			BaseFilename: mod.BaseFilename,
			ID:           mod.Metadata.ID,
			Name:         mod.FriendlyName(),
			Version:      mod.Metadata.Version.String(),
		}
	} else {
		return ui.ModViewModel{
			ID:        id,
			IsUnknown: true,
		}
	}
}

func (a *App) GetViewModel() ui.BisectionViewModel {
	vm := ui.BisectionViewModel{
		IsReady:         false,
		Loader:          a.loader,
		PreferredLoader: a.cliArgs.Loader,
	}
	if !a.IsBisectionReady() {
		return vm
	}

	engine := a.bisectSvc.Engine()
	enumState := a.bisectSvc.EnumerationState()
	state := engine.GetCurrentState()
	currentPlan, _ := engine.GetCurrentTestPlan()
	allMods := a.bisectSvc.StateManager().GetAllMods()

	isVerification := currentPlan != nil && currentPlan.IsVerificationStep

	vm.IsReady = true
	vm.IsComplete = state.IsComplete
	vm.IsVerificationStep = isVerification
	vm.IsHalted = state.IsHalted
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

	vm.ModsInfo = make(map[string]ui.ModViewModel, len(allMods))
	for id := range allMods {
		vm.ModsInfo[id] = makeModVM(id, allMods)
	}

	return vm
}

// GetResultViewModel processes raw bisection data into a clean structured view model.
func (a *App) GetResultViewModel() (result ui.ResultViewModel) {
	if !a.IsBisectionReady() {
		result.State = ui.StateNotReady
		return result
	}

	state := a.bisectSvc.Engine().GetCurrentState()

	// Can only continue into a new round if the current round is complete
	result.CanContinueSearch = state.IsComplete && len(state.GetCandidateSet()) > 0

	if !state.IsComplete && state.LastFoundElement == "" {
		// First iteration in progress
		result.State = ui.StateNoResultsYet
		return result
	}

	modState := a.bisectSvc.StateManager()
	currentPlan := a.bisectSvc.Engine().GetActiveTestPlan()

	modMap := modState.GetAllMods()
	allModsSet := sets.MakeSet(modState.GetAllModIDs())
	generallyUnresolvable := modState.Resolver().CalculateTransitivelyUnresolvableMods(allModsSet)

	for _, cs := range a.bisectSvc.EnumerationState().FoundConflictSets {
		result.PreviousConflictSets = append(result.PreviousConflictSets, buildConflictSetReport(cs, allModsSet, modMap, generallyUnresolvable, modState))
	}

	// Always map the currently active/latest conflict group to CurrentConflict
	if len(state.ConflictSet) > 0 {
		result.CurrentConflict = buildConflictSetReport(state.ConflictSet, allModsSet, modMap, generallyUnresolvable, modState)
	}

	result.State = ui.StateInProgress
	if state.IsComplete {
		result.State = ui.StateComplete
	}

	// Calculate global dependency health
	details := modState.Resolver().CalculateUnresolvableModsDetails(allModsSet)
	if len(details.DirectlyUnresolvable) > 0 {
		result.GenerallyUnresolvable = buildGenerallyUnresolvableReport(details, modMap)
	}

	result.IsVerificationStep = currentPlan != nil && currentPlan.IsVerificationStep

	return result
}

// Helpers to build sub-components cleanly

func buildCascadingDisablesSlice(conflictSet, allModsSet sets.Set, modMap map[string]*mods.Mod, generallyUnresolvable sets.Set, modState *mods.StateManager) ([]ui.CascadingDisables, sets.Set) {
	union := sets.Set{}
	var list []ui.CascadingDisables

	for _, id := range sets.MakeSlice(conflictSet) {
		item := ui.CascadingDisables{Mod: makeModVM(id, modMap)}

		perModUnresolvable := modState.Resolver().CalculateTransitivelyUnresolvableMods(sets.Subtract(allModsSet, sets.MakeSet([]string{id})))
		perModSpecific := sets.Subtract(perModUnresolvable, generallyUnresolvable)

		for extraID := range perModSpecific {
			union[extraID] = struct{}{}
		}

		for _, depID := range sets.MakeSlice(perModSpecific) {
			item.AlsoRequireDisable = append(item.AlsoRequireDisable, makeModVM(depID, modMap))
		}
		list = append(list, item)
	}
	return list, union
}

func buildConflictSetReport(conflictSet, allModsSet sets.Set, modMap map[string]*mods.Mod, generallyUnresolvable sets.Set, modState *mods.StateManager) ui.ConflictSetReport {
	modsSlice, union := buildCascadingDisablesSlice(conflictSet, allModsSet, modMap, generallyUnresolvable, modState)

	fullSetUnresolvable := modState.Resolver().CalculateTransitivelyUnresolvableMods(sets.Subtract(allModsSet, conflictSet))
	fullSetSpecific := sets.Subtract(fullSetUnresolvable, generallyUnresolvable)
	extraIfAll := sets.Subtract(fullSetSpecific, union)

	var footerRefs []ui.ModViewModel
	for _, depID := range sets.MakeSlice(extraIfAll) {
		footerRefs = append(footerRefs, makeModVM(depID, modMap))
	}

	return ui.ConflictSetReport{
		Mods:              modsSlice,
		IfAllDisabledAlso: footerRefs,
	}
}

func buildGenerallyUnresolvableReport(details mods.UnresolvableModDetails, modMap map[string]*mods.Mod) []ui.UnresolvedDependencyReport {
	causedByRoot := make(map[string]sets.Set)
	for transitiveID, roots := range details.TransitivelyUnresolvable {
		for rootID := range roots {
			if _, ok := causedByRoot[rootID]; !ok {
				causedByRoot[rootID] = sets.Set{}
			}
			causedByRoot[rootID][transitiveID] = struct{}{}
		}
	}

	var topLevelSlice []string
	for modID := range details.DirectlyUnresolvable {
		topLevelSlice = append(topLevelSlice, modID)
	}
	sort.Strings(topLevelSlice)

	var reports []ui.UnresolvedDependencyReport
	for _, modID := range topLevelSlice {
		report := ui.UnresolvedDependencyReport{Mod: makeModVM(modID, modMap)}

		if failedDeps := details.DirectlyUnresolvable[modID]; len(failedDeps) > 0 {
			sort.Strings(failedDeps)
			for _, depID := range failedDeps {
				report.UnmetDependencies = append(report.UnmetDependencies, makeModVM(depID, modMap))
			}
		}

		if caused, ok := causedByRoot[modID]; ok && len(caused) > 0 {
			causedSlice := sets.MakeSlice(caused)
			sort.Strings(causedSlice)
			for _, depID := range causedSlice {
				report.RequiredByTransitive = append(report.RequiredByTransitive, makeModVM(depID, modMap))
			}
		}
		reports = append(reports, report)
	}
	return reports
}
