package ui

import (
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

// PageLogID is the unique identifier for the LogPage.
const PageLogID = "log_page"

// LogPage is a wrapper around a TextView to conform to the Page interface.
type LogPage struct {
	*tview.Flex
	app        AppInterface
	statusText *tview.TextView
}

// NewLogPage creates a new LogPage instance.
func NewLogPage(app AppInterface) Page {
	logView := app.GetLogTextView()
	if logView == nil {
		logView = tview.NewTextView().SetText("Error: Log view not initialized.")
	}

	wrapper := tview.NewFlex().SetDirection(tview.FlexRow)
	frame := NewTitleFrame(logView, "Log")
	wrapper.AddItem(frame, 0, 1, true)

	page := &LogPage{
		Flex:       wrapper,
		app:        app,
		statusText: tview.NewTextView().SetDynamicColors(true),
	}

	wrapper.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		switch event.Key() {
		case tcell.KeyEscape, tcell.KeyCtrlL:
			go app.QueueUpdateDraw(app.Navigation().PopPage)
			return nil
		}
		return event
	})

	page.statusText.SetText("Viewing application logs...")
	return page
}

// Primitive returns the underlying tview.Primitive.
func (p *LogPage) Primitive() tview.Primitive {
	return p
}

// GetActionPrompts returns the key actions for the log page.
func (p *LogPage) GetActionPrompts() map[string]string {
	return map[string]string{
		"ESC/Ctrl+L": "Close Log",
	}
}

// GetStatusPrimitive returns the tview.Primitive that displays the page's status
func (p *LogPage) GetStatusPrimitive() *tview.TextView {
	return p.statusText
}
