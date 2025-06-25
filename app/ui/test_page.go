package ui

import (
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

const PageTestID = "test_page"

// TestPage instructs the user to perform a manual test.
type TestPage struct {
	*tview.Flex
	app AppInterface

	successBtn *tview.Button
	failBtn    *tview.Button
	backBtn    *tview.Button
	statusText *tview.TextView

	// callbacks
	onSuccess func()
	onFailure func()
	onCancel  func()
}

// NewTestPage creates a new TestPage.
func NewTestPage(app AppInterface, isVerification bool, onSuccess, onFailure, onCancel func()) Page {
	p := &TestPage{
		Flex:       tview.NewFlex(),
		app:        app,
		statusText: tview.NewTextView().SetDynamicColors(true),
		onSuccess:  onSuccess,
		onFailure:  onFailure,
		onCancel:   onCancel,
	}

	p.statusText.SetText("Report Manual Test Outcome")

	message := `
[::b]Mod files have been updated for the next test.

Please launch Minecraft now.

Once the game has loaded (or crashed), report the outcome below.`

	if isVerification {
		p.statusText.SetText("Verify Final Problematic Set")

		message = `
[::b]A potential problematic set has been found.

This test will run with ONLY the suspected problematic mods enabled.

Please launch Minecraft and confirm the failure persists.`
	}

	instructions := tview.NewTextView().
		SetDynamicColors(true).
		SetTextAlign(tview.AlignCenter).
		SetText(message)

	p.successBtn = tview.NewButton("Success (No Crash)").
		SetSelectedFunc(p.onSuccess)
	p.successBtn.SetDisabled(true)
	p.successBtn.SetDisabledStyle(tcell.StyleDefault.Foreground(tcell.ColorLightGray).Background(tcell.ColorDarkGray))
	p.successBtn.SetStyle(tcell.StyleDefault.Foreground(tcell.ColorGreen).Background(tcell.ColorWhite))
	p.successBtn.SetActivatedStyle(tcell.StyleDefault.Foreground(tcell.ColorBlack).Background(tcell.ColorGreen))

	p.failBtn = tview.NewButton("Failure (Crash)").
		SetSelectedFunc(p.onFailure)
	p.failBtn.SetDisabled(true)
	p.failBtn.SetDisabledStyle(tcell.StyleDefault.Foreground(tcell.ColorLightGray).Background(tcell.ColorDarkGray))
	p.failBtn.SetStyle(tcell.StyleDefault.Foreground(tcell.ColorRed).Background(tcell.ColorWhite))
	p.failBtn.SetActivatedStyle(tcell.StyleDefault.Foreground(tcell.ColorBlack).Background(tcell.ColorRed))

	p.backBtn = tview.NewButton("Back (Cancel Step)").
		SetSelectedFunc(p.onCancel)
	p.backBtn.SetDisabled(true)
	p.backBtn.SetDisabledStyle(tcell.StyleDefault.Foreground(tcell.ColorLightGray).Background(tcell.ColorDarkGray))
	p.backBtn.SetStyle(tcell.StyleDefault.Foreground(tcell.ColorBlue).Background(tcell.ColorWhite))
	p.backBtn.SetActivatedStyle(tcell.StyleDefault.Foreground(tcell.ColorBlack).Background(tcell.ColorBlue))

	// prevent accidental input
	go func() {
		time.Sleep(300 * time.Millisecond)
		p.app.QueueUpdateDraw(func() {
			p.successBtn.SetDisabled(false)
			p.failBtn.SetDisabled(false)
			p.backBtn.SetDisabled(false)
		})
	}()

	buttonFlex := tview.NewFlex().
		SetDirection(tview.FlexColumn).
		AddItem(tview.NewBox(), 0, 1, false). // Spacer
		AddItem(p.backBtn, 0, 1, true).
		AddItem(tview.NewBox(), 2, 0, false).
		AddItem(p.successBtn, 0, 1, true).
		AddItem(tview.NewBox(), 2, 0, false).
		AddItem(p.failBtn, 0, 1, true).
		AddItem(tview.NewBox(), 0, 1, false) // Spacer

	p.SetDirection(tview.FlexRow).
		AddItem(NewHorizontalSeparator(tcell.ColorWhite), 1, 0, false).
		AddItem(tview.NewBox(), 0, 1, false).
		AddItem(instructions, 0, 2, false).
		AddItem(buttonFlex, 3, 0, true).
		AddItem(tview.NewBox(), 0, 1, false)

	p.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		if event.Key() == tcell.KeyEscape {
			p.onCancel()
			return nil
		}

		if event.Key() == tcell.KeyRune {
			switch event.Rune() {
			case 'a', 'A':
				p.onSuccess()
				return nil
			case 'd', 'D':
				p.onFailure()
				return nil
			}
		}

		return event
	})

	return p
}

// Primitive returns the underlying tview.Primitive.
func (p *TestPage) Primitive() tview.Primitive {
	return p
}

// GetActionPrompts returns the key actions for the test page.
func (p *TestPage) GetActionPrompts() map[string]string {
	return map[string]string{
		"ESC": "Back (Cancel Step)", "A": "Success", "D": "Failure",
	}
}

// GetStatusPrimitive returns the tview.Primitive that displays the page's status
func (p *TestPage) GetStatusPrimitive() *tview.TextView {
	return p.statusText
}

// GetFocusablePrimitives implements the Focusable interface for the MainPage.
func (p *TestPage) GetFocusablePrimitives() []tview.Primitive {
	return []tview.Primitive{
		p.successBtn,
		p.failBtn,
		p.backBtn,
	}
}
