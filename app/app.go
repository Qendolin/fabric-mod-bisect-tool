package app

import (
	"context"
	"fmt"
	"sort"
	"strings"
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
	logHandler    *ui.LogHandler
	focusManager  *ui.FocusManager

	// Core services
	ModLoader mods.ModLoaderService
	ModState  *mods.StateManager
	Resolver  *mods.DependencyResolver
	Activator *systemrunner.ModActivator
	Runner    *systemrunner.Runner
	Searcher  *conflict.Searcher

	appCtx    context.Context
	cancelApp context.CancelFunc

	shutdownWg sync.WaitGroup
}

// NewApp creates and initializes the TUI application.
func NewApp() *App {
	appCtx, cancelApp := context.WithCancel(context.Background())

	a := &App{
		Application: tview.NewApplication(),
		appCtx:      appCtx,
		cancelApp:   cancelApp,
	}

	a.layoutManager = ui.NewLayoutManager()
	a.navManager = ui.NewNavigationManager(a, a.layoutManager.Pages())
	a.dialogManager = ui.NewDialogManager(a)
	a.focusManager = ui.NewFocusManager(a)
	a.SetRoot(a.layoutManager.RootPrimitive(), true).EnableMouse(true)

	a.setupGlobalInputCapture()
	a.setupCoreServices()

	return a
}

// setupGlobalInputCapture defines application-wide keybindings.
func (a *App) setupGlobalInputCapture() {
	a.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		focusManager := a.GetFocusManager()

		if event.Key() == tcell.KeyTab {
			if focusManager.Cycle(a.navManager.GetCurrentPage().Primitive(), true) {
				return nil
			}
		}
		if event.Key() == tcell.KeyBacktab {
			if focusManager.Cycle(a.navManager.GetCurrentPage().Primitive(), false) {
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

// InitLogging re-initializes the logging system to include the UI channel writer.
func (a *App) InitLogging(logPath string) error {
	logChannel := make(chan []byte, 100)
	a.logHandler = ui.NewLogHandler(a, logChannel, &a.shutdownWg)

	channelWriter := logging.NewChannelWriter(a.appCtx, logChannel)
	return logging.Init(logPath, channelWriter)
}

// setupCoreServices initializes the backend logic components.
func (a *App) setupCoreServices() {
	a.ModLoader = mods.NewModLoaderService()
	a.Resolver = mods.NewDependencyResolver()
}

// Run starts the tview application event loop.
func (a *App) Run() error {
	go a.logHandler.Start(a.appCtx)

	a.navManager.ShowPage(ui.PageSetupID, ui.NewSetupPage(a), true)

	return a.Application.Run()
}

// Stop gracefully stops the application.
func (a *App) Stop() {
	a.cancelApp()
	a.shutdownWg.Wait()
	logging.Close()
	a.Application.Stop()
}

// AppInterface methods to be called by UI components
func (a *App) GetApplicationContext() context.Context { return a.appCtx }
func (a *App) GetLogTextView() *tview.TextView        { return a.logHandler.TextView() }
func (a *App) GetModLoader() mods.ModLoaderService    { return a.ModLoader }
func (a *App) Navigation() *ui.NavigationManager      { return a.navManager }
func (a *App) Dialogs() *ui.DialogManager             { return a.dialogManager }
func (a *App) Layout() *ui.LayoutManager              { return a.layoutManager }
func (a *App) GetFocusManager() *ui.FocusManager      { return a.focusManager }
func (a *App) GetSearcher() *conflict.Searcher        { return a.Searcher }

func (a *App) OnModsLoaded(modsPath string, allMods map[string]*mods.Mod, potentialProviders mods.PotentialProvidersMap, sortedModIDs []string) {
	logging.Infof("App: Mods loaded. Initializing services and transitioning to Main Page.")
	a.ModState = mods.NewStateManager(allMods, potentialProviders)
	a.ModState.OnStateChanged = a.handleModStateChange
	a.Activator = systemrunner.NewModActivator(modsPath, allMods)
	a.Searcher = conflict.NewSearcher(a.ModState)
	a.Searcher.Start(sortedModIDs)

	mainPage := ui.NewMainPage(a)
	a.navManager.ShowPage(ui.PageMainID, mainPage, true)
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
	loadingPage := ui.NewLoadingPage(a, modsPath)
	a.navManager.ShowPage(ui.PageLoadingID, loadingPage, true)

	if lp, ok := loadingPage.(*ui.LoadingPage); ok {
		lp.StartLoading()
	}
}

// Step initiates the next bisection test.
func (a *App) Step() {
	if a.Searcher == nil || a.Searcher.IsComplete() {
		a.layoutManager.SetStatusText("Search is not active or is already complete.")
		return
	}

	testSet, _, needsTest := a.Searcher.PrepareNextTest()
	if !needsTest {
		if page, ok := a.navManager.GetCurrentPage().(ui.SearchStateObserver); ok {
			page.RefreshSearchState()
		}
		return
	}

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
		a.navManager.PopPage() // Pop the TestPage
		a.processTestResult(systemrunner.GOOD, changes)
	}
	onFailure := func() {
		a.navManager.PopPage()
		a.processTestResult(systemrunner.FAIL, changes)
	}
	onCancel := func() {
		a.navManager.PopPage()
		a.cancelTest(changes)
	}

	testPage := ui.NewTestPage(a, a.Searcher.IsVerificationStep(), onSuccess, onFailure, onCancel)
	a.navManager.PushPage(ui.PageTestID, testPage)
}

func (a *App) CancelTest(changes []systemrunner.BatchStateChange) {
	if a.Activator != nil {
		a.Activator.Revert(changes)
	}
	// Do not call StepBack here, as no search step was actually taken.
	// Simply pop the page to return to the previous state.
	a.navManager.PopPage()
}

func (a *App) SubmitTestResult(result systemrunner.Result, changes []systemrunner.BatchStateChange) {
	// Revert file changes before processing the result.
	if a.Activator != nil {
		a.Activator.Revert(changes)
	}

	a.navManager.PopPage() // Close the TestPage
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

	if a.Searcher.IsComplete() {
		a.handleSearchCompletion(a.Searcher.GetCurrentState())
	} else if a.Searcher.LastFoundElement() != "" {
		// An intermediate culprit was just found, and the search isn't over yet
		a.handleIntermediateResult(a.Searcher.GetCurrentState())
	}
}

func (a *App) ShowResultPage(title, message, explanation string) {
	resultPage := ui.NewResultPage(a, title, message, explanation)
	a.navManager.PushPage(ui.PageResultID, resultPage)
}

// Add a new helper function for handling search completion
func (a *App) handleSearchCompletion(finalState conflict.SearchSnapshot) {
	title := "Search Complete"
	var message, explanation string

	if len(finalState.ConflictSet) > 0 {
		conflictMods := mapKeysFromStruct(finalState.ConflictSet)
		message = fmt.Sprintf("Found [yellow]%d[-:-:-] problematic mod(s):\n\n[::b]%s", len(conflictMods), strings.Join(conflictMods, "\n"))
		explanation = "\nWhat to do next:\n  - Try disabling these mods and launching the game.\n  - Report the incompatibility to the mod authors."
	} else {
		message = "No conflict was found."
		explanation = "The bisection process completed without isolating a specific cause for failure.\nThis could mean the issue is not related to a specific mod or combination of mods."
	}

	a.ShowResultPage(title, message, explanation)
}

// Add a new helper function for handling intermediate results
func (a *App) handleIntermediateResult(currentState conflict.SearchSnapshot) {
	title := "Intermediate Result Found"
	conflictMods := mapKeysFromStruct(currentState.ConflictSet)
	message := fmt.Sprintf("Found [yellow]%d[-:-:-] problematic mod(s) so far:\n\n[::b]%s", len(conflictMods), strings.Join(conflictMods, "\n"))
	explanation := "\nThe last test failed, indicating more mods are required for the conflict.\nPress 'Step' to continue searching for the next problematic mod."

	a.ShowResultPage(title, message, explanation)
}

func mapKeysFromStruct(m map[string]struct{}) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
