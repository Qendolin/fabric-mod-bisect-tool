package app_test

import (
	"archive/zip"
	"bytes"
	"context"
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
	// Suppress logging to console during tests by default.
	if err := logging.Init("test.log"); err != nil {
		t.Fatalf("Failed to init logging: %v", err)
	}
	defer logging.Close()

	// Define a larger pool of mods (a-z) for better test coverage
	baseModSpecs := make(map[string]string)
	var allBaseModIDs []string
	for i := 0; i < 26; i++ {
		char := string(rune('a' + i))
		modID := fmt.Sprintf("mod_%s", char)
		filename := fmt.Sprintf("mod-%s-1.0.jar", char)
		baseModSpecs[filename] = fmt.Sprintf(`{"id": "%s", "version": "1.0", "name": "Mod %s"}`, modID, char)
		allBaseModIDs = append(allBaseModIDs, modID)
	}
	sort.Strings(allBaseModIDs) // Ensure sorted order for all mod IDs

	testCases := []struct {
		name                string
		modSpecsOverride    map[string]string // Specific mod definitions that override base
		initialModIDs       []string          // If different from allBaseModIDs
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
				// Override mod_a to depend on mod_c
				specs["mod-a-1.0.jar"] = `{"id": "mod_a", "version": "1.0", "name": "Mod A", "depends": {"mod_c": ">=1.0"}}`
				return specs
			}(),
			initialModIDs:       allBaseModIDs,
			problematicSet:      map[string]struct{}{"mod_a": {}, "mod_c": {}},
			expectedConflictSet: map[string]struct{}{"mod_a": {}}, // 'a' is the minimal trigger
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
				// Define mod_x with an unresolvable dependency
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
			// 1. Setup
			modsDir := filepath.Join(testDir, tc.name)
			_ = setupDummyMods(t, modsDir, tc.modSpecsOverride) // Ignore returned IDs; use initialModIDs
			defer os.RemoveAll(modsDir)

			// 2. Initialize Core Services
			modLoader := mods.NewModLoaderService()
			allMods, potentialProviders, sortedModIDs, err := modLoader.LoadMods(modsDir, nil)
			if err != nil {
				t.Fatalf("LoadMods failed: %v", err)
			}
			if !reflect.DeepEqual(sortedModIDs, tc.initialModIDs) {
				t.Fatalf("Initial mod IDs mismatch. Expected %v, got %v", tc.initialModIDs, sortedModIDs)
			}

			modState := mods.NewStateManager(allMods, potentialProviders)
			resolver := mods.NewDependencyResolver()
			searcher := conflict.NewSearcher(modState)

			modState.OnStateChanged = searcher.HandleExternalStateChange

			searcher.Start(sortedModIDs)

			// 3. Test Loop
			testCount := 0
			for !searcher.IsComplete() && searcher.LastError() == nil {
				testCount++
				t.Logf("\n--- STEP %d ---", testCount)
				s := searcher.GetCurrentState()
				t.Logf("  Current ConflictSet: %v", mapKeys(s.ConflictSet))
				t.Logf("  Remaining Candidates: %v", s.Candidates)

				if !searcher.NeedsTest() {
					t.Log("  No more tests needed. Searcher concluded.")
					break
				}
				testSet := searcher.CalculateNextTestSet()
				statuses := modState.GetModStatusesSnapshot()

				t.Logf("  Preparing test with targets: %v", mapKeys(testSet))

				// Resolve dependencies for the test set
				effectiveSet, _ := resolver.ResolveEffectiveSet(
					systemrunner.SetToSlice(testSet),
					modState.GetAllMods(),
					modState.GetPotentialProviders(),
					statuses,
				)

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

				t.Logf("  > Test Result: %s", result)

				searcher.ResumeWithResult(context.Background(), result)
			}

			if err := searcher.LastError(); err != nil {
				t.Fatalf("Searcher finished with error: %v", err)
			}

			// 4. Assert Results
			t.Logf("\n--- TEST COMPLETE ---")
			finalConflictSet := searcher.GetCurrentState().ConflictSet
			t.Logf("  Final ConflictSet found: %v", mapKeys(finalConflictSet))

			if !reflect.DeepEqual(finalConflictSet, tc.expectedConflictSet) {
				// Convert to sorted slices for readable error message
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
