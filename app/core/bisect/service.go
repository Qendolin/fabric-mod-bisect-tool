package bisect

import (
	"fmt"

	"github.com/Qendolin/fabric-mod-bisect-tool/app/core/imcs"
	"github.com/Qendolin/fabric-mod-bisect-tool/app/core/mods"
	"github.com/Qendolin/fabric-mod-bisect-tool/app/core/sets"
	"github.com/Qendolin/fabric-mod-bisect-tool/app/logging"
)

// Service encapsulates the entire bisection business logic.
type Service struct {
	state     *mods.StateManager
	activator *mods.Activator
	engine    *imcs.Engine

	OnStateChange func()
}

// NewService creates a new bisect service from pre-loaded components.
func NewService(stateMgr *mods.StateManager, activator *mods.Activator, engine *imcs.Engine) (*Service, error) {
	if err := activator.EnableAll(); err != nil {
		return nil, fmt.Errorf("failed to enable all mods on startup: %w", err)
	}

	svc := &Service{
		state:     stateMgr,
		activator: activator,
		engine:    engine,
	}
	stateMgr.OnStateChanged = svc.handleStateChange
	return svc, nil
}

// handleStateChange is called when a mod's forced status changes.
func (s *Service) handleStateChange() {
	logging.Debugf("BisectService: State change detected. Reconciling engine state.")
	validCandidates := s.getValidCandidates()
	s.engine.Reconcile(validCandidates)
	if s.OnStateChange != nil {
		s.OnStateChange()
	}
}

// --- Direct Component Access ---
func (s *Service) StateManager() *mods.StateManager { return s.state }
func (s *Service) Activator() *mods.Activator       { return s.activator }
func (s *Service) Engine() *imcs.Engine             { return s.engine }

// --- High-Level Workflow Methods ---

// GetCurrentState returns a read-only snapshot of the engine's state.
func (s *Service) GetCurrentState() imcs.SearchState {
	return s.engine.GetCurrentState()
}

// StartNewSearch resets the bisection process.
func (s *Service) StartNewSearch() {
	s.engine.MergePendingAdditions()
	s.engine.StartNewSearch()
	if s.OnStateChange != nil {
		s.OnStateChange()
	}
}

// PlanAndExecuteTestStep is the single entry point for the UI's "Step" action.
func (s *Service) PlanAndExecuteTestStep() (changes []mods.BatchStateChange, plan *imcs.TestPlan, err error) {
	plan, err = s.engine.PlanNextTest()
	if err != nil {
		return nil, nil, err
	}

	logging.Debugf("BisectService: Plan generated. Resolving effective set for test targets: %v", sets.FormatSet(plan.ModIDsToTest))

	effectiveSet, _ := s.state.ResolveEffectiveSet(plan.ModIDsToTest)

	// Optional: Log the full resolution path for extreme detail.
	// var resLog []string
	// for _, info := range resolutionPath {
	//     resLog = append(resLog, fmt.Sprintf("  - %s (%s)", info.ModID, info.Reason))
	// }
	// logging.Debugf("BisectService: Full resolution path:\n%s", strings.Join(resLog, "\n"))

	logging.Debugf("BisectService: Effective set contains %d mods: %v", len(effectiveSet), sets.FormatSet(effectiveSet))

	changes, err = s.activator.Apply(effectiveSet)
	if err != nil {
		s.activator.Revert(changes)
		return nil, nil, fmt.Errorf("failed to apply file changes: %w", err)
	}

	return changes, plan, nil
}

// SubmitTestResult processes the outcome of a test.
func (s *Service) SubmitTestResult(result imcs.TestResult, changes []mods.BatchStateChange) {
	s.activator.Revert(changes)
	if err := s.engine.SubmitTestResult(result); err != nil {
		logging.Errorf("BisectService: Failed to submit test result to engine: %v", err)
	}
	if s.OnStateChange != nil {
		s.OnStateChange()
	}
}

// UndoLastStep reverts the search to its previous state.
func (s *Service) UndoLastStep() {
	if s.engine.Undo() && s.OnStateChange != nil {
		s.OnStateChange()
	}
}

// CancelTest reverts file changes and invalidates the current test plan.
func (s *Service) CancelTest(changes []mods.BatchStateChange) {
	s.activator.Revert(changes)
	s.engine.InvalidateActivePlan()
}

// --- Helper Methods ---

// GetExecutionLog returns the log of completed tests.
func (s *Service) GetExecutionLog() *imcs.ExecutionLog {
	if s.engine == nil {
		return nil
	}
	return s.engine.GetExecutionLog()
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
