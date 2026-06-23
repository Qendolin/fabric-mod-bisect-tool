package ui

import (
	"github.com/Qendolin/fabric-mod-bisect-tool/pkg/core/imcs"
	"github.com/Qendolin/fabric-mod-bisect-tool/pkg/core/sets"
)

// BisectionViewModel provides a snapshot of the current bisection state,
// tailored for UI consumption. It decouples the UI from the underlying engine's implementation.
type BisectionViewModel struct {
	IsReady            bool
	IsComplete         bool
	IsVerificationStep bool
	StepCount          int
	Iteration          int
	Round              int
	EstimatedMaxTests  int
	LastTestResult     imcs.TestResult
	LastFoundElement   string
	AllModIDs          []string
	AllConflictSets    []sets.Set
	CurrentConflictSet sets.Set
	CandidateSet       sets.Set
	ClearedSet         sets.Set
	PendingAdditions   sets.Set
	CurrentTestPlan    *imcs.TestPlan
	ExecutionLog       []imcs.CompletedTest
	QuiltSupport       bool
	NeoForgeSupport    bool
	CanUndo            bool
}
