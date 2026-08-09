package guiapp

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

func (a *App) OnTestReady() {
	a.Run(func() {
		a.mainScreen.ShowTestPrompt()
		a.Update()
	})
}

func (a *App) OnIterationComplete() {
	a.SetActiveScreen(a.resultScreen)
}
