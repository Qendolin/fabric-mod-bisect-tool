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

func (p *MainPage) setupLayout() {
	p.overviewText = tview.NewTextView().SetDynamicColors(true)
	p.stepButton = tview.NewButton("Start").SetSelectedFunc(p.app.Step)
	DefaultStyleButton(p.stepButton)

	p.undoButton = tview.NewButton("Undo").SetSelectedFunc(p.confirmUndo)
	DefaultStyleButton(p.undoButton)

	buttonFlex := tview.NewFlex().
		AddItem(p.stepButton, 0, 1, true).
		AddItem(nil, 1, 0, false).
		AddItem(p.undoButton, 0, 1, false)
	buttonFlex.SetBorderPadding(1, 1, 0, 0)

	overviewCol1Flex := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(p.overviewText, 4, 0, false).
		AddItem(p.overviewWidget, 1, 0, false)

	overviewFlex := tview.NewFlex().
		AddItem(overviewCol1Flex, 0, 1, false).
		AddItem(tview.NewBox(), 1, 0, false).
		AddItem(NewVerticalSeparator(tcell.ColorGray), 1, 0, false).
		AddItem(tview.NewBox(), 1, 0, false).
		AddItem(buttonFlex, 31, 0, true)
	overviewFlex.SetBorderPadding(0, 0, 1, 1)

	p.tabs = NewTabbedPanes()
	p.tabs.SetBorderPadding(0, 0, 1, 1)

	// -- Tab 1: Search Pool --
	p.candidatesList = NewSearchableList()
	p.candidatesList.SetItems([]string{"---"})
	p.candidatesTitle = NewTitleFrame(p.candidatesList, "Candidates (Being Searched)")
	p.knownGoodList = NewSearchableList()
	p.knownGoodList.SetItems([]string{"---"})
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
	p.testGroupList.SetItems([]string{"---"})
	p.testGroupTitle = NewTitleFrame(p.testGroupList, "Mods in Next Test Group")

	p.implicitDepsList = NewSearchableList()
	p.implicitDepsList.SetItems([]string{"---"})
	p.implicitDepsTitle = NewTitleFrame(p.implicitDepsList, "Implicitly Included Dependencies")

	testGroupFlex := tview.NewFlex().
		AddItem(p.testGroupTitle, 0, 1, true).
		AddItem(nil, 1, 0, false).
		AddItem(p.implicitDepsTitle, 0, 1, true)

	testGroupPage := NewFocusWrapper(testGroupFlex, func() []tview.Primitive {
		return []tview.Primitive{p.testGroupList, p.implicitDepsList}
	})
	p.tabs.AddTab("Test Group", testGroupPage)

	// -- Tab 3: Problematic Mods --
	p.problematicModsList = NewSearchableList()
	p.problematicModsList.SetItems([]string{"---"})
	p.problematicModsTitle = NewTitleFrame(p.problematicModsList, "Problematic Mods")
	problematicPage := NewFocusWrapper(p.problematicModsTitle, func() []tview.Primitive {
		return []tview.Primitive{p.problematicModsList}
	})
	p.tabs.AddTab("Problematic Mods", problematicPage)

	p.AddItem(NewTitleFrame(overviewFlex, "Overview"), 6, 0, true).
		AddItem(NewTitleFrame(p.tabs, "Sets"), 0, 1, false)
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

// RefreshSearchState refreshes the page with the latest searcher state.
func (p *MainPage) RefreshSearchState() {
	searchProcess := p.app.GetSearchProcess()
	if searchProcess == nil {
		p.overviewText.SetText("Status: Waiting for mods to be loaded...")
		p.stepButton.SetDisabled(true)
		return
	}

	state := searchProcess.GetCurrentState()
	activePlan := searchProcess.GetActiveTestPlan()

	// --- 1. Last Result ---
	lastResultStr := "N/A"
	if state.LastTestResult != systemrunner.UNKNOWN {
		color := "green"
		if state.LastTestResult == systemrunner.FAIL {
			color = "red"
		}
		lastResultStr = fmt.Sprintf("[%s]%s[-:-:-]", color, state.LastTestResult)
	}

	// --- 2. Current Status & Button Text ---
	var currentStatus, buttonText string
	if state.IsComplete {
		currentStatus = "Search Complete"
		buttonText = "Done"
	} else if activePlan != nil {
		if activePlan.IsVerificationStep {
			currentStatus = "Verifying final conflict set..."
		} else {
			currentStatus = fmt.Sprintf("Test in progress (Iter %d)...", len(state.ConflictSet)+1)
		}
		buttonText = "Step" // Button should be disabled when test is in progress.
	} else if state.IsVerifyingConflictSet {
		currentStatus = "Ready to verify conflict set"
		buttonText = "Verify"
	} else if searchProcess.GetStepCount() > 0 || len(state.ConflictSet) > 0 {
		currentStatus = "Ready for next step"
		buttonText = "Step"
	} else {
		currentStatus = "Ready to start bisection"
		buttonText = "Start"
	}
	p.statusText.SetText(currentStatus)
	p.stepButton.SetLabel(buttonText)
	p.stepButton.SetDisabled(state.IsComplete || activePlan != nil)

	// --- 3. Progress Estimation ---
	estimatedMaxTests := searchProcess.GetEstimatedMaxTests()

	overview := fmt.Sprintf(
		"Status: %s\nProgress: Test %d / ~%d\nLast Result: %s\nFound Problems: %d",
		currentStatus, searchProcess.GetStepCount(), estimatedMaxTests, lastResultStr, len(state.ConflictSet),
	)
	p.overviewText.SetText(overview)

	// --- 4. List Population & Overview Widget (Rest of the function) ---
	modCount := len(state.AllModIDs)

	var candidates []string
	var knownGoodSet map[string]struct{}
	if step, ok := state.GetCurrentStep(); ok {
		candidates = step.Candidates
		knownGoodSet = step.Background
		if state.IsVerifyingConflictSet {
			candidates = setToSlice(state.ConflictSet)
		}
	} else {
		candidates = state.Candidates
		knownGoodSet = state.Background
	}

	p.candidatesList.SetItems(p.formatModList(candidates))
	p.candidatesTitle.SetTitle(fmt.Sprintf("Candidates (Being Searched): %d / %d", len(candidates), modCount))

	if len(state.ConflictSet) > 0 {
		p.problematicModsList.SetItems(p.formatModList(setToSlice(state.ConflictSet)))
	} else {
		p.problematicModsList.SetItems([]string{"---"})
	}
	p.problematicModsTitle.SetTitle(fmt.Sprintf("Problematic Mods: %d", len(state.ConflictSet)))

	if len(knownGoodSet) > 0 {
		knownGoodSet = subtractSet(knownGoodSet, state.ConflictSet)
		p.knownGoodList.SetItems(p.formatModList(setToSlice(knownGoodSet)))
	} else {
		p.knownGoodList.SetItems([]string{"---"})
	}
	p.knownGoodTitle.SetTitle(fmt.Sprintf("Known Good (Background): %d", len(knownGoodSet)))

	// Get the next test set for preview purposes.
	nextPlan, err := searchProcess.GetNextTestPlan()
	if err == nil && nextPlan != nil {
		testSet := nextPlan.ModIDsToTest
		p.testGroupList.SetItems(p.formatModList(setToSlice(nextPlan.ModIDsToTest)))
		p.testGroupTitle.SetTitle(fmt.Sprintf("Mods in Next Test Group: %d", len(nextPlan.ModIDsToTest)))

		effectiveSet, _ := p.app.GetResolver().ResolveEffectiveSet(
			systemrunner.SetToSlice(testSet),
			p.app.GetModState().GetAllMods(),
			p.app.GetModState().GetPotentialProviders(),
			p.app.GetModState().GetModStatusesSnapshot(),
		)

		// Subtract the explicit test set to find the implicit dependencies.
		implicitDeps := subtractSet(effectiveSet, testSet)

		if len(implicitDeps) > 0 {
			p.implicitDepsList.SetItems(p.formatModList(setToSlice(implicitDeps)))
		} else {
			p.implicitDepsList.SetItems([]string{"---"})
		}
		p.implicitDepsTitle.SetTitle(fmt.Sprintf("Implicitly Included Dependencies: %d", len(implicitDeps)))
	} else {
		p.testGroupList.SetItems([]string{"---"})
		p.testGroupTitle.SetTitle("Mods in Next Test Group: 0")
		p.implicitDepsList.SetItems([]string{"---"})
		p.implicitDepsTitle.SetTitle("Implicitly Included Dependencies: 0")
	}

	// c1 should be the same as testSet but I'm not sure
	_, c1, c2 := state.GetBisectionSets()
	if state.IsVerifyingConflictSet {
		// This makes the display more intuitive
		c1 = state.ConflictSet
		c2 = map[string]struct{}{}
	}
	effective, _ := p.app.GetResolver().ResolveEffectiveSet(setToSlice(c1), p.app.GetModState().GetAllMods(), p.app.GetModState().GetPotentialProviders(), p.app.GetModState().GetModStatusesSnapshot())
	p.overviewWidget.SetAllMods(state.AllModIDs)
	p.overviewWidget.UpdateState(state.ConflictSet, knownGoodSet, c1, c2, effective)
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
