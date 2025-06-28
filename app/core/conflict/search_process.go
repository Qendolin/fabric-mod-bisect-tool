package conflict

import (
	"errors"
	"fmt"
	"math"

	"github.com/Qendolin/fabric-mod-bisect-tool/app/core/mods"
	"github.com/Qendolin/fabric-mod-bisect-tool/app/core/systemrunner"
	"github.com/Qendolin/fabric-mod-bisect-tool/app/logging"
)

// SearchProcess orchestrates the bisection search, owning the state and using
// the stateless algorithm to advance it.
type SearchProcess struct {
	modState *mods.StateManager // Needed for mod data access

	algorithm    *IMCSAlgorithm
	undoStack    *UndoStack
	executionLog *ExecutionLog

	currentState SearchState
	activePlan   *TestPlan
}

// NewSearchProcess creates a new search orchestrator.
func NewSearchProcess(modState *mods.StateManager) *SearchProcess {
	return &SearchProcess{
		modState:     modState,
		algorithm:    NewIMCSAlgorithm(),
		undoStack:    NewUndoStack(),
		executionLog: NewExecutionLog(),
	}
}

// StartNewSearch initializes a new search process.
func (sp *SearchProcess) StartNewSearch() {
	allModIDs := sp.modState.GetAllModIDs()
	logging.Infof("SearchProcess: Starting new search with %d total mods.", len(allModIDs))
	sp.currentState = NewInitialState(allModIDs)
	sp.undoStack.Clear()
	sp.executionLog.Clear()
	sp.activePlan = nil
}

// GetNextTestPlan calculates and returns the next test plan based on the current
// state without changing any internal state. This is an idempotent getter for UI/preview.
func (sp *SearchProcess) GetNextTestPlan() (*TestPlan, error) {
	if sp.activePlan != nil {
		return nil, fmt.Errorf("a test is already in progress and must be completed or cancelled")
	}
	// Note: We call the algorithm's PlanNextTest, which is the pure function.
	plan, err := sp.algorithm.PlanNextTest(sp.currentState)
	if err != nil {
		return nil, err
	}
	return plan, nil
}

// PlanNextTest commits to the next test plan, changing the process state to "awaiting result".
// This is a non-idempotent action.
func (sp *SearchProcess) PlanNextTest() (*TestPlan, error) {
	if sp.activePlan != nil {
		return nil, fmt.Errorf("a test is already in progress")
	}

	plan, err := sp.algorithm.PlanNextTest(sp.currentState)
	if err != nil {
		logging.Warnf("SearchProcess: Could not plan next test: %v", err)
		return nil, err
	}

	sp.activePlan = plan
	logging.Infof("SearchProcess: Committed to test plan with %d mods.", len(plan.ModIDsToTest))
	return plan, nil
}

// InvalidateActivePlan cancels any in-progress test plan, usually due to an
// external state change that makes the test irrelevant.
func (sp *SearchProcess) InvalidateActivePlan() {
	if sp.activePlan != nil {
		logging.Warnf("SearchProcess: Invalidating active test plan due to external state change.")
		sp.activePlan = nil
	}
}

// SubmitTestResult provides the result for the active test, advancing the search.
// It implicitly operates on the currently active plan.
func (sp *SearchProcess) SubmitTestResult(result systemrunner.Result) error {
	if sp.activePlan == nil {
		msg := "no active test plan to submit a result for"
		logging.Warnf("SearchProcess: " + msg)
		return errors.New(msg)
	}

	logging.Infof("SearchProcess: Submitting result '%s' for active test.", result)
	// The plan that was active before this step.
	committedPlan := *sp.activePlan

	// Push the state *before* applying the result to the undo stack.
	sp.undoStack.Push(sp.currentState)

	// Log the completed test for the UI history.
	completedTest := CompletedTest{
		Plan:            committedPlan,
		Result:          result,
		StateBeforeTest: sp.currentState,
	}
	sp.executionLog.Log(completedTest)

	// Calculate the next state.
	newState, err := sp.algorithm.ApplyResult(sp.currentState, committedPlan, result)
	if err != nil {
		logging.Errorf("SearchProcess: Error applying result: %v", err)
		// Even on error, we should clear the active plan to avoid getting stuck.
		sp.activePlan = nil
		return err
	}

	sp.currentState = newState
	sp.activePlan = nil // Ready for the next test.

	if sp.currentState.IsComplete {
		logging.Infof("SearchProcess: Search is now complete. Final conflict set: %v", setToSlice(sp.currentState.ConflictSet))
	}

	return nil
}

// Undo reverts to the previous state in the search.
// It also clears any active plan, as it is no longer valid.
func (sp *SearchProcess) Undo() bool {
	logging.Info("SearchProcess: Attempting to undo last step.")
	previousState, err := sp.undoStack.Pop()
	if err != nil {
		logging.Warnf("SearchProcess: Cannot undo: %v", err)
		return false
	}
	sp.currentState = previousState
	sp.activePlan = nil // Invalidate any active plan after an undo.
	logging.Info("SearchProcess: Successfully reverted to previous state.")
	return true
}

// GetCurrentState returns a read-only view of the current search state.
func (sp *SearchProcess) GetCurrentState() SearchState {
	return sp.currentState
}

// GetExecutionLog provides access to the log of completed tests.
func (sp *SearchProcess) GetExecutionLog() *ExecutionLog {
	return sp.executionLog
}

// GetStepCount returns the number of committed steps in the current search path.
// This is the correct value to display to the user as the current progress.
func (sp *SearchProcess) GetStepCount() int {
	return sp.undoStack.Size()
}

// GetActiveTestPlan returns the plan currently being tested.
func (sp *SearchProcess) GetActiveTestPlan() *TestPlan {
	if sp.activePlan == nil {
		return nil
	}
	// Return a copy to prevent mutation
	planCopy := *sp.activePlan
	return &planCopy
}

// WasLastTestVerification checks if the most recently completed test was the final verification step.
func (sp *SearchProcess) WasLastTestVerification() bool {
	lastTest, found := sp.executionLog.GetLastTest()
	if !found {
		return false
	}
	return lastTest.Plan.IsVerificationStep
}

// GetEstimatedMaxTests provides an estimated upper bound for the total tests.
// This estimate increases as more problems are found, which is intended.
func (sp *SearchProcess) GetEstimatedMaxTests() int {
	// The initial candidates are all mods participating in the search.
	// This value is also not exact, but an upper bound.
	numInitialCandidates := len(sp.currentState.AllModIDs)

	// Number of problems found is the size of the ConflictSet.
	problemsFound := len(sp.currentState.ConflictSet)

	if problemsFound == 0 && !sp.currentState.IsComplete {
		problemsFound = 1
	}

	if numInitialCandidates == 0 || problemsFound == 0 {
		return 0
	}

	// The formula: problems * (ceil(log2(n)) + 1)
	// The +1 accounts for the verification step.
	return problemsFound * (int(math.Ceil(math.Log2(float64(numInitialCandidates)))) + 1)
}
