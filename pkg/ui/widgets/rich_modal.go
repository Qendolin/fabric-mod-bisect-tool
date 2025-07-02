package widgets

import (
	"strings"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

// RichModal is a centered message window that offers control over width and
// supports separately-aligned text sections using a Flex layout.
type RichModal struct {
	*tview.Box

	flex             *tview.Flex
	contentFlex      *tview.Flex
	centeredTextView *tview.TextView
	detailsTextView  *tview.TextView
	form             *tview.Form

	minWidth int
	maxWidth int

	done func(buttonIndex int, buttonLabel string)
}

// NewRichModal returns a new RichModal message window.
func NewRichModal() *RichModal {
	m := &RichModal{
		Box:      tview.NewBox().SetBorder(true),
		minWidth: 40,
		maxWidth: 120,
	}

	m.SetBorderPadding(1, 1, 1, 1) // Set padding on the modal's box itself.

	m.centeredTextView = tview.NewTextView().
		SetDynamicColors(true).
		SetTextAlign(tview.AlignCenter)

	m.detailsTextView = tview.NewTextView().
		SetDynamicColors(true).
		SetTextAlign(tview.AlignLeft).
		SetWordWrap(false)

	m.form = tview.NewForm().
		SetButtonsAlign(tview.AlignCenter).
		SetButtonBackgroundColor(tview.Styles.PrimitiveBackgroundColor).
		SetButtonTextColor(tview.Styles.PrimaryTextColor)

	m.form.SetBorderPadding(1, 0, 0, 0)

	m.contentFlex = tview.NewFlex().
		SetDirection(tview.FlexRow).
		AddItem(m.centeredTextView, 0, 1, false).
		AddItem(m.detailsTextView, 0, 1, false)

	m.flex = tview.NewFlex().
		SetDirection(tview.FlexRow).
		AddItem(m.contentFlex, 0, 1, false).
		AddItem(m.form, 2, 0, true) // Form for buttons, fixed height of 3 (for 1 button row + small internal padding)

	m.SetBackgroundColor(tview.Styles.ContrastBackgroundColor) // Set initial colors.

	m.form.SetCancelFunc(func() {
		if m.done != nil {
			m.done(-1, "")
		}
	})

	return m
}

// SetBackgroundColor sets the background color of the modal and its contents.
func (m *RichModal) SetBackgroundColor(color tcell.Color) *RichModal {
	m.Box.SetBackgroundColor(color)
	m.flex.SetBackgroundColor(color)
	m.centeredTextView.SetBackgroundColor(color)
	m.detailsTextView.SetBackgroundColor(color)
	m.form.SetBackgroundColor(color)
	return m
}

// SetTextColor sets the color of the main centered text.
func (m *RichModal) SetTextColor(color tcell.Color) *RichModal {
	m.centeredTextView.SetTextColor(color)
	return m
}

// SetDetailsTextColor sets the color of the left-aligned details text.
func (m *RichModal) SetDetailsTextColor(color tcell.Color) *RichModal {
	m.detailsTextView.SetTextColor(color)
	return m
}

// SetButtonBackgroundColor sets the background color of the buttons.
func (m *RichModal) SetButtonBackgroundColor(color tcell.Color) *RichModal {
	m.form.SetButtonBackgroundColor(color)
	return m
}

// SetButtonTextColor sets the color of the button texts.
func (m *RichModal) SetButtonTextColor(color tcell.Color) *RichModal {
	m.form.SetButtonTextColor(color)
	return m
}

// SetButtonStyle sets the style of the buttons when they are not focused.
func (m *RichModal) SetButtonStyle(style tcell.Style) *RichModal {
	m.form.SetButtonStyle(style)
	return m
}

// SetButtonActivatedStyle sets the style of the buttons when they are focused.
func (m *RichModal) SetButtonActivatedStyle(style tcell.Style) *RichModal {
	m.form.SetButtonActivatedStyle(style)
	return m
}

// SetDoneFunc sets a handler which is called when one of the buttons was pressed.
func (m *RichModal) SetDoneFunc(handler func(buttonIndex int, buttonLabel string)) *RichModal {
	m.done = handler
	return m
}

// SetCenteredText sets the main, centered message text of the window.
func (m *RichModal) SetCenteredText(text string) *RichModal {
	m.centeredTextView.SetText(tview.TranslateANSI(text))
	return m
}

// SetDetailsText sets the optional, left-aligned details text of the window.
func (m *RichModal) SetDetailsText(text string) *RichModal {
	m.detailsTextView.SetText(tview.TranslateANSI(text))
	return m
}

// SetMinWidth sets the minimum width of the modal's content area.
func (m *RichModal) SetMinWidth(min int) *RichModal {
	m.minWidth = min
	return m
}

// SetMaxWidth sets the maximum width of the modal's content area.
func (m *RichModal) SetMaxWidth(max int) *RichModal {
	m.maxWidth = max
	return m
}

// AddButtons adds buttons to the window.
func (m *RichModal) AddButtons(labels []string) *RichModal {
	for index, label := range labels {
		m.form.AddButton(label, func() {
			if m.done != nil {
				m.done(index, label)
			}
		})
	}
	return m
}

// ClearButtons removes all buttons from the window.
func (m *RichModal) ClearButtons() *RichModal {
	m.form.ClearButtons()
	return m
}

// SetFocus shifts the focus to the button with the given index.
func (m *RichModal) SetFocus(index int) *RichModal {
	m.form.SetFocus(index)
	return m
}

// Focus is called when this primitive receives focus.
func (m *RichModal) Focus(delegate func(p tview.Primitive)) {
	delegate(m.flex)
}

// HasFocus returns whether or not this primitive has focus.
func (m *RichModal) HasFocus() bool {
	return m.flex.HasFocus()
}

// Draw draws this primitive onto the screen.
func (m *RichModal) Draw(screen tcell.Screen) {
	screenWidth, screenHeight := screen.Size()

	// === 1. Calculate final modal width ===
	buttonsWidth := 0
	buttonCount := m.form.GetButtonCount()
	if buttonCount > 0 {
		for i := 0; i < buttonCount; i++ {
			button := m.form.GetButton(i)
			buttonsWidth += tview.TaggedStringWidth(button.GetLabel()) + 4
		}
		buttonsWidth += (buttonCount - 1) * 2 // Spacing between buttons
	}

	centeredText := m.centeredTextView.GetText(true)
	detailsText := m.detailsTextView.GetText(true)

	maxLineWidth := 0
	for _, line := range strings.Split(detailsText, "\n") {
		maxLineWidth = max(maxLineWidth, len(line))
	}

	modalWidth := maxLineWidth + 4 // 2 for border, 2 for padding
	modalWidth = max(modalWidth, int(float32(screenWidth)*2/5))
	if modalWidth < m.minWidth {
		modalWidth = m.minWidth
	}
	if modalWidth > m.maxWidth {
		modalWidth = m.maxWidth
	}
	if modalWidth < buttonsWidth+4 { // +4 for padding and border
		modalWidth = buttonsWidth + 4
	}
	if modalWidth > screenWidth {
		modalWidth = screenWidth
	}

	// === 2. Calculate final modal height ===
	// First, determine the actual content width available inside the modal's border/padding.
	innerX, innerY, _, innerHeight := m.GetInnerRect()
	m.SetRect(innerX, innerY, modalWidth, innerHeight)
	_, _, innerWidth, _ := m.GetInnerRect()
	contentWidth := innerWidth // This is the width the text views will use for wrapping.

	hCentered := 0
	if centeredText != "" {
		hCentered = len(tview.WordWrap(centeredText, contentWidth))
	}

	hDetails := 0
	if detailsText != "" {
		hDetails = len(tview.WordWrap(detailsText, contentWidth))
	}

	m.contentFlex.ResizeItem(m.centeredTextView, hCentered, 0)

	// Show/hide spacer between text blocks.
	spacerHeight := 0
	if hCentered > 0 && hDetails > 0 {
		spacerHeight = 1
	}
	m.detailsTextView.SetBorderPadding(spacerHeight, 0, 0, 0)

	m.contentFlex.ResizeItem(m.detailsTextView, hDetails+spacerHeight, 0)

	// Fixed height for the form is 2.
	contentHeight := hCentered + hDetails + spacerHeight + 2

	modalHeight := contentHeight + 4 // 2 for border, 2 for padding
	if modalHeight > screenHeight {
		modalHeight = screenHeight
	}

	// === 3. Set Rect and Draw ===
	x := (screenWidth - modalWidth) / 2
	y := (screenHeight - modalHeight) / 2
	m.SetRect(x, y, modalWidth, modalHeight)

	m.Box.DrawForSubclass(screen, m)
	innerX, innerY, innerWidth, innerHeight = m.GetInnerRect()
	m.flex.SetRect(innerX, innerY, innerWidth, innerHeight)
	m.flex.Draw(screen)
}

// MouseHandler delegates mouse events to the flex layout.
func (m *RichModal) MouseHandler() func(action tview.MouseAction, event *tcell.EventMouse, setFocus func(p tview.Primitive)) (consumed bool, capture tview.Primitive) {
	return m.flex.MouseHandler()
}

// InputHandler delegates input events to the flex layout.
func (m *RichModal) InputHandler() func(event *tcell.EventKey, setFocus func(p tview.Primitive)) {
	return m.flex.InputHandler()
}
