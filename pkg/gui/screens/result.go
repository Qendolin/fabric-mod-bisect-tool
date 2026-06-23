package screens

import (
	"fmt"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"github.com/Qendolin/fabric-mod-bisect-tool/pkg/core/sets"
)

type ResultScreen struct {
	app        App
	content    fyne.CanvasObject
	resultsBox *fyne.Container
}

func NewResultScreen(app App) *ResultScreen {
	s := &ResultScreen{app: app}
	s.build()
	return s
}

// buildModList generates the rows for problematic mods
func (s *ResultScreen) buildModList(modsList []string) fyne.CanvasObject {
	modListContainer := container.NewVBox()

	modState := s.app.GetStateManager()
	allMods := modState.GetAllMods()

	for _, modID := range modsList {
		icon := widget.NewIcon(theme.WarningIcon())

		modName := modID
		jarName := "Unknown Jar File"

		if mod, ok := allMods[modID]; ok {
			modName = fmt.Sprintf("%s (%s)", mod.FriendlyName(), mod.Metadata.Version)
			jarName = fmt.Sprintf("%s.jar", mod.BaseFilename)
		}

		label := widget.NewRichText()
		label.Segments = append(label.Segments, &widget.TextSegment{Style: widget.RichTextStyle{TextStyle: fyne.TextStyle{Bold: true}}, Text: modName})
		label.Segments = append(label.Segments, &widget.TextSegment{Style: widget.RichTextStyle{TextStyle: fyne.TextStyle{Italic: true}}, Text: jarName})

		row := container.NewHBox(icon, label)
		modListContainer.Add(row)
	}

	return container.NewPadded(modListContainer)
}

func (s *ResultScreen) build() {
	title := widget.NewLabelWithStyle("Bisection Complete", fyne.TextAlignCenter, fyne.TextStyle{Bold: true})
	header := container.NewVBox(title, widget.NewSeparator())

	s.resultsBox = container.NewVBox()

	// Put the dynamic VBox in a single page-level scroll container
	scrollContainer := container.NewVScroll(container.NewPadded(s.resultsBox))

	btnRestart := widget.NewButtonWithIcon("Restart Bisection", theme.MediaReplayIcon(), func() {
		s.app.ResetSearch()
		s.app.SwitchToMainPage()
	})

	btnQuit := widget.NewButtonWithIcon("Quit", theme.CancelIcon(), func() {
		s.app.ShowQuitDialog()
	})
	// Make Quit the primary action button
	btnQuit.Importance = widget.HighImportance

	footerNav := container.NewHBox(layout.NewSpacer(), btnRestart, btnQuit)
	footer := container.NewVBox(widget.NewSeparator(), footerNav)

	mainContent := container.NewBorder(header, footer, nil, nil, scrollContainer)
	s.content = container.NewPadded(mainContent)
}

func (s *ResultScreen) UpdateState() {
	vm := s.app.GetViewModel()
	s.resultsBox.RemoveAll()

	hasConflicts := false

	// 1. Current Active Conflict
	if len(vm.CurrentConflictSet) > 0 {
		hasConflicts = true
		mods := sets.MakeSlice(vm.CurrentConflictSet)

		s.resultsBox.Add(widget.NewLabelWithStyle("Current Conflict", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}))
		s.resultsBox.Add(widget.NewLabel("These mods are currently causing the issue:"))
		s.resultsBox.Add(s.buildModList(mods))
		s.resultsBox.Add(widget.NewSeparator())
	}

	// 2. Previously Found Conflict Sets
	for i, conflictSet := range vm.AllConflictSets {
		hasConflicts = true
		mods := sets.MakeSlice(conflictSet)
		title := fmt.Sprintf("Independent Conflict Set #%d", i+1)

		s.resultsBox.Add(widget.NewLabelWithStyle(title, fyne.TextAlignLeading, fyne.TextStyle{Bold: true}))
		s.resultsBox.Add(widget.NewLabel("This is a separate group of conflicting mods:"))
		s.resultsBox.Add(s.buildModList(mods))
		s.resultsBox.Add(widget.NewSeparator())
	}

	// 3. Next Steps / Actions Panel
	if !hasConflicts {
		s.resultsBox.Add(widget.NewLabelWithStyle("No Conflicts Found", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}))
		successLabel := widget.NewLabel("The bisection process completed without isolating a specific cause for failure. The issue might be external to the mods in this folder.")
		successLabel.Wrapping = fyne.TextWrapWord
		s.resultsBox.Add(successLabel)
	} else {
		s.resultsBox.Add(widget.NewLabelWithStyle("What to do next", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}))

		instructionText := widget.NewLabel("To fix your issue, disable all the problematic mods listed above and relaunch the game.\n\nOnce confirmed, please consider reporting the incompatibility to the respective mod authors.")
		instructionText.Wrapping = fyne.TextWrapWord
		s.resultsBox.Add(instructionText)

		// Display Continue Search Option if candidates remain
		if vm.IsComplete && len(vm.CandidateSet) > 0 {
			s.resultsBox.Add(widget.NewSeparator())

			s.resultsBox.Add(widget.NewLabelWithStyle("Still having issues?", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}))

			continueExplainer := widget.NewLabel("If you disabled the mods above but your game still has the issue, there might be additional conflicting mods. You can continue the bisection process to find them among the remaining candidates.")
			continueExplainer.Wrapping = fyne.TextWrapWord
			s.resultsBox.Add(continueExplainer)

			btnContinue := widget.NewButtonWithIcon("Continue Search", theme.SearchIcon(), func() {
				dialog.ShowConfirm(
					"Continue Search",
					"This will start a new search for the next conflict set within the remaining mods. Continue?",
					func(confirmed bool) {
						if confirmed {
							s.app.ContinueSearch()
							s.app.SwitchToMainPage()
						}
					}, s.app.GetWindow())
			})
			// Wrapped in HBox so it doesn't stretch to the edges of the window
			s.resultsBox.Add(container.NewHBox(btnContinue))
		}
	}

	// 4. Cleared Mods (Hidden behind Accordion)
	clearedList := sets.MakeSlice(vm.ClearedSet)
	if len(clearedList) > 0 {
		clearedText := widget.NewLabel(strings.Join(clearedList, ", "))
		clearedText.Wrapping = fyne.TextWrapWord
		accordionItem := widget.NewAccordionItem(fmt.Sprintf("View Cleared Mods (%d)", len(clearedList)), container.NewPadded(clearedText))

		s.resultsBox.Add(widget.NewSeparator())
		s.resultsBox.Add(widget.NewAccordion(accordionItem))
	}

	s.resultsBox.Refresh()
}

func (s *ResultScreen) GetContent() fyne.CanvasObject {
	return s.content
}
