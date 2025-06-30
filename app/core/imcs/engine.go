package imcs

import (
	"errors"
	"fmt"
	"math"

	"github.com/Qendolin/fabric-mod-bisect-tool/app/core/sets"
	"github.com/Qendolin/fabric-mod-bisect-tool/app/logging"
)

// Engine orchestrates the bisection search, owning the state and using
// the stateless algorithm to advance it.
type Engine struct {
	allItems []string
	// Items that will be added to the search at the end of the current iteration
	pendingAdditions sets.Set

	algorithm    *IMCSAlgorithm
	undoStack    *UndoStack
	executionLog *ExecutionLog

	state      SearchState
	activePlan *TestPlan
}

// NewEngine creates a new search orchestrator.
func NewEngine(allItems []string) *Engine {
	return &Engine{
		allItems:         allItems,
		algorithm:        NewIMCSAlgorithm(),
		undoStack:        NewUndoStack(),
		executionLog:     NewExecutionLog(),
		pendingAdditions: make(sets.Set),
	}
}

// StartNewSearch initializes a new search process.
func (e *Engine) StartNewSearch() {
	logging.Infof("IMCSEngine: Starting new search with %d total mods.", len(e.allItems))
	e.state = NewInitialState(e.allItems)
	e.undoStack.Clear()
	e.executionLog.Clear()
	e.activePlan = nil
}

// GetNextTestPlan calculates and returns the next test plan based on the current
// state without changing any internal state. This is an idempotent getter for UI/preview.
func (e *Engine) GetNextTestPlan() (*TestPlan, error) {
	if e.activePlan != nil {
		return nil, fmt.Errorf("a test is already in progress and must be completed or cancelled")
	}
	// Note: We call the algorithm's PlanNextTest, which is the pure function.
	plan, err := e.algorithm.PlanNextTest(e.state)
	if err != nil {
		return nil, err
	}
	return plan, nil
}

// PlanNextTest commits to the next test plan, changing the process state to "awaiting result".
// This is a non-idempotent action.
func (e *Engine) PlanNextTest() (*TestPlan, error) {
	if e.activePlan != nil {
		return nil, fmt.Errorf("a test is already in progress")
	}

	plan, err := e.algorithm.PlanNextTest(e.state)
	if err != nil {
		logging.Warnf("IMCSEngine: Could not plan next test: %v", err)
		return nil, err
	}

	e.activePlan = plan
	logging.Infof("IMCSEngine: Committed to test plan with %d mods.", len(plan.ModIDsToTest))
	return plan, nil
}

// InvalidateActivePlan cancels any in-progress test plan, usually due to an
// external state change that makes the test irrelevant.
func (e *Engine) InvalidateActivePlan() {
	if e.activePlan != nil {
		logging.Warnf("IMCSEngine: Invalidating active test plan due to external state change.")
		e.activePlan = nil
	}
}

// SubmitTestResult provides the result for the active test, advancing the search.
// It implicitly operates on the currently active plan.
func (e *Engine) SubmitTestResult(result TestResult) error {
	if e.activePlan == nil {
		msg := "no active test plan to submit a result for"
		logging.Warnf("IMCSEngine: " + msg)
		return errors.New(msg)
	}

	logging.Infof("IMCSEngine: Submitting result '%s' for active test.", result)
	// The plan that was active before this step.
	committedPlan := *e.activePlan

	// Push the state *before* applying the result to the undo stack.
	e.undoStack.Push(e.state)

	// Log the completed test for the UI history.
	completedTest := CompletedTest{
		Plan:            committedPlan,
		Result:          result,
		StateBeforeTest: e.state,
	}
	e.executionLog.Log(completedTest)

	// Calculate the next state.
	newState, err := e.algorithm.ApplyResult(e.state, committedPlan, result)
	if err != nil {
		logging.Errorf("IMCSEngine: Error applying result: %v", err)
		// Even on error, we should clear the active plan to avoid getting stuck.
		e.activePlan = nil
		return err
	}

	e.state = newState
	e.activePlan = nil // Ready for the next test.

	if e.state.IsComplete {
		logging.Infof("IMCSEngine: Search is now complete. Final conflict set: %v", sets.MakeSlice(e.state.ConflictSet))
	}

	// The merge logic for pending additions.
	if e.WasLastTestVerification() && len(e.pendingAdditions) > 0 {
		e.MergePendingAdditions()
	}

	return nil
}

// MergePendingAdditions merges any deferred items into the main candidate pool.
// This is called at safe-boundary points, like the end of an iteration or a manual reset.
func (e *Engine) MergePendingAdditions() {
	if len(e.pendingAdditions) == 0 {
		return
	}
	logging.Infof("IMCSEngine: Merging %d pending item(s) into candidate pool.", len(e.pendingAdditions))
	e.AddCandidates(e.pendingAdditions)
	e.pendingAdditions = make(sets.Set) // Clear the pending list.
}

// Reconcile intelligently synchronizes the engine's internal state with a
// new set of valid candidates from an external source. It handles pruning of
// invalid items from all parts of the current search state and defers the
// addition of new items until the end of the current bisection iteration.
func (e *Engine) Reconcile(validCandidates sets.Set) {
	// 1. Invalidate any active plan, as the underlying assumptions have changed.
	e.InvalidateActivePlan()

	// 2. Determine the full set of items the engine currently considers part of the search.
	currentEngineItems := sets.MakeSet(e.state.Candidates)
	currentAndPending := sets.Union(currentEngineItems, e.pendingAdditions)

	// 3. Calculate what needs to be removed and what needs to be added.
	removals := sets.Subtract(currentAndPending, validCandidates)
	additions := sets.Subtract(validCandidates, currentAndPending)

	// 4. Immediately apply all removals.
	if len(removals) > 0 {
		logging.Infof("IMCSEngine: Reconcile removed %d item(s) from search.", len(removals))
		e.pendingAdditions = sets.Subtract(e.pendingAdditions, removals)
		e.RemoveCandidates(removals)
	}

	// 5. Defer all additions.
	if len(additions) > 0 {
		logging.Infof("IMCSEngine: Reconcile deferred addition of %d item(s).", len(additions))
		e.pendingAdditions = sets.Union(e.pendingAdditions, additions)
	}
}

// RemoveCandidates safely prunes a set of items from all aspects of the
// current search state, including the candidate list, conflict set, stable set,
// and any in-progress bisection steps on the search stack.
func (e *Engine) RemoveCandidates(removals sets.Set) {
	e.state.ConflictSet = sets.Subtract(e.state.ConflictSet, removals)
	e.state.StableSet = sets.Subtract(e.state.StableSet, removals)
	e.state.Candidates = sets.SubtractSlices(e.state.Candidates, sets.MakeSlice(removals))

	// Rebuild the search stack, pruning removed candidates from each step.
	newStack := make([]SearchStep, 0, len(e.state.SearchStack))
	for _, step := range e.state.SearchStack {
		step.Candidates = sets.SubtractSlices(step.Candidates, sets.MakeSlice(removals))
		// Only keep steps that still have candidates to test.
		if len(step.Candidates) > 0 {
			newStack = append(newStack, step)
		}
	}
	e.state.SearchStack = newStack
}

// AddCandidates safely adds a set of new items to the main candidate pool
// for future bisection iterations.
func (e *Engine) AddCandidates(additions sets.Set) {
	newCandidates := sets.MakeSet(e.state.Candidates)
	for item := range additions {
		newCandidates[item] = struct{}{}
	}
	e.state.Candidates = sets.MakeSlice(newCandidates)
}

// Undo reverts to the previous state in the search.
// It also clears any active plan, as it is no longer valid.
func (e *Engine) Undo() bool {
	logging.Info("IMCSEngine: Attempting to undo last step.")
	previousState, err := e.undoStack.Pop()
	if err != nil {
		logging.Warnf("IMCSEngine: Cannot undo: %v", err)
		return false
	}
	e.state = previousState
	e.activePlan = nil // Invalidate any active plan after an undo.
	logging.Info("IMCSEngine: Successfully reverted to previous state.")
	return true
}

// GetCurrentState returns a read-only view of the current search state.
func (e *Engine) GetCurrentState() SearchState {
	return e.state
}

// GetExecutionLog provides access to the log of completed tests.
func (e *Engine) GetExecutionLog() *ExecutionLog {
	return e.executionLog
}

// GetStepCount returns the number of committed steps in the current search path.
// This is the correct value to display to the user as the current progress.
func (e *Engine) GetStepCount() int {
	return e.undoStack.Size()
}

// GetActiveTestPlan returns the plan currently being tested.
func (e *Engine) GetActiveTestPlan() *TestPlan {
	if e.activePlan == nil {
		return nil
	}
	// Return a copy to prevent mutation
	planCopy := *e.activePlan
	return &planCopy
}

// WasLastTestVerification checks if the most recently completed test was the final verification step.
func (e *Engine) WasLastTestVerification() bool {
	lastTest, found := e.executionLog.GetLastTest()
	if !found {
		return false
	}
	return lastTest.Plan.IsVerificationStep
}

// GetEstimatedMaxTests provides an estimated upper bound for the total tests.
// This estimate increases as more problems are found, which is intended.
func (e *Engine) GetEstimatedMaxTests() int {
	// The initial candidates are all mods participating in the search.
	// This value is also not exact, but an upper bound.
	numInitialCandidates := len(e.state.AllModIDs)

	// Number of problems found is the size of the ConflictSet.
	problemsFound := len(e.state.ConflictSet)

	if problemsFound == 0 && !e.state.IsComplete {
		problemsFound = 1
	}

	if numInitialCandidates == 0 || problemsFound == 0 {
		return 0
	}

	// The formula: problems * (ceil(log2(n)) + 1)
	// The +1 accounts for the verification step.
	return problemsFound * (int(math.Ceil(math.Log2(float64(numInitialCandidates)))) + 1)
}

// GetPendingAdditions returns the items that will be added to the search at the end of the current iteration
func (e *Engine) GetPendingAdditions() sets.Set {
	return e.pendingAdditions
}
