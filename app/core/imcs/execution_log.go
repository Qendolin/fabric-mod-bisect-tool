package imcs

import "github.com/Qendolin/fabric-mod-bisect-tool/app/core/sets"

// TestPlan is an immutable object representing a single, well-defined test.
type TestPlan struct {
	ModIDsToTest       sets.Set
	IsVerificationStep bool
}

// CompletedTest is a record of a test that was planned and executed.
type CompletedTest struct {
	Plan            TestPlan
	Result          TestResult
	StateBeforeTest SearchState
}

// ExecutionLog records a linear history of all completed tests for display.
type ExecutionLog struct {
	entries []CompletedTest
}

// NewExecutionLog creates a new, empty log.
func NewExecutionLog() *ExecutionLog {
	return &ExecutionLog{
		entries: make([]CompletedTest, 0),
	}
}

// Append appends another execution log to the current one.
func (el *ExecutionLog) Append(other *ExecutionLog) {
	if other == nil {
		return
	}
	el.entries = append(el.entries, other.entries...)
}

// Log adds a new completed test to the log.
func (el *ExecutionLog) Log(test CompletedTest) {
	el.entries = append(el.entries, test)
}

// GetEntries returns a copy of all recorded entries.
func (el *ExecutionLog) GetEntries() []CompletedTest {
	entriesCopy := make([]CompletedTest, len(el.entries))
	copy(entriesCopy, el.entries)
	return entriesCopy
}

// Clear resets the log.
func (el *ExecutionLog) Clear() {
	el.entries = make([]CompletedTest, 0)
}

// Size returns the number of entires
func (el *ExecutionLog) Size() int {
	return len(el.entries)
}

// GetLastTest returns the most recently completed test, if one exists.
func (el *ExecutionLog) GetLastTest() (CompletedTest, bool) {
	if len(el.entries) == 0 {
		return CompletedTest{}, false
	}
	return el.entries[len(el.entries)-1], true
}
