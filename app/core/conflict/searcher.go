package conflict

import (
	"context"
	"fmt"
	"math"
	"sort"

	"github.com/Qendolin/fabric-mod-bisect-tool/app/core/mods"
	"github.com/Qendolin/fabric-mod-bisect-tool/app/core/systemrunner"
	"github.com/Qendolin/fabric-mod-bisect-tool/app/logging"
)

// Searcher implements the Iterative Minimal Conflict Search (IMCS) algorithm.
type Searcher struct {
	// Dependencies
	modState *mods.StateManager

	// Static data
	initialCandidatesCount int
	allModIDs              []string

	// State
	history          *HistoryManager
	current          SearchSnapshot
	isComplete       bool
	lastFoundError   error
	testsExecuted    int
	problemsFound    int
	lastFoundElement string
}

// NewSearcher creates a new conflict searcher.
func NewSearcher(modState *mods.StateManager) *Searcher {
	return &Searcher{
		modState: modState,
		history:  NewHistoryManager(),
	}
}

// Start begins a new conflict search.
func (s *Searcher) Start(allModIDs []string) {
	logging.Info("IMCS: Starting new conflict search.")
	s.history.Clear()
	s.isComplete = false
	s.lastFoundError = nil
	s.testsExecuted = 0
	s.problemsFound = 0
	s.allModIDs = allModIDs

	modStatuses := s.modState.GetModStatusesSnapshot()
	background := make(map[string]struct{})
	initialCandidatesSlice := make([]string, 0, len(s.allModIDs))

	for _, id := range s.allModIDs {
		if status, ok := modStatuses[id]; ok {
			if status.ForceDisabled || status.ManuallyGood {
				continue
			}
			if status.ForceEnabled {
				background[id] = struct{}{}
			} else {
				initialCandidatesSlice = append(initialCandidatesSlice, id)
			}
		}
	}
	sort.Strings(initialCandidatesSlice)
	s.initialCandidatesCount = len(initialCandidatesSlice)

	s.current = SearchSnapshot{
		ConflictSet:    make(map[string]struct{}),
		Candidates:     initialCandidatesSlice,
		Background:     background,
		SearchStack:    make([]SearchStep, 0),
		LastTestResult: "",
	}
	s.startNextElementSearch()
	s.logNextStep()
}

// GetCurrentState returns the current search snapshot.
func (s *Searcher) GetCurrentState() SearchSnapshot { return s.current }

// IsComplete checks if the search has finished.
func (s *Searcher) IsComplete() bool { return s.isComplete }

// LastError returns the last error encountered.
func (s *Searcher) LastError() error { return s.lastFoundError }

// GetTestsExecuted returns the number of tests performed.
func (s *Searcher) GetTestsExecuted() int { return s.testsExecuted }

// GetEstimatedMaxTests provides an estimated upper bound for the total tests.
func (s *Searcher) GetEstimatedMaxTests() int {
	numCandidates := s.initialCandidatesCount
	problems := s.problemsFound
	if problems == 0 && !s.isComplete {
		problems = 1
	}
	if numCandidates == 0 || problems == 0 {
		return 0
	}
	return problems * (int(math.Ceil(math.Log2(float64(numCandidates)))) + 1)
}

// GetAllModIDs returns all mod IDs that were initially considered for the search.
func (s *Searcher) GetAllModIDs() []string { return s.allModIDs }

// LastFoundElement returns the problematic element of the current iteration. It is reset at the start of each iteration, and set when a single element is found, before verification.
func (s *Searcher) LastFoundElement() string {
	return s.lastFoundElement
}

func (s *Searcher) IsVerificationStep() bool {
	return s.current.IsVerifyingConflictSet
}

// CalculateNextTestSet calculates the set of mods for the next test run.
func (s *Searcher) CalculateNextTestSet() map[string]struct{} {
	if s.isComplete {
		return nil
	}

	if s.current.IsVerifyingConflictSet {
		// If verifying, the next test set is the current ConflictSet (plus background)
		return union(s.current.Background, s.current.ConflictSet)
	}

	if len(s.current.SearchStack) > 0 {
		step := s.current.SearchStack[len(s.current.SearchStack)-1]
		if len(step.Candidates) == 1 {
			// Base case of binary search
			return union(step.Background, sliceToSet(step.Candidates))
		} else {
			// Divide and conquer step: test the first half
			c1 := step.Candidates[:len(step.Candidates)/2]
			return union(step.Background, sliceToSet(c1))
		}
	}
	return nil
}

func (s *Searcher) NeedsTest() bool {
	if s.isComplete || s.lastFoundError != nil {
		return false
	}
	return s.current.IsVerifyingConflictSet || len(s.current.SearchStack) > 0
}

// ResumeWithResult provides the outcome of the last test and advances the search.
func (s *Searcher) ResumeWithResult(ctx context.Context, result systemrunner.Result) {
	if result != "" {
		s.testsExecuted++
	}
	s.current.LastTestResult = result
	s.history.Push(s.current)

	if s.current.IsVerifyingConflictSet {
		s.handleVerificationResult(result)
		return
	}
	s.handleFindNextResult(result)

	if !s.isComplete {
		s.logNextStep()
	}
}

func (s *Searcher) HandleExternalStateChange() {
	modStatuses := s.modState.GetModStatusesSnapshot()
	newBackground := make(map[string]struct{})
	for id := range s.current.Background {
		status, ok := modStatuses[id]
		if ok && !status.ForceDisabled && !status.ManuallyGood {
			newBackground[id] = struct{}{}
		}
	}
	for id, status := range modStatuses {
		if status.ForceEnabled {
			newBackground[id] = struct{}{}
		}
	}
	s.current.Background = newBackground

	var newCandidates []string
	for _, id := range s.current.Candidates {
		status, ok := modStatuses[id]
		if ok && !status.ForceDisabled && !status.ManuallyGood && !status.ForceEnabled {
			newCandidates = append(newCandidates, id)
		}
	}
	sort.Strings(newCandidates)
	s.current.Candidates = newCandidates

	var newStack []SearchStep
	for _, step := range s.current.SearchStack {
		var newStepCandidates []string
		for _, id := range step.Candidates {
			if status, ok := modStatuses[id]; ok && !status.ForceDisabled && !status.ManuallyGood && !status.ForceEnabled {
				newStepCandidates = append(newStepCandidates, id)
			}
		}
		if len(newStepCandidates) > 0 {
			newStack = append(newStack, SearchStep{
				Background: step.Background,
				Candidates: newStepCandidates,
			})
		}
	}
	s.current.SearchStack = newStack

	if len(s.current.SearchStack) == 0 && !s.current.IsVerifyingConflictSet {
		s.startNextElementSearch()
	}
}

// StepBack reverts to the previous state.
func (s *Searcher) StepBack() bool {
	snapshot, err := s.history.Pop()
	if err != nil {
		logging.Warnf("IMCS: Failed to step back: %v", err)
		return false
	}
	s.current = snapshot
	s.isComplete = false
	s.lastFoundError = nil
	if s.testsExecuted > 0 {
		s.testsExecuted--
	}
	if len(s.current.ConflictSet) < s.problemsFound {
		s.problemsFound = len(s.current.ConflictSet)
	}
	logging.Info("IMCS: Stepped back to previous search state.")
	return true
}

func (s *Searcher) logNextStep() {
	if s.isComplete || s.lastFoundError != nil {
		return
	}

	testSet := s.CalculateNextTestSet()
	if testSet == nil {
		return // No next test to log
	}

	currentPhase := "Unknown"
	if s.IsVerificationStep() {
		currentPhase = "ConflictSet Verification"
	} else if len(s.current.SearchStack) > 0 {
		step := s.current.SearchStack[len(s.current.SearchStack)-1]
		currentPhase = fmt.Sprintf("Binary Search (Candidates: %d)", len(step.Candidates))
	}

	logging.Infof("IMCS: Next Test (%d/%d): Phase: %s, Set: %v",
		s.testsExecuted+1,
		s.GetEstimatedMaxTests(),
		currentPhase,
		mapKeysFromStruct(testSet),
	)
}

// startNextElementSearch corresponds to the top of the main `FindConflictSet` loop.
func (s *Searcher) startNextElementSearch() {
	if len(s.current.Candidates) == 0 {
		s.isComplete = true
		logging.Infof("IMCS: Search complete. No more candidates to search.")
		return
	}

	s.lastFoundElement = ""
	searchContext := union(s.current.Background, s.current.ConflictSet)
	s.current.SearchStack = []SearchStep{newSearchStep(searchContext, s.current.Candidates)}
	logging.Infof("IMCS: Starting new element search (Iteration %d). Background: %v, Candidates: %v",
		s.problemsFound+1, mapKeysFromStruct(searchContext), s.current.Candidates)
}

// handleFindNextResult drives the state machine for the binary search.
func (s *Searcher) handleFindNextResult(result systemrunner.Result) {
	if len(s.current.SearchStack) == 0 {
		logging.Info("IMCS: Binary search for current element concluded without finding a culprit. Terminating overall search.")
		s.isComplete = true // Should not be reached if handled properly, but acts as a safeguard.
		return
	}
	step := s.current.SearchStack[len(s.current.SearchStack)-1]

	logging.Infof("IMCS: Processing binary search step #%d: Background=%v, Candidates=%v, Result=%s",
		s.testsExecuted, mapKeysFromStruct(step.Background), step.Candidates, result)

	if len(step.Candidates) == 1 {
		if result == systemrunner.FAIL {
			s.processFoundElement(step.Candidates[0])
		} else {
			// The entire binary search completed without finding a single new culprit.
			s.isComplete = true
			logging.Info("IMCS: Binary search concluded without finding a new conflict element. Search is complete.")
		}
		return
	}

	s.current.SearchStack = s.current.SearchStack[:len(s.current.SearchStack)-1]
	mid := len(step.Candidates) / 2
	c1, c2 := step.Candidates[:mid], step.Candidates[mid:]

	if result == systemrunner.FAIL {
		logging.Infof("IMCS: Test with left half '%v' FAILED. Descending into left half.", c1)
		s.current.SearchStack = append(s.current.SearchStack, newSearchStep(step.Background, c1))
	} else {
		logging.Infof("IMCS: Test with left half '%v' GOOD. Adding to background, descending into right half '%v'.", c1, c2)
		newBackground := union(step.Background, sliceToSet(c1))
		s.current.SearchStack = append(s.current.SearchStack, newSearchStep(newBackground, c2))
	}
}

// processFoundElement handles finding a new conflict element.
func (s *Searcher) processFoundElement(elementID string) {
	s.problemsFound++
	s.current.ConflictSet[elementID] = struct{}{}
	s.current.Candidates = removeFromStringSlice(s.current.Candidates, elementID)
	s.current.IsVerifyingConflictSet = true
	s.lastFoundElement = elementID
	logging.Infof("IMCS: ConflictSet updated to %v. Remaining Candidates: %v",
		mapKeysFromStruct(s.current.ConflictSet), s.current.Candidates)
}

// handleVerificationResult processes the result of the `test(ConflictSet)` optimization.
func (s *Searcher) handleVerificationResult(result systemrunner.Result) {
	s.current.IsVerifyingConflictSet = false
	if result == systemrunner.FAIL {
		logging.Info("IMCS: Current ConflictSet is sufficient to cause failure. Search complete.")
		s.isComplete = true
	} else {
		logging.Info("IMCS: Current ConflictSet is NOT sufficient. Continuing search for next element.")
		s.startNextElementSearch()
	}
}

// --- Helper functions ---
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

func sliceToSet(slice []string) map[string]struct{} {
	set := make(map[string]struct{}, len(slice))
	for _, item := range slice {
		set[item] = struct{}{}
	}
	return set
}

func removeFromStringSlice(slice []string, toRemove string) []string {
	for i, v := range slice {
		if v == toRemove {
			return append(slice[:i], slice[i+1:]...)
		}
	}
	return slice
}

func mapKeysFromStruct(m map[string]struct{}) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
