package ui

import (
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

// FocusWrapper is a lightweight struct that wraps a tview.Primitive to make it
// conform to our Focusable interface, using a delegate function to
// determine its focusable children.
type FocusWrapper struct {
	*tview.Box
	content       tview.Primitive
	getFocusables func() []tview.Primitive
}

// NewFocusWrapper creates a new wrapper with a dynamic function to get focusable children.
func NewFocusWrapper(p tview.Primitive, getFocusables func() []tview.Primitive) *FocusWrapper {
	return &FocusWrapper{
		Box:           tview.NewBox(), // Embed a box for basic primitive properties
		content:       p,
		getFocusables: getFocusables,
	}
}

// NewFocusWrapperWithStatic is a convenience constructor that takes a static slice of primitives.
func NewFocusWrapperWithStatic(p tview.Primitive, focusables ...tview.Primitive) *FocusWrapper {
	return &FocusWrapper{
		Box:     tview.NewBox(),
		content: p,
		getFocusables: func() []tview.Primitive {
			return focusables
		},
	}
}

// Draw delegates the drawing to the wrapped primitive.
func (fw *FocusWrapper) Draw(screen tcell.Screen) {
	// First, set the wrapped primitive's rectangle to our own
	fw.content.SetRect(fw.GetRect())
	// Then, draw the wrapped primitive
	fw.content.Draw(screen)
}

// GetRect returns the rectangle of the wrapped primitive.
func (fw *FocusWrapper) GetRect() (int, int, int, int) {
	return fw.content.GetRect()
}

// SetRect sets the rectangle of the wrapped primitive.
func (fw *FocusWrapper) SetRect(x, y, width, height int) {
	fw.content.SetRect(x, y, width, height)
}

// InputHandler returns the handler for this primitive.
func (fw *FocusWrapper) InputHandler() func(event *tcell.EventKey, setFocus func(p tview.Primitive)) {
	return fw.content.InputHandler()
}

// Blur is called when this primitive loses focus.
func (fw *FocusWrapper) Focus(delegate func(p tview.Primitive)) {
	delegate(fw.content)
}

// Blur is called when this primitive loses focus.
func (fw *FocusWrapper) Blur() {
	fw.content.Blur()
}

// HasFocus returns whether or not this primitive has focus.
func (fw *FocusWrapper) HasFocus() bool {
	return fw.content.HasFocus()
}

// MouseHandler returns the mouse handler for this primitive.
func (fw *FocusWrapper) MouseHandler() func(action tview.MouseAction, event *tcell.EventMouse, setFocus func(p tview.Primitive)) (consumed bool, capture tview.Primitive) {
	return fw.content.MouseHandler()
}

// GetFocusablePrimitives implements the Focusable interface by calling the delegate function.
func (fw *FocusWrapper) GetFocusablePrimitives() []tview.Primitive {
	if fw.getFocusables != nil {
		return fw.getFocusables()
	}
	return []tview.Primitive{fw.content}
}
