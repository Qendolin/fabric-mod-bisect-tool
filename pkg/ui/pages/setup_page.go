package pages

import (
	"fmt"
	"path/filepath"
	"runtime/debug"
	"strings"

	"github.com/Qendolin/fabric-mod-bisect-tool/pkg/ui"
	"github.com/Qendolin/fabric-mod-bisect-tool/pkg/ui/widgets"
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

// PageSetupID is the unique identifier for the SetupPage.
const PageSetupID = "setup_page"

// SetupPage represents the initial setup screen.
type SetupPage struct {
	*tview.Flex
	app        ui.AppInterface
	statusText *tview.TextView

	inputField    *tview.InputField
	quiltCheckbox *tview.Checkbox
	loadButton    *tview.Button
	quitButton    *tview.Button
}

// NewSetupPage creates a new SetupPage instance.
func NewSetupPage(app ui.AppInterface) *SetupPage {
	p := &SetupPage{
		Flex:       tview.NewFlex().SetDirection(tview.FlexRow),
		app:        app,
		statusText: tview.NewTextView().SetDynamicColors(true),
	}

	vm := app.GetViewModel()

	p.inputField = tview.NewInputField().
		SetLabel("Mods Directory Path: ").
		SetFieldWidth(0)
	p.inputField.SetPlaceholder("C:\\Users\\Example\\.minecraft\\mods").
		SetFieldTextColor(tcell.ColorBlack).
		SetPlaceholderTextColor(tcell.ColorGray)
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

	p.quiltCheckbox = tview.NewCheckbox().SetLabel("Quilt Support: ")
	p.quiltCheckbox.SetChecked(vm.QuiltSupport).
		SetCheckedString("[green]Yes[-]").
		SetUncheckedString("[red]No[-]").
		SetActivatedStyle(tcell.StyleDefault.Background(tcell.ColorBlue)).
		SetFieldBackgroundColor(tview.Styles.PrimitiveBackgroundColor)

	p.loadButton = tview.NewButton("Load Mods").SetSelectedFunc(func() {
		cleaned := strings.TrimSpace(p.inputField.GetText())
		cleaned = strings.TrimPrefix(cleaned, "\"")
		cleaned = strings.TrimSuffix(cleaned, "\"")
		cleaned = strings.TrimSpace(cleaned)
		if cleaned == "" {
			app.Dialogs().ShowErrorDialog("Error", "The mods path cannot be empty.", nil, nil)
			return
		}
		app.StartLoadingProcess(filepath.Clean(cleaned), p.quiltCheckbox.IsChecked())
	})
	widgets.DefaultStyleButton(p.loadButton)

	p.quitButton = tview.NewButton("Quit").SetSelectedFunc(func() {
		go app.QueueUpdateDraw(func() { app.Dialogs().ShowQuitDialog() })
	})
	widgets.DefaultStyleButton(p.quitButton)

	buttonsFlex := tview.NewFlex().
		SetDirection(tview.FlexColumn).
		AddItem(p.loadButton, 30, 0, true).
		AddItem(nil, 0, 1, false).
		AddItem(p.quitButton, 30, 0, true)

	setupFlex := tview.NewFlex().
		SetDirection(tview.FlexRow).
		AddItem(p.inputField, 1, 0, true).
		AddItem(p.quiltCheckbox, 1, 0, false).
		AddItem(nil, 1, 0, false).
		AddItem(buttonsFlex, 3, 0, false)
	setupFlex.SetBorderPadding(1, 1, 1, 1)

	buildTime := ""
	buildRevision := ""
	buildInfo := "Unknown"
	if info, ok := debug.ReadBuildInfo(); ok {
		for _, setting := range info.Settings {
			if setting.Key == "vcs.time" {
				buildTime = setting.Value
			}
			if setting.Key == "vcs.revision" {
				buildRevision = setting.Value
			}
		}
	}

	if buildTime != "" {
		buildInfo = buildTime
	}
	if buildRevision != "" {
		if buildTime != "" {
			buildInfo += " - "
		}
		buildInfo += buildRevision
	}

	instructions := tview.NewTextView().
		SetDynamicColors(true).
		SetText(fmt.Sprintf(`
[::b]Instructions:[-:-:-]
  - Enter the path to your Minecraft mods folder.
  - Paste path: [darkcyan::b]Ctrl+V[-:-:-] or [darkcyan::b]Right Click[-:-:-] (in most terminals).

[::b]Tool Information:[-:-:-]
  - Build: %s
  - Author: Qendolin
  - Source: https://github.com/Qendolin/fabric-mod-bisect-tool
`, buildInfo))
	instructions.SetBorderPadding(0, 0, 1, 1)

	p.AddItem(widgets.NewTitleFrame(setupFlex, "Setup"), 9, 0, true).
		AddItem(widgets.NewTitleFrame(instructions, "Info"), 0, 1, false)

	p.statusText.SetText("Welcome to the Fabric Mod Bisect Tool by Qendolin! Paste the path to your 'mods' directory below.")

	return p
}

// GetActionPrompts returns the key actions for the setup page.
func (p *SetupPage) GetActionPrompts() []ui.ActionPrompt {
	return []ui.ActionPrompt{}
}

// GetStatusPrimitive returns the tview.Primitive that displays the page's status
func (p *SetupPage) GetStatusPrimitive() *tview.TextView {
	return p.statusText
}

func (p *SetupPage) GetFocusablePrimitives() []tview.Primitive {
	return []tview.Primitive{
		p.inputField,
		p.quiltCheckbox,
		p.loadButton,
		p.quitButton,
	}
}
