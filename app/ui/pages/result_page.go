package pages

import (
	"fmt"
	"strings"

	"github.com/Qendolin/fabric-mod-bisect-tool/app/core/imcs"
	"github.com/Qendolin/fabric-mod-bisect-tool/app/core/sets"
	"github.com/Qendolin/fabric-mod-bisect-tool/app/ui"
	"github.com/Qendolin/fabric-mod-bisect-tool/app/ui/widgets"
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

const PageResultID = "result_page"

// ResultPage displays the final or intermediate results of the bisection search.
type ResultPage struct {
	*tview.Flex
	app         ui.AppInterface
	statusText  *tview.TextView
	resultView  *tview.TextView
	closeButton *tview.Button
}

// NewResultPage creates a new ResultPage.
func NewResultPage(app ui.AppInterface, state imcs.SearchState, dependers sets.Set) *ResultPage {
	p := &ResultPage{
		Flex:       tview.NewFlex().SetDirection(tview.FlexRow),
		app:        app,
		statusText: tview.NewTextView().SetDynamicColors(true),
	}

	title, message, explanation := p.formatContent(state, sets.MakeSlice(dependers))

	p.resultView = tview.NewTextView().
		SetDynamicColors(true).
		SetWordWrap(true).
		SetText(message)
	p.resultView.SetBorderPadding(1, 0, 1, 1)

	explanationView := tview.NewTextView().
		SetDynamicColors(true).
		SetText(explanation)
	explanationView.SetBorderPadding(1, 1, 1, 1)

	messageFrame := widgets.NewTitleFrame(p.resultView, "Result")
	explanationFrame := widgets.NewTitleFrame(explanationView, "What to do next")

	p.closeButton = tview.NewButton("Close").
		SetSelectedFunc(func() {
			p.app.Navigation().CloseModal()
		})
	widgets.DefaultStyleButton(p.closeButton)

	buttonLayout := tview.NewFlex().
		AddItem(tview.NewBox(), 0, 1, false).
		AddItem(p.closeButton, 15, 1, true).
		AddItem(tview.NewBox(), 0, 1, false)

	p.AddItem(messageFrame, 0, 2, false).
		AddItem(explanationFrame, 5, 0, false).
		AddItem(buttonLayout, 3, 0, true)

	p.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		if event.Key() == tcell.KeyEscape || event.Key() == tcell.KeyEnter {
			app.Navigation().CloseModal()
			return nil
		}
		return event
	})

	p.statusText.SetText(title)

	return p
}

// formatContent generates the appropriate text based on the search state.
func (p *ResultPage) formatContent(state imcs.SearchState, dependers []string) (title, message, explanation string) {
	modState := p.app.GetStateManager()
	mods := modState.GetAllMods()

	conflictMods := sets.MakeSlice(state.ConflictSet)
	var messageBuilder strings.Builder

	if state.IsComplete {
		title = "Search Complete"
		if len(conflictMods) > 0 {
			messageBuilder.WriteString(fmt.Sprintf("Found [yellow::b]%d[-:-:-] problematic mod(s):\n", len(conflictMods)))
			for _, id := range conflictMods {
				modInfo := ""
				if mod, ok := mods[id]; ok {
					modInfo = fmt.Sprintf("(%s %s) from '%s.jar'", mod.FriendlyName(), mod.FabricInfo.Version, mod.BaseFilename)
				}
				messageBuilder.WriteString(fmt.Sprintf("  - [red::b]%s[-:-:-] %s\n", id, modInfo))
			}

			if len(dependers) > 0 {
				messageBuilder.WriteString(fmt.Sprintf("\nThese %d mod(s) must also be disabled as they depend on the problematic mods:\n", len(dependers)))
				for _, id := range dependers {
					modInfo := ""
					if mod, ok := mods[id]; ok {
						modInfo = fmt.Sprintf("(%s %s) from '%s.jar'", mod.FriendlyName(), mod.FabricInfo.Version, mod.BaseFilename)
					}
					messageBuilder.WriteString(fmt.Sprintf("  - [yellow]%s[-:-:-] %s\n", id, modInfo))
				}
			}
			explanation = "To fix the issue, disable all mods listed above and then relaunch the game.\nOnce confirmed, please report the incompatibility to the mod authors."

		} else {
			messageBuilder.WriteString("No problematic mods were found.")
			explanation = "The bisection process completed without isolating a specific cause for failure."
		}
	} else if state.LastFoundElement != "" {
		title = "Intermediate Result"
		messageBuilder.WriteString(fmt.Sprintf("Found [yellow::b]%d[-:-:-] problematic mod(s) so far:\n", len(conflictMods)))
		for _, id := range conflictMods {
			modInfo := ""
			if mod, ok := mods[id]; ok {
				modInfo = fmt.Sprintf("(%s %s)", mod.FriendlyName(), mod.FabricInfo.Version)
			}
			messageBuilder.WriteString(fmt.Sprintf("  - [red::b]%s[-:-:-] %s\n", id, modInfo))
		}
		explanation = "The last test isolated a new conflict, but there may be more!\nPress '[::b]S[-:-:-]' on the main page to continue the search."
	}

	message = messageBuilder.String()
	return
}

// GetActionPrompts returns the key actions for the page.
func (p *ResultPage) GetActionPrompts() []ui.ActionPrompt {
	return []ui.ActionPrompt{
		{Input: "↑/↓", Action: "Scroll Text"},
	}
}

// GetStatusPrimitive returns the tview.Primitive that displays the page's status
func (p *ResultPage) GetStatusPrimitive() *tview.TextView {
	return p.statusText
}

// GetStatusPrimitive returns the tview.Primitive that displays the page's status
func (p *ResultPage) GetFocusablePrimitives() []tview.Primitive {
	return []tview.Primitive{p.resultView, p.closeButton}
}
