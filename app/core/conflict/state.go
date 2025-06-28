package conflict

import (
	"sort"

	"github.com/Qendolin/fabric-mod-bisect-tool/app/core/systemrunner"
)

// SearchStep represents a single logical step (or frame) in the
// iterative implementation of the `FindNextConflictElement` bisection algorithm.
// Each step contains the local context (background and candidates) relevant
// to that specific recursive call, pushed onto the SearchState's SearchStack.
type SearchStep struct {
	// Mods assumed to be active for this specific bisection step.
	Background map[string]struct{}
	// Mods currently being searched within for this specific bisection step.
	// It is the union of C_1 and C_2.
	Candidates []string
}

// SearchState (renamed from SearchSnapshot) represents a complete, immutable state of the conflict search.
type SearchState struct {
	// --- Fields representing the state of the main "FindConflictSet" procedure ---

	// ConflictSet contains mods already identified as part of the minimal conflict set.
	ConflictSet map[string]struct{}
	// Candidates is the global pool of mods remaining to be searched for conflicts.
	// This set only shrinks when a new element is added to ConflictSet.
	Candidates []string
	// Background is the set of mods globally proven to be "good" and not part of any
	// conflict. It grows as bisections on candidate sets result in GOOD.
	Background map[string]struct{}

	// --- Fields representing the state of the "FindNextConflictElement" bisection ---

	// SearchStack is an iterative implementation of the recursive "FindNextConflictElement"
	// procedure. Each SearchStep on the stack is a snapshot of a recursive call's arguments
	// (the local background and local candidates for that bisection).
	SearchStack []SearchStep

	// --- Global Metadata and Flags ---

	// IsVerifyingConflictSet is true if the next test planned should be the final `test(ConflictSet)` optimization step.
	IsVerifyingConflictSet bool
	// AllModIDs is the universe of all mods, used for context and resetting candidates.
	AllModIDs []string
	// IsComplete is true if the search has concluded and no more tests are needed.
	IsComplete bool
	// LastFoundElement stores the ID of the most recently discovered conflict element.
	LastFoundElement string
	// LastTestResult stores the outcome of the test that produced this state.
	LastTestResult systemrunner.Result
}

// newSearchStep creates a search step with sorted candidates for determinism.
func newSearchStep(background map[string]struct{}, candidates []string) SearchStep {
	// Sorting ensures that the binary search partitioning is deterministic across runs.
	sort.Strings(candidates)
	return SearchStep{
		Background: background,
		Candidates: candidates,
	}
}

// NewInitialState creates the starting state for a new search.
func NewInitialState(allModIDs []string) SearchState {
	// Sort the initial list for deterministic behavior.
	sortedInitialCandidates := make([]string, len(allModIDs))
	copy(sortedInitialCandidates, allModIDs)
	sort.Strings(sortedInitialCandidates)

	return SearchState{
		ConflictSet:            make(map[string]struct{}),
		Candidates:             sortedInitialCandidates,
		Background:             make(map[string]struct{}),
		SearchStack:            make([]SearchStep, 0),
		IsVerifyingConflictSet: false,
		AllModIDs:              allModIDs,
		IsComplete:             false,
		LastFoundElement:       "",
		LastTestResult:         "",
	}
}

// GetCurrentStep returns the top of the search stack, which represents the
// state of the current "FindNextConflictElement" bisection.
func (s SearchState) GetCurrentStep() (SearchStep, bool) {
	if len(s.SearchStack) == 0 {
		return SearchStep{}, false
	}
	return s.SearchStack[len(s.SearchStack)-1], true
}

// GetBisectionSets determines the current C1 and C2 sets based on the state.
// It returns the background for the test, and the C1 and C2 sets.
func (s SearchState) GetBisectionSets() (background, c1, c2 map[string]struct{}) {
	var candidatesForStep []string

	if step, ok := s.GetCurrentStep(); ok {
		// Mid-bisection: use the context from the top of the stack.
		candidatesForStep = step.Candidates
		background = step.Background
	} else {
		// Start of a new bisection: use the top-level candidates.
		candidatesForStep = s.Candidates
		background = s.ConflictSet // The background for a new bisection is the conflict set.
	}

	c1Slice, c2Slice := split(candidatesForStep)
	c1 = stringSliceToSet(c1Slice)
	c2 = stringSliceToSet(c2Slice)
	return
}
