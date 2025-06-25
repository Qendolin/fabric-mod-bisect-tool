package conflict

import "errors"

var errHistoryEmpty = errors.New("history is empty")

// HistoryManager provides an undo/redo-style history for search states.
type HistoryManager struct {
	states []SearchSnapshot
}

// NewHistoryManager creates a new history manager.
func NewHistoryManager() *HistoryManager {
	return &HistoryManager{
		states: make([]SearchSnapshot, 0),
	}
}

// Push adds a new state to the history.
func (h *HistoryManager) Push(snapshot SearchSnapshot) {
	// Defensive copy of maps/slices to ensure the snapshot is truly immutable from future changes.
	// This is critical for reliable history.
	copiedConflictSet := make(map[string]struct{}, len(snapshot.ConflictSet))
	for k := range snapshot.ConflictSet {
		copiedConflictSet[k] = struct{}{}
	}

	copiedCandidates := make([]string, len(snapshot.Candidates))
	copy(copiedCandidates, snapshot.Candidates)

	copiedBackground := make(map[string]struct{}, len(snapshot.Background))
	for k := range snapshot.Background {
		copiedBackground[k] = struct{}{}
	}

	copiedSearchStack := make([]SearchStep, len(snapshot.SearchStack))
	for i, step := range snapshot.SearchStack {
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

	h.states = append(h.states, SearchSnapshot{
		ConflictSet:            copiedConflictSet,
		Candidates:             copiedCandidates,
		Background:             copiedBackground,
		SearchStack:            copiedSearchStack,
		IsVerifyingConflictSet: snapshot.IsVerifyingConflictSet,
	})
}

// Pop removes and returns the most recent state from the history.
func (h *HistoryManager) Pop() (SearchSnapshot, error) {
	if len(h.states) == 0 {
		return SearchSnapshot{}, errHistoryEmpty
	}
	lastIndex := len(h.states) - 1
	snapshot := h.states[lastIndex]
	h.states = h.states[:lastIndex]
	return snapshot, nil
}

// Clear removes all states from the history.
func (h *HistoryManager) Clear() {
	h.states = make([]SearchSnapshot, 0)
}

// Size returns the number of states in the history.
func (h *HistoryManager) Size() int {
	return len(h.states)
}
