package app

import (
	"fmt"
	"sort"
	"strings"

	"github.com/Qendolin/fabric-mod-bisect-tool/pkg/core/mods"
	"github.com/Qendolin/fabric-mod-bisect-tool/pkg/core/sets"
	"github.com/Qendolin/fabric-mod-bisect-tool/pkg/ui"
)

// GenerateLogReport creates a plain-text summary of the entire bisection process,
// including the detailed execution history and final conflict sets, suitable for logging.
func GenerateLogReport(vm ui.BisectionViewModel, stateMgr *mods.StateManager) string {
	var builder strings.Builder

	// --- Section 1: Detailed Execution History ---
	builder.WriteString("===== Bisection History (Execution Log) =====\n")
	if len(vm.ExecutionLog) > 0 {
		for i, entry := range vm.ExecutionLog {
			// Extract all necessary context from the state *before* the test was run.
			round := entry.StateBeforeTest.Round
			iteration := entry.StateBeforeTest.Iteration
			step := entry.StateBeforeTest.Step
			isVerification := entry.Plan.IsVerificationStep
			numModsInTest := len(entry.Plan.ModIDsToTest)
			result := entry.Result

			// Build the detailed log entry.
			var verificationTag string
			if isVerification {
				verificationTag = " [VERIFICATION]"
			}

			builder.WriteString(fmt.Sprintf(
				"#%-3d: Step %-3d | Round %d, Iter %d | Test(%d mods)%s -> %s\n",
				i+1,
				step,
				round,
				iteration,
				numModsInTest,
				verificationTag,
				result,
			))
		}
	} else {
		builder.WriteString("No tests were executed.\n")
	}

	// --- Section 2: Final Conflict Sets ---
	builder.WriteString("\n===== Final Conflict Sets =====\n")
	allFoundSets := vm.AllConflictSets
	if vm.IsComplete && len(vm.CurrentConflictSet) > 0 {
		allFoundSets = append(allFoundSets, vm.CurrentConflictSet)
	}

	if len(allFoundSets) == 0 {
		builder.WriteString("Bisection completed. No problematic mods were found.\n")
		return builder.String()
	}

	builder.WriteString(fmt.Sprintf("Found %d independent conflict set(s).\n", len(allFoundSets)))
	allMods := stateMgr.GetAllMods()
	allModsSet := sets.MakeSet(stateMgr.GetAllModIDs())

	// Calculate globally unresolvable mods (for reference)
	generallyUnresolvable := stateMgr.Resolver().CalculateTransitivelyUnresolvableMods(allModsSet)

	for i, conflictSet := range allFoundSets {
		builder.WriteString(fmt.Sprintf("\n--- Conflict Set #%d ---\n", i+1))
		for _, id := range sets.MakeSlice(conflictSet) {
			modInfo := ""
			if mod, ok := allMods[id]; ok {
				modInfo = fmt.Sprintf("(%s %s) from '%s.jar'", mod.FriendlyName(), mod.Metadata.Version, mod.BaseFilename)
			}
			builder.WriteString(fmt.Sprintf("  - %s %s\n", id, modInfo))
		}

		// Calculate unresolvable mods when conflictSet is disabled
		unresolvableDueToConflict := stateMgr.Resolver().CalculateTransitivelyUnresolvableMods(sets.Subtract(allModsSet, conflictSet))

		// Find only the ones that become unresolvable due to this specific conflict
		conflictSpecificUnresolvable := sets.Subtract(unresolvableDueToConflict, generallyUnresolvable)

		if len(conflictSpecificUnresolvable) > 0 {
			builder.WriteString("    └ Disabling this set would also require disabling:\n")
			for _, modID := range sets.MakeSlice(conflictSpecificUnresolvable) {
				if dep, depOk := allMods[modID]; depOk {
					builder.WriteString(fmt.Sprintf("      - %s from '%s.jar'\n", modID, dep.BaseFilename))
				} else {
					builder.WriteString(fmt.Sprintf("      - %s from unknown\n", modID))
				}
			}
		}
	}

	// Generally Unresolvable Mods
	details := stateMgr.Resolver().CalculateUnresolvableModsDetails(allModsSet)

	if len(details.DirectlyUnresolvable) > 0 {
		builder.WriteString("\n--- Mods with unresolved dependencies (may need manual review) ---\n")

		// Invert the transitive mapping to easily look up "Which mods does this root cause break?"
		causedByRoot := make(map[string]sets.Set)
		for transitiveID, roots := range details.TransitivelyUnresolvable {
			for rootID := range roots {
				if _, ok := causedByRoot[rootID]; !ok {
					causedByRoot[rootID] = sets.Set{}
				}
				causedByRoot[rootID][transitiveID] = struct{}{}
			}
		}

		// Sort top-level directly unresolvable mods for clean deterministic output
		topLevelSlice := make([]string, 0, len(details.DirectlyUnresolvable))
		for modID := range details.DirectlyUnresolvable {
			topLevelSlice = append(topLevelSlice, modID)
		}
		sort.Strings(topLevelSlice)

		for _, modID := range topLevelSlice {
			if mod, ok := allMods[modID]; ok {
				builder.WriteString(fmt.Sprintf("- %s (%s) from '%s.jar'\n", modID, mod.FriendlyName(), mod.BaseFilename))

				// 1. Display directly missing dependencies
				if failedDeps := details.DirectlyUnresolvable[modID]; len(failedDeps) > 0 {
					builder.WriteString("    └ Unresolved or unmet dependencies:\n")
					sort.Strings(failedDeps)
					for _, depID := range failedDeps {
						if providerMod, providerOk := allMods[depID]; providerOk {
							builder.WriteString(fmt.Sprintf("      - %s from '%s.jar'\n", depID, providerMod.BaseFilename))
						} else {
							builder.WriteString(fmt.Sprintf("      - %s from unknown\n", depID))
						}
					}
				}

				// 2. Display the transitively broken mods dependent on this root mod
				if caused, ok := causedByRoot[modID]; ok && len(caused) > 0 {
					builder.WriteString("    └ Disabling this mod would also require disabling:\n")
					causedSlice := sets.MakeSlice(caused)
					sort.Strings(causedSlice)
					for _, depID := range causedSlice {
						if depMod, depOk := allMods[depID]; depOk {
							builder.WriteString(fmt.Sprintf(fmt.Sprintf("      - %s from '%s.jar'\n", depID, depMod.BaseFilename)))
						} else {
							builder.WriteString(fmt.Sprintf(fmt.Sprintf("      - %s from unknown\n", depID)))
						}
					}
				}
			}
		}
	}

	return builder.String()
}
