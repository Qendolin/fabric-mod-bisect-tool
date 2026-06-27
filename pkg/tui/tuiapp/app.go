package tuiapp

import (
	"context"
	"sync"

	"github.com/Qendolin/fabric-mod-bisect-tool/pkg/logging"
	"github.com/Qendolin/fabric-mod-bisect-tool/pkg/tui"
	"github.com/Qendolin/fabric-mod-bisect-tool/pkg/tui/pages"
	"github.com/Qendolin/fabric-mod-bisect-tool/pkg/ui"
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

// App is the TUI implementation of ui.View.
type App struct {
	ui.Controller
	tviewApp      *tview.Application
	layoutManager *tui.LayoutManager
	navManager    *tui.NavigationManager
	dialogManager *tui.DialogManager
	logger        *logging.Logger
	focusManager  *tui.FocusManager

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
}

// NewApp creates and initializes the TUI application.
func NewApp(controller ui.Controller, logger *logging.Logger) *App {
	appCtx, cancelApp := context.WithCancel(context.Background())

	a := &App{
		Controller: controller,
		tviewApp:   tview.NewApplication(),
		appCtx:     appCtx,
		cancelApp:  cancelApp,
		logger:     logger,
	}

	a.layoutManager = tui.NewLayoutManager(a, a.appCtx)
	a.navManager = tui.NewNavigationManager(a, a.layoutManager.Pages())
	a.dialogManager = tui.NewDialogManager(a)
	a.focusManager = tui.NewFocusManager(a)
	a.tviewApp.SetRoot(a.layoutManager.RootPrimitive(), true).EnableMouse(true)

	a.setupPage = pages.NewSetupPage(a)
	a.mainPage = pages.NewMainPage(a)
	a.logPage = pages.NewLogPage(a)
	a.loadingPage = pages.NewLoadingPage(a)
	a.manageModsPage = pages.NewManageModsPage(a)
	a.historyPage = pages.NewHistoryPage(a)

	a.navManager.Register(tui.PageSetupID, a.setupPage)
	a.navManager.Register(tui.PageMainID, a.mainPage)
	a.navManager.Register(tui.PageLoadingID, a.loadingPage)
	a.navManager.Register(tui.PageManageModsID, a.manageModsPage)

	a.setupGlobalInputCapture()

	return a
}

// --- TUIApp Interface implementation ---
func (a *App) QueueUpdateDraw(f func()) {
	a.tviewApp.QueueUpdateDraw(f)
}

func (a *App) Navigation() *tui.NavigationManager { return a.navManager }
func (a *App) Dialogs() *tui.DialogManager        { return a.dialogManager }
func (a *App) Layout() *tui.LayoutManager         { return a.layoutManager }
func (a *App) GetLogger() *logging.Logger         { return a.logger }
func (a *App) GetFocus() tview.Primitive          { return a.tviewApp.GetFocus() }
func (a *App) SetFocus(p tview.Primitive)         { a.tviewApp.SetFocus(p) }

// --- ui.View Interface implementation ---
func (a *App) Run() error {
	a.navManager.SwitchTo(tui.PageSetupID)
	return a.tviewApp.Run()
}

func (a *App) Stop() {
	a.cancelApp()
	a.shutdownWg.Wait()
	a.tviewApp.Stop()
}

func (a *App) ShowErrorDialog(title, message string, err error, callback func()) {
	a.dialogManager.ShowErrorDialog(title, message, err, callback)
}

func (a *App) ShowInfoDialog(title, message, details string, callback func()) {
	a.dialogManager.ShowInfoDialog(title, message, details, callback)
}

func (a *App) ShowQuestionDialog(title, message, details string, onYes, onNo func()) {
	a.dialogManager.ShowQuestionDialog(title, message, details, onYes, onNo)
}

func (a *App) ShowQuitDialog() {
	a.dialogManager.ShowQuitDialog()
}

func (a *App) SwitchToSetupPage() {
	a.navManager.SwitchTo(tui.PageSetupID)
}

func (a *App) SwitchToLoadingPage() {
	a.navManager.SwitchTo(tui.PageLoadingID)
}

func (a *App) UpdateLoadingProgress(fileName string, i, count int) {
	a.QueueUpdateDraw(func() {
		a.loadingPage.UpdateProgress(fileName, i, count)
	})
}

func (a *App) SwitchToMainPage() {
	a.navManager.SwitchTo(tui.PageMainID)
}

func (a *App) SwitchToResultPage() {
	resultPage := pages.NewResultPage(a)
	a.navManager.ShowModal(tui.PageResultID, resultPage)
}

func (a *App) ShowTestModal(isVerification bool, onSuccess, onFailure, onCancel func()) {
	onSuccessWrapped := func() { a.navManager.CloseModal(); onSuccess() }
	onFailureWrapped := func() { a.navManager.CloseModal(); onFailure() }
	onCancelWrapped := func() { a.navManager.CloseModal(); onCancel() }

	testPage := pages.NewTestPage(a, isVerification, onSuccessWrapped, onFailureWrapped, onCancelWrapped)
	a.navManager.ShowModal(tui.PageTestID, testPage)
}

func (a *App) CloseModal() {
	a.navManager.CloseModal()
}

func (a *App) RefreshSearchState() {
	if obs, ok := a.navManager.GetCurrentPage(true).(tui.SearchStateObserver); ok {
		go func() {
			defer logging.HandlePanic()
			a.QueueUpdateDraw(func() {
				obs.RefreshSearchState()
			})
		}()
	}
}

// setupGlobalInputCapture defines application-wide keybindings.
func (a *App) setupGlobalInputCapture() {
	a.tviewApp.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		if event.Key() == tcell.KeyTab {
			if a.focusManager.Cycle(a.navManager.GetCurrentPage(true), true) {
				return nil
			}
		}
		if event.Key() == tcell.KeyBacktab {
			if a.focusManager.Cycle(a.navManager.GetCurrentPage(true), false) {
				return nil
			}
		}

		if event.Modifiers()&tcell.ModCtrl != 0 {
			switch event.Key() {
			case tcell.KeyCtrlL:
				if a.navManager.GetCurrentPageID(true) != tui.PageLogID {
					a.navManager.ShowModal(tui.PageLogID, a.logPage)
					return nil
				}
			case tcell.KeyCtrlC:
				go func() {
					defer logging.HandlePanic()
					a.QueueUpdateDraw(a.dialogManager.ShowQuitDialog)
				}()
				return nil
			case tcell.KeyCtrlH, tcell.KeyDEL: // For some fucked up reason Ctrl+H is sent as DEL in some terminals
				if a.navManager.GetCurrentPageID(true) != tui.PageHistoryID {
					a.navManager.ShowModal(tui.PageHistoryID, a.historyPage)
					return nil
				}
			}
		}
		return event
	})
}
