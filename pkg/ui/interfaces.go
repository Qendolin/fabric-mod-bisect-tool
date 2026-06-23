package ui

import (
	"github.com/Qendolin/fabric-mod-bisect-tool/pkg/core/mods"
)

// Controller defines the business logic operations that the UI can invoke.
type Controller interface {
	StartLoadingProcess(modsPath string, quiltSupport, neoForgeSupport bool)
	GetViewModel() BisectionViewModel
	GetStateManager() *mods.StateManager

	Step()
	Undo() bool
	ResetSearch()
	ContinueSearch()
	Reconcile(callback func())
	IsBisectionReady() bool
}

// View defines the operations that the business logic can request from the UI.
type View interface {
	Run() error
	Stop()
	QueueUpdateDraw(f func())

	// Dialogs
	ShowErrorDialog(title, message string, err error, callback func())
	ShowInfoDialog(title, message, details string, callback func())
	ShowQuestionDialog(title, message, details string, onYes, onNo func())
	ShowQuitDialog()

	// Navigation / UI State Updates
	SwitchToSetupPage()
	SwitchToLoadingPage()
	UpdateLoadingProgress(fileName string, i, count int)
	SwitchToMainPage()
	SwitchToResultPage()
	ShowTestModal(isVerification bool, onSuccess, onFailure, onCancel func())
	CloseModal()
	RefreshSearchState()
}
