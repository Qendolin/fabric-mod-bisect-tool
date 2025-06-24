package ui

import "github.com/rivo/tview"

// ModalPage is a simple wrapper around a tview.Modal to conform to the Page interface.
type ModalPage struct {
	*tview.Modal
}

// NewModalPage creates a new ModalPage.
func NewModalPage(modal *tview.Modal) *ModalPage {
	return &ModalPage{Modal: modal}
}

// Primitive returns the underlying tview.Primitive.
func (p *ModalPage) Primitive() tview.Primitive {
	return p
}

// GetActionPrompts returns an empty map as modals have their own buttons.
func (p *ModalPage) GetActionPrompts() map[string]string {
	return nil
}

// GetStatusPrimitive returns the tview.Primitive that displays the page's status
func (p *ModalPage) GetStatusPrimitive() *tview.TextView {
	return nil
}
