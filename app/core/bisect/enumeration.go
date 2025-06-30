package bisect

import (
	"strings"

	"github.com/Qendolin/fabric-mod-bisect-tool/app/core/imcs"
	"github.com/Qendolin/fabric-mod-bisect-tool/app/core/sets"
)

// KnowledgeBase stores the results of test runs to avoid re-running them.
// It acts as a simple, passive cache.
type KnowledgeBase struct {
	cache map[string]imcs.TestResult
}

func NewKnowledgeBase() *KnowledgeBase {
	return &KnowledgeBase{cache: make(map[string]imcs.TestResult)}
}

// Get retrieves a cached result for a given set.
func (kb *KnowledgeBase) Get(set sets.Set) (imcs.TestResult, bool) {
	key := kb.generateKey(set)
	result, found := kb.cache[key]
	return result, found
}

// Store caches the result for a given set.
func (kb *KnowledgeBase) Store(set sets.Set, result imcs.TestResult) {
	key := kb.generateKey(set)
	kb.cache[key] = result
}

// Add this helper to enumeration.go
func (kb *KnowledgeBase) Remove(set sets.Set) {
	key := kb.generateKey(set)
	delete(kb.cache, key)
}

// generateKey creates a deterministic, sorted key from a set.
func (kb *KnowledgeBase) generateKey(set sets.Set) string {
	return strings.Join(sets.MakeSlice(set), ",")
}

// Enumeration holds the state for the entire process of finding multiple independent conflict sets.
type Enumeration struct {
	KnowledgeBase        *KnowledgeBase
	FoundConflictSets    []sets.Set
	MasterCandidateSet   sets.Set
	ArchivedExecutionLog *imcs.ExecutionLog
}

// NewEnumeration creates and initializes the state for a new enumeration process.
func NewEnumeration(allItems []string) *Enumeration {
	return &Enumeration{
		KnowledgeBase:        NewKnowledgeBase(),
		FoundConflictSets:    make([]sets.Set, 0),
		MasterCandidateSet:   sets.MakeSet(allItems),
		ArchivedExecutionLog: imcs.NewExecutionLog(),
	}
}

// AddFoundConflictSet records a newly found conflict set and removes its
// members from the master candidate pool for future searches.
func (e *Enumeration) AddFoundConflictSet(conflictSet sets.Set) {
	if len(conflictSet) == 0 {
		return
	}
	e.FoundConflictSets = append(e.FoundConflictSets, conflictSet)
	e.MasterCandidateSet = sets.Subtract(e.MasterCandidateSet, conflictSet)
}

// AppendLog archives the history from a completed bisection run.
func (e *Enumeration) AppendLog(log *imcs.ExecutionLog) {
	e.ArchivedExecutionLog.Append(log)
}
