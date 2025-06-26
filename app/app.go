package app

import (
	"context"
	"fmt"
	"sort"
	"sync"

	"github.com/Qendolin/fabric-mod-bisect-tool/app/core/conflict"
	"github.com/Qendolin/fabric-mod-bisect-tool/app/core/mods"
	"github.com/Qendolin/fabric-mod-bisect-tool/app/core/systemrunner"
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

	// Core services
	ModLoader mods.ModLoaderService
	ModState  *mods.StateManager
	Resolver  *mods.DependencyResolver
	Activator *systemrunner.ModActivator
	Runner    *systemrunner.Runner
	Searcher  *conflict.Searcher

	// Pages
	setupPage     *ui.SetupPage
	mainPage      *ui.MainPage
	logPage       *ui.LogPage
	loadingPage   *ui.LoadingPage
	mangeModsPage *ui.ManageModsPage

	appCtx    context.Context
	cancelApp context.CancelFunc

	shutdownWg sync.WaitGroup
}

// NewApp creates and initializes the TUI application.
func NewApp(logger *logging.Logger) *App {
	appCtx, cancelApp := context.WithCancel(context.Background())

	a := &App{
		Application: tview.NewApplication(),
		appCtx:      appCtx,
		cancelApp:   cancelApp,
		logger:      logger,
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
	a.mangeModsPage = ui.NewManageModsPage(a)

	a.navManager.Register(ui.PageSetupID, a.setupPage)
	a.navManager.Register(ui.PageMainID, a.mainPage)
	a.navManager.Register(ui.PageLogID, a.logPage)
	a.navManager.Register(ui.PageLoadingID, a.loadingPage)
	a.navManager.Register(ui.PageManageModsID, a.mangeModsPage)

	a.setupGlobalInputCapture()
	a.setupCoreServices()

	return a
}

// setupGlobalInputCapture defines application-wide keybindings.
func (a *App) setupGlobalInputCapture() {
	a.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		focusManager := a.GetFocusManager()

		if event.Key() == tcell.KeyTab {
			if focusManager.Cycle(a.navManager.GetCurrentPage(), true) {
				return nil
			}
		}
		if event.Key() == tcell.KeyBacktab {
			if focusManager.Cycle(a.navManager.GetCurrentPage(), false) {
				return nil
			}
		}
		if event.Key() == tcell.KeyCtrlL {
			go a.QueueUpdateDraw(a.navManager.ToggleLogPage)
			return nil
		}
		if event.Key() == tcell.KeyCtrlC {
			go a.QueueUpdateDraw(a.dialogManager.ShowQuitDialog)
			return nil
		}
		return event
	})
}

// setupCoreServices initializes the backend logic components.
func (a *App) setupCoreServices() {
	a.ModLoader = mods.NewModLoaderService()
	a.Resolver = mods.NewDependencyResolver()
}

// Run starts the tview application event loop.
func (a *App) Run() error {
	a.navManager.SwitchTo(ui.PageSetupID)
	screen, err := tcell.NewScreen()
	if err != nil {
		return err
	}
	if err = screen.Init(); err != nil {
		return err
	}
	screen.SetTitle("Fabric Mod Bisect Tool") // tivew doesn't expose this
	a.EnableMouse(true)
	a.EnablePaste(true)
	a.SetScreen(screen)
	return a.Application.Run()
}

// Stop gracefully stops the application.
func (a *App) Stop() {
	a.cancelApp()
	a.shutdownWg.Wait()
	a.Application.Stop()
}

// AppInterface methods to be called by UI components
func (a *App) GetApplicationContext() context.Context { return a.appCtx }
func (a *App) GetLogger() *logging.Logger             { return a.logger }
func (a *App) GetModLoader() mods.ModLoaderService    { return a.ModLoader }
func (a *App) Navigation() *ui.NavigationManager      { return a.navManager }
func (a *App) Dialogs() *ui.DialogManager             { return a.dialogManager }
func (a *App) Layout() *ui.LayoutManager              { return a.layoutManager }
func (a *App) GetFocusManager() *ui.FocusManager      { return a.focusManager }
func (a *App) GetSearcher() *conflict.Searcher        { return a.Searcher }
func (a *App) GetModState() *mods.StateManager        { return a.ModState }

func (a *App) OnModsLoaded(modsPath string, allMods map[string]*mods.Mod, potentialProviders mods.PotentialProvidersMap, sortedModIDs []string) {
	logging.Infof("App: Mods loaded. Initializing services and transitioning to Main Page.")
	a.ModState = mods.NewStateManager(allMods, potentialProviders)
	a.ModState.OnStateChanged = a.handleModStateChange
	a.Activator = systemrunner.NewModActivator(modsPath, allMods)
	if err := a.Activator.EnableAll(); err != nil {
		a.dialogManager.ShowErrorDialog("Initialization Error", fmt.Sprintf("Failed to enable all mods: %v", err), nil)
		return
	}
	a.Searcher = conflict.NewSearcher(a.ModState)
	a.Searcher.Start(sortedModIDs)

	a.navManager.SwitchTo(ui.PageMainID)
}

// Add the new handler for mod state changes
func (a *App) handleModStateChange() {
	if a.Searcher != nil {
		a.Searcher.HandleExternalStateChange()
	}
	// Queue a refresh for the currently visible page if it's an observer
	go a.QueueUpdateDraw(func() {
		if page, ok := a.navManager.GetCurrentPage().(ui.SearchStateObserver); ok {
			page.RefreshSearchState()
		}
	})
}

func (a *App) StartModLoad(modsPath string) {
	a.navManager.SwitchTo(ui.PageLoadingID)
	a.loadingPage.StartLoading(modsPath)
}

// Step initiates the next bisection test.
func (a *App) Step() {
	if a.Searcher == nil || a.Searcher.IsComplete() {
		a.layoutManager.SetStatusText("Search is not active or is already complete.")
		return
	}

	if !a.Searcher.NeedsTest() {
		if page, ok := a.navManager.GetCurrentPage().(ui.SearchStateObserver); ok {
			page.RefreshSearchState()
		}
		return
	}

	testSet := a.Searcher.CalculateNextTestSet()

	// The call to the resolver is now much cleaner
	effectiveSet, _ := a.Resolver.ResolveEffectiveSet(
		systemrunner.SetToSlice(testSet),
		a.ModState.GetAllMods(),
		a.ModState.GetPotentialProviders(),
		a.ModState.GetModStatusesSnapshot(),
	)

	changes, err := a.Activator.Apply(effectiveSet)
	if err != nil {
		a.dialogManager.ShowErrorDialog("File Error", fmt.Sprintf("Failed to apply mod file changes: %v", err), nil)
		return
	}

	onSuccess := func() {
		a.navManager.CloseModal()
		a.processTestResult(systemrunner.GOOD, changes)
	}
	onFailure := func() {
		a.navManager.CloseModal()
		a.processTestResult(systemrunner.FAIL, changes)
	}
	onCancel := func() {
		a.navManager.CloseModal()
		a.cancelTest(changes)
	}

	testPage := ui.NewTestPage(a, a.Searcher.IsVerificationStep(), onSuccess, onFailure, onCancel)
	a.navManager.ShowModal(ui.PageTestID, testPage)
}

func (a *App) CancelTest(changes []systemrunner.BatchStateChange) {
	if a.Activator != nil {
		a.Activator.Revert(changes)
	}
	// Do not call StepBack here, as no search step was actually taken.
	// Simply pop the page to return to the previous state.
	a.navManager.CloseModal()
}

func (a *App) SubmitTestResult(result systemrunner.Result, changes []systemrunner.BatchStateChange) {
	// Revert file changes before processing the result.
	if a.Activator != nil {
		a.Activator.Revert(changes)
	}

	a.navManager.CloseModal() // Close the TestPage
	if a.Searcher != nil {
		a.Searcher.ResumeWithResult(a.appCtx, result)
		if page, ok := a.navManager.GetCurrentPage().(ui.SearchStateObserver); ok {
			page.RefreshSearchState()
		}
	}
}

// Undo steps back the searcher state.
func (a *App) Undo() {
	if a.Searcher != nil {
		if a.Searcher.StepBack() {
			if page, ok := a.navManager.GetCurrentPage().(ui.SearchStateObserver); ok {
				page.RefreshSearchState()
			}
		}
	}
}

// ResetSearch resets the current search.
func (a *App) ResetSearch() {
	if a.Searcher != nil {
		a.Searcher.Start(a.Searcher.GetAllModIDs())
		if page, ok := a.navManager.GetCurrentPage().(ui.SearchStateObserver); ok {
			page.RefreshSearchState()
		}
	}
}

func (a *App) processTestResult(result systemrunner.Result, changes []systemrunner.BatchStateChange) {
	if a.Activator != nil {
		a.Activator.Revert(changes)
	}
	if a.Searcher != nil {
		a.Searcher.ResumeWithResult(a.appCtx, result)
		if page, ok := a.navManager.GetCurrentPage().(ui.SearchStateObserver); ok {
			page.RefreshSearchState()
		}
		a.displayResults()
	}
}
func (a *App) cancelTest(changes []systemrunner.BatchStateChange) {
	if a.Activator != nil {
		a.Activator.Revert(changes)
	}
}

func (a *App) displayResults() {
	if a.Searcher == nil {
		return
	}

	if a.Searcher.IsComplete() || (a.Searcher.LastFoundElement() != "" && !a.Searcher.IsVerificationStep()) {
		resultPage := ui.NewResultPage(a, a.Searcher.GetCurrentState())
		a.navManager.ShowModal(ui.PageResultID, resultPage)
	}
}

func mapKeysFromStruct(m map[string]struct{}) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
