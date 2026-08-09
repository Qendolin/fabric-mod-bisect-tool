package guiapp

import (
	"github.com/Qendolin/fabric-mod-bisect-tool/pkg/gui/screens"
	"github.com/Qendolin/fabric-mod-bisect-tool/pkg/ui"
)

func (a *App) Update() {
	a.window.Invalidate()
}

func (a *App) OnLoadingStarted() {
	a.SetActiveScreen(a.loadingScreen)
}

func (a *App) OnLoadingProgress(fileName string, i int, count int) {
	a.loadingScreen.UpdateProgress(fileName, i, count)
	a.Update()
}

func (a *App) OnBisectionReady() {
	a.SetActiveScreen(a.modSelectionScreen)
}

func (a *App) OnUnresolvableMods(mods []ui.UnresolvableModInfo) {
	a.Run(func() {
		a.SetActiveScreen(screens.NewUnresolvableScreen(a, mods))
	})
}

func (a *App) OnTestReady() {
	a.Run(func() {
		a.mainScreen.ShowTestPrompt()
		a.Update()
	})
}

func (a *App) OnIterationComplete() {
	a.SetActiveScreen(a.resultScreen)
}
