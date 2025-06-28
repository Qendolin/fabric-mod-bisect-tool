package systemrunner

import (
	"context"
	"fmt"
	"sort"

	"github.com/Qendolin/fabric-mod-bisect-tool/app/core/mods"
	"github.com/Qendolin/fabric-mod-bisect-tool/app/logging"
)

// Result indicates the outcome of a test run.
type Result string

const (
	// FAIL indicates the test exhibited the undesirable outcome.
	FAIL Result = "FAIL"
	// GOOD indicates the test ran successfully without the undesirable outcome.
	GOOD Result = "GOOD"
	// This is the initial value
	UNDEFINED Result = ""
)

// Executor is a function that performs the actual system test.
// It receives the effective set of mods that would be active, and returns the test result and any error.
type Executor func(ctx context.Context, effectiveMods map[string]struct{}) (Result, error)

// Runner orchestrates a single test run.
type Runner struct {
	activator *ModActivator
	resolver  *mods.DependencyResolver
	executor  Executor

	// Static data, safe for concurrent reads
	allMods            map[string]*mods.Mod
	potentialProviders mods.PotentialProvidersMap
}

// NewRunner creates a new test runner.
func NewRunner(
	activator *ModActivator,
	resolver *mods.DependencyResolver,
	executor Executor,
	allMods map[string]*mods.Mod,
	potentialProviders mods.PotentialProvidersMap,
) *Runner {
	return &Runner{
		activator:          activator,
		resolver:           resolver,
		executor:           executor,
		allMods:            allMods,
		potentialProviders: potentialProviders,
	}
}

// RunTest performs a complete test cycle for a given set of mods.
// It takes the set of target mods (those to be considered active for the test)
// and a snapshot of all mods' current statuses (force-enabled/disabled, manually-good).
// It returns the test result and any error encountered during the process.
func (r *Runner) RunTest(
	ctx context.Context,
	targetModIDsForTest map[string]struct{}, // Mods to be actively included in the test's dependency resolution.
	modStatuses map[string]mods.ModStatus, // Comprehensive snapshot of all mod states.
) (Result, error) {

	// 1. Resolve dependencies to get the full effective set of mods that should be active.
	effectiveModsToActivate, _ := r.resolver.ResolveEffectiveSet(
		SetToSlice(targetModIDsForTest), // Convert target set to slice for resolver
		r.allMods,
		r.potentialProviders,
		modStatuses,
	)

	logging.Infof("Runner: Resolved test set includes: %v", mapKeysFromStruct(effectiveModsToActivate))

	// 2. Apply file system changes.
	changes, err := r.activator.Apply(effectiveModsToActivate)
	if err != nil {
		return "", fmt.Errorf("failed to apply mod state: %w", err)
	}
	// Ensure state is reverted, regardless of test outcome.
	// This defer ensures cleanup even if the executor fails or panics.
	defer r.activator.Revert(changes)

	// 3. Execute the actual test, passing the effective set of mods.
	result, err := r.executor(ctx, effectiveModsToActivate) // Pass effectiveModsToActivate
	if err != nil {
		return "", fmt.Errorf("test executor failed: %w", err)
	}

	return result, nil
}

// setToSlice converts a map[string]struct{} (acting as a set) to a slice of strings.
func SetToSlice(s map[string]struct{}) []string {
	keys := make([]string, 0, len(s))
	for k := range s {
		keys = append(keys, k)
	}
	return keys
}

// mapKeysFromStruct returns a sorted slice of keys from a map[string]struct{} for consistent logging.
func mapKeysFromStruct(m map[string]struct{}) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
