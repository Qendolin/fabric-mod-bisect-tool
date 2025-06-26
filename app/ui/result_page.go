package ui

import (
	"fmt"
	"strings"
	"time"

	"github.com/Qendolin/fabric-mod-bisect-tool/app/core/conflict"
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

const PageResultID = "result_page"

// ResultPage displays the final or intermediate results of the bisection search.
type ResultPage struct {
	*tview.Flex
	app        AppInterface
	statusText *tview.TextView
}

// NewResultPage creates a new ResultPage.
func NewResultPage(app AppInterface, state conflict.SearchSnapshot) *ResultPage {
	p := &ResultPage{
		Flex:       tview.NewFlex().SetDirection(tview.FlexRow),
		app:        app,
		statusText: tview.NewTextView().SetDynamicColors(true),
	}

	title, message, explanation := p.formatContent(state)

	textView := tview.NewTextView().
		SetDynamicColors(true).
		SetText(message)
	textView.SetBorderPadding(0, 0, 1, 1)

	explanationView := tview.NewTextView().
		SetDynamicColors(true).
		SetText(explanation)
	explanationView.SetBorderPadding(0, 0, 1, 1)

	messageFrame := NewTitleFrame(textView, "Result")
	explanationFrame := NewTitleFrame(explanationView, "What to do next")

	closeButton := tview.NewButton("Close").
		SetSelectedFunc(func() {
			p.app.Navigation().CloseModal()
		})
	closeButton.SetDisabled(true)
	DefaultStyleButton(closeButton)

	// prevent accidental input
	go func() {
		time.Sleep(300 * time.Millisecond)
		p.app.QueueUpdateDraw(func() {
			closeButton.SetDisabled(false)
		})
	}()

	buttonLayout := tview.NewFlex().
		AddItem(tview.NewBox(), 0, 1, false).
		AddItem(closeButton, 15, 1, true).
		AddItem(tview.NewBox(), 0, 1, false)

	p.AddItem(messageFrame, 0, 1, false).
		AddItem(explanationFrame, 0, 1, false).
		AddItem(tview.NewBox(), 0, 1, false).
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
func (p *ResultPage) formatContent(state conflict.SearchSnapshot) (title, message, explanation string) {
	searcher := p.app.GetSearcher()
	modState := p.app.GetModState()
	mods := modState.GetAllMods()

	conflictMods := mapKeysFromStruct(state.ConflictSet)
	var conflictModsList []string
	for _, id := range conflictMods {
		modInfo := ""
		if mod, ok := mods[id]; ok {
			modInfo = fmt.Sprintf("(%s %s) in '%s.jar'", mod.FriendlyName(), mod.FabricInfo.Version, mod.BaseFilename)
		}
		conflictModsList = append(conflictModsList, fmt.Sprintf(" - [red::b]%s[-:-:-] %s", id, modInfo))
	}

	if searcher.IsComplete() {
		title = "Search Complete"
		if len(state.ConflictSet) > 0 {
			conflictMods := mapKeysFromStruct(state.ConflictSet)
			message = fmt.Sprintf("\nFound [yellow::b]%d[-:-:-] problematic mod(s):\n\n%s", len(conflictMods), strings.Join(conflictModsList, "\n"))
			explanation = "\n- Try disabling just these mods and launching the game to confirm.\n- Report the incompatibility to the mod authors."
		} else {
			message = "\nNo conflict was found."
			explanation = "\nThe bisection process completed without isolating a specific cause for failure."
		}
	} else if searcher.LastFoundElement() != "" {
		title = "Intermediate Result"
		message = fmt.Sprintf("\nFound [yellow::b]%d[-:-:-] problematic mod(s) so far:\n\n%s", len(conflictMods), strings.Join(conflictModsList, "\n"))
		explanation = "\nThe last test indicated that more mods are required to trigger the conflict.\n\nPress '[::b]S[-:-:-]' on the main page to continue searching for the next one."
	}
	return
}

// GetActionPrompts returns the key actions for the page.
func (p *ResultPage) GetActionPrompts() map[string]string {
	return map[string]string{
		"Enter/ESC": "Close",
	}
}

// GetStatusPrimitive returns the tview.Primitive that displays the page's status
func (p *ResultPage) GetStatusPrimitive() *tview.TextView {
	return p.statusText
}
