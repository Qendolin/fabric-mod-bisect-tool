package bisect

import (
	"fmt"

	"github.com/Qendolin/fabric-mod-bisect-tool/app/core/imcs"
	"github.com/Qendolin/fabric-mod-bisect-tool/app/core/mods"
	"github.com/Qendolin/fabric-mod-bisect-tool/app/core/sets"
	"github.com/Qendolin/fabric-mod-bisect-tool/app/logging"
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
	// 1. Reconcile the engine based on user-driven changes (Omit, Force Enable, etc.).
	s.engine.Reconcile(s.getValidCandidates())

	// 2. Explicitly reconcile and disable any mods that became unresolvable as a result.
	s.reconcileAndDisableUnresolvable()

	// 3. Notify the UI of the final, consistent state.
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

// AdvanceToNextTest is the single entry point for the UI's "Step" action.
// It will fast-forward through any cached test results.
func (s *Service) AdvanceToNextTest() (plan *imcs.TestPlan, changes []mods.BatchStateChange, err error) {
	for {
		// 1. Plan the very next logical test.
		plan, err = s.engine.PlanNextTest()
		if err != nil {
			return nil, nil, err // Search is complete or cannot proceed.
		}

		logging.Debugf("BisectService: Plan generated. Resolving effective set for test targets: %v", sets.FormatSet(plan.ModIDsToTest))

		// 2. Check if we already know the answer.
		// Do not fast-forward verification steps
		if result, found := s.enumState.KnowledgeBase.Get(plan.ModIDsToTest); found && !plan.IsVerificationStep {
			logging.Infof("BisectService: Found cached result for test plan: %s. Applying automatically.", result)
			// If cached, submit the result and loop immediately to the next plan.
			if err := s.engine.SubmitTestResult(result); err != nil {
				return nil, nil, err
			}
			continue
		}

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
	// Store the new knowledge before submitting to the engine.
	s.enumState.KnowledgeBase.Store(plan.ModIDsToTest, result)

	if err := s.engine.SubmitTestResult(result); err != nil {
		logging.Errorf("BisectService: Failed to submit test result to engine: %v", err)
	}
	if s.OnStateChange != nil {
		s.OnStateChange()
	}
}

// UndoLastStep orchestrates a complete undo operation. It reverts the bisection
// engine to its previous state and removes the knowledge gained from the last
// test from all relevant historical records, including the knowledge base.
// It returns true if the undo operation was successful
func (s *Service) UndoLastStep() bool {
	if s.engine == nil {
		return false
	}

	// 1. Tell the engine to perform its internal undo. This returns the undone action.
	undoneFrame, ok := s.engine.Undo()
	if !ok {
		logging.Warnf("BisectService: Undo failed. Engine could not revert its state.")
		return false
	}

	// 2. Use the plan from the undone frame to remove the now-stale
	// knowledge from the master KnowledgeBase.
	s.enumState.KnowledgeBase.Remove(undoneFrame.Plan.ModIDsToTest)
	logging.Infof("BisectService: Removed test result for set %v from knowledge base.", sets.MakeSlice(undoneFrame.Plan.ModIDsToTest))

	// 3. Notify the UI that the state has changed.
	if s.OnStateChange != nil {
		s.OnStateChange()
	}

	return true
}

// reconcileAndDisableUnresolvable iteratively finds and disables any mods that have
// become unresolvable given the current set of available mods. It is the single
// source of truth for enforcing dependency consistency.
func (s *Service) reconcileAndDisableUnresolvable() sets.Set {
	// Start with the set of all mods that are currently considered activatable.
	currentlyAvailable := sets.MakeSet(s.state.GetAllModIDs())
	for id, status := range s.state.GetModStatusesSnapshot() {
		if !status.IsActivatable() {
			delete(currentlyAvailable, id)
		}
	}

	allNewlyDisabled := make(sets.Set)

	// Iteratively find and disable unresolvable mods until the set is stable.
	for {
		unresolvableInThisPass := s.state.Resolver().CalculateUnresolvableMods(currentlyAvailable)
		if len(unresolvableInThisPass) == 0 {
			break // The set is stable, no more unresolvable mods found.
		}

		logging.Warnf("BisectService.Reconcile: Found %d unresolvable mods in this pass: %v", len(unresolvableInThisPass), sets.MakeSlice(unresolvableInThisPass))

		// Disable these mods and remove them from our working set for the next iteration.
		s.state.SetForceDisabledBatch(sets.MakeSlice(unresolvableInThisPass), true)
		currentlyAvailable = sets.Subtract(currentlyAvailable, unresolvableInThisPass)
		allNewlyDisabled = sets.Union(allNewlyDisabled, unresolvableInThisPass)
	}

	return allNewlyDisabled
}

// CancelTest reverts file changes and invalidates the current test plan.
func (s *Service) CancelTest(changes []mods.BatchStateChange) {
	s.activator.Revert(changes)
	s.engine.InvalidateActivePlan()
}

// ContinueSearch archives the current conflict set, prepares for the next round by
// auto-disabling any newly unresolvable mods, and then starts a new bisection
// search on the remaining candidate mods.
func (s *Service) ContinueSearch() *ContinueSearchResult {
	result := &ContinueSearchResult{}
	lastEngine := s.engine
	lastState := lastEngine.GetCurrentState()
	lastFoundConflictSet := lastState.ConflictSet

	logging.Infof("BisectService: Starting 'Continue Search' for Round %d.", lastState.Round+1)

	// --- Phase 1: Archive previous results ---
	// This action modifies s.enumState.MasterCandidateSet by removing the found conflict.
	s.enumState.AddFoundConflictSet(lastFoundConflictSet)

	// --- Phase 2: Reconcile and disable unresolvable mods ---
	// This is a direct action with observable consequences.
	newlyDisabled := s.reconcileAndDisableUnresolvable()
	if len(newlyDisabled) > 0 {
		result.NewlyDisabledMods = sets.MakeSlice(newlyDisabled)
	}

	// --- Phase 3: Calculate final candidate set and provide detailed debug logging ---
	// The candidates for the next round are what's left in the master set AFTER
	// also removing any mods that were just auto-disabled.
	finalNextRoundCandidates := sets.Subtract(s.enumState.MasterCandidateSet, newlyDisabled)

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

	// --- Phase 4: Create the new engine ---
	nextState := imcs.NewInitialState()
	nextState.AllModIDs = s.state.GetAllModIDs()
	nextState.Candidates = sets.MakeSlice(finalNextRoundCandidates)
	nextState.Round = lastState.Round + 1
	s.engine = imcs.NewEngine(nextState)
	logging.Infof("BisectService: Initialized new engine for Round %d.", s.engine.GetCurrentState().Round)

	// --- Phase 5: Trigger UI Update ---
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

// getValidCandidates now uses the term "Omitted".
func (s *Service) getValidCandidates() sets.Set {
	validCandidates := make(sets.Set)
	for id, status := range s.state.GetModStatusesSnapshot() {
		if status.IsSearchCandidate() {
			validCandidates[id] = struct{}{}
		}
	}
	return validCandidates
}
