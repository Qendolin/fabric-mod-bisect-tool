package ui

import (
	"github.com/Qendolin/fabric-mod-bisect-tool/pkg/core/imcs"
	"github.com/Qendolin/fabric-mod-bisect-tool/pkg/core/mods"
	"github.com/Qendolin/fabric-mod-bisect-tool/pkg/core/sets"
)

type ModViewModel struct {
	BaseFilename string
	ID           string
	Name         string
	Version      string
	IsUnknown    bool
}

// ModStatusOverride is the user-editable, mutually exclusive state of a mod.
type ModStatusOverride string

const (
	ModOverrideNone          ModStatusOverride = "none"
	ModOverrideForceEnabled  ModStatusOverride = "force_enabled"
	ModOverrideForceDisabled ModStatusOverride = "force_disabled"
	ModOverrideOmitted       ModStatusOverride = "omitted"
)

// ModStatusViewModel is a serializable snapshot of a mod's runtime status.
type ModStatusViewModel struct {
	ModViewModel
	Override ModStatusOverride // User-editable, mutually exclusive state.

	// Readonly flags, set only by the engine. Not settable via the controller.
	IsMissing      bool
	IsProblematic  bool
	IsUnresolvable bool

	IsUserEditable bool // Derived: whether the user may change Override.
}

type SearchState string

const (
	StateNotReady     SearchState = "NotReady"     // Bisection hasn't started
	StateNoResultsYet SearchState = "NoResultsYet" // Running but no conflict isolated yet
	StateInProgress   SearchState = "InProgress"   // Active with partial/intermediate results
	StateComplete     SearchState = "Complete"     // Bisection process completely finished
)

// CascadingDisables captures the side-effects of removing a single mod.
type CascadingDisables struct {
	Mod                ModViewModel   // The target mod
	AlsoRequireDisable []ModViewModel // Other mods broken transitively by removing this mod
}

// ConflictSetReport details an isolated group of mutually incompatible mods.
type ConflictSetReport struct {
	Mods              []CascadingDisables // List of conflicting mods and their specific cascades
	IfAllDisabledAlso []ModViewModel      // Extra cascades that occur ONLY if the entire set is disabled
}

// UnresolvedDependencyReport captures pre-existing dependency errors unrelated to conflicts.
type UnresolvedDependencyReport struct {
	Mod                  ModViewModel   // The broken mod
	UnmetDependencies    []ModViewModel // Dependencies missing directly from the environment
	RequiredByTransitive []ModViewModel // Other mods that break downstream if this mod is disabled
}

// UnresolvableModAction is the per-mod decision on the unresolvable mods screen.
type UnresolvableModAction string

const (
	// UnresolvableModActionDisable keeps the mod disabled and out of the search.
	UnresolvableModActionDisable UnresolvableModAction = "disable"
	// UnresolvableModActionIgnore keeps the mod active by dropping its failing dependencies.
	UnresolvableModActionIgnore UnresolvableModAction = "ignore"
)

// UnresolvableModInfo describes a mod that cannot be activated because some of
// its dependencies failed to resolve, along with those failing dependencies.
type UnresolvableModInfo struct {
	Mod ModViewModel
	// DepsDisplay is a human-readable rendering of each failing dependency
	// including its version predicate, e.g. "nonexistent (>=1.0)", one entry per dependency.
	DepsDisplay []string
}

type ResultViewModel struct {
	State                 SearchState
	IsVerificationStep    bool                         // True if awaiting user confirmation test
	CurrentConflict       ConflictSetReport            // Growth track of current iteration's conflict
	PreviousConflictSets  []ConflictSetReport          // Isolated conflict groups from current/prior rounds
	GenerallyUnresolvable []UnresolvedDependencyReport // Environment dependency errors
	CanContinueSearch     bool                         // System evaluation for extra exploration
}

// BisectionViewModel provides a snapshot of the current bisection state,
// tailored for UI consumption. It decouples the UI from the underlying engine's implementation.
type BisectionViewModel struct {
	IsReady            bool
	IsComplete         bool
	IsVerificationStep bool
	IsHalted           bool
	StepCount          int
	Iteration          int
	Round              int
	EstimatedMaxTests  int
	LastTestResult     imcs.TestResult
	LastFoundElement   string
	AllModIDs          []string
	ModsInfo           map[string]ModViewModel
	AllConflictSets    []sets.Set
	CurrentConflictSet sets.Set
	CandidateSet       sets.Set
	ClearedSet         sets.Set
	PendingAdditions   sets.Set
	CurrentTestPlan    *imcs.TestPlan
	ExecutionLog       []imcs.CompletedTest
	// Loader is the mod loader the search was actually started with ("" before
	// loading begins). It may differ from PreferredLoader.
	Loader mods.RunLoader
	// PreferredLoader is the mod loader requested via the command line. It is a
	// preference, not necessarily the loader the search actually runs with.
	PreferredLoader mods.RunLoader
	CanUndo          bool
}
