package pages

import (
	"fmt"
	"strings"

	"github.com/Qendolin/fabric-mod-bisect-tool/pkg/core/sets"
	"github.com/Qendolin/fabric-mod-bisect-tool/pkg/ui"
	"github.com/Qendolin/fabric-mod-bisect-tool/pkg/ui/widgets"
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

const PageResultID = "result_page"

// ResultPage displays the final or intermediate results of the bisection search.
type ResultPage struct {
	*tview.Flex
	app            ui.AppInterface
	statusText     *tview.TextView
	resultView     *tview.TextView
	closeButton    *tview.Button
	continueButton *tview.Button
}

// NewResultPage creates a new ResultPage.
func NewResultPage(app ui.AppInterface) *ResultPage {
	p := &ResultPage{
		Flex:       tview.NewFlex().SetDirection(tview.FlexRow),
		app:        app,
		statusText: tview.NewTextView().SetDynamicColors(true),
	}

	vm := app.GetViewModel()

	title, message, explanation := p.formatContent(&vm)

	p.resultView = tview.NewTextView().
		SetDynamicColors(true).
		SetWordWrap(true).
		SetText(message)
	p.resultView.SetBorderPadding(1, 0, 1, 1)

	explanationView := tview.NewTextView().
		SetDynamicColors(true).
		SetText(explanation)
	explanationView.SetBorderPadding(1, 1, 1, 1)

	messageFrame := widgets.NewTitleFrame(p.resultView, "Result")
	explanationFrame := widgets.NewTitleFrame(explanationView, "What to do next")

	p.closeButton = tview.NewButton("Close").
		SetSelectedFunc(func() {
			p.app.Navigation().CloseModal()
		})
	widgets.DefaultStyleButton(p.closeButton)

	p.continueButton = tview.NewButton("Continue Search").
		SetSelectedFunc(func() {
			p.app.Dialogs().ShowQuestionDialog(
				"Confirmation",
				"This will start a new search for the next conflict set within the remaining mods. Continue?",
				"",
				func() { // OnYes
					p.app.Navigation().CloseModal()
					p.app.ContinueSearch()
				},
				nil, // OnNo
			)
		})
	widgets.DefaultStyleButton(p.continueButton)

	// Determine if the "Continue Search" button should be shown.
	canContinue := vm.IsComplete && len(vm.CandidateSet) > 0
	buttonLayout := tview.NewFlex().
		AddItem(tview.NewBox(), 0, 1, false)

	if canContinue {
		buttonLayout.AddItem(p.closeButton, 15, 0, true)
		buttonLayout.AddItem(tview.NewBox(), 1, 0, false)
		buttonLayout.AddItem(p.continueButton, 20, 0, false)
	} else {
		p.continueButton.SetDisabled(true)
		buttonLayout.AddItem(p.closeButton, 15, 0, true)
	}
	buttonLayout.AddItem(tview.NewBox(), 0, 1, false)

	p.AddItem(messageFrame, 0, 2, false).
		AddItem(explanationFrame, 7, 0, false).
		AddItem(buttonLayout, 3, 0, true)

	p.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		if event.Key() == tcell.KeyEscape {
			app.Navigation().CloseModal()
			return nil
		}
		return event
	})

	p.statusText.SetText(title)

	return p
}

// formatContent generates the appropriate text based on the bisection ViewModel.
func (p *ResultPage) formatContent(vm *ui.BisectionViewModel) (title, message, explanation string) {

	if !vm.IsReady {
		title = "Search In Progress"
		message = "No results yet."
		explanation = "You haven't started the bisection yet."
		return
	}

	modState := p.app.GetStateManager()
	mods := modState.GetAllMods()

	// Combine all found conflict sets for display. For a complete search,
	// this includes the final set found.
	allFoundSets := vm.AllConflictSets
	if vm.IsComplete && len(vm.CurrentConflictSet) > 0 {
		allFoundSets = append(allFoundSets, vm.CurrentConflictSet)
	}

	var messageBuilder strings.Builder

	if vm.IsComplete {
		title = "Search Complete"
		if len(allFoundSets) > 0 {
			// Adapt wording based on the number of conflicts found.
			if len(allFoundSets) == 1 {
				messageBuilder.WriteString(fmt.Sprintf("Found [yellow::b]%d[-:-:-] problematic mod(s):\n", len(allFoundSets[0])))
			} else {
				messageBuilder.WriteString(fmt.Sprintf("Found [yellow::b]%d[-:-:-] independent conflict sets:\n", len(allFoundSets)))
			}

			// Display each conflict set.
			for i, conflictSet := range allFoundSets {
				if len(allFoundSets) > 1 {
					messageBuilder.WriteString(fmt.Sprintf("\n[::u]Conflict Set #%d[-:-:-]\n", i+1))
				}
				for _, id := range sets.MakeSlice(conflictSet) {
					modInfo := ""
					if mod, ok := mods[id]; ok {
						modInfo = fmt.Sprintf("(%s %s) from '%s.jar'", mod.FriendlyName(), mod.FabricInfo.Version, mod.BaseFilename)
					}
					messageBuilder.WriteString(fmt.Sprintf("  - [red::b]%s[-:-:-] %s\n", id, modInfo))
				}

				allModsSet := sets.MakeSet(modState.GetAllModIDs())
				unresolvable := modState.Resolver().CalculateTransitivelyUnresolvableMods(sets.Subtract(allModsSet, conflictSet))

				if len(unresolvable) > 0 {
					messageBuilder.WriteString("    [gray]└ Disabling this set would also require disabling:\n")
					unresolvableMods := sets.MakeSlice(unresolvable)
					for _, modID := range unresolvableMods {
						mod := mods[modID]
						messageBuilder.WriteString(fmt.Sprintf("      - [yellow]%s[-:-:-] from '%s.jar'\n", modID, mod.BaseFilename))
					}
				}
			}

			// Build the explanation text.
			explanation = "To fix the issue, disable all mods listed above and then relaunch the game.\nOnce confirmed, please report the incompatibility to the mod authors."
			if len(vm.CandidateSet) > 0 {
				explanation += "\n\nIf issues persist, use 'Continue Search' to find other conflicts."
			}

		} else {
			messageBuilder.WriteString("No problematic mods were found.")
			explanation = "The bisection process completed without isolating a specific cause for failure."
		}
	} else if vm.LastFoundElement != "" {
		// This branch handles the "Intermediate Result" dialog.
		title = "Intermediate Result"
		if len(allFoundSets) > 0 {
			messageBuilder.WriteString(fmt.Sprintf("Found [yellow::b]%d[-:-:-] conflict set(s) so far. The most recent is:\n", len(allFoundSets)))
			// Show only the most recently completed conflict set.
			lastSet := allFoundSets[len(allFoundSets)-1]
			for _, id := range sets.MakeSlice(lastSet) {
				modInfo := ""
				if mod, ok := mods[id]; ok {
					modInfo = fmt.Sprintf("(%s %s)", mod.FriendlyName(), mod.FabricInfo.Version)
				}
				messageBuilder.WriteString(fmt.Sprintf("  - [red::b]%s[-:-:-] %s\n", id, modInfo))
			}
		} else {
			// Fallback for the very first found element.
			messageBuilder.WriteString(fmt.Sprintf("Found [yellow::b]%d[-:-:-] problematic mod(s) so far:\n", len(vm.CurrentConflictSet)))
			for _, id := range sets.MakeSlice(vm.CurrentConflictSet) {
				modInfo := ""
				if mod, ok := mods[id]; ok {
					modInfo = fmt.Sprintf("(%s %s)", mod.FriendlyName(), mod.FabricInfo.Version)
				}
				messageBuilder.WriteString(fmt.Sprintf("  - [red::b]%s[-:-:-] %s\n", id, modInfo))
			}
		}

		explanation = "The last test isolated a new conflict, but the bisection is not yet complete.\nPress '[::b]S[-:-:-]' on the main page to continue the search."
	} else {
		title = "Search In Progress"
		messageBuilder.WriteString("No results yet.")
		explanation = "You haven't completed a bisection iteration.\nPress '[::b]S[-:-:-]' on the main page to continue the search."
	}

	message = messageBuilder.String()
	return
}

// GetActionPrompts returns the key actions for the page.
func (p *ResultPage) GetActionPrompts() []ui.ActionPrompt {
	return []ui.ActionPrompt{
		{Input: "↑/↓", Action: "Scroll Text"},
	}
}

// GetStatusPrimitive returns the tview.Primitive that displays the page's status
func (p *ResultPage) GetStatusPrimitive() *tview.TextView {
	return p.statusText
}

// GetStatusPrimitive returns the tview.Primitive that displays the page's status
func (p *ResultPage) GetFocusablePrimitives() []tview.Primitive {
	primitives := []tview.Primitive{p.closeButton}
	if !p.continueButton.IsDisabled() {
		primitives = append(primitives, p.continueButton)
	}
	primitives = append(primitives, p.resultView)
	return primitives
}
