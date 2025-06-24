package ui

import (
	"github.com/Qendolin/fabric-mod-bisect-tool/app/logging"
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

type DialogManager struct {
	app AppInterface
}

func NewDialogManager(app AppInterface) *DialogManager {
	return &DialogManager{app: app}
}

// ShowErrorDialog displays a modal dialog with an error message.
func (m *DialogManager) ShowErrorDialog(title, message string, onDismiss func()) {
	modal := tview.NewModal().
		SetText(message).
		AddButtons([]string{"Dismiss"}).
		SetDoneFunc(func(buttonIndex int, buttonLabel string) {
			go m.app.QueueUpdateDraw(func() {
				m.app.Navigation().PopPage()
				if onDismiss != nil {
					onDismiss()
				}
			})
		})
	modal.SetBorderColor(tcell.ColorRed).SetTitle(" " + title + " ").SetTitleAlign(tview.AlignLeft)
	m.app.Navigation().PushPage("error_dialog", NewModalPage(modal))
}

// ShowQuitDialog displays a confirmation dialog before quitting.
func (m *DialogManager) ShowQuitDialog() {
	modal := tview.NewModal().
		SetText("Are you sure you want to quit?").
		AddButtons([]string{"Cancel", "Quit without Saving", "Quit and Save"}).
		SetDoneFunc(func(buttonIndex int, buttonLabel string) {
			go m.app.QueueUpdateDraw(func() {
				m.app.Navigation().PopPage()
				switch buttonLabel {
				case "Quit and Save":
					logging.Info("App: Quitting and saving state (not implemented yet).")
					m.app.Stop()
				case "Quit without Saving":
					logging.Info("App: Quitting without saving state.")
					m.app.Stop()
				case "Cancel":
				}
			})
		})
	m.app.Navigation().PushPage("quit_dialog", NewModalPage(modal))
}
