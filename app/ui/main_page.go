package ui

import (
	"fmt"
	"sort"

	"github.com/Qendolin/fabric-mod-bisect-tool/app/core/conflict"
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
	overviewText   *tview.TextView
	stepButton     *tview.Button
	undoButton     *tview.Button
	tabs           *TabbedPanes
	statusText     *tview.TextView
	overviewWidget *OverviewWidget

	// Tab Content
	candidatesList       *SearchableList
	candidatesTitle      *TitleFrame
	knownGoodList        *SearchableList
	knownGoodTitle       *TitleFrame
	testGroupList        *SearchableList
	testGroupTitle       *TitleFrame
	implicitDepsList     *SearchableList
	implicitDepsTitle    *TitleFrame
	problematicModsList  *SearchableList
	problematicModsTitle *TitleFrame
}

// NewMainPage creates a new MainPage instance.
func NewMainPage(app AppInterface) *MainPage {
	p := &MainPage{
		Flex:           tview.NewFlex().SetDirection(tview.FlexRow),
		app:            app,
		statusText:     tview.NewTextView().SetDynamicColors(true),
		overviewWidget: NewOverviewWidget(nil),
	}
	p.setupLayout()
	p.SetInputCapture(p.inputHandler())
	p.RefreshSearchState()
	p.statusText.SetText("Mods loaded, ready to start bisection.")
	return p
}

// setupLayout initializes and arranges all the UI components of the page.
func (p *MainPage) setupLayout() {
	// --- Overview Area ---
	p.overviewText = tview.NewTextView().SetDynamicColors(true)
	// Can't initialize it yet
	p.overviewWidget = NewOverviewWidget(nil)

	// This new container holds the text and the visual widget.
	overviewContentFlex := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(p.overviewText, 4, 0, false).
		AddItem(p.overviewWidget, 1, 0, false)

	// --- Action Buttons ---
	p.stepButton = tview.NewButton("Start").SetSelectedFunc(p.app.Step)
	DefaultStyleButton(p.stepButton)
	p.undoButton = tview.NewButton("Undo").SetSelectedFunc(p.confirmUndo)
	DefaultStyleButton(p.undoButton)
	buttonFlex := tview.NewFlex().
		AddItem(p.stepButton, 0, 1, true).
		AddItem(nil, 1, 0, false).
		AddItem(p.undoButton, 0, 1, false)
	buttonFlex.SetBorderPadding(1, 1, 0, 0)

	// --- Top-level Flex for Overview Section ---
	overviewFlex := tview.NewFlex().
		AddItem(overviewContentFlex, 0, 1, false).
		AddItem(tview.NewBox(), 1, 0, false).
		AddItem(NewVerticalSeparator(tcell.ColorGray), 1, 0, false).
		AddItem(tview.NewBox(), 1, 0, false).
		AddItem(buttonFlex, 31, 0, true)
	overviewFlex.SetBorderPadding(0, 0, 1, 1)

	// --- Tabs Setup ---
	p.tabs = NewTabbedPanes()
	p.tabs.SetBorderPadding(0, 0, 1, 1)
	p.setupTabPanes()

	// --- Final Page Layout ---
	p.AddItem(NewTitleFrame(overviewFlex, "Overview"), 6, 0, true).
		AddItem(NewTitleFrame(p.tabs, "Sets"), 0, 1, false)
}

// setupTabPanes populates the tabbed container with its pages.
func (p *MainPage) setupTabPanes() {
	p.candidatesList = NewSearchableList()
	p.knownGoodList = NewSearchableList()
	p.candidatesTitle = NewTitleFrame(p.candidatesList, "Candidates (Being Searched)")
	p.knownGoodTitle = NewTitleFrame(p.knownGoodList, "Known Good (For This Search)")
	searchPoolFlex := tview.NewFlex().
		AddItem(p.candidatesTitle, 0, 1, true).
		AddItem(nil, 1, 0, false).
		AddItem(p.knownGoodTitle, 0, 1, true)
	p.tabs.AddTab("Search Pool", NewFocusWrapper(searchPoolFlex, func() []tview.Primitive {
		return []tview.Primitive{p.candidatesList, p.knownGoodList}
	}))

	p.testGroupList = NewSearchableList()
	p.implicitDepsList = NewSearchableList()
	p.testGroupTitle = NewTitleFrame(p.testGroupList, "Mods in Next Test Group")
	p.implicitDepsTitle = NewTitleFrame(p.implicitDepsList, "Implicitly Included Dependencies")
	testGroupFlex := tview.NewFlex().
		AddItem(p.testGroupTitle, 0, 1, true).
		AddItem(nil, 1, 0, false).
		AddItem(p.implicitDepsTitle, 0, 1, true)
	p.tabs.AddTab("Test Group", NewFocusWrapper(testGroupFlex, func() []tview.Primitive {
		return []tview.Primitive{p.testGroupList, p.implicitDepsList}
	}))

	p.problematicModsList = NewSearchableList()
	p.problematicModsTitle = NewTitleFrame(p.problematicModsList, "Problematic Mods")
	p.tabs.AddTab("Problematic Mods", NewFocusWrapper(p.problematicModsTitle, func() []tview.Primitive {
		return []tview.Primitive{p.problematicModsList}
	}))
}

func (p *MainPage) inputHandler() func(event *tcell.EventKey) *tcell.EventKey {
	return func(event *tcell.EventKey) *tcell.EventKey {

		// I don't know a proper fix for this
		if _, ok := p.app.GetFocus().(*tview.InputField); ok {
			return event
		}

		// If page-wide hotkeys are pressed, handle them.
		switch event.Key() {
		case tcell.KeyRune:
			switch event.Rune() {
			case 's', 'S':
				p.app.Step()
				return nil
			case 'u', 'U':
				p.confirmUndo()
				return nil
			case 'm', 'M':
				p.app.Navigation().SwitchTo(PageManageModsID)
				return nil
			case 'r', 'R':
				p.app.Dialogs().ShowQuestionDialog("Are you sure you want to reset the search?", func() {
					p.app.ResetSearch()
				}, nil)
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
		p.undoButton,
		p.tabs,
	}
}

// RefreshSearchState is the main entry point for updating the page's content based on the latest state of the search process.
func (p *MainPage) RefreshSearchState() {
	searchProcess := p.app.GetSearchProcess()
	if searchProcess == nil {
		p.overviewText.SetText("Status: Waiting for mods to be loaded...")
		p.stepButton.SetDisabled(true)
		return
	}

	state := searchProcess.GetCurrentState()
	p.updateOverview(state, searchProcess)
	p.updateModLists(state, searchProcess)
	p.updateTestGroupTab(state, searchProcess)
	p.updateOverviewWidget(state, searchProcess)
}

func (p *MainPage) GetActionPrompts() map[string]string {
	return map[string]string{
		"S": "Step", "U": "Undo", "M": "Manage Mods", "R": "Reset", "Tab": "Next Element", "Arrows": "Navigate",
	}
}

// GetStatusPrimitive returns the tview.Primitive that displays the page's status
func (p *MainPage) GetStatusPrimitive() *tview.TextView {
	return p.statusText
}

// updateOverview updates the main status text and action buttons.
func (p *MainPage) updateOverview(state conflict.SearchState, sp *conflict.SearchProcess) {
	status, buttonText := p.determineStatusAndButtonText(state, sp)
	p.statusText.SetText(status)
	p.stepButton.SetLabel(buttonText)
	p.stepButton.SetDisabled(sp.GetActiveTestPlan() != nil)

	lastResultStr := "N/A"
	if state.LastTestResult != systemrunner.UNDEFINED && state.LastTestResult != "" {
		color := "green"
		if state.LastTestResult == systemrunner.FAIL {
			color = "red"
		}
		lastResultStr = fmt.Sprintf("[%s]%s[-:-:-]", color, state.LastTestResult)
	}

	overviewText := fmt.Sprintf(
		"Status: %s\nProgress: Test %d / ~%d\nLast Result: %s\nFound Problems: %d",
		status, sp.GetStepCount(), sp.GetEstimatedMaxTests(), lastResultStr, len(state.ConflictSet),
	)
	p.overviewText.SetText(overviewText)
}

// determineStatusAndButtonText computes the user-facing status string and the label for the main action button.
func (p *MainPage) determineStatusAndButtonText(state conflict.SearchState, sp *conflict.SearchProcess) (status, buttonText string) {
	activePlan := sp.GetActiveTestPlan()
	isVerifying := (activePlan != nil && activePlan.IsVerificationStep) || (activePlan == nil && state.IsVerifyingConflictSet)

	switch {
	case state.IsComplete:
		return "Search Complete", "Results"
	case activePlan != nil:
		if activePlan.IsVerificationStep {
			return "Verifying final conflict set...", "Step"
		}
		return fmt.Sprintf("Test in progress (Iter %d)...", len(state.ConflictSet)+1), "Step"
	case isVerifying:
		return "Ready to verify conflict set", "Verify"
	case sp.GetStepCount() > 0 || len(state.ConflictSet) > 0:
		return "Ready for next step", "Step"
	default:
		return "Ready to start bisection", "Start"
	}
}

// updateModLists populates the Candidates, Known Good, and Problematic lists.
func (p *MainPage) updateModLists(state conflict.SearchState, sp *conflict.SearchProcess) {
	modCount := len(state.AllModIDs)

	p.updateList(p.candidatesList, p.candidatesTitle, state.Candidates, "Candidates (Being Searched): %d / %d", modCount)
	p.updateList(p.problematicModsList, p.problematicModsTitle, setToSlice(state.ConflictSet), "Problematic Mods: %d", 0)

	// The background for display is the global background, which accumulates good mods.
	goodMods := setToSlice(subtractSet(state.Background, state.ConflictSet))
	p.updateList(p.knownGoodList, p.knownGoodTitle, goodMods, "Known Good (Background): %d", 0)
}

// updateTestGroupTab populates the lists in the "Test Group" tab.
func (p *MainPage) updateTestGroupTab(state conflict.SearchState, sp *conflict.SearchProcess) {
	nextPlan, err := sp.GetNextTestPlan()
	if err != nil || nextPlan == nil {
		p.updateList(p.testGroupList, p.testGroupTitle, nil, "Mods in Next Test Group: %d", 0)
		p.updateList(p.implicitDepsList, p.implicitDepsTitle, nil, "Implicitly Included Dependencies: %d", 0)
		return
	}

	testSet := nextPlan.ModIDsToTest
	p.updateList(p.testGroupList, p.testGroupTitle, setToSlice(testSet), "Mods in Next Test Group: %d", 0)

	// Calculate and display implicit dependencies.
	effectiveSet, _ := p.app.GetResolver().ResolveEffectiveSet(
		systemrunner.SetToSlice(testSet),
		p.app.GetModState().GetAllMods(),
		p.app.GetModState().GetPotentialProviders(),
		p.app.GetModState().GetModStatusesSnapshot(),
	)
	implicitDeps := subtractSet(effectiveSet, testSet)
	p.updateList(p.implicitDepsList, p.implicitDepsTitle, setToSlice(implicitDeps), "Implicitly Included Dependencies: %d", 0)
}

// updateOverviewWidget updates the visual overview bar.
func (p *MainPage) updateOverviewWidget(state conflict.SearchState, sp *conflict.SearchProcess) {
	nextPlan, err := sp.GetNextTestPlan()
	if err != nil {
		p.overviewWidget.UpdateState(state.ConflictSet, nil, nil, nil)
		return
	}

	testSet := nextPlan.ModIDsToTest
	background, candidates := state.GetBisectionSets()

	if state.IsVerifyingConflictSet {
		// This makes the display more intuitive
		candidates = map[string]struct{}{}
	}

	// Don't show problematic mods as good
	goodMods := subtractSet(background, state.ConflictSet)

	// Calculate the full effective set for the test.
	effective, _ := p.app.GetResolver().ResolveEffectiveSet(
		setToSlice(testSet),
		p.app.GetModState().GetAllMods(),
		p.app.GetModState().GetPotentialProviders(),
		p.app.GetModState().GetModStatusesSnapshot(),
	)

	p.overviewWidget.SetAllMods(state.AllModIDs)
	p.overviewWidget.UpdateState(state.ConflictSet, goodMods, candidates, effective)
}

// updateList is a helper to populate a SearchableList and its title.
func (p *MainPage) updateList(list *SearchableList, titleFrame *TitleFrame, mods []string, titleFmt string, total int) {
	if len(mods) > 0 {
		list.SetItems(p.formatModList(mods))
	} else {
		list.SetItems([]string{"---"})
	}

	if total > 0 {
		titleFrame.SetTitle(fmt.Sprintf(titleFmt, len(mods), total))
	} else {
		titleFrame.SetTitle(fmt.Sprintf(titleFmt, len(mods)))
	}
}

func (p *MainPage) formatModList(modIDs []string) []string {
	allMods := p.app.GetModState().GetAllMods()

	formatted := make([]string, len(modIDs))
	for i, id := range modIDs {
		if mod, ok := allMods[id]; ok {
			formatted[i] = fmt.Sprintf("%s [gray](%s)[-:-:-]", id, mod.FriendlyName())
		} else {
			formatted[i] = id
		}
	}
	return formatted
}

func (p *MainPage) confirmUndo() {
	p.app.Dialogs().ShowQuestionDialog("Are you sure you want to undo the last step?", func() {
		p.app.Undo()
	}, nil)
}

func setToSlice(m map[string]struct{}) []string {
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

func split(mods []string) ([]string, []string) {
	if len(mods) == 0 {
		return []string{}, []string{}
	}
	mid := (len(mods) + 1) / 2
	return mods[:mid], mods[mid:]
}

func stringSliceToSet(s []string) map[string]struct{} {
	set := make(map[string]struct{})
	for _, item := range s {
		set[item] = struct{}{}
	}
	return set
}

func union(a, b map[string]struct{}) map[string]struct{} {
	res := make(map[string]struct{}, len(a)+len(b))
	for k := range a {
		res[k] = struct{}{}
	}
	for k := range b {
		res[k] = struct{}{}
	}
	return res
}
