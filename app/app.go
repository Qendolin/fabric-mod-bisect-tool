package app

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/Qendolin/fabric-mod-bisect-tool/app/core/bisect"
	"github.com/Qendolin/fabric-mod-bisect-tool/app/core/imcs"
	"github.com/Qendolin/fabric-mod-bisect-tool/app/core/mods"
	"github.com/Qendolin/fabric-mod-bisect-tool/app/embeds"
	"github.com/Qendolin/fabric-mod-bisect-tool/app/logging"
	"github.com/Qendolin/fabric-mod-bisect-tool/app/ui"
	"github.com/Qendolin/fabric-mod-bisect-tool/app/ui/pages"
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

// App orchestrates the TUI application, managing the lifecycle and core services.
type App struct {
	*tview.Application
	layoutManager *ui.LayoutManager
	navManager    *ui.NavigationManager
	dialogManager *ui.DialogManager
	logger        *logging.Logger
	focusManager  *ui.FocusManager

	// Core Service (only initialized after successful loading)
	bisectSvc *bisect.Service

	// Pages
	setupPage      *pages.SetupPage
	mainPage       *pages.MainPage
	logPage        *pages.LogPage
	loadingPage    *pages.LoadingPage
	manageModsPage *pages.ManageModsPage
	historyPage    *pages.HistoryPage

	appCtx    context.Context
	cancelApp context.CancelFunc

	shutdownWg sync.WaitGroup

	cliArgs CLIArgs
}

// NewApp creates and initializes the TUI application.
func NewApp(logger *logging.Logger, cliArgs *CLIArgs) *App {
	appCtx, cancelApp := context.WithCancel(context.Background())

	a := &App{
		Application: tview.NewApplication(),
		appCtx:      appCtx,
		cancelApp:   cancelApp,
		logger:      logger,
		cliArgs:     *cliArgs,
	}

	a.layoutManager = ui.NewLayoutManager()
	a.navManager = ui.NewNavigationManager(a, a.layoutManager.Pages())
	a.dialogManager = ui.NewDialogManager(a)
	a.focusManager = ui.NewFocusManager(a)
	a.SetRoot(a.layoutManager.RootPrimitive(), true).EnableMouse(true)

	a.setupPage = pages.NewSetupPage(a)
	a.mainPage = pages.NewMainPage(a)
	a.logPage = pages.NewLogPage(a)
	a.loadingPage = pages.NewLoadingPage(a)
	a.manageModsPage = pages.NewManageModsPage(a)
	a.historyPage = pages.NewHistoryPage(a)

	a.navManager.Register(ui.PageSetupID, a.setupPage)
	a.navManager.Register(ui.PageMainID, a.mainPage)
	a.navManager.Register(ui.PageLogID, a.logPage)
	a.navManager.Register(ui.PageLoadingID, a.loadingPage)
	a.navManager.Register(ui.PageManageModsID, a.manageModsPage)
	a.navManager.Register(ui.PageHistoryID, a.historyPage)

	a.setupGlobalInputCapture()

	return a
}

// StartLoadingProcess is called by the SetupPage to begin loading mods.
func (a *App) StartLoadingProcess(modsPath string) {
	a.navManager.SwitchTo(ui.PageLoadingID)
	a.loadingPage.StartLoading(modsPath)

	go func() {
		overrides := a.loadAndMergeOverrides(modsPath)

		loader := mods.ModLoader{ModParser: mods.ModParser{QuiltParsing: a.cliArgs.QuiltSupport}}
		logging.Infof("App: Loading mods from '%s', Quilt Support: %v", modsPath, a.cliArgs.QuiltSupport)
		allMods, providers, _, loadErr := loader.LoadMods(modsPath, overrides, func(fileName string) {
			a.QueueUpdateDraw(func() {
				a.loadingPage.UpdateProgress(fileName)
			})
		})

		// Signal the main thread to handle the result.
		a.QueueUpdateDraw(func() {
			a.onLoadingComplete(modsPath, allMods, providers, loadErr)
		})
	}()
}

// onLoadingComplete handles the result of the mod loading process.
func (a *App) onLoadingComplete(modsPath string, allMods map[string]*mods.Mod, providers mods.PotentialProvidersMap, err error) {
	if err != nil {
		a.dialogManager.ShowErrorDialog("Loading Error", fmt.Sprintf("Failed to load mods: %v", err), func() {
			a.navManager.SwitchTo(ui.PageSetupID)
		})
		return
	}
	if len(allMods) == 0 {
		a.dialogManager.ShowErrorDialog("Information", "No mods were found in the specified directory.", func() {
			a.navManager.SwitchTo(ui.PageSetupID)
		})
		return
	}

	// Loading was successful, now create the runtime services.
	stateMgr := mods.NewStateManager(allMods, providers)
	activator := mods.NewModActivator(modsPath, allMods)

	svc, err := bisect.NewService(stateMgr, activator)
	if err != nil {
		a.dialogManager.ShowErrorDialog("Initialization Error", err.Error(), func() {
			a.navManager.SwitchTo(ui.PageSetupID)
		})
		return
	}

	a.bisectSvc = svc
	a.bisectSvc.OnStateChange = a.handleCoreStateChange
	a.bisectSvc.ResetSearch()
	a.navManager.SwitchTo(ui.PageMainID)
	a.handleCoreStateChange()
}

func (a *App) handleCoreStateChange() {
	if obs, ok := a.navManager.GetCurrentPage().(ui.SearchStateObserver); ok {
		go a.QueueUpdateDraw(func() { obs.RefreshSearchState() })
	}
}

// Step orchestrates the next bisection test.
func (a *App) Step() {
	if a.bisectSvc == nil {
		return
	}
	plan, changes, err := a.bisectSvc.AdvanceToNextTest()
	if err != nil {
		a.handleStepError(err)
		return
	}

	onSuccess := func() { a.navManager.CloseModal(); a.bisectSvc.SubmitTestResult(imcs.TestResultGood, changes) }
	onFailure := func() { a.navManager.CloseModal(); a.bisectSvc.SubmitTestResult(imcs.TestResultFail, changes) }
	onCancel := func() { a.navManager.CloseModal(); a.bisectSvc.CancelTest(changes) }

	testPage := pages.NewTestPage(a, plan.IsVerificationStep, onSuccess, onFailure, onCancel)
	a.navManager.ShowModal(ui.PageTestID, testPage)
}

func (a *App) ContinueSearch() {
	if a.bisectSvc == nil {
		return
	}
	logging.Debugf("App: ContinueSearch action triggered.")
	// The service layer handles all the complex logic atomically.
	result := a.bisectSvc.ContinueSearch()

	// If the operation resulted in mods being auto-disabled, we must inform the user.
	// This happens *after* the state has already been updated.
	if len(result.NewlyDisabledMods) > 0 {
		a.dialogManager.ShowInfoDialog(
			"Unresolvable Mods Disabled",
			fmt.Sprintf("To continue the search, the following mods were automatically disabled because their dependencies can no longer be met:\n\n[yellow]%s[-]", strings.Join(result.NewlyDisabledMods, ", ")),
			nil,
		)
	}
}

func (a *App) Undo() {
	logging.Debugf("App: Undo action triggered.")
	a.bisectSvc.UndoLastStep()
}

func (a *App) ResetSearch() {
	logging.Debugf("App: ResetSearch faction triggered.")
	a.bisectSvc.ResetSearch()
}

func (a *App) Run() error {
	a.navManager.SwitchTo(ui.PageSetupID)
	return a.Application.Run()
}

func (a *App) Stop() {
	a.cancelApp()
	a.shutdownWg.Wait()
	a.Application.Stop()
}

// displayResults shows the result page when the search is coplete.
func (a *App) displayResults() {
	if a.bisectSvc == nil {
		return
	}
	state := a.bisectSvc.GetCurrentState()
	if state.IsComplete || a.bisectSvc.Engine().WasLastTestVerification() {
		resultPage := pages.NewResultPage(a)
		a.navManager.ShowModal(ui.PageResultID, resultPage)
	}
}

// setupGlobalInputCapture defines application-wide keybindings.
func (a *App) setupGlobalInputCapture() {
	a.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		if event.Key() == tcell.KeyTab {
			if a.focusManager.Cycle(a.navManager.GetCurrentPage(), true) {
				return nil
			}
		}
		if event.Key() == tcell.KeyBacktab {
			if a.focusManager.Cycle(a.navManager.GetCurrentPage(), false) {
				return nil
			}
		}

		if event.Modifiers()&tcell.ModCtrl != 0 {
			switch event.Key() {
			case tcell.KeyCtrlL:
				go a.QueueUpdateDraw(a.navManager.ToggleLogPage)
				return nil
			case tcell.KeyCtrlC:
				go a.QueueUpdateDraw(a.dialogManager.ShowQuitDialog)
				return nil
			case tcell.KeyCtrlH:
				go a.QueueUpdateDraw(a.navManager.ToggleHistoryPage)
				return nil
			}
		}
		return event
	})
}

// loadAndMergeOverrides handles the layered loading and merging of dependency overrides.
func (a *App) loadAndMergeOverrides(modsPath string) *mods.DependencyOverrides {
	var allOverrides []*mods.DependencyOverrides

	// Priority 1: Current Working Directory
	cwd, _ := os.Getwd()
	cwdPath := filepath.Join(cwd, "fabric_loader_dependencies.json")
	if cwdOverrides, err := mods.LoadDependencyOverridesFromPath(cwdPath); err != nil {
		// A "not found" error is expected and should be ignored silently.
		if !os.IsNotExist(err) {
			// Any other error (e.g., malformed JSON, permissions) should be logged.
			logging.Warnf("App: Could not load dependency overrides from '%s': %v", cwdPath, err)
		}
	} else {
		logging.Infof("App: Loaded dependency overrides from current directory.")
		allOverrides = append(allOverrides, cwdOverrides)
	}

	// Priority 2: Standard config directory
	configPath := filepath.Join(modsPath, "..", "config", "fabric_loader_dependencies.json")
	if configOverrides, err := mods.LoadDependencyOverridesFromPath(configPath); err != nil {
		if !os.IsNotExist(err) {
			logging.Warnf("App: Could not load dependency overrides from '%s': %v", configPath, err)
		}
	} else {
		logging.Infof("App: Loaded dependency overrides from config directory.")
		allOverrides = append(allOverrides, configOverrides)
	}

	// Priority 3: Embedded overrides
	if !a.cliArgs.NoEmbeddedOverrides {
		if embedded, err := mods.LoadDependencyOverrides(bytes.NewReader(embeds.GetEmbeddedOverrides())); err != nil {
			// This indicates a problem with the embedded file itself, which is a developer error.
			logging.Errorf("App: Failed to load embedded dependency overrides: %v", err)
		} else {
			logging.Infof("App: Loaded embedded dependency overrides.")
			allOverrides = append(allOverrides, embedded)
		}
	}

	// Merge all loaded overrides based on priority.
	return mods.MergeDependencyOverrides(allOverrides...)
}

// handleStepError inspects an error from the bisection service and takes the
// appropriate UI action, such as displaying results or showing an error dialog.
func (a *App) handleStepError(err error) {
	// Check if the error is the specific "search complete" signal.
	if errors.Is(err, imcs.ErrSearchComplete) {
		a.displayResults()
		return
	}

	a.dialogManager.ShowErrorDialog("Bisection Error", `An error occurred and the next step could not be prepared.
If another program, like Minecraft, is currently acessing your mods, please close it.

Please check the application log for details.`, nil)
}

// --- AppInterface Implementation ---
func (a *App) GetFocus() tview.Primitive         { return a.Application.GetFocus() }
func (a *App) SetFocus(p tview.Primitive)        { a.Application.SetFocus(p) }
func (a *App) Navigation() *ui.NavigationManager { return a.navManager }
func (a *App) Dialogs() *ui.DialogManager        { return a.dialogManager }
func (a *App) Layout() *ui.LayoutManager         { return a.layoutManager }
func (a *App) GetLogger() *logging.Logger        { return a.logger }

func (a *App) GetViewModel() ui.BisectionViewModel {
	if a.bisectSvc == nil {
		return ui.BisectionViewModel{IsReady: false}
	}

	engine := a.bisectSvc.Engine()
	enumState := a.bisectSvc.EnumerationState()
	state := engine.GetCurrentState()
	currentPlan, _ := engine.GetCurrentTestPlan()

	isVerification := currentPlan != nil && currentPlan.IsVerificationStep

	return ui.BisectionViewModel{
		IsReady:            true,
		IsComplete:         state.IsComplete,
		IsVerificationStep: isVerification,
		StepCount:          engine.GetStepCount(),
		Iteration:          state.Iteration,
		Round:              state.Round,
		EstimatedMaxTests:  engine.GetEstimatedMaxTests(),
		LastTestResult:     state.LastTestResult,
		AllConflictSets:    enumState.FoundConflictSets,
		CurrentConflictSet: state.ConflictSet,
		LastFoundElement:   state.LastFoundElement,
		AllModIDs:          state.AllModIDs,
		CandidateSet:       state.GetCandidateSet(),
		ClearedSet:         state.GetClearedSet(),
		PendingAdditions:   engine.GetPendingAdditions(),
		CurrentTestPlan:    currentPlan,
		ExecutionLog:       a.bisectSvc.GetCombinedExecutionLog(),
	}
}

func (a *App) GetStateManager() *mods.StateManager { return a.bisectSvc.StateManager() }
