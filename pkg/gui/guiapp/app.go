package guiapp

import (
	"context"
	"fmt"
	"sync"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/dialog"
	"github.com/Qendolin/fabric-mod-bisect-tool/pkg/gui/screens"
	"github.com/Qendolin/fabric-mod-bisect-tool/pkg/logging"
	"github.com/Qendolin/fabric-mod-bisect-tool/pkg/ui"
)

// App is the GUI implementation of ui.View using Fyne.
type App struct {
	ui.Controller
	fyneApp    fyne.App
	mainWindow fyne.Window
	logger     *logging.Logger

	// Screens
	setupScreen   *screens.SetupScreen
	loadingScreen *screens.LoadingScreen
	mainScreen    *screens.MainScreen
	resultScreen  *screens.ResultScreen

	appCtx    context.Context
	cancelApp context.CancelFunc

	shutdownWg sync.WaitGroup
}

func NewApp(controller ui.Controller, logger *logging.Logger) *App {
	fyneApp := app.NewWithID("fabric-mod-bisect-tool")
	mainWindow := fyneApp.NewWindow("Mod Bisect Tool")
	mainWindow.Resize(fyne.NewSize(800, 600))

	appCtx, cancelApp := context.WithCancel(context.Background())

	a := &App{
		Controller: controller,
		fyneApp:    fyneApp,
		mainWindow: mainWindow,
		logger:     logger,
		appCtx:     appCtx,
		cancelApp:  cancelApp,
	}

	a.fyneApp.Settings().SetTheme(&CustomTheme{})

	a.setupScreen = screens.NewSetupScreen(a)
	a.loadingScreen = screens.NewLoadingScreen(a)
	a.mainScreen = screens.NewMainScreen(a)
	a.resultScreen = screens.NewResultScreen(a)

	a.mainWindow.SetCloseIntercept(func() {
		a.ShowQuitDialog()
	})

	return a
}

// GetWindow returns the Fyne window
func (a *App) GetWindow() fyne.Window {
	return a.mainWindow
}

// NewWindow returns a new Fyne window
func (a *App) NewWindow(title string) fyne.Window {
	return a.fyneApp.NewWindow(title)
}

// --- ui.View Interface implementation ---
func (a *App) Run() error {
	a.SwitchToSetupPage()
	a.mainWindow.ShowAndRun()
	return nil
}

func (a *App) Stop() {
	a.cancelApp()
	a.shutdownWg.Wait()
	a.fyneApp.Quit()
}

func (a *App) QueueUpdateDraw(f func()) {
	fyne.Do(f)
}

func (a *App) ShowErrorDialog(title, message string, err error, callback func()) {
	fullMsg := message
	if err != nil {
		fullMsg = message + "\n\nError: " + err.Error()
	}
	d := dialog.NewError(fmt.Errorf("%s", fullMsg), a.mainWindow)
	d.SetOnClosed(func() {
		if callback != nil {
			callback()
		}
	})
	d.Show()
}

func (a *App) ShowInfoDialog(title, message, details string, callback func()) {
	fullMsg := message
	if details != "" {
		fullMsg += "\n\n" + details
	}
	d := dialog.NewInformation(title, fullMsg, a.mainWindow)
	d.SetOnClosed(func() {
		if callback != nil {
			callback()
		}
	})
	d.Show()
}

func (a *App) ShowQuestionDialog(title, message, details string, onYes, onNo func()) {
	fullMsg := message
	if details != "" {
		fullMsg += "\n\n" + details
	}
	d := dialog.NewConfirm(title, fullMsg, func(yes bool) {
		if yes && onYes != nil {
			onYes()
		} else if !yes && onNo != nil {
			onNo()
		}
	}, a.mainWindow)
	d.Show()
}

func (a *App) ShowQuitDialog() {
	dialog.ShowConfirm("Quit", "Are you sure you want to quit?\nUnsaved progress will be lost.", func(yes bool) {
		if yes {
			a.Stop()
		}
	}, a.mainWindow)
}

func (a *App) SwitchToSetupPage() {
	a.mainWindow.SetContent(a.setupScreen.GetContent())
}

func (a *App) SwitchToLoadingPage() {
	a.mainWindow.SetContent(a.loadingScreen.GetContent())
}

func (a *App) UpdateLoadingProgress(fileName string, i, count int) {
	a.loadingScreen.UpdateProgress(fileName, i, count)
}

func (a *App) SwitchToMainPage() {
	fyne.Do(func() {
		a.mainScreen.UpdateState()
		a.mainWindow.SetContent(a.mainScreen.GetContent())
	})
}

func (a *App) SwitchToResultPage() {
	fyne.Do(func() {
		a.resultScreen.UpdateState()
		a.mainWindow.SetContent(a.resultScreen.GetContent())
	})
}

func (a *App) ShowTestModal(isVerification bool, onSuccess, onFailure, onCancel func()) {
	a.mainScreen.ShowTestPrompt(isVerification, onSuccess, onFailure, onCancel)
}

func (a *App) CloseModal() {
	a.mainScreen.HideTestPrompt()
}

func (a *App) RefreshSearchState() {
	a.mainScreen.UpdateState()
}
