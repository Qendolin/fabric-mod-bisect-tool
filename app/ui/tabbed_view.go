package ui

import (
	"strings"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

// TabBar is a primitive that draws a bar of selectable tabs.
type TabBar struct {
	*tview.Box
	tabs      []string
	activeTab int

	// Called when the user navigates between tabs.
	onCycle func(forward bool)
	// Called when a tab is selected, e.g., via a mouse click.
	onSelect func(index int)
}

// NewTabBar creates a new TabBar.
func NewTabBar() *TabBar {
	return &TabBar{
		Box: tview.NewBox(),
	}
}

// Draw renders the tab bar.
func (tb *TabBar) Draw(screen tcell.Screen) {
	tb.Box.Draw(screen)
	x, y, width, _ := tb.GetInnerRect()
	if width <= 0 {
		return
	}

	// Styles defined as tview tags
	blurredActiveTag := "[white::bu]"
	focusedActiveTag := "[white:blue:bu]"
	inactiveTag := "[white:]"
	resetTag := "[-:-:-]"
	separator := "[gray:]|[-:-:-]"
	activeBracketStart := "["
	activeBracketEnd := "]"

	var builder strings.Builder
	numTabs := len(tb.tabs)

	for i, label := range tb.tabs {
		isTabActive := (i == tb.activeTab)
		currentTag := inactiveTag
		if isTabActive {
			if tb.HasFocus() {
				currentTag = focusedActiveTag
			} else {
				currentTag = blurredActiveTag
			}
		}

		builder.WriteString(currentTag) // Apply base tag for the entire tab segment

		if isTabActive {
			builder.WriteString(activeBracketStart)
			builder.WriteString(" ") // Space after opening bracket
		} else {
			// Add leading space for all inactive tabs, with an extra for the first one
			if i == 0 {
				builder.WriteString(" ")
			}
			builder.WriteString(" ")
		}

		builder.WriteString(tview.Escape(label)) // Tab label

		if isTabActive {
			builder.WriteString(" ") // Space before closing bracket
			builder.WriteString(activeBracketEnd)
		} else {
			// Add trailing space for all inactive tabs, with an extra for the last one
			builder.WriteString(" ")
			if i == numTabs-1 {
				builder.WriteString(" ")
			}
		}
		builder.WriteString(resetTag) // Reset tag after the tab segment

		// Add separator if not the last tab and neither current nor next tab is active
		if i < numTabs-1 && !isTabActive && !(i+1 == tb.activeTab) {
			builder.WriteString(separator)
		}
	}

	// Print the entire constructed string at once
	tview.Print(screen, builder.String(), x, y, width, tview.AlignLeft, tcell.ColorDefault)
}

// InputHandler handles keyboard navigation for the tab bar.
func (tb *TabBar) InputHandler() func(event *tcell.EventKey, setFocus func(p tview.Primitive)) {
	return tb.WrapInputHandler(func(event *tcell.EventKey, setFocus func(p tview.Primitive)) {
		switch event.Key() {
		case tcell.KeyRight:
			if tb.onCycle != nil {
				tb.onCycle(true)
			}
		case tcell.KeyLeft:
			if tb.onCycle != nil {
				tb.onCycle(false)
			}
		}
	})
}

// MouseHandler handles mouse clicks on the tab bar.
func (tb *TabBar) MouseHandler() func(action tview.MouseAction, event *tcell.EventMouse, setFocus func(p tview.Primitive)) (consumed bool, capture tview.Primitive) {
	return tb.WrapMouseHandler(func(action tview.MouseAction, event *tcell.EventMouse, setFocus func(p tview.Primitive)) (consumed bool, capture tview.Primitive) {
		if !tb.InRect(event.Position()) {
			return false, nil
		}

		if action == tview.MouseLeftClick {
			setFocus(tb)
			x, _, _, _ := tb.GetInnerRect()
			clickX, _ := event.Position()

			currentX := x
			for i, label := range tb.tabs {
				tabWidth := len(label) + 2 // 2 for padding
				if clickX >= currentX && clickX < currentX+tabWidth {
					if tb.onSelect != nil {
						tb.onSelect(i)
					}
					break
				}
				currentX += tabWidth + 1 // 1 for separator
			}
			consumed = true
		}
		return
	})
}

// SetTabs updates the tab labels and active tab index.
func (tb *TabBar) SetTabs(tabs []string, activeTab int) {
	tb.tabs = tabs
	tb.activeTab = activeTab
}

// TabbedPanes is a custom widget that provides a tabbed view.
type TabbedPanes struct {
	*tview.Flex
	tabBar      *TabBar
	content     *tview.Pages
	tabs        []string
	activeTab   int
	onTabChange func(index int, label string)
}

// NewTabbedPanes creates a new TabbedPanes widget.
func NewTabbedPanes() *TabbedPanes {
	t := &TabbedPanes{
		Flex:      tview.NewFlex().SetDirection(tview.FlexRow),
		tabBar:    NewTabBar(),
		content:   tview.NewPages(),
		tabs:      []string{},
		activeTab: 0,
	}

	t.AddItem(t.tabBar, 1, 0, false).
		AddItem(t.content, 0, 1, false)

	t.tabBar.onCycle = func(forward bool) {
		if forward {
			t.SelectNext()
		} else {
			t.SelectPrevious()
		}
	}
	t.tabBar.onSelect = t.SetActiveTab

	return t
}

func (t *TabbedPanes) Focus(delegate func(p tview.Primitive)) {
	delegate(t.tabBar)
}

// GetFocusablePrimitives implements the Focusable interface.
func (t *TabbedPanes) GetFocusablePrimitives() []tview.Primitive {
	activeContent := t.GetActiveContent()
	if activeContent != nil {
		return []tview.Primitive{t.tabBar, activeContent}
	}
	return []tview.Primitive{t.tabBar}
}

// AddTab adds a new tab with a given label and content primitive.
func (t *TabbedPanes) AddTab(label string, content tview.Primitive) {
	t.tabs = append(t.tabs, label)
	t.content.AddPage(label, content, true, len(t.tabs) == 1)
	t.tabBar.SetTabs(t.tabs, t.activeTab)
}

// SetOnTabChange sets a callback function to be executed when the active tab changes.
func (t *TabbedPanes) SetOnTabChange(handler func(index int, label string)) {
	t.onTabChange = handler
}

// SetActiveTab sets the currently active tab by its index.
func (t *TabbedPanes) SetActiveTab(index int) {
	if index < 0 || index >= len(t.tabs) {
		return
	}
	t.activeTab = index
	t.content.SwitchToPage(t.tabs[index])
	t.tabBar.SetTabs(t.tabs, t.activeTab) // Update visuals
	if t.onTabChange != nil {
		t.onTabChange(index, t.tabs[index])
	}
}

// GetActiveTab returns the index of the currently active tab.
func (t *TabbedPanes) GetActiveTab() int {
	return t.activeTab
}

// GetActiveContent returns the primitive of the currently active tab.
func (t *TabbedPanes) GetActiveContent() tview.Primitive {
	if t.activeTab < 0 || t.activeTab >= len(t.tabs) {
		return nil
	}
	_, primitive := t.content.GetFrontPage()
	return primitive
}

// SelectNext selects the next tab in the sequence.
func (t *TabbedPanes) SelectNext() {
	if len(t.tabs) == 0 {
		return
	}
	t.SetActiveTab((t.activeTab + 1) % len(t.tabs))
}

// SelectPrevious selects the previous tab in the sequence.
func (t *TabbedPanes) SelectPrevious() {
	if len(t.tabs) == 0 {
		return
	}
	t.SetActiveTab((t.activeTab - 1 + len(t.tabs)) % len(t.tabs))
}
