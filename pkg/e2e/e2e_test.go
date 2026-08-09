package e2e

import (
	"archive/zip"
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Qendolin/fabric-mod-bisect-tool/pkg/app"
	"github.com/Qendolin/fabric-mod-bisect-tool/pkg/core/bisect"
	"github.com/Qendolin/fabric-mod-bisect-tool/pkg/core/imcs"
	"github.com/Qendolin/fabric-mod-bisect-tool/pkg/core/sets"
	"github.com/Qendolin/fabric-mod-bisect-tool/pkg/logging"
	"github.com/Qendolin/fabric-mod-bisect-tool/pkg/ui"
)

// modSpec defines the structure for creating a dummy mod.
type modSpec struct {
	JSONContent string
	NestedJars  map[string]modSpec
	RawFiles    map[string]string
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
	for path, content := range spec.RawFiles {
		rawFile, err := zipWriter.Create(path)
		if err != nil {
			return nil, err
		}
		if _, err = rawFile.Write([]byte(content)); err != nil {
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

const timeout = 30 * time.Second

// newTestApp creates an app with a fresh MockView and starts loading mods from a
// temp directory populated with the given specs. It returns the app, the mock,
// and the temp mods directory.
func newTestApp(t *testing.T, specs map[string]modSpec) (*app.App, *MockView, string) {
	t.Helper()
	modsDir := t.TempDir()
	setupDummyMods(t, modsDir, specs)

	mainLogger := logging.NewLogger()
	cliArgs := &app.CLIArgs{NoEmbeddedOverrides: true}
	a := app.NewApp(mainLogger, cliArgs)
	mock := NewMockView()
	a.SetView(mock)
	a.StartLoadingProcess(modsDir, false, false)
	return a, mock, modsDir
}

// newLoadedApp creates an app, starts loading, and waits until loading has
// completed successfully (OnBisectionReady fired).
func newLoadedApp(t *testing.T, specs map[string]modSpec) (*app.App, *MockView, string) {
	t.Helper()
	a, mock, modsDir := newTestApp(t, specs)
	mock.WaitReady(t, timeout)
	return a, mock, modsDir
}

// TestLoadAndBisectionReady asserts the loading lifecycle: OnLoadingStarted,
// progress callbacks, OnBisectionReady, and a populated view model.
func TestLoadAndBisectionReady(t *testing.T) {
	specs := map[string]modSpec{
		"mod-a-1.0.jar": {JSONContent: `{"id": "mod_a", "version": "1.0"}`},
		"mod-b-1.0.jar": {JSONContent: `{"id": "mod_b", "version": "1.0"}`},
		"mod-c-1.0.jar": {JSONContent: `{"id": "mod_c", "version": "1.0"}`},
	}
	a, mock, _ := newLoadedApp(t, specs)

	calls := mock.Calls()
	if len(calls) == 0 || calls[0] != "OnLoadingStarted" {
		t.Errorf("expected OnLoadingStarted as the first call, got: %v", calls)
	}
	if !mock.HasCall("OnLoadingProgress") {
		t.Error("expected at least one OnLoadingProgress call")
	}
	if !mock.HasCall("OnBisectionReady") {
		t.Error("expected OnBisectionReady")
	}

	vm := a.GetViewModel()
	if !vm.IsReady {
		t.Error("expected IsReady to be true")
	}
	if len(vm.AllModIDs) != 3 {
		t.Errorf("expected 3 mods, got %v", vm.AllModIDs)
	}
	for _, id := range []string{"mod_a", "mod_b", "mod_c"} {
		if _, ok := vm.ModsInfo[id]; !ok {
			t.Errorf("missing mod info for %s", id)
		}
	}
	if vm.ForceQuiltSupport || vm.ForceNeoForgeSupport {
		t.Error("expected Quilt/NeoForge support flags to be false")
	}
}

// TestDisabledModsReportRightAfterLoading asserts behavior (3): the "Disabled
// Mods" report dialog is shown right after loading (during onLoadingComplete's
// reconcile), not on the first step.
func TestDisabledModsReportRightAfterLoading(t *testing.T) {
	specs := map[string]modSpec{
		"mod-a-1.0.jar": {JSONContent: `{"id": "mod_a", "version": "1.0"}`},
		"mod-b-1.0.jar": {JSONContent: `{"id": "mod_b", "version": "1.0"}`},
		// mod_c has an unresolvable dependency, so reconciliation must flag it.
		"mod-c-1.0.jar": {JSONContent: `{"id": "mod_c", "version": "1.0", "depends": {"nonexistent": ">=1.0"}}`},
	}
	a, mock, _ := newTestApp(t, specs)

	// The dialog must arrive without any Step having been invoked.
	inv := mock.WaitDialog(t, timeout)
	if inv.Kind != DialogInfoBisectionUnresolvableModsDisabled {
		t.Fatalf("expected Disabled Mods report dialog, got %s", inv.Kind)
	}
	if _, ok := inv.DisabledMods["mod_c"]; !ok {
		t.Errorf("expected mod_c in disabled set, got %v", sets.MakeSlice(inv.DisabledMods))
	}
	if mock.HasCall("OnTestReady") {
		t.Error("dialog should have been shown before any Step (no OnTestReady yet)")
	}
	inv.Respond(true)

	mock.WaitReady(t, timeout)

	statuses := a.GetModStatuses()
	if !statuses["mod_c"].IsUnresolvable {
		t.Error("expected mod_c to be marked unresolvable")
	}
	vm := a.GetViewModel()
	if _, inCandidates := vm.CandidateSet["mod_c"]; inCandidates {
		t.Error("expected mod_c to be excluded from candidates")
	}
}

// TestStepSubmitUndoResetLifecycle drives the step/test/undo/reset lifecycle
// and asserts the always-update invariant (Update is called after each op).
func TestStepSubmitUndoResetLifecycle(t *testing.T) {
	specs := map[string]modSpec{
		"mod-a-1.0.jar": {JSONContent: `{"id": "mod_a", "version": "1.0"}`},
		"mod-b-1.0.jar": {JSONContent: `{"id": "mod_b", "version": "1.0"}`},
		"mod-c-1.0.jar": {JSONContent: `{"id": "mod_c", "version": "1.0"}`},
	}
	a, mock, _ := newLoadedApp(t, specs)

	// Step
	before := mock.UpdateCount()
	a.Step()
	if !mock.HasCall("OnTestReady") {
		t.Fatal("expected OnTestReady after Step")
	}
	if mock.UpdateCount() <= before {
		t.Error("Step must end with view.Update()")
	}
	if plan := a.GetViewModel().CurrentTestPlan; plan == nil {
		t.Error("expected an active test plan after Step")
	}

	// Submit a test result (search proceeds)
	before = mock.UpdateCount()
	a.SubmitTestResult(imcs.TestResultGood)
	if mock.UpdateCount() <= before {
		t.Error("SubmitTestResult must end with view.Update()")
	}

	// Undo
	before = mock.UpdateCount()
	if err := a.Undo(); err != nil {
		t.Fatalf("Undo failed: %v", err)
	}
	if mock.UpdateCount() <= before {
		t.Error("Undo must end with view.Update()")
	}

	// Undo again: stack empty, error returned but no crash
	if err := a.Undo(); !errors.Is(err, bisect.ErrUndoStackEmpty) {
		t.Errorf("expected ErrUndoStackEmpty, got %v", err)
	}

	// Reset
	before = mock.UpdateCount()
	a.ResetSearch()
	if mock.UpdateCount() <= before {
		t.Error("ResetSearch must end with view.Update()")
	}
	vm := a.GetViewModel()
	if vm.IsComplete {
		t.Error("expected search not complete after reset")
	}
}

// TestMissingFilesReconcileAndRestep asserts behavior (4): when a mod file goes
// missing during a step, the question dialog is shown; accepting it marks the
// mod missing, reconciles, and re-steps.
func TestMissingFilesReconcileAndRestep(t *testing.T) {
	specs := map[string]modSpec{
		"mod-a-1.0.jar": {JSONContent: `{"id": "mod_a", "version": "1.0"}`},
		"mod-b-1.0.jar": {JSONContent: `{"id": "mod_b", "version": "1.0"}`},
		"mod-c-1.0.jar": {JSONContent: `{"id": "mod_c", "version": "1.0"}`},
	}
	a, mock, modsDir := newLoadedApp(t, specs)

	// Remove a mod file that is expected to be active during the next step.
	if err := os.Remove(filepath.Join(modsDir, "mod-a-1.0.jar")); err != nil {
		t.Fatalf("failed to remove mod file: %v", err)
	}

	// Step is blocking from the caller's side: run it on a goroutine and answer
	// the dialog from the test goroutine.
	done := make(chan struct{})
	go func() {
		defer close(done)
		defer logging.HandlePanic()
		a.Step()
	}()

	inv := mock.WaitDialog(t, timeout)
	if inv.Kind != DialogQuestionBisectionContinueWithMissingMods {
		t.Fatalf("expected question dialog about missing mods, got %s", inv.Kind)
	}
	if _, ok := inv.MissingMods["mod_a"]; !ok {
		t.Errorf("expected mod_a in missing set, got %v", sets.MakeSlice(inv.MissingMods))
	}
	inv.Respond(true)

	select {
	case <-done:
	case <-time.After(timeout):
		t.Fatal("Step did not complete after responding to the dialog")
	}

	statuses := a.GetModStatuses()
	if !statuses["mod_a"].IsMissing {
		t.Error("expected mod_a to be marked missing after accepting the dialog")
	}
	if !mock.HasCall("OnTestReady") {
		t.Error("expected a re-step (OnTestReady) after reconciliation")
	}
}

// TestContinueSearchAfterComplete verifies that ContinueSearch reconciles
// explicitly and advances to a new round after the search completes.
func TestContinueSearchAfterComplete(t *testing.T) {
	specs := map[string]modSpec{
		"mod-a-1.0.jar": {JSONContent: `{"id": "mod_a", "version": "1.0"}`},
		"mod-b-1.0.jar": {JSONContent: `{"id": "mod_b", "version": "1.0"}`},
		"mod-c-1.0.jar": {JSONContent: `{"id": "mod_c", "version": "1.0"}`},
	}
	a, mock, _ := newLoadedApp(t, specs)

	// Run the bisection to completion (all results good => no conflict).
	for i := 0; i < 100 && !a.GetViewModel().IsComplete; i++ {
		a.Step()
		if a.GetViewModel().IsComplete {
			break
		}
		a.SubmitTestResult(imcs.TestResultGood)
	}
	if !a.GetViewModel().IsComplete {
		t.Fatal("search did not complete")
	}
	if rvm := a.GetResultViewModel(); rvm.State != ui.StateComplete {
		t.Errorf("expected StateComplete, got %s", rvm.State)
	}

	before := mock.UpdateCount()
	a.ContinueSearch()
	if mock.UpdateCount() <= before {
		t.Error("ContinueSearch must end with view.Update()")
	}
	if round := a.GetViewModel().Round; round != 2 {
		t.Errorf("expected Round 2 after ContinueSearch, got %d", round)
	}
}

// TestContinueSearchNotCompleteErrors verifies the error dialog when
// ContinueSearch is invoked before the search is complete.
func TestContinueSearchNotCompleteErrors(t *testing.T) {
	specs := map[string]modSpec{
		"mod-a-1.0.jar": {JSONContent: `{"id": "mod_a", "version": "1.0"}`},
		"mod-b-1.0.jar": {JSONContent: `{"id": "mod_b", "version": "1.0"}`},
	}
	a, mock, _ := newLoadedApp(t, specs)

	done := make(chan struct{})
	go func() {
		defer close(done)
		defer logging.HandlePanic()
		a.ContinueSearch()
	}()

	inv := mock.WaitDialog(t, timeout)
	if inv.Kind != DialogErrorBisectionCannotContinue {
		t.Fatalf("expected 'cannot continue' error dialog, got %s", inv.Kind)
	}
	inv.Respond(true)

	select {
	case <-done:
	case <-time.After(timeout):
		t.Fatal("ContinueSearch did not complete after responding")
	}
}

// TestCommitAndDiscardOverrides verifies staged overrides are applied atomically
// on Commit, trigger a reconciliation, and can be discarded.
func TestCommitAndDiscardOverrides(t *testing.T) {
	specs := map[string]modSpec{
		"mod-a-1.0.jar": {JSONContent: `{"id": "mod_a", "version": "1.0"}`},
		"mod-b-1.0.jar": {JSONContent: `{"id": "mod_b", "version": "1.0"}`},
		"mod-c-1.0.jar": {JSONContent: `{"id": "mod_c", "version": "1.0"}`},
	}
	a, mock, _ := newLoadedApp(t, specs)

	ctrl := a.GetModStatusController()

	// Stage then discard: nothing applied.
	ctrl.SetOverride("mod_a", ui.ModOverrideForceDisabled)
	ctrl.Discard()
	if st := a.GetModStatuses()["mod_a"]; st.Override != ui.ModOverrideNone {
		t.Errorf("expected Override None after Discard, got %s", st.Override)
	}

	// Stage then commit.
	ctrl.SetOverride("mod_b", ui.ModOverrideForceDisabled)
	before := mock.UpdateCount()
	ctrl.Commit()
	if mock.UpdateCount() <= before {
		t.Error("Commit must end with view.Update()")
	}
	if st := a.GetModStatuses()["mod_b"]; st.Override != ui.ModOverrideForceDisabled {
		t.Errorf("expected ForceDisabled override after Commit, got %s", st.Override)
	}
	vm := a.GetViewModel()
	if _, inCandidates := vm.CandidateSet["mod_b"]; inCandidates {
		t.Error("expected mod_b to be removed from candidates after force-disabling")
	}
}

// TestLoadErrors covers the loading error dialogs.
func TestLoadErrors(t *testing.T) {
	t.Run("no mods", func(t *testing.T) {
		a, mock, _ := newTestApp(t, map[string]modSpec{})
		inv := mock.WaitDialog(t, timeout)
		if inv.Kind != DialogErrorModLoadingNoMods {
			t.Fatalf("expected no-mods dialog, got %s", inv.Kind)
		}
		inv.Respond(true)
		if a.IsBisectionReady() {
			t.Error("bisection should not be ready after a load error")
		}
	})

	t.Run("nonexistent path", func(t *testing.T) {
		modsDir := filepath.Join(t.TempDir(), "does-not-exist")
		mainLogger := logging.NewLogger()
		cliArgs := &app.CLIArgs{NoEmbeddedOverrides: true}
		a := app.NewApp(mainLogger, cliArgs)
		mock := NewMockView()
		a.SetView(mock)
		a.StartLoadingProcess(modsDir, false, false)

		inv := mock.WaitDialog(t, timeout)
		if inv.Kind != DialogErrorModLoadingGeneric {
			t.Fatalf("expected generic load error dialog, got %s", inv.Kind)
		}
		inv.Respond(true)
		if a.IsBisectionReady() {
			t.Error("bisection should not be ready after a load error")
		}
	})
}

// TestGetResultViewModelNotReady asserts the view model reports not-ready before
// any mods are loaded.
func TestGetResultViewModelNotReady(t *testing.T) {
	mainLogger := logging.NewLogger()
	cliArgs := &app.CLIArgs{NoEmbeddedOverrides: true}
	a := app.NewApp(mainLogger, cliArgs)
	mock := NewMockView()
	a.SetView(mock)

	if rvm := a.GetResultViewModel(); rvm.State != ui.StateNotReady {
		t.Errorf("expected StateNotReady before loading, got %s", rvm.State)
	}
	if vm := a.GetViewModel(); vm.IsReady {
		t.Error("expected IsReady false before loading")
	}
}

// TestFailDrivenBisectionVerification drives the search with failing results so
// a conflict element is isolated, a verification step is reached, and the search
// completes, asserting the result view model.
func TestFailDrivenBisectionVerification(t *testing.T) {
	specs := map[string]modSpec{
		"mod-a-1.0.jar": {JSONContent: `{"id": "mod_a", "version": "1.0"}`},
		"mod-b-1.0.jar": {JSONContent: `{"id": "mod_b", "version": "1.0"}`},
		"mod-c-1.0.jar": {JSONContent: `{"id": "mod_c", "version": "1.0"}`},
	}
	a, mock, _ := newLoadedApp(t, specs)

	// First bisection step: tests the first half of the candidates.
	a.Step()
	a.SubmitTestResult(imcs.TestResultFail)

	// Second step narrows to a single candidate; this isolates mod_a.
	a.Step()
	a.SubmitTestResult(imcs.TestResultFail)

	// Now the engine should be verifying the isolated conflict set.
	a.Step()
	if !a.GetViewModel().IsVerificationStep {
		t.Fatal("expected the third plan to be a verification step")
	}
	if cs := a.GetViewModel().CurrentConflictSet; len(cs) != 1 {
		t.Fatalf("expected a single conflict element, got %v", sets.MakeSlice(cs))
	}
	a.SubmitTestResult(imcs.TestResultFail)

	if !mock.HasCall("OnIterationComplete") {
		t.Error("expected OnIterationComplete after completion")
	}
	rvm := a.GetResultViewModel()
	if rvm.State != ui.StateComplete {
		t.Errorf("expected StateComplete, got %s", rvm.State)
	}
	if len(rvm.CurrentConflict.Mods) == 0 || rvm.CurrentConflict.Mods[0].Mod.ID != "mod_a" {
		t.Errorf("expected CurrentConflict to contain mod_a, got %+v", rvm.CurrentConflict)
	}
}

// TestCancelTest verifies that CancelTest invalidates the active plan (so the
// next Step can re-plan instead of failing with ErrTestInProgress) and updates
// the view.
func TestCancelTest(t *testing.T) {
	specs := map[string]modSpec{
		"mod-a-1.0.jar": {JSONContent: `{"id": "mod_a", "version": "1.0"}`},
		"mod-b-1.0.jar": {JSONContent: `{"id": "mod_b", "version": "1.0"}`},
	}
	a, mock, _ := newLoadedApp(t, specs)

	a.Step()
	if a.GetViewModel().CurrentTestPlan == nil {
		t.Fatal("expected an active plan before CancelTest")
	}

	before := mock.UpdateCount()
	a.CancelTest()
	if mock.UpdateCount() <= before {
		t.Error("CancelTest must end with view.Update()")
	}

	// If the plan were not invalidated, the next Step would hit
	// ErrTestInProgress and show a prepare-error dialog. It must re-plan.
	before = mock.UpdateCount()
	a.Step()
	if mock.UpdateCount() <= before {
		t.Error("Step after CancelTest must end with view.Update()")
	}
	if a.GetViewModel().CurrentTestPlan == nil {
		t.Error("expected a fresh plan after Step following CancelTest")
	}
}

// TestMissingFilesExpectedDeletions asserts the info dialog branch of the
// missing-files flow: a mod in a known conflict set that goes missing triggers
// ShowDialogInfoBisectionModsMissingExpected (not the question dialog).
func TestMissingFilesExpectedDeletions(t *testing.T) {
	specs := map[string]modSpec{
		"mod-a-1.0.jar": {JSONContent: `{"id": "mod_a", "version": "1.0"}`},
		"mod-b-1.0.jar": {JSONContent: `{"id": "mod_b", "version": "1.0"}`},
		"mod-c-1.0.jar": {JSONContent: `{"id": "mod_c", "version": "1.0"}`},
	}
	a, mock, modsDir := newLoadedApp(t, specs)

	// Isolate mod_a as a conflict element (same as the fail-driven test).
	a.Step()
	a.SubmitTestResult(imcs.TestResultFail)
	a.Step()
	a.SubmitTestResult(imcs.TestResultFail)
	if _, ok := a.GetViewModel().CurrentConflictSet["mod_a"]; !ok {
		t.Fatal("expected mod_a to be in the conflict set")
	}

	// Delete the enabled file of the known-problematic mod.
	if err := os.Remove(filepath.Join(modsDir, "mod-a-1.0.jar")); err != nil {
		t.Fatalf("failed to remove mod file: %v", err)
	}

	done := make(chan struct{})
	go func() {
		defer close(done)
		defer logging.HandlePanic()
		a.Step()
	}()

	inv := mock.WaitDialog(t, timeout)
	if inv.Kind != DialogInfoBisectionModsMissingExpected {
		t.Fatalf("expected 'expected missing mods' info dialog, got %s", inv.Kind)
	}
	if _, ok := inv.MissingMods["mod_a"]; !ok {
		t.Errorf("expected mod_a in missing set, got %v", sets.MakeSlice(inv.MissingMods))
	}
	inv.Respond(true)

	select {
	case <-done:
	case <-time.After(timeout):
		t.Fatal("Step did not complete after responding to the dialog")
	}

	if st := a.GetModStatuses()["mod_a"]; !st.IsMissing {
		t.Error("expected mod_a to be marked missing")
	}
}

// TestRestoreInitialModState asserts that RestoreInitialModState re-enables all
// mod files on disk after steps disabled some of them.
func TestRestoreInitialModState(t *testing.T) {
	specs := map[string]modSpec{
		"mod-a-1.0.jar": {JSONContent: `{"id": "mod_a", "version": "1.0"}`},
		"mod-b-1.0.jar": {JSONContent: `{"id": "mod_b", "version": "1.0"}`},
		"mod-c-1.0.jar": {JSONContent: `{"id": "mod_c", "version": "1.0"}`},
	}
	a, mock, modsDir := newLoadedApp(t, specs)

	// A step activates a subset of mods, disabling the rest on disk.
	a.Step()
	a.SubmitTestResult(imcs.TestResultGood)

	assertDisabledFiles := func(t *testing.T, want bool) {
		t.Helper()
		entries, err := os.ReadDir(modsDir)
		if err != nil {
			t.Fatalf("failed to read mods dir: %v", err)
		}
		hasDisabled := false
		for _, e := range entries {
			if strings.HasSuffix(e.Name(), ".jar.disabled") {
				hasDisabled = true
			}
		}
		if want && !hasDisabled {
			t.Error("expected at least one disabled mod file before restore")
		}
		if !want && hasDisabled {
			t.Error("expected no disabled mod files after restore")
		}
	}

	assertDisabledFiles(t, true)
	a.RestoreInitialModState()
	assertDisabledFiles(t, false)
	_ = mock
}

// TestResolveEffectiveSetWithDependencies asserts ResolveEffectiveSet pulls in
// transitive dependencies of the requested mods.
func TestResolveEffectiveSetWithDependencies(t *testing.T) {
	specs := map[string]modSpec{
		"mod-a-1.0.jar": {JSONContent: `{"id": "mod_a", "version": "1.0"}`},
		"mod-b-1.0.jar": {JSONContent: `{"id": "mod_b", "version": "1.0", "depends": {"mod_a": ">=1.0"}}`},
	}
	a, _, _ := newLoadedApp(t, specs)

	effective := a.GetModStatusController().ResolveEffectiveSet(sets.MakeSet([]string{"mod_b"}))
	for _, id := range []string{"mod_a", "mod_b"} {
		if _, ok := effective[id]; !ok {
			t.Errorf("expected %s in effective set, got %v", id, sets.MakeSlice(effective))
		}
	}
}
