package bisect

import (
	"fmt"

	"github.com/Qendolin/fabric-mod-bisect-tool/app/core/imcs"
	"github.com/Qendolin/fabric-mod-bisect-tool/app/core/mods"
)

// Service encapsulates the entire bisection business logic.
type Service struct {
	state     *mods.StateManager
	activator *mods.ModActivator
	engine    *imcs.Engine

	OnStateChange func()
}

// NewService creates a new bisect service from pre-loaded components.
func NewService(stateMgr *mods.StateManager, activator *mods.ModActivator, engine *imcs.Engine) (*Service, error) {
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
	s.engine.InvalidateActivePlan() // A forced status change invalidates any planned test.
	if s.OnStateChange != nil {
		s.OnStateChange()
	}
}

// --- Direct Component Access ---
func (s *Service) StateManager() *mods.StateManager { return s.state }
func (s *Service) Activator() *mods.ModActivator    { return s.activator }
func (s *Service) Engine() *imcs.Engine             { return s.engine }

// --- High-Level Workflow Methods ---

// GetCurrentState returns a read-only snapshot of the engine's state.
func (s *Service) GetCurrentState() imcs.SearchState {
	return s.engine.GetCurrentState()
}

// StartNewSearch resets the bisection process.
func (s *Service) StartNewSearch() {
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

	effectiveSet, _ := s.state.ResolveEffectiveSet(plan.ModIDsToTest)

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
	s.engine.SubmitTestResult(result)
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
