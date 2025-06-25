package conflict

import (
	"sort"

	"github.com/Qendolin/fabric-mod-bisect-tool/app/core/systemrunner"
)

// SearchStep represents one logical step in the IMCS algorithm's binary search.
// It's used to manage the non-recursive, stateful search process.
type SearchStep struct {
	Background map[string]struct{} // Mods assumed to be active (context for the test).
	Candidates []string            // Mods currently being searched within. A slice for deterministic splitting.
}

// SearchSnapshot represents a complete, saveable state of the conflict searcher.
// This is used for history and rollbacks.
type SearchSnapshot struct {
	ConflictSet    map[string]struct{} // Mods already identified as part of the minimal conflict set.
	Candidates     []string            // Remaining mods yet to be searched for conflict elements.
	Background     map[string]struct{} // Mods currently assumed to be active during the main loop's search.
	SearchStack    []SearchStep        // Stack representing the current binary search for a single element.
	IsCheckingDone bool                // True if the next test is the final `test(ConflictSet)` optimization.
	LastTestResult systemrunner.Result // Stores the result of the last test performed for this state.
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
