package bisect

import (
	"fmt"
	"sort"

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
	if err := activator.EnableAll(); err != nil {
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
	logging.Debugf("BisectService: State change detected. Reconciling engine state.")
	validCandidates := s.getValidCandidates()
	s.engine.Reconcile(validCandidates)

	currentCandidates := sets.Union(sets.MakeSet(s.engine.GetCurrentState().Candidates), s.engine.GetPendingAdditions())

	unresolvableMods := s.state.Resolver().CalculateUnresolvableMods(currentCandidates)

	modsToDisable := make([]string, 0)
	currentStatuses := s.state.GetModStatusesSnapshot()
	for modID := range unresolvableMods {
		if !currentStatuses[modID].ForceDisabled {
			modsToDisable = append(modsToDisable, modID)
		}
	}

	if len(modsToDisable) > 0 {
		logging.Infof("Reconciling state: Auto-disabling %d mods with now-unmet dependencies: %v", len(modsToDisable), modsToDisable)
		// This will re-trigger handleStateChange, but that's okay. The second pass will be a no-op.
		s.state.SetForceDisabledBatch(modsToDisable, true)
		return // Exit early to let the second pass complete the reconciliation.
	}

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

		logging.Debugf("BisectService: Effective set contains %d mods: %v", len(effectiveSet), sets.FormatSet(effectiveSet))

		// Optional: Log the full resolution path for extreme detail.
		// var resLog []string
		// for _, info := range resolutionPath {
		//     resLog = append(resLog, fmt.Sprintf("  - %s (%s)", info.ModID, info.Reason))
		// }
		// logging.Debugf("BisectService: Full resolution path:\n%s", strings.Join(resLog, "\n"))

		changes, err = s.activator.Apply(effectiveSet)
		if err != nil {
			s.activator.Revert(changes)
			return nil, nil, fmt.Errorf("failed to apply file changes: %w", err)
		}
		return plan, changes, nil
	}
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
func (s *Service) UndoLastStep() {
	if s.engine == nil {
		return
	}

	// 1. Tell the engine to perform its internal undo. This returns the undone action.
	undoneFrame, ok := s.engine.Undo()
	if !ok {
		logging.Warnf("BisectService: Undo failed. Engine could not revert its state.")
		return
	}

	// 2. Use the plan from the undone frame to remove the now-stale
	// knowledge from the master KnowledgeBase.
	s.enumState.KnowledgeBase.Remove(undoneFrame.Plan.ModIDsToTest)
	logging.Infof("BisectService: Removed test result for set %v from knowledge base.", sets.MakeSlice(undoneFrame.Plan.ModIDsToTest))

	// 3. Notify the UI that the state has changed.
	if s.OnStateChange != nil {
		s.OnStateChange()
	}
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
	lastFoundConflictSet := lastEngine.GetCurrentState().ConflictSet

	// Archive the results from the completed run.
	s.enumState.AddFoundConflictSet(lastFoundConflictSet)
	s.enumState.AppendLog(lastEngine.GetExecutionLog())

	// 1. Determine the candidates that *should* be available for the next round.
	nextRoundCandidates := sets.Subtract(s.enumState.MasterCandidateSet, lastFoundConflictSet)

	// 2. Calculate which of these remaining candidates are now impossible to resolve.
	unresolvableMods := s.state.Resolver().CalculateUnresolvableMods(nextRoundCandidates)

	// 3. If any mods are unresolvable, force-disable them. This is the main action.
	if len(unresolvableMods) > 0 {
		modsToDisable := make([]string, 0)
		currentStatuses := s.state.GetModStatusesSnapshot()
		for modID := range unresolvableMods {
			if !currentStatuses[modID].ForceDisabled {
				modsToDisable = append(modsToDisable, modID)
			}
		}

		if len(modsToDisable) > 0 {
			sort.Strings(modsToDisable)
			result.NewlyDisabledMods = modsToDisable
			logging.Warnf("BisectService: Auto-disabling %d mods due to unmet dependencies: %v", len(modsToDisable), modsToDisable)
			s.state.SetForceDisabledBatch(modsToDisable, true)
		}
	}

	// Create a new, clean engine for the next search.
	nextState := imcs.NewInitialState()
	nextState.AllModIDs = s.state.GetAllModIDs()
	// The candidates for the new engine must account for the mods we just disabled.
	finalNextRoundCandidates := sets.Subtract(nextRoundCandidates, unresolvableMods)
	nextState.Candidates = sets.MakeSlice(finalNextRoundCandidates)
	nextState.Round = lastEngine.GetCurrentState().Round + 1

	s.engine = imcs.NewEngine(nextState)
	logging.Infof("BisectService: Initialized new engine for Round %d with %d candidates.", s.engine.GetCurrentState().Round, len(nextState.Candidates))

	// If no mods were disabled, the state didn't change, so we must trigger a
	// manual refresh. If mods *were* disabled, handleStateChange already triggered it.
	if len(result.NewlyDisabledMods) == 0 && s.OnStateChange != nil {
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
	allModIDs := sets.MakeSet(s.state.GetAllModIDs())
	nonCandidateSet := make(sets.Set)
	for id, status := range s.state.GetModStatusesSnapshot() {
		// A mod is NOT a candidate if it's disabled, omitted, or force-enabled.
		if status.ForceDisabled || status.Omitted || status.ForceEnabled {
			nonCandidateSet[id] = struct{}{}
		}
	}
	return sets.Subtract(allModIDs, nonCandidateSet)
}
