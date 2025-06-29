package ui

import (
	"path/filepath"
	"strings"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

// PageSetupID is the unique identifier for the SetupPage.
const PageSetupID = "setup_page"

// SetupPage represents the initial setup screen.
type SetupPage struct {
	*tview.Flex
	app        AppInterface
	statusText *tview.TextView

	inputField      *tview.InputField
	loadButton      *tview.Button
	loadStateButton *tview.Button
	quitButton      *tview.Button
}

// NewSetupPage creates a new SetupPage instance.
func NewSetupPage(app AppInterface) *SetupPage {
	p := &SetupPage{
		Flex:       tview.NewFlex().SetDirection(tview.FlexRow),
		app:        app,
		statusText: tview.NewTextView().SetDynamicColors(true),
	}

	p.inputField = tview.NewInputField().
		SetLabel("Mods Path: ").
		SetFieldWidth(0)
	p.inputField.SetFocusFunc(func() {
		p.inputField.SetFieldBackgroundColor(tcell.ColorBlue)
	})
	p.inputField.SetBlurFunc(func() {
		p.inputField.SetFieldBackgroundColor(tcell.ColorSlateGray)
	})
	p.inputField.SetDoneFunc(func(key tcell.Key) {
		if key == tcell.KeyEnter {
			p.app.SetFocus(p.loadButton)
		}
	})

	p.loadButton = tview.NewButton("Load Mods").SetSelectedFunc(func() {
		if strings.TrimSpace(p.inputField.GetText()) == "" {
			app.Dialogs().ShowErrorDialog("Error", "Mods path cannot be empty.", nil)
			return
		}
		app.StartLoadingProcess(filepath.Clean(p.inputField.GetText()))
	})

	DefaultStyleButton(p.loadButton)

	p.loadStateButton = tview.NewButton("Load Saved State").SetSelectedFunc(func() {
		p.statusText.SetText("Loading saved state (not implemented yet)...")
	})
	DefaultStyleButton(p.loadStateButton)

	p.quitButton = tview.NewButton("Quit").SetSelectedFunc(func() {
		go app.QueueUpdateDraw(func() { app.Dialogs().ShowQuitDialog() })
	})
	DefaultStyleButton(p.quitButton)

	buttonsFlex := tview.NewFlex().
		SetDirection(tview.FlexColumn).
		AddItem(p.loadButton, 30, 0, true).
		AddItem(nil, 1, 0, false).
		AddItem(p.loadStateButton, 30, 0, true).
		AddItem(nil, 0, 1, false).
		AddItem(p.quitButton, 30, 0, true)

	setupFlex := tview.NewFlex().
		SetDirection(tview.FlexRow).
		AddItem(p.inputField, 1, 0, true).
		AddItem(nil, 1, 0, false).
		AddItem(buttonsFlex, 3, 0, false)
	setupFlex.SetBorderPadding(1, 1, 1, 1)

	instructions := tview.NewTextView().
		SetDynamicColors(true).
		SetText(`
[::b]Instructions:[-:-:-]
  - Enter the path to your Minecraft mods folder.
  - Paste path: [darkcyan::b]Ctrl+V[-:-:-] or [darkcyan::b]Right Click[-:-:-] (in most terminals).

[::b]Tool Information:[-:-:-]
  - Version: 0.1.0
  - Author: Qendolin
  - License: MIT
`)
	instructions.SetBorderPadding(0, 0, 1, 1)

	p.AddItem(NewTitleFrame(setupFlex, "Setup"), 8, 0, true).
		AddItem(NewTitleFrame(instructions, "Info"), 0, 1, false)

	return p
}

// GetActionPrompts returns the key actions for the setup page.
func (p *SetupPage) GetActionPrompts() map[string]string {
	return map[string]string{
		"Tab":   "Next Field",
		"Enter": "Activate Button",
	}
}

// GetStatusPrimitive returns the tview.Primitive that displays the page's status
func (p *SetupPage) GetStatusPrimitive() *tview.TextView {
	return p.statusText
}

func (p *SetupPage) GetFocusablePrimitives() []tview.Primitive {
	return []tview.Primitive{
		p.inputField,
		p.loadButton,
		p.loadStateButton,
		p.quitButton,
	}
}
