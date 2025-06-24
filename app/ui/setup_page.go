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
	app AppInterface
}

// NewSetupPage creates a new SetupPage instance.
func NewSetupPage(app AppInterface) Page {
	page := &SetupPage{
		Flex: tview.NewFlex().SetDirection(tview.FlexRow),
		app:  app,
	}

	form := tview.NewForm().
		SetLabelColor(tcell.ColorBlue).
		SetButtonActivatedStyle(tcell.StyleDefault.Background(tcell.ColorGreen).Foreground(tcell.ColorBlack))

	form.AddInputField("Mods Path", "", 60, nil, nil)
	form.AddButton("Load Mods", func() {
		modsPath := form.GetFormItemByLabel("Mods Path").(*tview.InputField).GetText()
		modsPath = filepath.Clean(modsPath)
		if modsPath == "" {
			app.ShowErrorDialog("Error", "Mods path cannot be empty.", nil)
			return
		}
		// Delegate the loading process to the App, which will show the loading page
		app.StartModLoad(modsPath)
	})

	form.AddButton("Load Saved State", func() {
		app.SetPageStatus("Loading saved state (not implemented yet)...")
	}).AddButton("Quit", func() {
		go app.QueueUpdateDraw(app.ShowQuitDialog)
	})

	formWrapper := NewTitleFrame(form, "Setup")

	// Instructions and info section
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

	page.AddItem(formWrapper, 0, 1, true).
		AddItem(infoWrapper, 0, 1, false)

	app.SetPageStatus("Enter mods path and load, or load a saved state.")
	return page
}

// Primitive returns the underlying tview.Primitive for this page.
func (p *SetupPage) Primitive() tview.Primitive {
	return p.Flex
}

// GetActionPrompts returns the key actions for the setup page.
func (p *SetupPage) GetActionPrompts() map[string]string {
	return map[string]string{
		"Tab":   "Next Field",
		"Enter": "Activate Button",
	}
}
