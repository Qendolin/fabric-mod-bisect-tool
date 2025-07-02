package app

import (
	"fmt"
	"strings"

	"github.com/Qendolin/fabric-mod-bisect-tool/app/core/mods"
	"github.com/Qendolin/fabric-mod-bisect-tool/app/core/sets"
	"github.com/Qendolin/fabric-mod-bisect-tool/app/ui"
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

	for i, conflictSet := range allFoundSets {
		builder.WriteString(fmt.Sprintf("\n--- Conflict Set #%d ---\n", i+1))
		for _, id := range sets.MakeSlice(conflictSet) {
			modInfo := ""
			if mod, ok := allMods[id]; ok {
				modInfo = fmt.Sprintf("(%s %s) from '%s.jar'", mod.FriendlyName(), mod.FabricInfo.Version, mod.BaseFilename)
			}
			builder.WriteString(fmt.Sprintf("  - %s %s\n", id, modInfo))
		}

		unresolvable := stateMgr.Resolver().CalculateTransitivelyUnresolvableMods(sets.Subtract(allModsSet, conflictSet))
		if len(unresolvable) > 0 {
			builder.WriteString("    └ Dependent mods that may also need disabling:\n")
			for _, modID := range sets.MakeSlice(unresolvable) {
				builder.WriteString(fmt.Sprintf("      - %s\n", modID))
			}
		}
	}

	return builder.String()
}
