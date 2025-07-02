// imcs_algorithm.go
package imcs

import (
	"github.com/Qendolin/fabric-mod-bisect-tool/pkg/core/sets"
	"github.com/Qendolin/fabric-mod-bisect-tool/pkg/logging"
)

// IMCSAlgorithm contains the pure, stateless logic for the bisection search.
type IMCSAlgorithm struct{}

// NewIMCSAlgorithm creates a new algorithm instance.
func NewIMCSAlgorithm() *IMCSAlgorithm {
	return &IMCSAlgorithm{}
}

// PlanNextTest determines the next test to run based on the current state.
// This logic now directly mirrors the decision points in the formal IMCS algorithm.
func (a *IMCSAlgorithm) PlanNextTest(state SearchState) (*TestPlan, error) {
	if state.IsComplete {
		return nil, ErrSearchComplete
	}

	// Priority 1: A bisection search for an element is in progress.
	if len(state.SearchStack) > 0 {
		currentStep := state.SearchStack[len(state.SearchStack)-1]
		c1, _ := sets.Split(currentStep.Candidates)
		testSet := sets.Union(currentStep.StableSet, sets.MakeSet(c1))
		logging.Debugf("IMCSAlgorithm.PlanNextTest: Continuing bisection (stack depth %d). StableSet: %v, Candidates: %v. Testing first half: %v", len(state.SearchStack), sets.FormatSet(currentStep.StableSet), currentStep.Candidates, c1)
		return &TestPlan{ModIDsToTest: testSet, IsVerificationStep: false}, nil
	}

	// Priority 2: No bisection, but we need to run the `test(ConflictSet)` optimization.
	if state.IsVerifyingConflictSet {
		logging.Debugf("IMCSAlgorithm.PlanNextTest: Planning verification test for ConflictSet: %v", sets.FormatSet(state.ConflictSet))
		// This test only includes the conflict set itself, no other context.
		return &TestPlan{ModIDsToTest: state.ConflictSet, IsVerificationStep: true}, nil
	}

	// Priority 3: No bisection, no verification. Time to start a new search for the next element.
	if len(state.Candidates) == 0 {
		// All candidates have been processed. The search is over.
		return nil, ErrSearchComplete
	}

	// This is the start of a "FindNextConflictElement" call. Plan the first test.
	// The StableSet for this new bisection is the globally confirmed ConflictSet.
	stableSet := state.ConflictSet
	c1, _ := sets.Split(state.Candidates)
	testSet := sets.Union(stableSet, sets.MakeSet(c1))

	logging.Debugf("IMCSAlgorithm.PlanNextTest: Starting new bisection. StableSet: %v, All Candidates: %v. Testing first half: %v", sets.FormatSet(stableSet), state.Candidates, c1)

	return &TestPlan{ModIDsToTest: testSet, IsVerificationStep: false}, nil
}

// ApplyResult takes a state, a completed test plan, and its result,
// and returns the new, updated state. This is a pure function.
func (a *IMCSAlgorithm) ApplyResult(state SearchState, plan TestPlan, result TestResult) SearchState {
	newState := deepCopyState(state)
	newState.LastTestResult = result
	newState.IsVerifyingConflictSet = false // Flag is consumed after one use.
	newState.Step++

	// --- Handle Verification Step Result ---
	if plan.IsVerificationStep {
		newState.Step = 0
		if result == TestResultFail {
			// The current ConflictSet is sufficient to cause a crash. We are done.
			logging.Info("IMCSAlgorithm: Verification PASSED. ConflictSet is minimal.")
			newState.IsComplete = true
		} else { // GOOD
			// The current ConflictSet is not sufficient. Continue the search for more elements.
			logging.Info("IMCSAlgorithm: Verification FAILED. ConflictSet not sufficient, continuing search.")
			newState.Iteration++
		}
		return newState
	}

	// --- Handle Bisection Step Result ---
	var stepToProcess SearchStep
	if len(state.SearchStack) == 0 {
		// If the stack was empty, this test was the start of a new bisection.
		// The context for this step is the global ConflictSet and Candidates.
		stepToProcess = newSearchStep(state.ConflictSet, state.Candidates)
	} else {
		// A bisection was in progress. The context is the top of the stack.
		stepToProcess = state.SearchStack[len(state.SearchStack)-1]
	}

	c1, c2 := sets.Split(stepToProcess.Candidates)

	if result == TestResultFail {
		// The conflict is in the first half (c1).
		if len(c1) == 1 { // Base Case: Test of a single element + context failed.
			foundMod := c1[0]
			logging.Infof("IMCSAlgorithm: Bisection found a conflict element: %s", foundMod)

			// Update the GLOBAL state with the new finding.
			newState.ConflictSet[foundMod] = struct{}{}
			newState.StableSet = newState.ConflictSet
			newState.Candidates = sets.SubtractSlices(newState.Candidates, []string{foundMod})
			newState.LastFoundElement = foundMod

			// The iteration count should be incremented here when following the formal algorithm strictly

			// Terminate this bisection, clear the stack, and flag for verification.
			newState.SearchStack = make([]SearchStep, 0)
			newState.IsVerifyingConflictSet = true
		} else { // Recursive Case: Continue bisection within c1.
			// The StableSet does not change as we descend into a failing partition.
			newState.SearchStack = append(newState.SearchStack, newSearchStep(stepToProcess.StableSet, c1))
		}
	} else { // GOOD
		// The test on c1 + context passed. The conflict must be in c2.

		// Pop the stack
		if len(newState.SearchStack) > 0 {
			newState.SearchStack = newState.SearchStack[:len(newState.SearchStack)-1]
		}

		if len(c2) > 0 {
			// The next bisection step for c2 uses an expanded StableSet, including the "good" chunk c1.
			newStableSetForNextStep := sets.Union(stepToProcess.StableSet, sets.MakeSet(c1))
			logging.Debugf("IMCSAlgorithm.ApplyResult: Test was GOOD. Adding %v to stable set for next step.", c1)
			newState.SearchStack = append(newState.SearchStack, newSearchStep(newStableSetForNextStep, c2))
		} else {
			// c2 is empty, meaning c1 was the last candidate(s) in this branch.
			// Since it was GOOD, this bisection branch is exhausted. If the stack is also
			// now empty, the entire bisection is over and found nothing.
			if len(newState.SearchStack) == 0 {
				logging.Info("IMCSAlgorithm: Bisection finished, no conflict found. Search is complete.")
				newState.IsComplete = true
			}
		}
	}

	logging.Debugf("IMCSAlgorithm.ApplyResult: Applied result '%s'. New state: IsComplete=%t, ConflictSet=%v, StackDepth=%d", result, newState.IsComplete, sets.FormatSet(newState.ConflictSet), len(newState.SearchStack))

	return newState
}
