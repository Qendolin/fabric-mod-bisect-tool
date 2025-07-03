package bisect

import (
	"errors"
	"fmt"

	"github.com/Qendolin/fabric-mod-bisect-tool/pkg/core/imcs"
	"github.com/Qendolin/fabric-mod-bisect-tool/pkg/core/mods"
	"github.com/Qendolin/fabric-mod-bisect-tool/pkg/core/sets"
	"github.com/Qendolin/fabric-mod-bisect-tool/pkg/logging"
)

// ErrNeedsReconciliation is returned by service methods that require a consistent
// state to operate, but detect that the state has been dirtied by user actions.
var ErrNeedsReconciliation = errors.New("system state is inconsistent and needs reconciliation")
var ErrUndoStackEmpty = errors.New("cannot undo: undo stack is empty")

// ActionReport describes the outcome of a state-changing operation like
// reconciliation or advancing to the next search round.
type ActionReport struct {
	ModsSetProblematic  sets.Set
	ModsSetUnresolvable sets.Set
	HasChanges          bool
}

// Service encapsulates the entire bisection business logic.
type Service struct {
	state     *mods.StateManager
	activator *mods.Activator
	engine    *imcs.Engine

	enumState *Enumeration

	// The OnStateChange callback is now only for simple UI redraw notifications.
	OnStateChange func()

	// A flag indicating that the system's dependency state may be inconsistent.
	needsReconciliation bool
	isReconciling       bool // Guard flag against infinite loops
}

// NewService creates a new bisect service from pre-loaded components.
func NewService(stateMgr *mods.StateManager, activator *mods.Activator) (*Service, error) {
	if err := activator.EnableAll(stateMgr.GetModStatusesSnapshot()); err != nil {
		return nil, fmt.Errorf("failed to enable all mods on startup: %w", err)
	}

	initialState := imcs.NewInitialState()
	initialState.AllModIDs = stateMgr.GetAllModIDs()
	initialState.Candidates = stateMgr.GetAllModIDs()
	engine := imcs.NewEngine(initialState)

	svc := &Service{
		state:     stateMgr,
		activator: activator,
		engine:    engine,
		enumState: NewEnumeration(),
	}

	// When the StateManager changes, mark the service as needing reconciliation
	// and then forward the notification to the UI.
	stateMgr.OnStateChanged = func() {
		if svc.isReconciling {
			return
		}
		svc.needsReconciliation = true
		if svc.OnStateChange != nil {
			svc.OnStateChange()
		}
	}

	// Perform an initial reconciliation to ensure a clean starting state.
	svc.ReconcileState()

	return svc, nil
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

// NeedsReconciliation returns true if the system state may be inconsistent
// and a call to ReconcileState is required before performing major operations.
func (s *Service) NeedsReconciliation() bool {
	return s.needsReconciliation
}

// ReconcileState checks for and resolves dependency inconsistencies. It is safe
// to call multiple times; it will do nothing if the state is already consistent.
// It returns a report of any mods whose state was changed.
func (s *Service) ReconcileState() (report ActionReport) {
	if !s.needsReconciliation {
		return
	}

	s.isReconciling = true
	defer func() { s.isReconciling = false }()

	logging.Debugf("BisectService: Reconciling system state.")

	// Calculate what the set of unresolvable mods should be.
	expectedUnresolvable := s.state.Resolver().CalculateTransitivelyUnresolvableMods(s.getActivatableMods())

	// Get the set of mods currently marked as unresolvable.
	currentlyUnresolvable := make(sets.Set)
	for id, status := range s.state.GetModStatusesSnapshot() {
		if status.IsUnresolvable {
			currentlyUnresolvable[id] = struct{}{}
		}
	}

	// Determine which mods need their state updated.
	newlyUnresolvable := sets.Subtract(expectedUnresolvable, currentlyUnresolvable)
	newlyResolvable := sets.Subtract(currentlyUnresolvable, expectedUnresolvable)

	// Commit the state changes.
	modStateChanged := false
	if len(newlyUnresolvable) > 0 {
		s.state.SetUnresolvableBatch(sets.MakeSlice(newlyUnresolvable), true)
		modStateChanged = true
	}
	if len(newlyResolvable) > 0 {
		s.state.SetUnresolvableBatch(sets.MakeSlice(newlyResolvable), false)
		modStateChanged = true
	}

	// After reconciliation, the bisection engine's view of candidates might be stale.
	engineStateChanged := s.engine.Reconcile(s.getSearchCandidates())

	// The state is now consistent, so clear the flag.
	s.needsReconciliation = false

	report.HasChanges = modStateChanged || engineStateChanged
	report.ModsSetUnresolvable = newlyUnresolvable

	// A reconciliation always implies the state may have changed in ways that
	// require a UI refresh (e.g., engine's pending additions cleared).
	// Notify the app so it can trigger a redraw.
	if report.HasChanges && s.OnStateChange != nil {
		s.OnStateChange()
	}

	return
}

// PlanAndApplyNextTest is the single entry point for the UI's "Step" action.
// It will fail if the system state is inconsistent.
func (s *Service) PlanAndApplyNextTest() (plan *imcs.TestPlan, changes []mods.BatchStateChange, err error) {
	// 1. Guard Clause: Refuse to operate on an inconsistent state.
	if s.NeedsReconciliation() {
		return nil, nil, ErrNeedsReconciliation
	}

	// 2. Plan the very next logical test.
	plan, err = s.engine.PlanNextTest()
	if err != nil {
		return nil, nil, err // Search is complete or cannot proceed.
	}

	logging.Debugf("BisectService: Plan generated. Resolving effective set for test targets: %v", sets.FormatSet(plan.ModIDsToTest))

	// 3. Resolve and activate the mod set for the test.
	logging.Info("BisectService: Resolving effective set for test targets.")
	effectiveSet, resolutionPath := s.state.ResolveEffectiveSet(plan.ModIDsToTest)
	logging.Infof("BisectService: %v", resolutionPath)

	statuses := s.state.GetModStatusesSnapshot()
	finalEffectiveSet := s.finalizeEffectiveSet(effectiveSet, statuses)

	logging.Debugf("BisectService: Final effective set contains %d mods: %v", len(finalEffectiveSet), sets.FormatSet(finalEffectiveSet))

	changes, err = s.activator.Apply(finalEffectiveSet, statuses)

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
			// Any mod that is not activatable must be excluded.
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
func (s *Service) UndoLastStep() error {
	if s.engine == nil {
		return errors.New("cannot undo: engine is not initialized")
	}

	undoneFrame, ok := s.engine.Undo()
	if !ok {
		return errors.New("cannot undo: undo stack is empty")
	}
	logging.Infof("BisectService: Undone frame: Round %d, Iteration %d, Step %d.", undoneFrame.State.Round, undoneFrame.State.Iteration, undoneFrame.State.Step)

	// Undoing a step can change what's considered unresolvable.
	s.needsReconciliation = true
	if s.OnStateChange != nil {
		s.OnStateChange()
	}

	return nil
}

// CancelTest reverts file changes and invalidates the current test plan.
func (s *Service) CancelTest(changes []mods.BatchStateChange) {
	s.activator.Revert(changes)
	s.engine.InvalidateActivePlan()
}

// ContinueSearch transitions the system to the next search round. It archives
// the last result, reconciles the candidate list, creates a new engine,
// and returns a report of the changes.
func (s *Service) ContinueSearch() (ActionReport, error) {
	// This action can only be performed if the current search is complete.
	if !s.engine.GetCurrentState().IsComplete {
		return ActionReport{}, errors.New("cannot continue search: the current search is not yet complete")
	}

	lastEngine := s.engine
	lastState := lastEngine.GetCurrentState()
	lastConflictSet := lastState.ConflictSet

	// --- Phase 1: Enact Primary State Changes ---
	logging.Infof("BisectService: Starting 'Continue Search' for Round %d.", lastState.Round+1)

	// Mark the found conflict set as problematic. This is a primary state change
	// that will trigger our `OnStateChange` hook, setting `needsReconciliation` to true.
	s.state.SetProblematicBatch(sets.MakeSlice(lastConflictSet), true)

	// Archive the enumeration results for historical records.
	s.enumState.AddFoundConflictSet(lastConflictSet)
	s.enumState.AppendLog(lastEngine.GetExecutionLog())

	// --- Phase 2: Perform Reconciliation ---
	// Because the state is now dirty, we must reconcile it to determine any
	// newly unresolvable mods and bring the system to a consistent state.
	report := s.ReconcileState()
	report.ModsSetProblematic = lastConflictSet // Add problematic mods to the final report.
	report.HasChanges = report.HasChanges || len(lastConflictSet) > 0

	// --- Phase 3: Create New Engine ---
	// Now that the state is consistent, we can get the final list of candidates.
	finalCandidates := s.getSearchCandidates()

	if logging.IsDebugEnabled() {
		logging.Debugf("BisectService: === Continue Search Round %d Summary ===", lastState.Round+1)
		logging.Debugf("  - Last Conflict Found %d: %v", len(lastConflictSet), sets.FormatSet(lastConflictSet))
		logging.Debugf("  - All Found Conflict Sets %d: %v", len(s.enumState.FoundConflictSets), s.enumState.FoundConflictSets)
		logging.Debugf("  - Mods Marked Problematic This Round %d: %v", len(report.ModsSetProblematic), sets.FormatSet(report.ModsSetProblematic))
		logging.Debugf("  - Mods Newly Unresolvable (Auto-Disabled) This Round %d: %v", len(report.ModsSetUnresolvable), sets.FormatSet(report.ModsSetUnresolvable))
		logging.Debugf("  - Final Candidate List for New Engine %d: %v", len(finalCandidates), sets.FormatSet(finalCandidates))
		logging.Debugf("BisectService: ===========================================")
	}

	nextState := imcs.NewInitialState()
	nextState.AllModIDs = s.state.GetAllModIDs()
	nextState.Candidates = sets.MakeSlice(finalCandidates)
	nextState.Round = lastState.Round + 1
	s.engine = imcs.NewEngine(nextState)
	logging.Infof("BisectService: Initialized new engine for Round %d.", s.engine.GetCurrentState().Round)

	return report, nil
}

// ResetSearch performs a hard reset of the entire bisection process.
func (s *Service) ResetSearch() {
	allModIDs := s.state.GetAllModIDs()

	// Re-initialize the entire enumeration state from scratch.
	s.enumState = NewEnumeration()

	// Reset system-set statuses for all mods.
	s.state.SetProblematicBatch(allModIDs, false)
	s.state.SetUnresolvableBatch(allModIDs, false)

	// Create a new engine with the original full set of candidates.
	initialState := imcs.NewInitialState()
	initialState.AllModIDs = allModIDs
	initialState.Candidates = allModIDs
	s.engine = imcs.NewEngine(initialState)

	// Mark state as dirty to force a full reconciliation on the next action.
	s.needsReconciliation = true
	if s.OnStateChange != nil {
		s.OnStateChange()
	}
}

// --- Helper Methods ---

// GetCurrentExecutionLog returns the log of completed tests from the active engine.
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
	if s.enumState == nil || s.enumState.ArchivedExecutionLog == nil {
		return nil
	}

	combinedEntries := s.enumState.ArchivedExecutionLog.GetEntries()

	if s.engine != nil {
		if currentLog := s.engine.GetExecutionLog(); currentLog != nil {
			combinedEntries = append(combinedEntries, currentLog.GetEntries()...)
		}
	}

	return combinedEntries
}

// getSearchCandidates identifies and returns the set of mods that are currently
// considered active participants (candidates) in the bisection search.
func (s *Service) getSearchCandidates() sets.Set {
	searchCandidates := make(sets.Set)
	for id, status := range s.state.GetModStatusesSnapshot() {
		if status.IsSearchCandidate() {
			searchCandidates[id] = struct{}{}
		}
	}
	return searchCandidates
}

// getActivatableMods identifies and returns the set of all mods that can be
// enabled, including Omitted mods which may be required as dependencies.
func (s *Service) getActivatableMods() sets.Set {
	activatableMods := make(sets.Set)
	for id, status := range s.state.GetModStatusesSnapshot() {
		if status.IsActivatable() {
			activatableMods[id] = struct{}{}
		}
	}
	return activatableMods
}
