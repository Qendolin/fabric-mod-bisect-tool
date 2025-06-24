package app

import (
	"bytes"
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/Qendolin/fabric-mod-bisect-tool/app/core/conflict"
	"github.com/Qendolin/fabric-mod-bisect-tool/app/core/mods"
	"github.com/Qendolin/fabric-mod-bisect-tool/app/core/systemrunner"
	"github.com/Qendolin/fabric-mod-bisect-tool/app/logging"
	"github.com/Qendolin/fabric-mod-bisect-tool/app/ui"
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

// App orchestrates the TUI application, managing pages, global state, and core services.
type App struct {
	*tview.Application
	pages  *tview.Pages // Main container for all application pages/views
	root   *tview.Flex  // Top-level flex for consistent layout
	header *tview.Flex  // Contains tool name and status/error counts
	footer *tview.Flex  // Contains action hints

	// Global UI elements
	statusTextView *tview.TextView // Page-specific status/instructions
	errorCounters  *tview.TextView // Warnings and errors counter

	// UI log viewer
	logTextView *tview.TextView
	logChannel  chan []byte

	// Core services (models)
	ModLoader mods.ModLoaderService
	ModState  *mods.StateManager
	Resolver  *mods.DependencyResolver
	Activator *systemrunner.ModActivator
	Runner    *systemrunner.Runner
	Searcher  *conflict.Searcher

	// Internal state
	warnCount     int
	errorCount    int
	logCountMutex sync.Mutex
	logBatch      [][]byte
	logBatchMutex sync.Mutex
	logUpdate     *time.Ticker

	activePageID   string
	pageStack      []tview.Primitive
	pageIDs        map[tview.Primitive]string
	pagePrimitives map[string]tview.Primitive

	testResultChan chan systemrunner.Result
	testErrorChan  chan error

	appCtx    context.Context
	cancelApp context.CancelFunc

	uiPageManager ui.PageManager
}

// NewApp creates and initializes the TUI application.
func NewApp(pageManager ui.PageManager) *App {
	appCtx, cancelApp := context.WithCancel(context.Background())

	a := &App{
		Application:    tview.NewApplication(),
		pages:          tview.NewPages(),
		root:           tview.NewFlex().SetDirection(tview.FlexRow),
		header:         tview.NewFlex(),
		footer:         tview.NewFlex(),
		statusTextView: tview.NewTextView().SetDynamicColors(true),
		errorCounters:  tview.NewTextView().SetDynamicColors(true).SetTextAlign(tview.AlignRight),
		pageIDs:        make(map[tview.Primitive]string),
		pagePrimitives: make(map[string]tview.Primitive),
		testResultChan: make(chan systemrunner.Result),
		testErrorChan:  make(chan error),
		appCtx:         appCtx,
		cancelApp:      cancelApp,
		uiPageManager:  pageManager,
		logChannel:     make(chan []byte, 100),
	}

	a.setupLayout()
	a.setupGlobalInputCapture()
	a.setupCoreServices()

	return a
}

// setupLayout configures the consistent header and footer layout.
func (a *App) setupLayout() {
	a.header.AddItem(a.statusTextView, 0, 1, false).
		AddItem(a.errorCounters, 0, 1, false)

	a.root.SetBorder(true).
		SetTitle(" Fabric Mod Bisect Tool ").
		SetTitleAlign(tview.AlignLeft)

	a.root.AddItem(a.header, 1, 0, false).
		AddItem(a.pages, 0, 1, true).
		AddItem(a.footer, 1, 0, false)

	a.SetRoot(a.root, true).EnableMouse(true)
}

// setupGlobalInputCapture defines application-wide keybindings.
func (a *App) setupGlobalInputCapture() {
	a.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		if event.Key() == tcell.KeyCtrlL {
			go a.QueueUpdateDraw(a.ToggleLogPage)
			return nil
		}
		if event.Key() == tcell.KeyCtrlC {
			go a.QueueUpdateDraw(a.ShowQuitDialog)
			return nil
		}
		return event
	})
}

// InitLogging re-initializes the logging system to include the UI channel writer.
func (a *App) InitLogging(logDir, logFileName string, initialLogs *bytes.Buffer) error {
	a.logTextView = tview.NewTextView().
		SetDynamicColors(true).
		SetScrollable(true).
		SetWrap(true)

	if initialLogs != nil {
		fmt.Fprint(a.logTextView, initialLogs.String())
	}

	channelWriter := logging.NewChannelWriter(a.appCtx, a.logChannel)
	return logging.Init(logDir, logFileName, channelWriter)
}

// startLogProcessor starts the goroutine that consumes logs from the channel
// and batches them for efficient UI updates.
func (a *App) startLogProcessor() {
	a.logUpdate = time.NewTicker(100 * time.Millisecond) // Tick every 100ms to trigger a batch write

	go func() {
		for {
			select {
			case <-a.appCtx.Done():
				a.logUpdate.Stop()
				return
			case logMsg, ok := <-a.logChannel:
				if !ok {
					return
				}
				a.processLogMessage(logMsg)
			case <-a.logUpdate.C:
				a.flushLogBatch()
			}
		}
	}()
}

// processLogMessage adds a log message to the current batch.
func (a *App) processLogMessage(msg []byte) {
	a.logBatchMutex.Lock()
	defer a.logBatchMutex.Unlock()

	// Add message to the batch for later writing.
	a.logBatch = append(a.logBatch, msg)

	// Pre-calculate counts here, but do not trigger a UI update.
	content := string(msg)
	newWarnings := strings.Count(content, "WARN:")
	newErrors := strings.Count(content, "ERROR:")

	if newWarnings > 0 || newErrors > 0 {
		a.logCountMutex.Lock()
		a.warnCount += newWarnings
		a.errorCount += newErrors
		a.logCountMutex.Unlock()
	}
}

// flushLogBatch writes the entire accumulated batch of logs and updates counters in one UI call.
func (a *App) flushLogBatch() {
	a.logBatchMutex.Lock()
	defer a.logBatchMutex.Unlock()

	if len(a.logBatch) == 0 {
		return
	}

	// Create a single string from the batch.
	var batchContent strings.Builder
	for _, msg := range a.logBatch {
		batchContent.Write(msg)
	}

	// Important: Reset the batch inside the lock.
	a.logBatch = nil

	// Queue a single draw to update both the log text and the error counters.
	go a.QueueUpdateDraw(func() {
		// Write the batched log text.
		fmt.Fprint(a.logTextView, batchContent.String())
		a.logTextView.ScrollToEnd()

		// Update the error counters in the same UI update cycle.
		a.updateErrorCounters()
	})
}

// updateErrorCounters updates the display for warning and error counts.
func (a *App) updateErrorCounters() {
	a.logCountMutex.Lock()
	defer a.logCountMutex.Unlock()
	warnColor := tcell.ColorYellow
	errorColor := tcell.ColorRed
	if a.warnCount == 0 {
		warnColor = tcell.ColorGray
	}
	if a.errorCount == 0 {
		errorColor = tcell.ColorGray
	}

	a.errorCounters.SetText(fmt.Sprintf("[yellow]Warnings: [white:%s]%d[-:-:-] [red]Errors: [white:%s]%d[-:-:-]",
		warnColor.Name(), a.warnCount, errorColor.Name(), a.errorCount))
}

// setupCoreServices initializes the backend logic components.
func (a *App) setupCoreServices() {
	a.ModLoader = mods.NewModLoaderService()
	a.Resolver = mods.NewDependencyResolver()
}

// Run starts the tview application event loop.
func (a *App) Run() error {
	a.startLogProcessor()
	a.ShowPage(ui.PageSetupID, a.uiPageManager.NewSetupPage(a), true)
	go a.handleTestResults()
	return a.Application.Run()
}

// Stop gracefully stops the application.
func (a *App) Stop() {
	logging.Close()
	a.cancelApp()
	close(a.logChannel)
	a.Application.Stop()
}

// ShowPage adds a page to the main Pages container and sets it as the current page.
func (a *App) ShowPage(pageID string, page ui.Page, resize bool) {
	a.pages.AddAndSwitchToPage(pageID, page.Primitive(), resize)
	a.activePageID = pageID
	a.pagePrimitives[pageID] = page.Primitive()
	a.SetFocus(page.Primitive())
	a.updateErrorCounters()
	a.SetFooter(page.GetActionPrompts())
}

// PushPage adds an overlay page to the stack.
func (a *App) PushPage(pageID string, page ui.Page) {
	a.pages.AddPage(pageID, page.Primitive(), true, true)
	a.pageStack = append(a.pageStack, page.Primitive())
	a.pageIDs[page.Primitive()] = pageID
	a.pagePrimitives[pageID] = page.Primitive()
	a.SetFocus(page.Primitive())
	a.SetFooter(page.GetActionPrompts())
}

// PopPage removes the top-most page from the stack.
func (a *App) PopPage() {
	if len(a.pageStack) > 0 {
		topPage := a.pageStack[len(a.pageStack)-1]
		a.pageStack = a.pageStack[:len(a.pageStack)-1]

		if pageID, exists := a.pageIDs[topPage]; exists {
			a.pages.RemovePage(pageID)
			delete(a.pageIDs, topPage)
			delete(a.pagePrimitives, pageID)
		}
	}

	var focusTarget tview.Primitive
	var newFooter map[string]string
	if len(a.pageStack) > 0 {
		focusTarget = a.pageStack[len(a.pageStack)-1]
		if page, ok := focusTarget.(ui.Page); ok {
			newFooter = page.GetActionPrompts()
		}
	} else {
		focusTarget = a.pagePrimitives[a.activePageID]
		if page, ok := focusTarget.(ui.Page); ok {
			newFooter = page.GetActionPrompts()
		}
	}
	if focusTarget != nil {
		a.SetFocus(focusTarget)
	}
	a.SetFooter(newFooter)
}

// ToggleLogPage shows/hides the log page overlay.
func (a *App) ToggleLogPage() {
	if frontID, _ := a.pages.GetFrontPage(); frontID == ui.PageLogID {
		a.PopPage()
	} else {
		logPage := a.uiPageManager.NewLogPage(a)
		a.PushPage(ui.PageLogID, logPage)
	}
}

// ShowErrorDialog displays a modal dialog with an error message.
func (a *App) ShowErrorDialog(title, message string, onDismiss func()) {
	modal := tview.NewModal().
		SetText(message).
		AddButtons([]string{"Dismiss"}).
		SetDoneFunc(func(buttonIndex int, buttonLabel string) {
			go a.QueueUpdateDraw(func() {
				a.PopPage()
				if onDismiss != nil {
					onDismiss()
				}
			})
		})
	modal.SetTitle(" " + title + " ").SetTitleAlign(tview.AlignLeft)
	a.PushPage("error_dialog", ui.NewModalPage(modal))
	a.SetFocus(modal)
}

// ShowQuitDialog displays a confirmation dialog before quitting.
func (a *App) ShowQuitDialog() {
	modal := tview.NewModal().
		SetText("Are you sure you want to quit?").
		AddButtons([]string{"Cancel", "Quit without Saving", "Quit and Save"}).
		SetDoneFunc(func(buttonIndex int, buttonLabel string) {
			go a.QueueUpdateDraw(func() {
				a.PopPage()
				switch buttonLabel {
				case "Quit and Save":
					logging.Info("App: Quitting and saving state (not implemented yet).")
					a.Stop()
				case "Quit without Saving":
					logging.Info("App: Quitting without saving state.")
					a.Stop()
				case "Cancel":
				}
			})
		})
	a.PushPage("quit_dialog", ui.NewModalPage(modal))
	a.SetFocus(modal)
}

// SetPageStatus updates the status message in the header.
func (a *App) SetPageStatus(message string) {
	go a.QueueUpdateDraw(func() {
		a.statusTextView.SetText(message)
	})
}

// SetFooter updates the action hints grid.
func (a *App) SetFooter(prompts map[string]string) {
	go a.QueueUpdateDraw(func() {
		a.footer.Clear()

		if prompts == nil {
			return
		}

		globalPrompts := map[string]string{
			"Ctrl+L": "Logs",
			"Ctrl+C": "Quit",
		}

		var allPrompts []string
		for key, desc := range globalPrompts {
			allPrompts = append(allPrompts, fmt.Sprintf("[darkcyan::b]%s[-:-:-]: %s", key, desc))
		}

		var pageKeys []string
		for key := range prompts {
			pageKeys = append(pageKeys, key)
		}
		sort.Strings(pageKeys)

		for _, key := range pageKeys {
			desc := prompts[key]
			allPrompts = append(allPrompts, fmt.Sprintf("[darkcyan::b]%s[-:-:-]: %s", key, desc))
		}

		fullText := " " + strings.Join(allPrompts, " | ")
		a.footer.AddItem(tview.NewTextView().SetDynamicColors(true).SetText(fullText), 0, 1, false)
	})
}

// GetApplicationContext returns the application's root context.
func (a *App) GetApplicationContext() context.Context {
	return a.appCtx
}

// GetLogTextView returns the application's shared log text view.
func (a *App) GetLogTextView() *tview.TextView {
	return a.logTextView
}

// GetModLoader returns the application's mod loader service.
func (a *App) GetModLoader() mods.ModLoaderService {
	return a.ModLoader
}

// GetModLoader returns the application's ui page manager
func (a *App) GetPageManager() ui.PageManager {
	return a.uiPageManager
}

// OnModsLoaded is the callback for when mod loading is complete.
func (a *App) OnModsLoaded(allMods map[string]*mods.Mod, potentialProviders mods.PotentialProvidersMap, sortedModIDs []string) {
	// TODO: Transition to the Main Page here
	logging.Infof("App: Mods loaded successfully. %d mods found. Transitioning to main page (not implemented).", len(allMods))
	a.SetPageStatus("Mods loaded. Ready to start bisect.")
}

// StartModLoad orchestrates showing the loading page and starting the async load.
func (a *App) StartModLoad(modsPath string) {
	loadingPage := a.uiPageManager.NewLoadingPage(a, modsPath)
	a.ShowPage(ui.PageLoadingID, loadingPage, true)

	// StartLoading is a method on the LoadingPage itself.
	if lp, ok := loadingPage.(*ui.LoadingPage); ok {
		lp.StartLoading()
	}
}

// handleTestResults listens for test outcomes from the Runner and updates the searcher.
func (a *App) handleTestResults() {
	for {
		select {
		case <-a.appCtx.Done():
			return
		case result := <-a.testResultChan:
			go a.QueueUpdateDraw(func() {
				a.PopPage()
				if a.Searcher != nil {
					a.Searcher.ResumeWithResult(a.appCtx, result)
				}
			})
		case err := <-a.testErrorChan:
			go a.QueueUpdateDraw(func() {
				a.PopPage()
				a.ShowErrorDialog("Test Error", fmt.Sprintf("An error occurred during test: %v", err), nil)
				if a.Searcher != nil {
					a.Searcher.ResumeWithResult(a.appCtx, systemrunner.Result(""))
				}
			})
		}
	}
}
