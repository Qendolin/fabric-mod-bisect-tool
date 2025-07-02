package widgets

import (
	"fmt"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

// TitleFrame is a primitive that wraps another primitive, adding a horizontal
// rule at the top with an optional title.
type TitleFrame struct {
	*tview.Box
	content tview.Primitive // The primitive being wrapped
	title   string
	color   tcell.Color // Color for the horizontal line and title
}

// NewTitleFrame creates a new TitleFrame.
func NewTitleFrame(content tview.Primitive, title string) *TitleFrame {
	f := &TitleFrame{
		Box:     tview.NewBox().SetBorder(false), // No default border drawing from Box, we'll draw it.
		content: content,
		title:   title,
		color:   tcell.ColorWhite, // Default color for the separator
	}
	return f
}

// Draw draws the TitleFrame.
func (f *TitleFrame) Draw(screen tcell.Screen) {
	// Draw the content of the base Box (e.g., background color if set by tview)
	f.Box.Draw(screen)

	x, y, width, height := f.GetRect() // Get our own drawing area

	lineRune := tview.BoxDrawingsLightHorizontal
	if f.HasFocus() {
		lineRune = tview.BoxDrawingsHeavyHorizontal
	}

	// Draw the horizontal line at the top
	lineY := y
	style := tcell.StyleDefault.Background(tview.Styles.PrimitiveBackgroundColor).Foreground(f.color) // White color for the line
	for i := 0; i < width; i++ {
		screen.SetContent(x+i, lineY, lineRune, nil, style)
	}

	// Draw the title on top of the line
	if f.title != "" {
		titleText := " " + tview.Escape(f.title) + " "
		if f.HasFocus() {
			titleText = fmt.Sprintf("%s[::ur]%s[-:-:-]%s", string(tview.BlockRightHalfBlock), tview.Escape(f.title), string(tview.BlockLeftHalfBlock))
		}
		tview.Print(screen, titleText, x+1, lineY, width-2, tview.AlignLeft, f.color)
	}

	// Calculate the drawing area for the wrapped content
	// The content starts 1 row below the horizontal line/title
	contentX := x
	contentY := y + 1
	contentWidth := width
	contentHeight := height - 1

	// Ensure content area is valid
	if contentHeight <= 0 {
		return // Not enough height to draw content below the header
	}

	// Set the content's rectangle and draw it
	f.content.SetRect(contentX, contentY, contentWidth, contentHeight)
	f.content.Draw(screen)
}

// Focus is called when this primitive receives focus.
func (f *TitleFrame) SetTitle(title string) {
	f.title = title
}

// Focus is called when this primitive receives focus.
func (f *TitleFrame) Focus(delegate func(p tview.Primitive)) {
	if f.content != nil {
		delegate(f.content)
	} else {
		f.Box.Focus(delegate)
	}
}

// HasFocus returns whether or not this primitive has focus.
func (f *TitleFrame) HasFocus() bool {
	if f.content == nil {
		return f.Box.HasFocus()
	}
	return f.content.HasFocus()
}

// MouseHandler returns the mouse handler for this primitive.
func (f *TitleFrame) MouseHandler() func(action tview.MouseAction, event *tcell.EventMouse, setFocus func(p tview.Primitive)) (consumed bool, capture tview.Primitive) {
	return f.WrapMouseHandler(func(action tview.MouseAction, event *tcell.EventMouse, setFocus func(p tview.Primitive)) (consumed bool, capture tview.Primitive) {
		if !f.InRect(event.Position()) {
			return false, nil
		}

		// Pass mouse events on to contained primitive.
		if f.content != nil {
			consumed, capture = f.content.MouseHandler()(action, event, setFocus)
			if consumed {
				return true, capture
			}
		}

		// Clicking on the frame parts.
		if action == tview.MouseLeftDown {
			setFocus(f)
			consumed = true
		}

		return
	})
}

// InputHandler returns the handler for this primitive.
func (f *TitleFrame) InputHandler() func(event *tcell.EventKey, setFocus func(p tview.Primitive)) {
	return f.WrapInputHandler(func(event *tcell.EventKey, setFocus func(p tview.Primitive)) {
		if f.content == nil {
			return
		}
		if handler := f.content.InputHandler(); handler != nil {
			handler(event, setFocus)
			return
		}
	})
}

// PasteHandler returns the handler for this primitive.
func (f *TitleFrame) PasteHandler() func(pastedText string, setFocus func(p tview.Primitive)) {
	return f.WrapPasteHandler(func(pastedText string, setFocus func(p tview.Primitive)) {
		if f.content == nil {
			return
		}
		if handler := f.content.PasteHandler(); handler != nil {
			handler(pastedText, setFocus)
			return
		}
	})
}
