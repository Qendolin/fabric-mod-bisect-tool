package app_test

import (
	"archive/zip"
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"testing"

	"github.com/Qendolin/fabric-mod-bisect-tool/app/core/conflict"
	"github.com/Qendolin/fabric-mod-bisect-tool/app/core/mods"
	"github.com/Qendolin/fabric-mod-bisect-tool/app/core/systemrunner"
	"github.com/Qendolin/fabric-mod-bisect-tool/app/logging"
	"github.com/titanous/json5"
)

const testDir = "testdata"

// setupDummyMods is a test helper to create a temporary mods directory and files.
func setupDummyMods(t *testing.T, modsDir string, modSpecs map[string]string) []string {
	t.Helper()
	if err := os.RemoveAll(modsDir); err != nil && !os.IsNotExist(err) {
		t.Fatalf("failed to clean mods dir '%s': %v", modsDir, err)
	}
	if err := os.MkdirAll(modsDir, 0755); err != nil {
		t.Fatalf("failed to create mods dir '%s': %v", modsDir, err)
	}

	var createdModIDs []string
	for filename, jsonContent := range modSpecs {
		jarPath := filepath.Join(modsDir, filename)
		zipBuf := new(bytes.Buffer)
		zipWriter := zip.NewWriter(zipBuf)

		// Add fabric.mod.json
		modJsonFile, err := zipWriter.Create("fabric.mod.json")
		if err != nil {
			t.Fatalf("failed to create fabric.mod.json entry for %s: %v", filename, err)
		}
		_, err = modJsonFile.Write([]byte(jsonContent))
		if err != nil {
			t.Fatalf("failed to write fabric.mod.json content for %s: %v", filename, err)
		}
		// Add a dummy class file to make it a valid JAR
		dummyClassFile, err := zipWriter.Create("com/example/Main.class")
		if err != nil {
			t.Fatalf("failed to create dummy class entry for %s: %v", filename, err)
		}
		_, err = dummyClassFile.Write([]byte{0xCA, 0xFE, 0xBA, 0xBE})
		if err != nil {
			t.Fatalf("failed to write dummy class content for %s: %v", filename, err)
		}

		if err := zipWriter.Close(); err != nil {
			t.Fatalf("failed to close zip writer for %s: %v", filename, err)
		}
		err = os.WriteFile(jarPath, zipBuf.Bytes(), 0644)
		if err != nil {
			t.Fatalf("failed to write dummy mod file %s: %v", jarPath, err)
		}

		var fmj mods.FabricModJson
		if err := json5.Unmarshal([]byte(jsonContent), &fmj); err != nil {
			t.Fatalf("failed to unmarshal JSON for ID extraction from %s: %v", filename, err)
		}
		createdModIDs = append(createdModIDs, fmj.ID)
	}
	sort.Strings(createdModIDs)
	return createdModIDs
}

// Helper function for readable set logging
func mapKeys(m map[string]struct{}) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// TestIMCS_Integration runs a full integration test of the core IMCS logic.
func TestIMCS_Integration(t *testing.T) {
	mainLogger := logging.NewLogger()
	mainLogger.SetDebug(true)
	logFile, err := os.OpenFile(filepath.Join(testDir, "test.log"), os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0666)
	if err != nil {
		os.Stderr.WriteString("Failed to open log file: " + err.Error())
		os.Exit(1)
	}
	defer logFile.Close()
	mainLogger.SetWriter(logFile)
	logging.SetDefault(mainLogger)

	baseModSpecs := make(map[string]string)
	var allBaseModIDs []string
	for i := 0; i < 26; i++ {
		char := string(rune('a' + i))
		modID := fmt.Sprintf("mod_%s", char)
		filename := fmt.Sprintf("mod-%s-1.0.jar", char)
		baseModSpecs[filename] = fmt.Sprintf(`{"id": "%s", "version": "1.0", "name": "Mod %s"}`, modID, char)
		allBaseModIDs = append(allBaseModIDs, modID)
	}
	sort.Strings(allBaseModIDs)

	testCases := []struct {
		name                string
		modSpecsOverride    map[string]string
		initialModIDs       []string
		problematicSet      map[string]struct{}
		expectedConflictSet map[string]struct{}
	}{
		{
			name:                "1-Mod Independent Conflict",
			modSpecsOverride:    baseModSpecs,
			initialModIDs:       allBaseModIDs,
			problematicSet:      map[string]struct{}{"mod_m": {}},
			expectedConflictSet: map[string]struct{}{"mod_m": {}},
		},
		{
			name: "2-Mod Dependent Conflict",
			modSpecsOverride: func() map[string]string {
				specs := make(map[string]string)
				for k, v := range baseModSpecs {
					specs[k] = v
				}
				specs["mod-a-1.0.jar"] = `{"id": "mod_a", "version": "1.0", "name": "Mod A", "depends": {"mod_c": ">=1.0"}}`
				return specs
			}(),
			initialModIDs:       allBaseModIDs,
			problematicSet:      map[string]struct{}{"mod_a": {}, "mod_c": {}},
			expectedConflictSet: map[string]struct{}{"mod_a": {}},
		},
		{
			name:                "2-Mod Independent Conflict",
			modSpecsOverride:    baseModSpecs,
			initialModIDs:       allBaseModIDs,
			problematicSet:      map[string]struct{}{"mod_b": {}, "mod_y": {}},
			expectedConflictSet: map[string]struct{}{"mod_b": {}, "mod_y": {}},
		},
		{
			name:                "3-Mod Independent Conflict",
			modSpecsOverride:    baseModSpecs,
			initialModIDs:       allBaseModIDs,
			problematicSet:      map[string]struct{}{"mod_d": {}, "mod_e": {}, "mod_f": {}},
			expectedConflictSet: map[string]struct{}{"mod_d": {}, "mod_e": {}, "mod_f": {}},
		},
		{
			name:                "No Conflict",
			modSpecsOverride:    baseModSpecs,
			initialModIDs:       allBaseModIDs,
			problematicSet:      map[string]struct{}{},
			expectedConflictSet: map[string]struct{}{},
		},
		{
			name: "No Conflict - Unmet Dependency",
			modSpecsOverride: func() map[string]string {
				specs := make(map[string]string)
				for k, v := range baseModSpecs {
					specs[k] = v
				}
				specs["mod-x-1.0.jar"] = `{"id": "mod_x", "version": "1.0", "name": "Mod X", "depends": {"non_existent_dep": "*"}}`
				return specs
			}(),
			initialModIDs:       allBaseModIDs,
			problematicSet:      map[string]struct{}{"mod_x": {}}, // If mod_x were active, it would cause a problem
			expectedConflictSet: map[string]struct{}{},            // But it can never be active, so no conflict is found

		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// 1. Setup - Unchanged
			modsDir := filepath.Join(testDir, tc.name)
			_ = setupDummyMods(t, modsDir, tc.modSpecsOverride)
			defer os.RemoveAll(modsDir)

			// 2. Initialize Core Services - Unchanged
			modLoader := mods.NewModLoaderService()
			allMods, potentialProviders, sortedModIDs, err := modLoader.LoadMods(modsDir, nil, nil)
			if err != nil {
				t.Fatalf("LoadMods failed: %v", err)
			}
			if !reflect.DeepEqual(sortedModIDs, tc.initialModIDs) {
				t.Fatalf("Initial mod IDs mismatch. Expected %v, got %v", tc.initialModIDs, sortedModIDs)
			}

			modState := mods.NewStateManager(allMods, potentialProviders)
			resolver := mods.NewDependencyResolver()

			searchProcess := conflict.NewSearchProcess(modState)
			searchProcess.StartNewSearch()

			// 3. Test Loop - Adapted for the new SearchProcess API
			testCount := 0
			for !searchProcess.GetCurrentState().IsComplete {
				testCount++
				t.Logf("\n--- STEP %d ---", testCount)
				if testCount > 100 {
					t.Fatalf("Exceeded test count limit (100)")
				}
				s := searchProcess.GetCurrentState()
				t.Logf("  Current ConflictSet: %v", mapKeys(s.ConflictSet))
				t.Logf("  Remaining Candidates: %v", s.Candidates)

				logging.Debugf("  [Harness] SearchProcess state at START of step: Candidates=%d, Background=%d, Stack=%d",
					len(s.Candidates), len(s.Background), len(s.SearchStack))

				// Get the plan for the next test. This is now a state-changing action.
				plan, err := searchProcess.PlanNextTest()
				if err != nil {
					t.Logf("  Planning finished: %v", err)
					break
				}
				logging.Debugf("  [Harness] SearchProcess returned a plan with %d mods to test.", len(plan.ModIDsToTest))
				testSet := plan.ModIDsToTest
				statuses := modState.GetModStatusesSnapshot()

				t.Logf("  Preparing test with targets: %v", mapKeys(testSet))

				// Resolve dependencies for the test set
				effectiveSet, _ := resolver.ResolveEffectiveSet(
					systemrunner.SetToSlice(testSet),
					modState.GetAllMods(),
					modState.GetPotentialProviders(),
					statuses,
				)

				logging.Debugf("  [Harness] Resolver returned an effectiveSet of %d mods.", len(effectiveSet))

				t.Logf("  > Effective mod set for test: %v", mapKeys(effectiveSet))

				// Simulate the test run
				isConflict := true
				if len(tc.problematicSet) == 0 {
					isConflict = false
				}
				for pMod := range tc.problematicSet {
					if _, ok := effectiveSet[pMod]; !ok {
						isConflict = false
						break
					}
				}

				result := systemrunner.GOOD
				if isConflict {
					result = systemrunner.FAIL
				}

				logging.Debugf("  [Harness] Simulating result: %s. Submitting this result to SearchProcess...", result)

				logging.Debugf("  [Harness Check] Simulating test. Problematic set: %v. Effective set: %v. Is Conflict? %t. Sending result: %s",
					mapKeys(tc.problematicSet), mapKeys(effectiveSet), isConflict, result)

				t.Logf("  > Test Result: %s", result)

				// Submit the result for the plan that was just run.
				if err := searchProcess.SubmitTestResult(result); err != nil {
					t.Fatalf("SubmitTestResult failed: %v", err)
				}

				finalStateInLoop := searchProcess.GetCurrentState()
				logging.Debugf("  [Harness] SearchProcess state at END of step: Candidates=%d, Background=%d, Stack=%d",
					len(finalStateInLoop.Candidates), len(finalStateInLoop.Background), len(finalStateInLoop.SearchStack))
			}

			// 4. Assert Results - Adapted for the new SearchProcess API
			t.Logf("\n--- TEST COMPLETE ---")
			finalState := searchProcess.GetCurrentState()
			finalConflictSet := finalState.ConflictSet
			t.Logf("  Final ConflictSet found: %v", mapKeys(finalConflictSet))

			if !reflect.DeepEqual(finalConflictSet, tc.expectedConflictSet) {
				var found, expected []string
				for k := range finalConflictSet {
					found = append(found, k)
				}
				for k := range tc.expectedConflictSet {
					expected = append(expected, k)
				}
				sort.Strings(found)
				sort.Strings(expected)

				t.Errorf("Test case '%s' failed.\nExpected conflict set %v,\nbut got %v", tc.name, expected, found)
			} else {
				t.Logf("Test case '%s' succeeded. Found %v as expected.", tc.name, mapKeys(finalConflictSet))
			}
		})
	}
}
