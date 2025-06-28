package conflict

import "errors"

var errHistoryEmpty = errors.New("history is empty")

// UndoStack (renamed from HistoryManager) provides an undo/redo-style history
// for search states.
type UndoStack struct {
	states []SearchState // Changed from SearchSnapshot to SearchState
}

// NewUndoStack creates a new undo stack.
func NewUndoStack() *UndoStack {
	return &UndoStack{
		states: make([]SearchState, 0),
	}
}

// Push adds a new state to the undo stack. It performs a deep copy.
func (s *UndoStack) Push(state SearchState) {
	// Perform a deep copy to ensure the pushed state is immutable.
	copiedState := deepCopyState(state)
	s.states = append(s.states, copiedState)
}

// Pop removes and returns the most recent state from the history.
func (s *UndoStack) Pop() (SearchState, error) {
	if len(s.states) == 0 {
		return SearchState{}, errHistoryEmpty
	}
	lastIndex := len(s.states) - 1
	snapshot := s.states[lastIndex]
	s.states = s.states[:lastIndex]
	return snapshot, nil
}

// Clear removes all states from the history.
func (s *UndoStack) Clear() {
	s.states = make([]SearchState, 0)
}

// Size returns the number of states in the history.
func (s *UndoStack) Size() int {
	return len(s.states)
}

// deepCopyState creates a new SearchState with all maps and slices copied.
func deepCopyState(state SearchState) SearchState {
	// This function contains the logic you had in your original Push method.
	copiedConflictSet := make(map[string]struct{}, len(state.ConflictSet))
	for k := range state.ConflictSet {
		copiedConflictSet[k] = struct{}{}
	}

	copiedCandidates := make([]string, len(state.Candidates))
	copy(copiedCandidates, state.Candidates)

	copiedBackground := make(map[string]struct{}, len(state.Background))
	for k := range state.Background {
		copiedBackground[k] = struct{}{}
	}

	copiedSearchStack := make([]SearchStep, len(state.SearchStack))
	for i, step := range state.SearchStack {
		copiedStepBackground := make(map[string]struct{}, len(step.Background))
		for k := range step.Background {
			copiedStepBackground[k] = struct{}{}
		}
		copiedStepCandidates := make([]string, len(step.Candidates))
		copy(copiedStepCandidates, step.Candidates)

		copiedSearchStack[i] = SearchStep{
			Background: copiedStepBackground,
			Candidates: copiedStepCandidates,
		}
	}

	return SearchState{
		ConflictSet:            copiedConflictSet,
		Candidates:             copiedCandidates,
		Background:             copiedBackground,
		SearchStack:            copiedSearchStack,
		IsVerifyingConflictSet: state.IsVerifyingConflictSet,
		AllModIDs:              state.AllModIDs, // This can be a shallow copy as it's immutable reference data
		IsComplete:             state.IsComplete,
		LastFoundElement:       state.LastFoundElement,
		LastTestResult:         state.LastTestResult,
	}
}
