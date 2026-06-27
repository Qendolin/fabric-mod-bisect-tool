package pages

import (
	"fmt"
	"sort"
	"strings"

	"github.com/Qendolin/fabric-mod-bisect-tool/pkg/core/mods"
	"github.com/Qendolin/fabric-mod-bisect-tool/pkg/core/sets"
	"github.com/Qendolin/fabric-mod-bisect-tool/pkg/tui"
	"github.com/Qendolin/fabric-mod-bisect-tool/pkg/tui/widgets"
	"github.com/Qendolin/fabric-mod-bisect-tool/pkg/ui"
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

const PageResultID = "result_page"

// ResultPage displays the final or intermediate results of the bisection search.
type ResultPage struct {
	*tview.Flex
	app            tui.TUIApp
	statusText     *tview.TextView
	resultView     *tview.TextView
	closeButton    *tview.Button
	continueButton *tview.Button
}

// NewResultPage creates a new ResultPage.
func NewResultPage(app tui.TUIApp) *ResultPage {
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
		return formatNotReadyContent()
	}

	modState := p.app.GetStateManager()
	modMap := modState.GetAllMods()

	// Combine all found conflict sets for display. For a complete search,
	// this includes the final set found.
	allFoundSets := vm.AllConflictSets
	if vm.IsComplete && len(vm.CurrentConflictSet) > 0 {
		allFoundSets = append(allFoundSets, vm.CurrentConflictSet)
	}

	if vm.IsComplete {
		return formatCompleteContent(vm, allFoundSets, modMap, modState)
	}

	if vm.LastFoundElement != "" {
		return formatInProgressContent(vm, modMap, modState)
	}

	// No conflict element has been found yet.
	return formatNoResultsYetContent()
}

// ---------------------------------------------------------------------------
// State-level formatters
// ---------------------------------------------------------------------------

// formatNotReadyContent: the bisection has not been started at all.
func formatNotReadyContent() (title, message, explanation string) {
	title = "Search In Progress"
	message = "No results yet."
	explanation = "You haven't started the bisection yet."
	return
}

// formatNoResultsYetContent: the search is running but no conflict element has been
// discovered yet.
func formatNoResultsYetContent() (title, message, explanation string) {
	title = "Search In Progress"
	message = "No results yet."
	explanation = "No conflicts have been found yet. Continue the search on the main page."
	return
}

// formatInProgressContent: at least one conflict element has been found, but the
// search is not yet complete. Handles two sub-states:
//
//   - Element found, awaiting verification: a new element was just isolated but
//     the verification test (does the set reproduce the issue alone?) has not run yet.
//     We do not know yet whether more mods are involved.
//
//   - Set incomplete, searching for next element: verification returned GOOD (the current
//     set is not sufficient by itself), or we are mid-bisection looking for the next
//     element. At least one more mod is known to be involved.
//
// In both sub-states the user can already fix the issue by disabling one of the found mods.
func formatInProgressContent(vm *ui.BisectionViewModel, modMap map[string]*mods.Mod, modState *mods.StateManager) (title, message, explanation string) {
	title = "Intermediate Result"

	var b strings.Builder

	allModsSet := sets.MakeSet(modState.GetAllModIDs())
	generallyUnresolvable := modState.Resolver().CalculateTransitivelyUnresolvableMods(allModsSet)

	// Show any fully completed conflict sets from previous rounds.
	if len(vm.AllConflictSets) > 0 {
		fmt.Fprintf(&b, "Found [yellow::b]%d[-:-:-] complete conflict set(s) in previous rounds.\n", len(vm.AllConflictSets))
		for i, cs := range vm.AllConflictSets {
			if len(vm.AllConflictSets) > 1 {
				fmt.Fprintf(&b, "\n[::u]Conflict #%d[-:-:-]\n", i+1)
			}
			writeConflictSet(&b, cs, allModsSet, modMap, generallyUnresolvable, modState)
		}
		b.WriteString("\n")
	}

	// Show the current, still-growing conflict set.
	fmt.Fprintf(&b, "Current conflict - [yellow::b]%d[-:-:-] mod(s) found so far:\n", len(vm.CurrentConflictSet))
	writeConflictSetMods(&b, vm.CurrentConflictSet, allModsSet, modMap, generallyUnresolvable, modState)

	switch {
	case vm.IsVerificationStep:
		// Element found, awaiting verification: completeness is unknown.
		b.WriteString("  [gray]- And possibly more...\n")
		explanation = "A new conflicting mod was found, but it is not yet known if more are involved.\n" +
			"You can already fix this conflict by disabling one of the mods above.\n" +
			"Or continue the search to verify whether the conflict set is complete."
	default:
		// Set incomplete, searching for next element: at least one more mod is known to exist.
		// This covers both a fresh GOOD verification result and being mid-bisection for the
		// next element (in either case, more mods are known to be part of the conflict).
		b.WriteString("  [gray]- And at least one more...\n")
		explanation = "This conflict involves more mods than found so far.\n" +
			"You can already fix this conflict by disabling one of the mods above.\n" +
			"Or continue the search to find the remaining mods."
	}

	message = b.String()
	return
}

// formatCompleteContent: the search has finished.
func formatCompleteContent(vm *ui.BisectionViewModel, allFoundSets []sets.Set, modMap map[string]*mods.Mod, modState *mods.StateManager) (title, message, explanation string) {
	title = "Search Complete"

	var b strings.Builder

	if len(allFoundSets) == 0 {
		b.WriteString("No problematic mods were found.")
		explanation = "The bisection process completed without isolating a specific cause for failure."
		message = b.String()
		return
	}

	allModsSet := sets.MakeSet(modState.GetAllModIDs())
	generallyUnresolvable := modState.Resolver().CalculateTransitivelyUnresolvableMods(allModsSet)

	if len(allFoundSets) == 1 {
		fmt.Fprintf(&b, "Found [yellow::b]1[-:-:-] conflict set:\n")
	} else {
		fmt.Fprintf(&b, "Found [yellow::b]%d[-:-:-] independent conflict sets:\n", len(allFoundSets))
	}

	for i, conflictSet := range allFoundSets {
		if len(allFoundSets) > 1 {
			fmt.Fprintf(&b, "\n[::u]Conflict #%d[-:-:-]\n", i+1)
		}
		writeConflictSet(&b, conflictSet, allModsSet, modMap, generallyUnresolvable, modState)
	}

	// Generally unresolvable mods section (dependency issues unrelated to conflicts).
	details := modState.Resolver().CalculateUnresolvableModsDetails(allModsSet)
	if len(details.DirectlyUnresolvable) > 0 {
		writeGenerallyUnresolvable(&b, details, modMap)
	}

	if len(allFoundSets) == 1 {
		explanation = "To fix this conflict, disable one of the mods listed above and relaunch the game.\nOnce resolved, please report the incompatibility to the mod authors."
	} else {
		explanation = "To fix each conflict, disable one mod from that conflict's list and relaunch the game.\nOnce resolved, please report the incompatibilities to the mod authors."
	}
	if len(vm.CandidateSet) > 0 {
		explanation += "\n\nIf issues persist, use 'Continue Search' to find other conflicts."
	}

	message = b.String()
	return
}

// ---------------------------------------------------------------------------
// Conflict-set section writers
// ---------------------------------------------------------------------------

// writeConflictSet writes the full block for a complete conflict set: per-mod entries
// (each with its own "also require disabling" sub-list) followed by a footer listing
// any extra mods that only cascade when the entire set is disabled together.
func writeConflictSet(b *strings.Builder, conflictSet, allModsSet sets.Set, modMap map[string]*mods.Mod, generallyUnresolvable sets.Set, modState *mods.StateManager) {
	perModUnresolvableUnion := writeConflictSetMods(b, conflictSet, allModsSet, modMap, generallyUnresolvable, modState)
	writeConflictSetFooter(b, conflictSet, allModsSet, modMap, generallyUnresolvable, perModUnresolvableUnion, modState)
}

// writeConflictSetMods writes one entry per mod in the set, each with an indented
// sub-list of any mods that would also need to be disabled as a side-effect.
// Returns the union of all per-mod cascading sets, used by writeConflictSetFooter
// to avoid showing duplicate information.
func writeConflictSetMods(b *strings.Builder, conflictSet, allModsSet sets.Set, modMap map[string]*mods.Mod, generallyUnresolvable sets.Set, modState *mods.StateManager) (perModUnresolvableUnion sets.Set) {
	perModUnresolvableUnion = sets.Set{}

	for _, id := range sets.MakeSlice(conflictSet) {
		modInfo := ""
		if mod, ok := modMap[id]; ok {
			modInfo = fmt.Sprintf("(%s %s) from '%s.jar'", mod.FriendlyName(), mod.Metadata.Version, mod.BaseFilename)
		}
		fmt.Fprintf(b, "  - [red::b]%s[-:-:-] %s\n", id, modInfo)

		// Per-mod cascading: what else would need to be disabled if only this one mod is removed?
		perModUnresolvable := modState.Resolver().CalculateTransitivelyUnresolvableMods(sets.Subtract(allModsSet, sets.MakeSet([]string{id})))
		perModSpecific := sets.Subtract(perModUnresolvable, generallyUnresolvable)

		for extraID := range perModSpecific {
			perModUnresolvableUnion[extraID] = struct{}{}
		}

		if len(perModSpecific) > 0 {
			b.WriteString("    [gray]└ Disabling this mod would also require disabling:\n")
			for _, depID := range sets.MakeSlice(perModSpecific) {
				if dep, ok := modMap[depID]; ok {
					fmt.Fprintf(b, "      [gray]- %s[-:-:-] from '%s.jar'\n", depID, dep.BaseFilename)
				} else {
					fmt.Fprintf(b, "      [gray]- %s[-:-:-] from unknown\n", depID)
				}
			}
		}
	}

	return
}

// writeConflictSetFooter appends a note about mods that would only become unresolvable
// when *all* mods in the conflict set are disabled simultaneously - i.e., extra cascading
// mods not already surfaced by any individual mod's entry above.
func writeConflictSetFooter(b *strings.Builder, conflictSet, allModsSet sets.Set, modMap map[string]*mods.Mod, generallyUnresolvable, perModUnresolvableUnion sets.Set, modState *mods.StateManager) {
	fullSetUnresolvable := modState.Resolver().CalculateTransitivelyUnresolvableMods(sets.Subtract(allModsSet, conflictSet))
	fullSetSpecific := sets.Subtract(fullSetUnresolvable, generallyUnresolvable)

	// Only show mods not already surfaced by the individual per-mod entries.
	extraIfAll := sets.Subtract(fullSetSpecific, perModUnresolvableUnion)
	if len(extraIfAll) == 0 {
		return
	}

	b.WriteString("  [gray]If you disable all mods in this conflict, you would also need to disable:\n")
	for _, depID := range sets.MakeSlice(extraIfAll) {
		if dep, ok := modMap[depID]; ok {
			fmt.Fprintf(b, "    [gray]- %s[-:-:-] from '%s.jar'\n", depID, dep.BaseFilename)
		} else {
			fmt.Fprintf(b, "    [gray]- %s[-:-:-] from unknown\n", depID)
		}
	}
}

// ---------------------------------------------------------------------------
// Generally-unresolvable section writer
// ---------------------------------------------------------------------------

// writeGenerallyUnresolvable appends the section listing mods with broken dependencies
// that are unrelated to any identified conflict set.
func writeGenerallyUnresolvable(b *strings.Builder, details mods.UnresolvableModDetails, modMap map[string]*mods.Mod) {
	b.WriteString("\n[gray]Mods with unresolved or unmet dependencies (may need manual review):\n")

	// Invert the transitive mapping to easily look up "which mods does this root cause break?"
	causedByRoot := make(map[string]sets.Set)
	for transitiveID, roots := range details.TransitivelyUnresolvable {
		for rootID := range roots {
			if _, ok := causedByRoot[rootID]; !ok {
				causedByRoot[rootID] = sets.Set{}
			}
			causedByRoot[rootID][transitiveID] = struct{}{}
		}
	}

	// Sort top-level directly unresolvable mods for deterministic output.
	topLevelSlice := make([]string, 0, len(details.DirectlyUnresolvable))
	for modID := range details.DirectlyUnresolvable {
		topLevelSlice = append(topLevelSlice, modID)
	}
	sort.Strings(topLevelSlice)

	for _, modID := range topLevelSlice {
		if mod, ok := modMap[modID]; ok {
			fmt.Fprintf(b, "[gray]  - %s from '%s.jar'\n", modID, mod.BaseFilename)

			// 1. Display directly missing dependencies.
			if failedDeps := details.DirectlyUnresolvable[modID]; len(failedDeps) > 0 {
				b.WriteString("[gray]    └ Unresolved or unmet dependencies:\n")
				sort.Strings(failedDeps)
				for _, depID := range failedDeps {
					if providerMod, ok := modMap[depID]; ok {
						fmt.Fprintf(b, "[gray]      - %s from '%s.jar'\n", depID, providerMod.BaseFilename)
					} else {
						fmt.Fprintf(b, "[gray]      - %s from unknown\n", depID)
					}
				}
			}

			// 2. Display transitively broken mods that depend on this root.
			if caused, ok := causedByRoot[modID]; ok && len(caused) > 0 {
				b.WriteString("[gray]    └ Disabling this mod would also require disabling:\n")
				causedSlice := sets.MakeSlice(caused)
				sort.Strings(causedSlice)
				for _, depID := range causedSlice {
					if depMod, ok := modMap[depID]; ok {
						fmt.Fprintf(b, "[gray]      - %s from '%s.jar'\n", depID, depMod.BaseFilename)
					} else {
						fmt.Fprintf(b, "[gray]      - %s from unknown\n", depID)
					}
				}
			}
		}
	}
}

// GetActionPrompts returns the key actions for the page.
func (p *ResultPage) GetActionPrompts() []tui.ActionPrompt {
	return []tui.ActionPrompt{
		{Input: "↑/↓", Action: "Scroll Text"},
	}
}

// GetStatusPrimitive returns the tview.Primitive that displays the page's status
func (p *ResultPage) GetStatusPrimitive() *tview.TextView {
	return p.statusText
}

// GetFocusablePrimitives returns the focusable primitives for the page.
func (p *ResultPage) GetFocusablePrimitives() []tview.Primitive {
	primitives := []tview.Primitive{p.closeButton}
	if !p.continueButton.IsDisabled() {
		primitives = append(primitives, p.continueButton)
	}
	primitives = append(primitives, p.resultView)
	return primitives
}
