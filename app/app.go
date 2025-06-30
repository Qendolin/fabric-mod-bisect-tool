package app

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/Qendolin/fabric-mod-bisect-tool/app/core/bisect"
	"github.com/Qendolin/fabric-mod-bisect-tool/app/core/imcs"
	"github.com/Qendolin/fabric-mod-bisect-tool/app/core/mods"
	"github.com/Qendolin/fabric-mod-bisect-tool/app/embeds"
	"github.com/Qendolin/fabric-mod-bisect-tool/app/logging"
	"github.com/Qendolin/fabric-mod-bisect-tool/app/ui"
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
	setupPage      *ui.SetupPage
	mainPage       *ui.MainPage
	logPage        *ui.LogPage
	loadingPage    *ui.LoadingPage
	manageModsPage *ui.ManageModsPage

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

	a.setupPage = ui.NewSetupPage(a)
	a.mainPage = ui.NewMainPage(a)
	a.logPage = ui.NewLogPage(a)
	a.loadingPage = ui.NewLoadingPage(a)
	a.manageModsPage = ui.NewManageModsPage(a)

	a.navManager.Register(ui.PageSetupID, a.setupPage)
	a.navManager.Register(ui.PageMainID, a.mainPage)
	a.navManager.Register(ui.PageLogID, a.logPage)
	a.navManager.Register(ui.PageLoadingID, a.loadingPage)
	a.navManager.Register(ui.PageManageModsID, a.manageModsPage)

	a.setupGlobalInputCapture()

	return a
}

// StartLoadingProcess is called by the SetupPage to begin loading mods.
func (a *App) StartLoadingProcess(modsPath string) {
	a.navManager.SwitchTo(ui.PageLoadingID)
	a.loadingPage.StartLoading(modsPath)

	go func() {
		overrides := a.loadAndMergeOverrides(modsPath)

		loader := mods.NewModLoaderService()
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
	engine := imcs.NewEngine(stateMgr.GetAllModIDs())

	svc, err := bisect.NewService(stateMgr, activator, engine)
	if err != nil {
		a.dialogManager.ShowErrorDialog("Initialization Error", err.Error(), func() {
			a.navManager.SwitchTo(ui.PageSetupID)
		})
		return
	}

	a.bisectSvc = svc
	a.bisectSvc.OnStateChange = a.handleCoreStateChange
	a.bisectSvc.StartNewSearch()
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
	changes, plan, err := a.bisectSvc.PlanAndExecuteTestStep()
	if err != nil {
		if a.bisectSvc.GetCurrentState().IsComplete {
			a.displayResults()
		} else {
			logging.Errorf("App: Failed to execute test plan: %v", err)
			userMessage := "Failed to apply mod file changes!\n" +
				"Please ensure Minecraft is closed.\n\n" +
				"For details see application log."
			a.dialogManager.ShowErrorDialog("File Error", userMessage, nil)
			return
		}
		return
	}

	onSuccess := func() { a.navManager.CloseModal(); a.bisectSvc.SubmitTestResult(imcs.TestResultGood, changes) }
	onFailure := func() { a.navManager.CloseModal(); a.bisectSvc.SubmitTestResult(imcs.TestResultFail, changes) }
	onCancel := func() { a.navManager.CloseModal(); a.bisectSvc.CancelTest(changes) }

	testPage := ui.NewTestPage(a, plan.IsVerificationStep, onSuccess, onFailure, onCancel)
	a.navManager.ShowModal(ui.PageTestID, testPage)
}

func (a *App) Undo()        { a.bisectSvc.UndoLastStep() }
func (a *App) ResetSearch() { a.bisectSvc.StartNewSearch() }
func (a *App) Run() error {
	a.navManager.SwitchTo(ui.PageSetupID)
	return a.Application.Run()
}
func (a *App) Stop() {
	a.cancelApp()
	a.shutdownWg.Wait()
	a.Application.Stop()
}

// displayResults shows the result page when the search is complete.
func (a *App) displayResults() {
	if a.bisectSvc == nil {
		return
	}
	state := a.bisectSvc.GetCurrentState()
	if state.IsComplete || a.bisectSvc.Engine().WasLastTestVerification() {
		// This now finds all direct and indirect dependers.
		dependers := a.bisectSvc.StateManager().FindTransitiveDependersOf(state.ConflictSet)

		resultPage := ui.NewResultPage(a, state, dependers)
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
	state := engine.GetCurrentState()
	nextPlan, _ := engine.GetNextTestPlan()
	activePlan := engine.GetActiveTestPlan()

	isVerification := (activePlan != nil && activePlan.IsVerificationStep) ||
		(activePlan == nil && nextPlan != nil && nextPlan.IsVerificationStep)

	// The iteration number is the number of conflict elements we are trying to find.
	// It's the number already found + 1, unless we are verifying the one just found.
	iteration := len(state.ConflictSet)
	if !isVerification {
		iteration++
	}

	return ui.BisectionViewModel{
		IsReady:            true,
		IsComplete:         state.IsComplete,
		IsVerificationStep: isVerification,
		StepCount:          engine.GetStepCount(),
		Iteration:          iteration,
		EstimatedMaxTests:  engine.GetEstimatedMaxTests(),
		LastTestResult:     state.LastTestResult,
		ConflictSet:        state.ConflictSet,
		AllModIDs:          state.AllModIDs,
		CandidateSet:       state.GetCandidateSet(),
		ClearedSet:         state.GetClearedSet(),
		PendingAdditions:   engine.GetPendingAdditions(),
		ActiveTestPlan:     activePlan,
		NextTestPlan:       nextPlan,
		ExecutionLog:       engine.GetExecutionLog().GetEntries(),
	}
}

func (a *App) GetStateManager() *mods.StateManager { return a.bisectSvc.StateManager() }
