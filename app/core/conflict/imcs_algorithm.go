// imcs_algorithm.go
package conflict

import (
	"fmt"
	"sort"

	"github.com/Qendolin/fabric-mod-bisect-tool/app/core/systemrunner"
	"github.com/Qendolin/fabric-mod-bisect-tool/app/logging"
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
		return nil, fmt.Errorf("search is already complete")
	}

	// Priority 1: A bisection search for an element is in progress.
	if len(state.SearchStack) > 0 {
		currentStep := state.SearchStack[len(state.SearchStack)-1]
		c1, _ := split(currentStep.Candidates)
		testSet := union(currentStep.Background, stringSliceToSet(c1))
		return &TestPlan{ModIDsToTest: testSet, IsVerificationStep: false}, nil
	}

	// Priority 2: No bisection, but we need to run the `test(ConflictSet)` optimization.
	if state.IsVerifyingConflictSet {
		// This test only includes the conflict set itself, no other context.
		return &TestPlan{ModIDsToTest: state.ConflictSet, IsVerificationStep: true}, nil
	}

	// Priority 3: No bisection, no verification. Time to start a new search for the next element.
	if len(state.Candidates) == 0 {
		// All candidates have been processed. The search is over.
		return nil, fmt.Errorf("search complete")
	}

	// This is the start of a "FindNextConflictElement" call. Plan the first test.
	// The background for this new bisection is the globally confirmed ConflictSet.
	bisectionBackground := state.ConflictSet
	c1, _ := split(state.Candidates)
	testSet := union(bisectionBackground, stringSliceToSet(c1))
	return &TestPlan{ModIDsToTest: testSet, IsVerificationStep: false}, nil
}

// ApplyResult takes a state, a completed test plan, and its result,
// and returns the new, updated state. This is a pure function.
func (a *IMCSAlgorithm) ApplyResult(state SearchState, plan TestPlan, result systemrunner.Result) (SearchState, error) {
	newState := deepCopyState(state)
	newState.LastTestResult = result
	newState.IsVerifyingConflictSet = false // Flag is consumed after one use.

	// --- Handle Verification Step Result ---
	if plan.IsVerificationStep {
		if result == systemrunner.FAIL {
			// The current ConflictSet is sufficient to cause a crash. We are done.
			logging.Info("IMCSAlgorithm: Verification PASSED. ConflictSet is minimal.")
			newState.IsComplete = true
		} else { // GOOD
			// The current ConflictSet is not sufficient. Continue the search for more elements.
			logging.Info("IMCSAlgorithm: Verification FAILED. ConflictSet not sufficient, continuing search.")
		}
		return newState, nil
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
		newState.SearchStack = newState.SearchStack[:len(newState.SearchStack)-1] // Pop
	}

	c1, c2 := split(stepToProcess.Candidates)

	if result == systemrunner.FAIL {
		// The conflict is in the first half (c1).
		if len(c1) == 1 { // Base Case: Test of a single element + context failed.
			foundMod := c1[0]
			logging.Infof("IMCSAlgorithm: Bisection found a conflict element: %s", foundMod)

			// Update the GLOBAL state with the new finding.
			newState.ConflictSet[foundMod] = struct{}{}
			newState.Background = newState.ConflictSet
			newState.Candidates = difference(newState.Candidates, []string{foundMod})
			newState.LastFoundElement = foundMod

			// Terminate this bisection, clear the stack, and flag for verification.
			newState.SearchStack = make([]SearchStep, 0)
			newState.IsVerifyingConflictSet = true
		} else { // Recursive Case: Continue bisection within c1.
			// The background does not change as we descend into a failing partition.
			newState.SearchStack = append(newState.SearchStack, newSearchStep(stepToProcess.Background, c1))
		}
	} else { // GOOD
		// The test on c1 + context passed. The conflict must be in c2.
		if len(c2) > 0 {
			// The next bisection step for c2 uses an expanded background,
			// including the "good" chunk c1.
			newBackgroundForNextStep := union(stepToProcess.Background, stringSliceToSet(c1))
			newState.SearchStack = append(newState.SearchStack, newSearchStep(newBackgroundForNextStep, c2))
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

	return newState, nil
}

// --- Helper functions for set operations ---

// split is a corrected helper that never returns an empty c1 for a non-empty slice.
// This is critical for handling single-element candidate sets correctly.
func split(mods []string) ([]string, []string) {
	if len(mods) == 0 {
		return []string{}, []string{}
	}
	mid := (len(mods) + 1) / 2
	return mods[:mid], mods[mid:]
}

func union(a, b map[string]struct{}) map[string]struct{} {
	res := make(map[string]struct{}, len(a)+len(b))
	for k := range a {
		res[k] = struct{}{}
	}
	for k := range b {
		res[k] = struct{}{}
	}
	return res
}

func stringSliceToSet(s []string) map[string]struct{} {
	set := make(map[string]struct{})
	for _, item := range s {
		set[item] = struct{}{}
	}
	return set
}

func setToSlice(set map[string]struct{}) []string {
	slice := make([]string, 0, len(set))
	for k := range set {
		slice = append(slice, k)
	}
	sort.Strings(slice)
	return slice
}

func difference(all []string, toRemove []string) []string {
	removeSet := stringSliceToSet(toRemove)
	var result []string
	for _, item := range all {
		if _, found := removeSet[item]; !found {
			result = append(result, item)
		}
	}
	return result
}
