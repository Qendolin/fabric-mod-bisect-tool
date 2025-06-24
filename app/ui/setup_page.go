package ui

import (
	"path/filepath"

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
}

// NewSetupPage creates a new SetupPage instance.
func NewSetupPage(app AppInterface) Page {
	page := &SetupPage{
		Flex:       tview.NewFlex().SetDirection(tview.FlexRow),
		app:        app,
		statusText: tview.NewTextView().SetDynamicColors(true),
	}

	form := tview.NewForm().
		SetLabelColor(tcell.ColorBlue).
		SetButtonActivatedStyle(tcell.StyleDefault.Background(tcell.ColorGreen).Foreground(tcell.ColorBlack))

	form.AddInputField("Mods Path", "", 60, nil, nil)
	form.AddButton("Load Mods", func() {
		modsPath := form.GetFormItemByLabel("Mods Path").(*tview.InputField).GetText()
		modsPath = filepath.Clean(modsPath)
		if modsPath == "" {
			app.Dialogs().ShowErrorDialog("Error", "Mods path cannot be empty.", nil)
			return
		}
		app.StartModLoad(modsPath)
	})

	form.AddButton("Load Saved State", func() {
		page.statusText.SetText("Loading saved state (not implemented yet)...")
	}).AddButton("Quit", func() {
		go app.Navigation().QueueUpdateDraw(func() { app.Dialogs().ShowQuitDialog() })
	})

	formWrapper := NewTitleFrame(form, "Setup")

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

	infoWrapper := NewTitleFrame(instructions, "Info")

	page.AddItem(formWrapper, 6, 0, true).
		AddItem(infoWrapper, 0, 1, false)

	return page
}

// Primitive returns the underlying tview.Primitive for this page.
func (p *SetupPage) Primitive() tview.Primitive {
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
