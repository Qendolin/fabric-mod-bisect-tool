package widgets

import (
	"fmt"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

// TitleFrame is a primitive that wraps another primitive, adding a horizontal
// rule at the top with an optional title bar implemented as a Flex.
type TitleFrame struct {
	*tview.Box
	content tview.Primitive // The primitive being wrapped
	title   string
	// titleBar holds primitives shown on the title line (flexed horizontally).
	titleBar  *tview.Flex
	titleItem *tview.TextView
	color     tcell.Color // Color for the horizontal line and title
}

// NewTitleFrame creates a new TitleFrame.
func NewTitleFrame(content tview.Primitive, title string) *TitleFrame {
	f := &TitleFrame{
		Box:     tview.NewBox().SetBorder(false), // No default border drawing from Box, we'll draw it.
		content: content,
		title:   title,
		color:   tcell.ColorWhite, // Default color for the separator
	}

	// Create the title bar as a horizontal Flex with a single TextView title item.
	tv := tview.NewTextView().SetDynamicColors(true).SetRegions(false)
	tv.SetText(tview.Escape(title))
	// Initial fixed size a couple chars wider than title
	initialSize := tview.TaggedStringWidth(" " + tview.Escape(title) + " ")
	f.titleBar = tview.NewFlex().SetDirection(tview.FlexColumn)
	f.titleBar.AddItem(tv, initialSize, 0, false)
	f.titleItem = tv

	return f
}

// AddTitleItem forwards to the internal titleBar's AddItem.
func (f *TitleFrame) AddTitleItem(item tview.Primitive, fixedSize int, proportion int, focus bool) *TitleFrame {
	f.titleBar.AddItem(item, fixedSize, proportion, focus)
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
	style := tcell.StyleDefault.Background(tview.Styles.PrimitiveBackgroundColor).Foreground(f.color) // Color for the line
	for i := 0; i < width; i++ {
		screen.SetContent(x+i, lineY, lineRune, nil, style)
	}

	// Draw the title bar (children) on top of the line. The titleBar occupies the
	// title row; leave 1 column padding on left and right.
	// Update title item text and size depending on focus so styling and padding
	// are updated live. TitleBar and titleItem are assumed present.
	tv := f.titleItem
	if f.HasFocus() {
		focused := fmt.Sprintf("%s[::ur]%s[-:-:-]%s", string(tview.BlockRightHalfBlock), tview.Escape(f.title), string(tview.BlockLeftHalfBlock))
		tv.SetText(focused)
		size := tview.TaggedStringWidth(focused)
		f.titleBar.ResizeItem(tv, size, 1)
	} else {
		normal := " " + tview.Escape(f.title) + " "
		tv.SetText(normal)
		size := tview.TaggedStringWidth(normal)
		f.titleBar.ResizeItem(tv, size, 1)
	}

	barX := x + 1
	barW := width - 2
	if barW < 0 {
		barW = 0
	}
	f.titleBar.SetRect(barX, lineY, barW, 1)
	f.titleBar.Draw(screen)

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

	// Set the content's rectangle and draw it (if present)
	if f.content != nil {
		f.content.SetRect(contentX, contentY, contentWidth, contentHeight)
		f.content.Draw(screen)
	}
}

// SetTitle updates the title text and resizes the title item to a fixed width
// that fits the text plus padding.
func (f *TitleFrame) SetTitle(title string) {
	f.title = title
	// Assume titleItem and titleBar exist and titleItem is a *tview.TextView
	tv := f.titleItem
	normal := " " + tview.Escape(title) + " "
	tv.SetText(normal)
	size := tview.TaggedStringWidth(normal)
	f.titleBar.ResizeItem(f.titleItem, size, 1)
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
	if f.titleBar.HasFocus() {
		return true
	}
	if f.content != nil {
		return f.content.HasFocus()
	}
	return f.Box.HasFocus()
}

// MouseHandler returns the mouse handler for this primitive.
func (f *TitleFrame) MouseHandler() func(action tview.MouseAction, event *tcell.EventMouse, setFocus func(p tview.Primitive)) (consumed bool, capture tview.Primitive) {
	return f.WrapMouseHandler(func(action tview.MouseAction, event *tcell.EventMouse, setFocus func(p tview.Primitive)) (consumed bool, capture tview.Primitive) {
		if !f.InRect(event.Position()) {
			return false, nil
		}

		mx, my := event.Position()
		x, y, width, _ := f.GetRect()
		lineY := y

		// If the click is on the title bar row, forward to titleBar first.
		barX := x + 1
		barW := width - 2
		if barW < 0 {
			barW = 0
		}
		if my == lineY && mx >= barX && mx < barX+barW {
			if handler := f.titleBar.MouseHandler(); handler != nil {
				consumed, capture = handler(action, event, setFocus)
				if consumed {
					return true, capture
				}
			}
		}

		// Pass mouse events on to contained primitive (main content)
		if f.content != nil {
			consumed, capture = f.content.MouseHandler()(action, event, setFocus)
			if consumed {
				return true, capture
			}
		}

		// Clicking on the frame parts.
		if action == tview.MouseLeftDown {
			// If no specific child handled it, focus the frame itself.
			setFocus(f)
			consumed = true
		}

		return
	})
}

// InputHandler returns the handler for this primitive.
func (f *TitleFrame) InputHandler() func(event *tcell.EventKey, setFocus func(p tview.Primitive)) {
	return f.WrapInputHandler(func(event *tcell.EventKey, setFocus func(p tview.Primitive)) {
		// If the title bar has focus, let it handle input first.
		if f.titleBar.HasFocus() {
			if handler := f.titleBar.InputHandler(); handler != nil {
				handler(event, setFocus)
				return
			}
		}

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
		// Route to title bar if it has focus
		if f.titleBar.HasFocus() {
			if handler := f.titleBar.PasteHandler(); handler != nil {
				handler(pastedText, setFocus)
				return
			}
		}

		if f.content == nil {
			return
		}
		if handler := f.content.PasteHandler(); handler != nil {
			handler(pastedText, setFocus)
			return
		}
	})
}

// GetFocusablePrimitives implements the Focusable interface.
func (f *TitleFrame) GetFocusablePrimitives() []tview.Primitive {
	if f.content != nil {
		return []tview.Primitive{f.titleBar, f.content}
	}
	return []tview.Primitive{f.titleBar}
}
