package ui

import (
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

// FlexList is a scrollable container for other primitives that behaves like a list,
// allowing selection and navigation of its child items.
type FlexList struct {
	*tview.Box
	flex           *tview.Flex
	items          []tview.Primitive
	itemHeights    []int
	selectedIndex  int
	offsetY        int // The vertical scroll offset of the top-most visible row
	changedFunc    func(index int)
	selectionColor tcell.Color
}

// NewFlexList creates a new FlexList.
func NewFlexList() *FlexList {
	return &FlexList{
		Box:            tview.NewBox(),
		flex:           tview.NewFlex().SetDirection(tview.FlexRow),
		items:          make([]tview.Primitive, 0),
		itemHeights:    make([]int, 0),
		selectedIndex:  -1,
		selectionColor: tcell.ColorDarkSlateGray,
	}
}

// SetSelectionColor sets the background color for the selected item.
func (fl *FlexList) SetSelectionColor(color tcell.Color) *FlexList {
	fl.selectionColor = color
	return fl
}

// AddItem adds a primitive to the list. Height is the fixed height of the item.
func (fl *FlexList) AddItem(item tview.Primitive, height int, proportion int, focus bool) *FlexList {
	fl.flex.AddItem(item, height, proportion, focus)
	fl.items = append(fl.items, item)
	fl.itemHeights = append(fl.itemHeights, height)
	return fl
}

// Clear removes all items from the list.
func (fl *FlexList) Clear() {
	fl.flex.Clear()
	fl.items = nil
	fl.itemHeights = nil
	fl.selectedIndex = -1
	fl.offsetY = 0
}

// GetItemCount returns the number of items in the list.
func (fl *FlexList) GetItemCount() int {
	return len(fl.items)
}

// GetCurrentItem returns the index of the currently selected item.
func (fl *FlexList) GetCurrentItem() int {
	return fl.selectedIndex
}

// SetChangedFunc sets a callback that is fired when the selection changes.
func (fl *FlexList) SetChangedFunc(handler func(index int)) *FlexList {
	fl.changedFunc = handler
	return fl
}

// SetCurrentItem sets the currently selected item by its index.
func (fl *FlexList) SetCurrentItem(index int) {
	if fl.GetItemCount() == 0 {
		return
	}
	if index < 0 {
		index = 0
	}
	if index >= len(fl.items) {
		index = len(fl.items) - 1
	}
	if index == fl.selectedIndex {
		return
	}

	fl.selectedIndex = index
	fl.ensureVisible()

	if fl.changedFunc != nil {
		fl.changedFunc(index)
	}
}

// ensureVisible adjusts offsetY to make the selected item visible.
func (fl *FlexList) ensureVisible() {
	if fl.selectedIndex < 0 {
		return
	}
	_, _, _, height := fl.GetInnerRect()
	itemTopY := 0
	for i := 0; i < fl.selectedIndex; i++ {
		itemTopY += fl.itemHeights[i]
	}
	itemBottomY := itemTopY + fl.itemHeights[fl.selectedIndex]
	if itemTopY < fl.offsetY {
		fl.offsetY = itemTopY
	}
	if itemBottomY > fl.offsetY+height {
		fl.offsetY = itemBottomY - height
	}
}

// Draw implements tview.Primitive.
func (fl *FlexList) Draw(screen tcell.Screen) {
	fl.Box.Draw(screen) // Draw the box and border first.
	x, y, width, height := fl.GetInnerRect()

	// This is the y-coordinate on the "infinite canvas" of all items.
	// We will iterate through items and advance this cursor.
	itemCanvasY := 0

	for i, item := range fl.items {
		itemHeight := fl.itemHeights[i]

		// --- Determine if this item is visible at all ---
		// An item is visible if any part of it falls between the top (offsetY) and bottom (offsetY + height) of our viewport.
		itemStartsInView := itemCanvasY < fl.offsetY+height
		itemEndsInView := itemCanvasY+itemHeight > fl.offsetY

		if itemStartsInView && itemEndsInView {
			// --- This item is at least partially visible. Now, calculate its exact position and size on screen. ---

			// This is the item's top row on the actual screen.
			// It might be negative if the item starts above the viewport.
			itemScreenY := y + itemCanvasY - fl.offsetY

			// We only draw if the *entire height* of the item can fit within the viewport.
			if itemScreenY >= y && (itemScreenY+itemHeight) <= (y+height) {

				// If this is the selected item, draw a colored background first.
				if fl.HasFocus() && i == fl.selectedIndex {
					bgStyle := tcell.StyleDefault.Background(fl.selectionColor)
					for row := 0; row < itemHeight; row++ {
						for col := 0; col < width; col++ {
							screen.SetContent(x+col, itemScreenY+row, ' ', nil, bgStyle)
						}
					}
				}

				// Set the item's drawing rectangle to its calculated position and draw it onto the screen.
				item.SetRect(x, itemScreenY, width, itemHeight)
				item.Draw(screen)
			}
		}

		// Advance the canvas cursor to the start of the next item.
		itemCanvasY += itemHeight

		// Optimization: if we've already drawn past the bottom of the screen, stop.
		if itemCanvasY >= fl.offsetY+height {
			break
		}
	}
}

// InputHandler handles keyboard input for selection and scrolling.
func (fl *FlexList) InputHandler() func(event *tcell.EventKey, setFocus func(p tview.Primitive)) {
	return fl.WrapInputHandler(func(event *tcell.EventKey, setFocus func(p tview.Primitive)) {
		if len(fl.items) == 0 {
			return
		}
		_, _, _, height := fl.GetInnerRect()
		pageHeightInItems := height / fl.itemHeights[0] // Approximation
		if pageHeightInItems == 0 {
			pageHeightInItems = 1
		}

		switch event.Key() {
		case tcell.KeyUp:
			fl.SetCurrentItem(fl.selectedIndex - 1)
		case tcell.KeyDown:
			fl.SetCurrentItem(fl.selectedIndex + 1)
		case tcell.KeyHome:
			fl.SetCurrentItem(0)
		case tcell.KeyEnd:
			fl.SetCurrentItem(len(fl.items) - 1)
		case tcell.KeyPgUp:
			fl.SetCurrentItem(fl.selectedIndex - pageHeightInItems)
		case tcell.KeyPgDn:
			fl.SetCurrentItem(fl.selectedIndex + pageHeightInItems)
		}
	})
}

// Focus delegates focus to the currently selected child item.
func (fl *FlexList) Focus(delegate func(p tview.Primitive)) {
	if fl.selectedIndex >= 0 && fl.selectedIndex < len(fl.items) {
		delegate(fl.items[fl.selectedIndex])
	} else {
		delegate(fl.flex)
	}
}

// HasFocus returns whether this primitive has focus.
func (sf *FlexList) HasFocus() bool {
	return sf.flex.HasFocus()
}
