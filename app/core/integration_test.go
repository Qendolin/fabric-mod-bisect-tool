package app_test

import (
	"archive/zip"
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/Qendolin/fabric-mod-bisect-tool/app/core/bisect"
	"github.com/Qendolin/fabric-mod-bisect-tool/app/core/imcs"
	"github.com/Qendolin/fabric-mod-bisect-tool/app/core/mods"
	"github.com/Qendolin/fabric-mod-bisect-tool/app/core/sets"
	"github.com/Qendolin/fabric-mod-bisect-tool/app/logging"
)

const testDir = "testdata"

// modSpec defines the structure for creating a dummy mod.
type modSpec struct {
	JSONContent string
	NestedJars  map[string]modSpec
}

// setupDummyMods creates a temporary mods directory and files.
func setupDummyMods(t *testing.T, modsDir string, specs map[string]modSpec) {
	t.Helper()
	if err := os.MkdirAll(modsDir, 0755); err != nil {
		t.Fatalf("failed to create mods dir '%s': %v", modsDir, err)
	}
	for filename, spec := range specs {
		jarPath := filepath.Join(modsDir, filename)
		jarBytes, err := createJarFromSpec(t, spec)
		if err != nil {
			t.Fatalf("failed to create JAR data for %s: %v", filename, err)
		}
		if err := os.WriteFile(jarPath, jarBytes, 0644); err != nil {
			t.Fatalf("failed to write dummy mod file %s: %v", jarPath, err)
		}
	}
}

// createJarFromSpec is a recursive helper to build a JAR file from a spec.
func createJarFromSpec(t *testing.T, spec modSpec) ([]byte, error) {
	t.Helper()
	zipBuf := new(bytes.Buffer)
	zipWriter := zip.NewWriter(zipBuf)
	if spec.JSONContent != "" {
		modJsonFile, err := zipWriter.Create("fabric.mod.json")
		if err != nil {
			return nil, err
		}
		if _, err = modJsonFile.Write([]byte(spec.JSONContent)); err != nil {
			return nil, err
		}
	}
	for nestedFilename, nestedSpec := range spec.NestedJars {
		nestedJarBytes, err := createJarFromSpec(t, nestedSpec)
		if err != nil {
			return nil, err
		}
		nestedJarFile, err := zipWriter.Create(nestedFilename)
		if err != nil {
			return nil, err
		}
		if _, err := nestedJarFile.Write(nestedJarBytes); err != nil {
			return nil, err
		}
	}
	if err := zipWriter.Close(); err != nil {
		return nil, err
	}
	return zipBuf.Bytes(), nil
}

// runBisectionTest is a test harness that executes the full bisection process.
func runBisectionTest(t *testing.T, svc *bisect.Service, allMods map[string]*mods.Mod, problematicSet sets.Set) sets.Set {
	t.Helper()
	svc.StartNewSearch()

	for testCount := 0; !svc.Engine().GetCurrentState().IsComplete; testCount++ {
		if testCount > 100 {
			t.Fatalf("Exceeded test count limit (100)")
		}

		plan, err := svc.Engine().PlanNextTest()
		if err != nil {
			t.Logf("Planning finished: %v", err)
			break
		}

		effectiveSet, _ := svc.StateManager().ResolveEffectiveSet(plan.ModIDsToTest)

		// CORRECTED: Build a set of all IDs provided by the effective set of mods.
		allProvidedIDsByEffectiveSet := sets.Copy(effectiveSet)
		for modID := range effectiveSet {
			if mod, ok := allMods[modID]; ok {
				for providedID := range mod.EffectiveProvides {
					allProvidedIDsByEffectiveSet[providedID] = struct{}{}
				}
			}
		}

		// CORRECTED: The test fails if the set of all provided IDs is a superset of the problematic set.
		isFailure := len(problematicSet) > 0 && len(sets.Subtract(problematicSet, allProvidedIDsByEffectiveSet)) == 0

		result := imcs.TestResultGood
		if isFailure {
			result = imcs.TestResultFail
		}

		t.Logf("Step %d: Testing %v -> Effective %v -> Result: %s", testCount+1, sets.MakeSlice(plan.ModIDsToTest), sets.MakeSlice(effectiveSet), result)
		svc.Engine().SubmitTestResult(result)
	}

	return svc.Engine().GetCurrentState().ConflictSet
}

// TestBisectService_Integration runs a full integration test of the bisect service.
func TestBisectService_Integration(t *testing.T) {
	mainLogger := logging.NewLogger()
	mainLogger.SetDebug(true)
	logFile, err := os.OpenFile(filepath.Join(testDir, "test.log"), os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0666)
	if err != nil {
		t.Fatalf("Failed to open log file: %v", err)
	}
	defer logFile.Close()
	mainLogger.SetWriter(logFile)
	logging.SetDefault(mainLogger)

	baseModSpecs := make(map[string]modSpec)
	for i := 0; i < 26; i++ {
		char := string(rune('a' + i))
		modID := fmt.Sprintf("mod_%s", char)
		filename := fmt.Sprintf("mod-%s-1.0.jar", char)
		baseModSpecs[filename] = modSpec{JSONContent: fmt.Sprintf(`{"id": "%s", "version": "1.0"}`, modID)}
	}

	testCases := []struct {
		name                string
		modSpecs            map[string]modSpec
		problematicSet      sets.Set
		stateManagerSetup   func(state *mods.StateManager)
		expectedConflictSet sets.Set
	}{
		{
			name:                "1-Mod Independent Conflict",
			modSpecs:            baseModSpecs,
			problematicSet:      sets.MakeSet([]string{"mod_m"}),
			expectedConflictSet: sets.MakeSet([]string{"mod_m"}),
		},
		{
			name:                "2-Mod Independent Conflict",
			modSpecs:            baseModSpecs,
			problematicSet:      sets.MakeSet([]string{"mod_b", "mod_y"}),
			expectedConflictSet: sets.MakeSet([]string{"mod_b", "mod_y"}),
		},
		{
			name: "2-Mod Dependent Conflict",
			modSpecs: func() map[string]modSpec {
				specs := deepCopySpecs(baseModSpecs)
				specs["mod-c-1.0.jar"] = modSpec{JSONContent: `{"id": "mod_c", "depends": {"mod_j": ">=1.0"}}`}
				return specs
			}(),
			problematicSet:      sets.MakeSet([]string{"mod_j"}),
			expectedConflictSet: sets.MakeSet([]string{"mod_c"}), // CORRECTED: The trigger is mod_c.
		},
		{
			name: "Dependency Provider Conflict",
			modSpecs: func() map[string]modSpec {
				specs := deepCopySpecs(baseModSpecs)
				specs["mod-a-1.0.jar"] = modSpec{JSONContent: `{"id": "mod_a", "depends": {"api": "1.0"}}`}
				specs["mod-b-1.0.jar"] = modSpec{JSONContent: `{"id": "mod_b", "provides": ["api"]}`}
				return specs
			}(),
			problematicSet:      sets.MakeSet([]string{"mod_b"}),
			expectedConflictSet: sets.MakeSet([]string{"mod_a"}), // CORRECTED: The trigger is mod_a.
		},
		{
			name: "Nested JAR Conflict",
			modSpecs: func() map[string]modSpec {
				specs := deepCopySpecs(baseModSpecs)
				specs["mod-a-1.0.jar"] = modSpec{
					JSONContent: `{"id": "mod_a", "jars": [{"file": "libs/nested.jar"}]}`,
					NestedJars: map[string]modSpec{
						"libs/nested.jar": {JSONContent: `{"id": "nested_b", "version": "1.0"}`},
					},
				}
				return specs
			}(),
			problematicSet:      sets.MakeSet([]string{"nested_b"}),
			expectedConflictSet: sets.MakeSet([]string{"mod_a"}),
		},
		{
			name:                "No Conflict",
			modSpecs:            baseModSpecs,
			problematicSet:      sets.MakeSet([]string{}),
			expectedConflictSet: sets.MakeSet([]string{}),
		},
		{
			name: "Unresolvable Dependency Prevents Conflict",
			modSpecs: func() map[string]modSpec {
				specs := deepCopySpecs(baseModSpecs)
				specs["mod-x-1.0.jar"] = modSpec{JSONContent: `{"id": "mod_x", "depends": {"non_existent": "1.0"}}`}
				return specs
			}(),
			problematicSet:      sets.MakeSet([]string{"mod_x"}),
			expectedConflictSet: sets.MakeSet([]string{}),
		},
		{
			name:           "Forced Disabled Conflict Is Ignored",
			modSpecs:       baseModSpecs,
			problematicSet: sets.MakeSet([]string{"mod_c"}),
			stateManagerSetup: func(state *mods.StateManager) {
				state.SetForceDisabled("mod_c", true)
			},
			expectedConflictSet: sets.MakeSet([]string{}),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			modsDir := filepath.Join(testDir, strings.ReplaceAll(tc.name, " ", "_"))
			if err := os.RemoveAll(modsDir); err != nil && !os.IsNotExist(err) {
				t.Fatalf("failed to clean mods dir: %v", err)
			}
			setupDummyMods(t, modsDir, tc.modSpecs)
			defer os.RemoveAll(modsDir)

			loader := mods.NewModLoaderService()
			allMods, providers, _, err := loader.LoadMods(modsDir, nil, nil)
			if err != nil {
				t.Fatalf("LoadMods failed: %v", err)
			}

			stateMgr := mods.NewStateManager(allMods, providers)
			activator := mods.NewModActivator(modsDir, allMods)
			engine := imcs.NewEngine(stateMgr.GetAllModIDs())

			if tc.stateManagerSetup != nil {
				tc.stateManagerSetup(stateMgr)
			}

			svc, err := bisect.NewService(stateMgr, activator, engine)
			if err != nil {
				t.Fatalf("NewService failed: %v", err)
			}

			finalConflictSet := runBisectionTest(t, svc, allMods, tc.problematicSet)

			if !reflect.DeepEqual(finalConflictSet, tc.expectedConflictSet) {
				t.Errorf("Test case '%s' failed.\nExpected: %v\nGot:      %v", tc.name, sets.MakeSlice(tc.expectedConflictSet), sets.MakeSlice(finalConflictSet))
			}
		})
	}
}

func deepCopySpecs(original map[string]modSpec) map[string]modSpec {
	cpy := make(map[string]modSpec)
	for k, v := range original {
		cpy[k] = v // This is a shallow copy of the inner map, but sufficient for these tests
	}
	return cpy
}
