package ui

import (
	"github.com/rivo/tview"
)

// NavigationManager handles page state and transitions.
type NavigationManager struct {
	app   AppInterface
	pages *tview.Pages

	pageStack      []tview.Primitive
	activePageID   string
	pageIDs        map[tview.Primitive]string
	pagePrimitives map[string]tview.Primitive
}

// NewNavigationManager creates a new manager for page navigation.
func NewNavigationManager(app AppInterface, pages *tview.Pages) *NavigationManager {
	return &NavigationManager{
		app:            app,
		pages:          pages,
		pageIDs:        make(map[tview.Primitive]string),
		pagePrimitives: make(map[string]tview.Primitive),
	}
}

// ShowPage adds a page to the main Pages container and sets it as the current page.
func (n *NavigationManager) ShowPage(pageID string, page Page, resize bool) {
	n.pages.AddAndSwitchToPage(pageID, page.Primitive(), resize)
	n.activePageID = pageID
	n.pagePrimitives[pageID] = page.Primitive()
	n.app.SetFocus(page.Primitive())
	n.app.Layout().SetFooter(page.GetActionPrompts())
	n.app.Layout().SetHeader(page.GetStatusPrimitive())
}

// PushPage adds an overlay page to the stack.
func (n *NavigationManager) PushPage(pageID string, page Page) {
	n.pages.AddPage(pageID, page.Primitive(), true, true)
	n.pageStack = append(n.pageStack, page.Primitive())
	n.pageIDs[page.Primitive()] = pageID
	n.pagePrimitives[pageID] = page.Primitive()
	n.app.SetFocus(page.Primitive())
	n.app.Layout().SetFooter(page.GetActionPrompts())
	n.app.Layout().SetHeader(page.GetStatusPrimitive())
}

// PopPage removes the top-most page from the stack.
func (n *NavigationManager) PopPage() {
	if len(n.pageStack) > 0 {
		topPage := n.pageStack[len(n.pageStack)-1]
		n.pageStack = n.pageStack[:len(n.pageStack)-1]

		if pageID, exists := n.pageIDs[topPage]; exists {
			n.pages.RemovePage(pageID)
			delete(n.pageIDs, topPage)
			delete(n.pagePrimitives, pageID)
		}
	}

	var focusTarget tview.Primitive
	if len(n.pageStack) > 0 {
		focusTarget = n.pageStack[len(n.pageStack)-1]
	} else {
		focusTarget = n.pagePrimitives[n.activePageID]
	}

	if page, ok := focusTarget.(Page); ok {
		n.app.Layout().SetFooter(page.GetActionPrompts())
		n.app.Layout().SetHeader(page.GetStatusPrimitive())
	}

	if focusTarget != nil {
		n.app.SetFocus(focusTarget)
	}
}

// ToggleLogPage shows/hides the log page overlay.
func (n *NavigationManager) ToggleLogPage() {
	if frontID, _ := n.pages.GetFrontPage(); frontID == PageLogID {
		n.PopPage()
	} else {
		logPage := NewLogPage(n.app)
		n.PushPage(PageLogID, logPage)
	}
}

// UIManagerInterface implementation for pages to use
func (n *NavigationManager) QueueUpdateDraw(f func()) {
	n.app.QueueUpdateDraw(f)
}
func (n *NavigationManager) SetFocus(p tview.Primitive) { n.app.SetFocus(p) }
func (n *NavigationManager) GetLogTextView() *tview.TextView {
	return n.app.(interface{ GetLogTextView() *tview.TextView }).GetLogTextView()
}

// TODO: remove activePageID?
func (n *NavigationManager) GetCurrentPageID() string {
	return n.activePageID
}
func (n *NavigationManager) GetCurrentPage() Page {
	if n.activePageID == "" {
		return nil
	}
	_, p := n.pages.GetFrontPage()
	return p.(Page)
}
