package ui

import (
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

// PageLogID is the unique identifier for the LogPage.
const PageLogID = "log_page"

// LogPage represents the full-screen log viewer.
type LogPage struct {
	*tview.Flex
	app AppInterface
}

// NewLogPage creates a new LogPage instance.
func NewLogPage(app AppInterface) Page {
	logView := app.GetLogTextView()
	if logView == nil {
		// This should not happen if app is initialized correctly.
		logView = tview.NewTextView().SetText("Error: Log view not initialized.")
	}

	wrapper := tview.NewFlex().SetDirection(tview.FlexRow)
	wrapper.AddItem(NewTitleFrame(logView, "Log"), 0, 1, true) // Let TextView fill the space

	page := &LogPage{
		Flex: wrapper,
		app:  app,
	}

	wrapper.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		switch event.Key() {
		case tcell.KeyEscape, tcell.KeyCtrlL:
			go app.QueueUpdateDraw(app.PopPage)
			return nil
		}
		return event
	})

	app.SetPageStatus("Viewing application logs...")
	return page
}

// Primitive returns the underlying tview.Primitive.
func (p *LogPage) Primitive() tview.Primitive {
	return p.Flex
}

// GetActionPrompts returns the key actions for the log page.
func (p *LogPage) GetActionPrompts() map[string]string {
	return map[string]string{
		"ESC/Ctrl+L": "Close Log",
	}
}
