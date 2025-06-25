package ui

import (
	"fmt"
	"sort"

	"github.com/Qendolin/fabric-mod-bisect-tool/app/core/systemrunner"
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

// PageMainID is the unique identifier for the MainPage.
const PageMainID = "main_page"

// MainPage is the primary view for the bisection process.
type MainPage struct {
	*tview.Flex
	app AppInterface

	// UI Components
	overviewText *tview.TextView
	stepButton   *tview.Button
	tabs         *TabbedPanes
	statusText   *tview.TextView

	// Tab Content
	candidatesList       *SearchableList
	candidatesTitle      *TitleFrame
	knownGoodList        *SearchableList
	knownGoodTitle       *TitleFrame
	testGroupList        *SearchableList
	testGroupTitle       *TitleFrame
	problematicModsList  *SearchableList
	problematicModsTitle *TitleFrame
}

// NewMainPage creates a new MainPage instance.
func NewMainPage(app AppInterface) Page {
	p := &MainPage{
		Flex:       tview.NewFlex().SetDirection(tview.FlexRow),
		app:        app,
		statusText: tview.NewTextView().SetDynamicColors(true),
	}
	p.setupLayout()
	p.SetInputCapture(p.inputHandler())
	p.RefreshSearchState()
	p.statusText.SetText("Mods loaded, ready to start bisection.")
	return p
}

func (p *MainPage) setupLayout() {
	p.overviewText = tview.NewTextView().SetDynamicColors(true)
	p.stepButton = tview.NewButton("Step").SetSelectedFunc(p.app.Step)

	overviewFlex := tview.NewFlex().
		AddItem(p.overviewText, 0, 1, false).
		AddItem(tview.NewBox(), 5, 0, false).
		AddItem(p.stepButton, 10, 0, false)
	overviewFlex.SetBorderPadding(0, 0, 1, 1)

	p.tabs = NewTabbedPanes()
	p.tabs.SetBorderPadding(0, 0, 1, 1)

	// -- Tab 1: Search Pool --
	p.candidatesList = NewSearchableList()
	p.candidatesTitle = NewTitleFrame(p.candidatesList, "Candidates (Being Searched)")
	p.knownGoodList = NewSearchableList()
	p.knownGoodTitle = NewTitleFrame(p.knownGoodList, "Known Good (For This Search)")
	searchPoolFlex := tview.NewFlex().
		AddItem(p.candidatesTitle, 0, 1, true).
		AddItem(nil, 1, 0, false).
		AddItem(p.knownGoodTitle, 0, 1, true)

	// Wrap the flex layout in our FocusWrapper
	searchPoolPage := NewFocusWrapper(searchPoolFlex, func() []tview.Primitive {
		return []tview.Primitive{p.candidatesList, p.knownGoodList}
	})
	p.tabs.AddTab("Search Pool", searchPoolPage)

	// -- Tab 2: Test Group --
	p.testGroupList = NewSearchableList()
	p.testGroupTitle = NewTitleFrame(p.testGroupList, "Mods in Next Test Group")
	testGroupPage := NewFocusWrapper(p.testGroupTitle, func() []tview.Primitive {
		return []tview.Primitive{p.testGroupList}
	})
	p.tabs.AddTab("Test Group", testGroupPage)

	// -- Tab 3: Problematic Mods --
	p.problematicModsList = NewSearchableList()
	p.problematicModsTitle = NewTitleFrame(p.problematicModsList, "Problematic Mods")
	problematicPage := NewFocusWrapper(p.problematicModsTitle, func() []tview.Primitive {
		return []tview.Primitive{p.problematicModsList}
	})
	p.tabs.AddTab("Problematic Mods", problematicPage)

	p.AddItem(NewTitleFrame(overviewFlex, "Overview"), 6, 0, false).
		AddItem(NewTitleFrame(p.tabs, "Sets"), 0, 1, true)
}

func (p *MainPage) inputHandler() func(event *tcell.EventKey) *tcell.EventKey {
	return func(event *tcell.EventKey) *tcell.EventKey {

		// If page-wide hotkeys are pressed, handle them.
		switch event.Key() {
		case tcell.KeyRune:
			switch event.Rune() {
			case 's', 'S':
				p.app.Step()
				return nil
			case 'u', 'U':
				p.app.Undo()
				return nil
			case 'm', 'M':
				p.statusText.SetText("Manage Mods... (not implemented)")
				return nil
			case 'r', 'R':
				p.app.ResetSearch()
				return nil
			}
		}

		return event // Return event if no one handled it
	}
}

// GetFocusablePrimitives implements the Focusable interface for the MainPage.
func (p *MainPage) GetFocusablePrimitives() []tview.Primitive {
	return []tview.Primitive{
		p.stepButton,
		p.tabs,
	}
}

// RefreshSearchState refreshes the page with the latest searcher state.
func (p *MainPage) RefreshSearchState() {
	searcher := p.app.GetSearcher()
	if searcher == nil {
		return
	}
	state := searcher.GetCurrentState()

	lastResultStr := "N/A"
	if state.LastTestResult != "" { // Check LastTestResult from the state
		color := "green"
		if state.LastTestResult == systemrunner.FAIL {
			color = "red"
		}
		lastResultStr = fmt.Sprintf("[%s]%s[-:-:-]", color, state.LastTestResult)
	}
	currentStatus := "Ready to start Bisection"
	if searcher.IsVerificationStep() {
		currentStatus = "Verifying set of problematic mods"
	} else if searcher.GetTestsExecuted() > 0 && !searcher.IsComplete() {
		currentStatus = "Searching for next problematic mod"
	}
	overview := fmt.Sprintf(
		"Status: %s\nProgress: Test %d / %d (estimated)\nLast Result: %s\nFound Problems: %d",
		currentStatus, searcher.GetTestsExecuted(), searcher.GetEstimatedMaxTests(), lastResultStr, len(state.ConflictSet),
	)
	p.overviewText.SetText(overview)
	p.statusText.SetText(currentStatus)

	modCount := len(searcher.GetAllModIDs())

	p.candidatesList.SetItems(state.Candidates)
	p.candidatesTitle.SetTitle(fmt.Sprintf("Candidates (Being Searched): %d / %d", len(state.Candidates), modCount))
	p.problematicModsList.SetItems(mapKeysFromStruct(state.ConflictSet))
	p.problematicModsTitle.SetTitle(fmt.Sprintf("Problematic Mods: %d", len(state.ConflictSet)))

	knownGoodInStep := difference(state.Background, state.ConflictSet)
	if len(state.SearchStack) > 0 {
		step := state.SearchStack[len(state.SearchStack)-1]
		knownGoodInStep = difference(step.Background, state.ConflictSet)
	}
	p.knownGoodList.SetItems(mapKeysFromStruct(knownGoodInStep))
	p.knownGoodTitle.SetTitle(fmt.Sprintf("Known Good (For This Search): %d / %d", len(knownGoodInStep), modCount))

	nextTestSet, _, hasTest := searcher.PrepareNextTest()
	if hasTest {
		p.testGroupList.SetItems(mapKeysFromStruct(nextTestSet))
	} else {
		p.testGroupList.SetItems([]string{"Search Complete"})
	}
	p.testGroupTitle.SetTitle(fmt.Sprintf("Mods in Next Test Group: %d / %d", len(nextTestSet), modCount))
}

func (p *MainPage) Primitive() tview.Primitive { return p }

func (p *MainPage) GetActionPrompts() map[string]string {
	return map[string]string{
		"S": "Step", "U": "Undo", "M": "Manage Mods", "R": "Reset", "Tab": "Next Element", "Arrows": "Navigate",
	}
}

// GetStatusPrimitive returns the tview.Primitive that displays the page's status
func (p *MainPage) GetStatusPrimitive() *tview.TextView {
	return p.statusText
}

func mapKeysFromStruct(m map[string]struct{}) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func difference(a, b map[string]struct{}) map[string]struct{} {
	diff := make(map[string]struct{})
	for k := range a {
		if _, found := b[k]; !found {
			diff[k] = struct{}{}
		}
	}
	return diff
}
