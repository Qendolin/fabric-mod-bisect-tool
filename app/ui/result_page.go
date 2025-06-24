package ui

import (
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
func NewResultPage(app AppInterface, title, message, explanation string) Page {
	p := &ResultPage{
		Flex:       tview.NewFlex().SetDirection(tview.FlexRow),
		app:        app,
		statusText: tview.NewTextView().SetDynamicColors(true),
	}

	textView := tview.NewTextView().
		SetDynamicColors(true).
		SetText(message + "\n\n" + explanation)

	frame := NewTitleFrame(textView, title)

	form := tview.NewForm().
		AddButton("Close", func() {
			app.Navigation().PopPage()
		}).
		SetButtonsAlign(tview.AlignCenter)

	p.AddItem(frame, 0, 1, false).
		AddItem(form, 3, 0, true)

	p.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		if event.Key() == tcell.KeyEscape || event.Key() == tcell.KeyEnter {
			app.Navigation().PopPage()
			return nil
		}
		return event
	})

	p.statusText.SetText("Showing Bisection result information")

	return p
}

// Primitive returns the underlying tview.Primitive.
func (p *ResultPage) Primitive() tview.Primitive {
	return p.Flex
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
