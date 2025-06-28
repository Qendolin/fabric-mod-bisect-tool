package ui

import (
	"fmt"
	"strings"

	"github.com/Qendolin/fabric-mod-bisect-tool/app/core/conflict"
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

const (
	historyRowHeight = 3 // Height of each entry in the master list (1 for text, 1 for overview)
)

const PageHistoryID = "history_page"

// HistoryPage displays the bisection history in a master-detail view.
type HistoryPage struct {
	*tview.Flex
	app AppInterface

	masterList *FlexList

	detailPanel          *tview.Flex
	detailOverviewWidget *OverviewWidget
	detailSummaryText    *tview.TextView
	detailSetsText       *tview.TextView

	historyCache []conflict.CompletedTest
}

// NewHistoryPage creates a new page for viewing bisection history.
func NewHistoryPage(app AppInterface) *HistoryPage {
	p := &HistoryPage{
		Flex: tview.NewFlex(),
		app:  app,
	}

	// Master Pane
	p.masterList = NewFlexList()
	p.masterList.SetBorderPadding(0, 0, 1, 1)
	p.masterList.SetChangedFunc(func(newIndex int) {
		p.updateDetailView(newIndex)
	})
	// Detail Pane
	p.detailPanel = tview.NewFlex().SetDirection(tview.FlexRow)
	p.detailPanel.SetBorderPadding(0, 0, 1, 1)

	// Create the overview widget with an empty list of mods initially.
	// It will be updated with the real list when the page is shown.
	p.detailOverviewWidget = NewOverviewWidget(nil)
	p.detailSummaryText = tview.NewTextView().SetDynamicColors(true)
	p.detailSetsText = tview.NewTextView().SetDynamicColors(true).SetWordWrap(true).SetRegions(true)

	detailOverviewFrame := NewTitleFrame(p.detailOverviewWidget, "Overview")
	detailOverviewFrame.SetBorder(false) // Remove redundant border if parent has one

	p.detailPanel.AddItem(p.detailSummaryText, 3, 0, false) // Height for summary
	p.detailPanel.AddItem(detailOverviewFrame, 3, 0, false) // Height for overview widget
	p.detailPanel.AddItem(p.detailSetsText, 0, 1, false)    // Remaining space for sets

	// Main Layout
	p.Flex.AddItem(NewTitleFrame(p.masterList, "History"), 0, 1, true).
		AddItem(tview.NewBox(), 1, 0, false).
		AddItem(NewTitleFrame(p.detailPanel, "Details"), 0, 2, false)

	p.setInputCapture()

	p.refreshHistory()

	return p
}

// setInputCapture handles navigation within the page.
func (p *HistoryPage) setInputCapture() {
	p.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {

		if event.Key() == tcell.KeyEscape || (event.Key() == tcell.KeyCtrlH && event.Modifiers()&tcell.ModCtrl != 0) {
			p.app.Navigation().CloseModal()
			return nil
		}

		return event
	})
}

// refreshHistory populates the master list with history entries.
func (p *HistoryPage) refreshHistory() {
	p.masterList.Clear()

	if p.app.GetSearchProcess() == nil {
		p.masterList.AddItem(tview.NewTextView().SetText("Process not started yet."), 1, 0, false)
		p.updateDetailView(-1)
		return
	}

	p.historyCache = p.app.GetSearchProcess().GetExecutionLog().GetEntries()
	allMods := p.app.GetModState().GetAllModIDs()
	p.detailOverviewWidget.SetAllMods(allMods)

	if len(p.historyCache) == 0 {
		p.masterList.AddItem(tview.NewTextView().SetText("No history yet."), 1, 0, false)
		p.updateDetailView(-1)
		return
	}

	for i, entry := range p.historyCache {
		iteration := len(entry.StateBeforeTest.ConflictSet) + 1
		// FIXME: duplicate code (not just this)
		if entry.Plan.IsVerificationStep {
			iteration -= 1
		}
		step := i + 1
		summary := fmt.Sprintf("Step %d (Iter %d): [yellow]%s[-]",
			step, iteration, entry.Result)

		summaryView := tview.NewTextView().SetDynamicColors(true).SetText(summary)
		summaryView.SetBackgroundColor(tcell.ColorNone)
		overview := NewOverviewWidget(allMods)
		overview.SetBackgroundColor(tcell.ColorNone)
		p.updateOverviewState(overview, &entry)

		// Create the self-contained row
		row := tview.NewFlex().SetDirection(tview.FlexRow).
			AddItem(summaryView, 1, 0, false).
			AddItem(overview, 1, 0, false)

		if i != len(p.historyCache)-1 {
			sep := NewHorizontalSeparator(tcell.ColorGray)
			sep.SetBackgroundColor(tcell.ColorNone)
			row.AddItem(sep, 1, 0, false)
		}

		// Each item added to FlexList is a complete visual block of height 3.
		p.masterList.AddItem(row, historyRowHeight, 0, false)
	}

	// Set initial selection to the last item. This will trigger the ChangedFunc.
	if p.masterList.GetItemCount() > 0 {
		p.masterList.SetCurrentItem(p.masterList.GetItemCount() - 1)
	} else {
		p.updateDetailView(-1) // Ensure detail view is cleared if no items
	}
}

func (p *HistoryPage) updateOverviewState(overview *OverviewWidget, entry *conflict.CompletedTest) {
	// Use the sets from the entry's historical state for
	good, candidates := entry.StateBeforeTest.GetBisectionSets()
	effective, _ := p.app.GetResolver().ResolveEffectiveSet(setToSlice(entry.Plan.ModIDsToTest), p.app.GetModState().GetAllMods(), p.app.GetModState().GetPotentialProviders(), p.app.GetModState().GetModStatusesSnapshot())
	if entry.StateBeforeTest.IsVerifyingConflictSet {
		// This makes the display more intuitive
		candidates = map[string]struct{}{}
	}
	// Don't show problematic mods as good
	good = subtractSet(good, entry.StateBeforeTest.ConflictSet)
	overview.UpdateState(entry.StateBeforeTest.ConflictSet, good, candidates, effective)
}

// updateDetailView shows the details for the selected history entry.
func (p *HistoryPage) updateDetailView(index int) {
	if index < 0 || index >= len(p.historyCache) {
		p.detailSummaryText.SetText("")
		p.detailOverviewWidget.UpdateState(nil, nil, nil, nil)
		p.detailSetsText.SetText("")
		return
	}

	entry := p.historyCache[index]
	p.updateOverviewState(p.detailOverviewWidget, &entry)

	// Derive summary text
	iteration := len(entry.StateBeforeTest.ConflictSet) + 1
	if entry.Plan.IsVerificationStep {
		iteration -= 1
	}
	step := index + 1
	numTested := len(entry.Plan.ModIDsToTest)
	numCandidates := len(entry.StateBeforeTest.Candidates)

	// Generate StateDescription on the fly for display
	stateDesc := ""
	if entry.Plan.IsVerificationStep {
		stateDesc = "This was a verification test of the current conflict set."
	} else if len(entry.StateBeforeTest.SearchStack) > 0 {
		stateDesc = "This was a bisection step within a candidate set."
	} else {
		stateDesc = "This was the initial test of a candidate set."
	}

	summary := fmt.Sprintf("Step %d - Iteration %d\nResult: [yellow]%s[-]\n%s\nTested: %d mods, Candidates: %d mods remaining",
		step, iteration, entry.Result, stateDesc, numTested, numCandidates)
	p.detailSummaryText.SetText(summary)

	// Display the sets. Convert maps to sorted slices for consistent display.
	problematicList := setToSlice(entry.StateBeforeTest.ConflictSet)
	testSetList := setToSlice(entry.Plan.ModIDsToTest)
	good := subtractSet(entry.StateBeforeTest.Background, entry.StateBeforeTest.ConflictSet)
	goodList := setToSlice(good)

	sets := fmt.Sprintf("[::b]Problematic Mods[-:-:-] (%d):\n%s\n\n[::b]Mods Tested[-:-:-] (%d):\n%s\n\n[::b]Good Mods[-:-:-] (%d):\n%s",
		len(problematicList), strings.Join(problematicList, "\n"),
		len(testSetList), strings.Join(testSetList, "\n"),
		len(goodList), strings.Join(goodList, "\n"))
	p.detailSetsText.SetText(sets)
}

// Page interface implementation
func (p *HistoryPage) GetActionPrompts() map[string]string {
	return map[string]string{
		"↑/↓":        "Navigate History",
		"PgUp/PgDn":  "Scroll Page",
		"Home/End":   "Scroll Top/Bottom",
		"ESC/Ctrl+H": "Close",
	}
}
func (p *HistoryPage) GetStatusPrimitive() *tview.TextView { return nil }

// GetFocusablePrimitives implements the Focusable interface.
func (p *HistoryPage) GetFocusablePrimitives() []tview.Primitive {
	return []tview.Primitive{
		p.masterList,
		p.detailSetsText,
	}
}
