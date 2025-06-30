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
				m.app.Navigation().CloseModal()
				if onDismiss != nil {
					onDismiss()
				}
			})
		})
	modal.SetTextColor(tcell.ColorWhite).
		SetTitleColor(tcell.ColorWhite).
		SetBackgroundColor(tcell.ColorDarkRed).
		SetBorderColor(tcell.ColorWhite)
	modal.Box.SetBackgroundColor(tcell.ColorDarkRed)
	modal.SetTitle(" " + title + " ").SetTitleAlign(tview.AlignLeft)
	m.app.Navigation().ShowModal("error_dialog", NewModalPage(modal))
}

// ShowQuitDialog displays a confirmation dialog before quitting.
func (m *DialogManager) ShowQuitDialog() {
	modal := tview.NewModal().
		SetText("Are you sure you want to quit?").
		AddButtons([]string{"Cancel", "Quit"}).
		SetDoneFunc(func(buttonIndex int, buttonLabel string) {
			go m.app.QueueUpdateDraw(func() {
				m.app.Navigation().CloseModal()
				switch buttonIndex {
				case 1:
					logging.Info("App: Quitting.")
					m.app.Stop()
				case 0:
				}
			})
		})
	modal.SetTextColor(tcell.ColorBlack).
		SetTitleColor(tcell.ColorBlack).
		SetBorderColor(tcell.ColorWhite)
	modal.SetTitle(" Quit ").SetTitleAlign(tview.AlignLeft)
	m.app.Navigation().ShowModal("quit_dialog", NewModalPage(modal))
}

func (m *DialogManager) ShowQuestionDialog(question string, onYes func(), onNo func()) {
	modal := tview.NewModal().
		SetText(question).
		AddButtons([]string{"No", "Yes"}).
		SetDoneFunc(func(buttonIndex int, buttonLabel string) {
			go m.app.QueueUpdateDraw(func() {
				m.app.Navigation().CloseModal()
				switch buttonLabel {
				case "Yes":
					if onYes != nil {
						onYes()
					}
				case "No":
					if onNo != nil {
						onNo()
					}
				}
			})
		})
	modal.SetTextColor(tcell.ColorBlack).
		SetTitleColor(tcell.ColorBlack).
		SetBorderColor(tcell.ColorWhite)
	modal.SetTitle(" Confirm ").SetTitleAlign(tview.AlignLeft)
	m.app.Navigation().ShowModal("yes_no_dialog", NewModalPage(modal))
}

// ShowInfoDialog displays a modal dialog with a neutral informational message.
func (m *DialogManager) ShowInfoDialog(title, message string, onDismiss func()) {
	modal := tview.NewModal().
		SetText(message).
		AddButtons([]string{"Dismiss"}).
		SetDoneFunc(func(buttonIndex int, buttonLabel string) {
			go m.app.QueueUpdateDraw(func() {
				m.app.Navigation().CloseModal()
				if onDismiss != nil {
					onDismiss()
				}
			})
		})
	modal.SetTextColor(tcell.ColorBlack).
		SetTitleColor(tcell.ColorBlack).
		SetBorderColor(tcell.ColorWhite)
	modal.SetTitle(" " + title + " ").SetTitleAlign(tview.AlignLeft)
	m.app.Navigation().ShowModal("info_dialog", NewModalPage(modal))
}

// ModalPage is a simple wrapper around a tview.Modal to conform to the Page interface.
type ModalPage struct {
	*tview.Modal
}

// NewModalPage creates a new ModalPage.
func NewModalPage(modal *tview.Modal) *ModalPage {
	return &ModalPage{Modal: modal}
}

// GetActionPrompts returns an empty map as modals have their own buttons.
func (p *ModalPage) GetActionPrompts() []ActionPrompt {
	return []ActionPrompt{}
}

// GetStatusPrimitive returns the tview.Primitive that displays the page's status
func (p *ModalPage) GetStatusPrimitive() *tview.TextView {
	return nil
}
