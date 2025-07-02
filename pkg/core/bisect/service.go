package bisect

import (
	"fmt"

	"github.com/Qendolin/fabric-mod-bisect-tool/pkg/core/imcs"
	"github.com/Qendolin/fabric-mod-bisect-tool/pkg/core/mods"
	"github.com/Qendolin/fabric-mod-bisect-tool/pkg/core/sets"
	"github.com/Qendolin/fabric-mod-bisect-tool/pkg/logging"
)

// ContinueSearchResult describes the outcome of a ContinueSearch operation.
type ContinueSearchResult struct {
	// NewlyDisabledMods contains the IDs of mods that were automatically
	// force-disabled because their dependencies became unresolvable.
	NewlyDisabledMods []string
}

// Service encapsulates the entire bisection business logic.
type Service struct {
	state     *mods.StateManager
	activator *mods.Activator
	engine    *imcs.Engine

	enumState *Enumeration

	OnStateChange func()
}

// NewService creates a new bisect service from pre-loaded components.
func NewService(stateMgr *mods.StateManager, activator *mods.Activator) (*Service, error) {
	if err := activator.EnableAll(stateMgr.GetModStatusesSnapshot()); err != nil {
		return nil, fmt.Errorf("failed to enable all mods on startup: %w", err)
	}

	initalState := imcs.NewInitialState()
	initalState.AllModIDs = stateMgr.GetAllModIDs()
	initalState.Candidates = stateMgr.GetAllModIDs()
	engine := imcs.NewEngine(initalState)

	svc := &Service{
		state:     stateMgr,
		activator: activator,
		engine:    engine,
		enumState: NewEnumeration(stateMgr.GetAllModIDs()),
	}
	stateMgr.OnStateChanged = svc.handleStateChange
	return svc, nil
}

// handleStateChange is called when a mod's forced status changes.
func (s *Service) handleStateChange() {
	logging.Debugf("BisectService: State change detected. Reconciling system state.")

	// 1. Determine which mods have become unresolvable due to the user's change.
	currentCandidates := s.getValidCandidates()
	unresolvableMods := s.state.Resolver().CalculateTransitivelyUnresolvableMods(currentCandidates)

	// 2. Disable them. This has a side effect that will be picked up by the next call.
	if len(unresolvableMods) > 0 {
		s.state.SetForceDisabledBatch(sets.MakeSlice(unresolvableMods), true)
	}

	// 3. Reconcile the engine based on the final, valid set of candidates.
	s.engine.Reconcile(s.getValidCandidates())

	// 4. Notify the UI of the final, consistent state.
	if s.OnStateChange != nil {
		s.OnStateChange()
	}
}

// --- Direct Component Access ---
func (s *Service) StateManager() *mods.StateManager { return s.state }
func (s *Service) Activator() *mods.Activator       { return s.activator }
func (s *Service) Engine() *imcs.Engine             { return s.engine }
func (s *Service) EnumerationState() *Enumeration   { return s.enumState }

// --- High-Level Workflow Methods ---

// GetCurrentState returns a read-only snapshot of the engine's state.
func (s *Service) GetCurrentState() imcs.SearchState {
	return s.engine.GetCurrentState()
}

// PlanAndApplyNextTest is the single entry point for the UI's "Step" action.
func (s *Service) PlanAndApplyNextTest() (plan *imcs.TestPlan, changes []mods.BatchStateChange, err error) {
	// 1. Plan the very next logical test.
	plan, err = s.engine.PlanNextTest()
	if err != nil {
		return nil, nil, err // Search is complete or cannot proceed.
	}

	logging.Debugf("BisectService: Plan generated. Resolving effective set for test targets: %v", sets.FormatSet(plan.ModIDsToTest))

	// 3. Not cached. We must perform the test manually.
	// Break the loop and return the plan and file changes to the UI.
	effectiveSet, _ := s.state.ResolveEffectiveSet(plan.ModIDsToTest)

	statuses := s.state.GetModStatusesSnapshot()
	finalEffectiveSet := s.finalizeEffectiveSet(effectiveSet, statuses)

	logging.Debugf("BisectService: Final effective set contains %d mods: %v", len(finalEffectiveSet), sets.FormatSet(finalEffectiveSet))

	changes, err = s.activator.Apply(finalEffectiveSet, statuses)

	// Optional: Log the full resolution path for extreme detail.
	// var resLog []string
	// for _, info := range resolutionPath {
	//     resLog = append(resLog, fmt.Sprintf("  - %s (%s)", info.ModID, info.Reason))
	// }
	// logging.Debugf("BisectService: Full resolution path:\n%s", strings.Join(resLog, "\n"))

	return plan, changes, err
}

// finalizeEffectiveSet takes the resolver's proposed set and applies manual overrides.
// It ensures that ForceEnabled mods are included and non-activatable mods are excluded.
func (s *Service) finalizeEffectiveSet(proposedSet sets.Set, statuses map[string]mods.ModStatus) sets.Set {
	finalSet := sets.Copy(proposedSet)

	for id, status := range statuses {
		if status.ForceEnabled {
			// A user override to force-enable a mod takes precedence.
			finalSet[id] = struct{}{}
		} else if !status.IsActivatable() {
			// Any mod that is not activatable (e.g., ForceDisabled, IsMissing) must be excluded.
			delete(finalSet, id)
		}
	}
	return finalSet
}

// SubmitTestResult processes the outcome of a test.
func (s *Service) SubmitTestResult(result imcs.TestResult, changes []mods.BatchStateChange) {
	s.activator.Revert(changes)
	plan := s.engine.GetActiveTestPlan()
	if plan == nil {
		logging.Error("BisectService: Attempted to submit result without an active plan.")
		return
	}

	if err := s.engine.SubmitTestResult(result); err != nil {
		logging.Errorf("BisectService: Failed to submit test result to engine: %v", err)
	}
	if s.OnStateChange != nil {
		s.OnStateChange()
	}
}

// UndoLastStep orchestrates a complete undo operation. It reverts the bisection engine to its previous state.
// It returns true if the undo operation was successful
func (s *Service) UndoLastStep() bool {
	if s.engine == nil {
		return false
	}

	undoneFrame, ok := s.engine.Undo()
	if !ok {
		logging.Warnf("BisectService: Undo failed. Engine could not revert its state.")
		return false
	}
	logging.Infof("BisectService: Undone frame: Round %d, Iteration %d, Step %d.", undoneFrame.State.Round, undoneFrame.State.Iteration, undoneFrame.State.Step)

	if s.OnStateChange != nil {
		s.OnStateChange()
	}

	return true
}

// prepareNewRoundCandidates takes the master list of all possible candidates for the
// next round, determines which ones have become unresolvable, disables them in the
// state manager, and returns the final, clean list of candidates.
func (s *Service) prepareNewRoundCandidates(masterCandidates sets.Set) (sets.Set, []string) {
	// First, determine which of the master candidates are still considered valid by the user.
	userValidCandidates := sets.Intersection(masterCandidates, s.getValidCandidates())

	// Now, check if any of these have become unresolvable.
	unresolvableMods := s.state.Resolver().CalculateTransitivelyUnresolvableMods(userValidCandidates)

	// Commit the state change: disable the unresolvable mods.
	if len(unresolvableMods) > 0 {
		logging.Warnf("BisectService.Reconcile: Found %d unresolvable mods: %v", len(unresolvableMods), sets.MakeSlice(unresolvableMods))
		s.state.SetForceDisabledBatch(sets.MakeSlice(unresolvableMods), true)
	}

	// The final candidates are the user-valid ones minus those that were just found to be unresolvable.
	finalCandidates := sets.Subtract(userValidCandidates, unresolvableMods)

	return finalCandidates, sets.MakeSlice(unresolvableMods)
}

// CancelTest reverts file changes and invalidates the current test plan.
func (s *Service) CancelTest(changes []mods.BatchStateChange) {
	s.activator.Revert(changes)
	s.engine.InvalidateActivePlan()
}

// ContinueSearch archives the current conflict set, prepares for the next round by
// auto-disabling any newly unresolvable mods, and then starts a new bisection
// search on the remaining candidate mods.
func (s *Service) ContinueSearch() ContinueSearchResult {
	lastEngine := s.engine
	lastState := lastEngine.GetCurrentState()

	logging.Infof("BisectService: Starting 'Continue Search' for Round %d.", lastState.Round+1)

	// --- Phase 1: Archive previous results ---
	s.enumState.AddFoundConflictSet(lastState.ConflictSet)
	s.enumState.AppendLog(lastEngine.GetExecutionLog())

	// --- Phase 2: Prepare the candidates for the next round ---
	// This single call handles all reconciliation and state updates.
	finalNextRoundCandidates, newlyDisabledMods := s.prepareNewRoundCandidates(s.enumState.MasterCandidateSet)
	result := ContinueSearchResult{NewlyDisabledMods: newlyDisabledMods}

	if logging.IsDebugEnabled() {
		// Calculate the set of mods the user has manually excluded.
		allMods := sets.MakeSet(s.state.GetAllModIDs())
		validCandidates := s.getValidCandidates()
		userExcluded := sets.Subtract(allMods, validCandidates)

		logging.Debugf("BisectService: === Calculating Next Round Candidates ===")
		logging.Debugf("  - Total Mods to Start: %d", len(allMods))
		logging.Debugf("  - Subtracting All Found Conflicts: %v", s.enumState.FoundConflictSets)
		logging.Debugf("  - Subtracting User Excluded (Omitted/Disabled): %v", sets.MakeSlice(userExcluded))
		logging.Debugf("  - Subtracting Auto-Disabled This Round: %v", result.NewlyDisabledMods)
		logging.Debugf("  - Final Candidate List for New Engine (%d mods): %v", len(finalNextRoundCandidates), sets.MakeSlice(finalNextRoundCandidates))
		logging.Debugf("BisectService: ===========================================")
	}

	// --- Phase 3: Create the new engine ---
	nextState := imcs.NewInitialState()
	nextState.AllModIDs = s.state.GetAllModIDs()
	nextState.Candidates = sets.MakeSlice(finalNextRoundCandidates)
	nextState.Round = lastState.Round + 1
	s.engine = imcs.NewEngine(nextState)
	logging.Infof("BisectService: Initialized new engine for Round %d.", s.engine.GetCurrentState().Round)

	// --- Phase 4: Trigger UI Update and return result ---
	if s.OnStateChange != nil {
		s.OnStateChange()
	}

	return result
}

// ResetSearch performs a hard reset of the entire bisection process.
// It discards all history, found conflicts, and cached results,
// restarting the search from the original set of mods.
func (s *Service) ResetSearch() {
	allModIDs := s.state.GetAllModIDs()
	// Re-initialize the entire enumeration state from scratch.
	s.enumState = NewEnumeration(allModIDs)
	// Create a new engine with the original full set of candidates.
	initialState := imcs.NewInitialState()
	initialState.AllModIDs = allModIDs
	initialState.Candidates = allModIDs
	s.engine = imcs.NewEngine(initialState)

	if s.OnStateChange != nil {
		s.OnStateChange()
	}
}

// --- Helper Methods ---

// GetCurrentExecutionLog returns the log of completed tests.
func (s *Service) GetCurrentExecutionLog() *imcs.ExecutionLog {
	if s.engine == nil {
		return nil
	}
	return s.engine.GetExecutionLog()
}

// GetCombinedExecutionLog returns a complete history of all test steps taken
// during the entire session, combining archived logs from previous enumeration
// runs with the log from the currently active bisection.
func (s *Service) GetCombinedExecutionLog() []imcs.CompletedTest {
	// Start with the history from all previously completed runs.
	if s.enumState == nil || s.enumState.ArchivedExecutionLog == nil {
		return nil
	}

	combinedEntries := s.enumState.ArchivedExecutionLog.GetEntries()

	// Append the history from the currently active run.
	if s.engine != nil {
		if currentLog := s.engine.GetExecutionLog(); currentLog != nil {
			combinedEntries = append(combinedEntries, currentLog.GetEntries()...)
		}
	}

	return combinedEntries
}

// getValidCandidates identifies and returns the set of mods that are currently
// considered active participants (candidates) in the bisection search.
func (s *Service) getValidCandidates() sets.Set {
	validCandidates := make(sets.Set)
	for id, status := range s.state.GetModStatusesSnapshot() {
		if status.IsSearchCandidate() {
			validCandidates[id] = struct{}{}
		}
	}
	return validCandidates
}
