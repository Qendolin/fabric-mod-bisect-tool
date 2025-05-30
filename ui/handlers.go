package ui

import (
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/Qendolin/fabric-mod-bisect-tool/app"
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

func HandleLoadModsAndStart(ctx *app.AppContext) {
	log.Printf("%sButton 'Load Mods & Start' pressed.", app.LogInfoPrefix)
	currentModsPath := ctx.GetModsPath()

	if currentModsPath == "" {
		ctx.UpdateInfo("Mods folder path cannot be empty.", true)
		log.Printf("%sMods path was empty.", app.LogWarningPrefix)
		return
	}

	absModsDir, err := filepath.Abs(currentModsPath)
	if err != nil {
		errMsg := fmt.Sprintf("Error getting absolute path for '%s': %v", currentModsPath, err)
		log.Printf("%s%s", app.LogErrorPrefix, errMsg)
		ctx.UpdateInfo(errMsg, true)
		return
	}

	fileInfo, err := os.Stat(absModsDir)
	if os.IsNotExist(err) {
		errMsg := fmt.Sprintf("Mods folder '%s' does not exist.", absModsDir)
		log.Printf("%s%s", app.LogErrorPrefix, errMsg)
		ctx.UpdateInfo(errMsg, true)
		return
	}
	if err != nil {
		errMsg := fmt.Sprintf("Error accessing mods folder '%s': %v", absModsDir, err)
		log.Printf("%s%s", app.LogErrorPrefix, errMsg)
		ctx.UpdateInfo(errMsg, true)
		return
	}
	if !fileInfo.IsDir() {
		errMsg := fmt.Sprintf("Path '%s' is not a directory.", absModsDir)
		log.Printf("%s%s", app.LogErrorPrefix, errMsg)
		ctx.UpdateInfo(errMsg, true)
		return
	}

	ctx.SetModsPath(absModsDir)
	ctx.ClearUILogs()
	if ctx.DebugLogView != nil {
		go ctx.App.QueueUpdateDraw(func() { ctx.DebugLogView.SetText("") })
	}

	ctx.Pages.SwitchToPage(PageNameLoading)
	log.Printf("%sSwitched to loading page. Starting async mod processing for: %s with strategy: %s",
		app.LogInfoPrefix, absModsDir, app.BisectionStrategyTypeStrings[ctx.BisectionStrategy])
	go ctx.PerformAsyncModLoading(PageNameInitialSetup, PageNameBisection, PageNameLoading)
}

func GlobalInputHandler(ctx *app.AppContext, event *tcell.EventKey) *tcell.EventKey {
	if event.Key() == tcell.KeyRune && (event.Rune() == 'q' || event.Rune() == 'Q') {
		focusedPrimitive := ctx.App.GetFocus()
		if _, isInput := focusedPrimitive.(*tview.InputField); isInput {
			return event
		}
		if _, isModal := focusedPrimitive.(*tview.Modal); isModal {
			return event
		}

		currentPage, _ := ctx.Pages.GetFrontPage()
		if currentPage == PageNameBisection || currentPage == PageNameModSelection || currentPage == PageNameInitialSetup {
			ctx.Pages.ShowPage(PageNameConfirmQuitModal)
			ctx.App.SetFocus(ctx.ConfirmQuitModal)
			return nil
		}
	}

	currentPage, _ := ctx.Pages.GetFrontPage()
	switch currentPage {
	case PageNameBisection:
		return handleBisectionPageInput(ctx, event)
	case PageNameModSelection:
		return handleModSelectionPageInput(ctx, event)
	}
	return event
}

func HandleQuestionModalDone(ctx *app.AppContext, buttonIndex int, buttonLabel string) {
	returnCtx := ctx.GetModalReturnContext()
	targetPage := returnCtx.PageName
	targetFocus := returnCtx.FocusPrimitive
	if targetPage == "" {
		targetPage = PageNameBisection
	}
	if targetFocus == nil {
		targetFocus = ctx.SearchSpaceList
	}

	ctx.Pages.HidePage(PageNameQuestionModal)
	ctx.Pages.SwitchToPage(targetPage)
	if targetFocus != nil {
		ctx.App.SetFocus(targetFocus)
	}

	if ctx.Bisector == nil {
		log.Printf("%sBisector is nil, cannot process modal response for '%s'.", app.LogWarningPrefix, buttonLabel)
		ctx.UpdateInfo("Error: Bisection process not active.", true)
		return
	}

	switch buttonLabel {
	case ModalButtonFailure, ModalButtonSuccess:
		issueOccurred := (buttonLabel == ModalButtonFailure)
		log.Printf("%sModal response: '%s' (Issue Occurred: %t)", app.LogInfoPrefix, buttonLabel, issueOccurred)

		done, nextQuestion, status := ctx.Bisector.ProcessUserFeedback(issueOccurred)
		ctx.UpdateBisectionLists() // Update lists after Bisector processes feedback

		if done {
			ctx.UpdateInfo(status+"\n\nPress 'R' to Reset, or 'Q' to Quit.", false)
		} else {
			ctx.UpdateInfo(status, false) // Display current iteration status
			if nextQuestion != "" {       // If there's a specific next question (e.g., for Group B)
				ctx.AskBisectionQuestion(nextQuestion, PageNameBisection, ctx.SearchSpaceList)
			}
			// If no nextQuestion, it implies a new iteration will be prepared by 'S' key
		}

	case ModalButtonInterrupt:
		log.Printf("%sModal response: Interrupt.", app.LogInfoPrefix)
		ctx.PopulateAllModsList()
		ctx.Pages.SwitchToPage(PageNameModSelection)
		ctx.App.SetFocus(ctx.ModSearchInput)
		ctx.UpdateInfo("Mod Management active. Make changes and press Esc to return to bisection.", false)
	default:
		log.Printf("%sUnknown button from modal: '%s' (index %d)", app.LogWarningPrefix, buttonLabel, buttonIndex)
	}
}

func HandleConfirmQuitModalDone(ctx *app.AppContext, buttonIndex int, buttonLabel string) {
	ctx.Pages.HidePage(PageNameConfirmQuitModal)

	currentPage, _ := ctx.Pages.GetFrontPage()
	var defaultFocus tview.Primitive
	switch currentPage {
	case PageNameInitialSetup:
		if ctx.SetupForm != nil && ctx.SetupForm.GetFormItemCount() > 0 {
			defaultFocus = ctx.SetupForm.GetFormItem(0)
		}
	case PageNameBisection:
		defaultFocus = ctx.SearchSpaceList
	case PageNameModSelection:
		defaultFocus = ctx.ModSearchInput
	default:
		if ctx.SetupForm != nil && ctx.SetupForm.GetFormItemCount() > 0 {
			defaultFocus = ctx.SetupForm.GetFormItem(0)
		} else {
			defaultFocus = ctx.Pages
		}
	}
	if defaultFocus != nil {
		ctx.App.SetFocus(defaultFocus)
	}

	if buttonLabel == ModalButtonQuitYes {
		log.Printf("%sUser confirmed quit.", app.LogInfoPrefix)
		ctx.App.Stop()
	} else {
		log.Printf("%sUser cancelled quit.", app.LogInfoPrefix)
	}
}

func handleBisectionPageInput(ctx *app.AppContext, event *tcell.EventKey) *tcell.EventKey {
	if event.Key() != tcell.KeyRune {
		return event
	}
	switch event.Rune() {
	case 's', 'S':
		handleStartBisectionStep(ctx)
	case 'u', 'U':
		handleUndo(ctx)
	case 'm', 'M':
		handleManageMods(ctx)
	case 'r', 'R':
		handleReset(ctx)
	default:
		return event
	}
	return nil
}

func handleStartBisectionStep(ctx *app.AppContext) {
	if ctx.Bisector == nil {
		ctx.UpdateInfo("Bisector not initialized. Please load mods first.", true)
		return
	}

	var question, status string
	done := false

	currentPhase := ctx.Bisector.GetCurrentTestingPhase()
	if currentPhase == app.PhasePrepareA {
		done, question, status = ctx.Bisector.PrepareNextTestOrConclude()
	} else if currentPhase == app.PhaseTestingA {
		status = fmt.Sprintf("Iteration %d. Re-prompting for Group A.", ctx.Bisector.GetIterationCount())
		question = ctx.Bisector.FormatQuestion("A", ctx.Bisector.GetCurrentGroupAOriginal(), ctx.Bisector.GetCurrentGroupAEffective())
	} else { // PhaseTestingB
		status = fmt.Sprintf("Iteration %d. Re-prompting for Group B.", ctx.Bisector.GetIterationCount())
		question = ctx.Bisector.FormatQuestion("B", ctx.Bisector.GetCurrentGroupBOriginal(), ctx.Bisector.GetCurrentGroupBEffective())
	}

	ctx.UpdateBisectionLists()
	ctx.UpdateInfo(status, false)

	if done {
		ctx.UpdateInfo(status+"\n\nPress 'R' to Reset, or 'Q' to Quit.", false)
	} else if question != "" {
		ctx.AskBisectionQuestion(question, PageNameBisection, ctx.SearchSpaceList)
	} else {
		log.Printf("%shandleStartBisectionStep: Not done, but no question generated. Phase: %d. Status: %s", app.LogErrorPrefix, currentPhase, status)
		// This can happen if a strategy decides not to test B and directly prepares for next iteration.
		// The status message from ProcessUserFeedback (via strategy) should guide the user.
	}
}

func handleUndo(ctx *app.AppContext) {
	if ctx.Bisector == nil {
		ctx.UpdateInfo("Bisector not initialized.", true)
		return
	}
	possible, msg := ctx.Bisector.UndoLastStep()
	ctx.UpdateInfo(msg, !possible)
	if possible {
		ctx.UpdateBisectionLists()
		ctx.PopulateAllModsList()
	}
}

func handleManageMods(ctx *app.AppContext) {
	if ctx.Bisector == nil {
		ctx.UpdateInfo("Bisector not initialized.", true)
		return
	}
	ctx.PopulateAllModsList()
	ctx.Pages.SwitchToPage(PageNameModSelection)
	ctx.App.SetFocus(ctx.ModSearchInput)
	ctx.UpdateInfo("Mod Management: Use E/D to toggle Force. Press Esc to return to bisection.", false)
}

func handleReset(ctx *app.AppContext) {
	if ctx.Bisector != nil {
		ctx.Bisector.RestoreInitialModState()
	}
	// Preserve mods path and selected strategy, but reset everything else.
	currentModsPath := ctx.GetModsPath()
	currentStrategy := ctx.BisectionStrategy // Store before reinitialize

	ctx.ReinitializeAppContextForSetup() // Resets bisector and UI elements

	ctx.SetModsPath(currentModsPath)        // Restore mods path
	ctx.BisectionStrategy = currentStrategy // Restore strategy

	go ctx.App.QueueUpdateDraw(func() {
		if setupForm := ctx.SetupForm; setupForm != nil && setupForm.GetFormItemCount() > 0 {
			// Update path input field
			if pathInput, ok := setupForm.GetFormItem(0).(*tview.InputField); ok {
				pathInput.SetText(ctx.GetModsPath())
			}
			// Update strategy dropdown
			if strategyDropDown, ok := setupForm.GetFormItem(1).(*tview.DropDown); ok {
				for i := range app.BisectionStrategyTypeStrings {
					if i == ctx.BisectionStrategy {
						strategyDropDown.SetCurrentOption(int(i))
						break
					}
				}
			}
			ctx.App.SetFocus(setupForm.GetFormItem(0))
		}
		ctx.Pages.SwitchToPage(PageNameInitialSetup)
	})
	log.Printf("%sBisection reset. Returned to Initial Setup. Strategy: %s", app.LogInfoPrefix, app.BisectionStrategyTypeStrings[currentStrategy])
	ctx.UpdateInfo("Tool has been reset.", false)
}

func handleModSelectionPageInput(ctx *app.AppContext, event *tcell.EventKey) *tcell.EventKey {
	// Focus handling is now primarily managed by the page's SetInputCapture in pages.go
	// This function will handle specific key actions for focused elements.
	if ctx.AllModsList.HasFocus() && event.Key() == tcell.KeyRune {

		if event.Modifiers()&tcell.ModShift != 0 {
			switch event.Rune() {
			case 'e', 'E':
				ctx.Bisector.ToggleForceEnable(ctx.Bisector.AllModIDsSorted...)
				ctx.PopulateAllModsList()
				return nil
			case 'd', 'D':
				ctx.Bisector.ToggleForceDisable(ctx.Bisector.AllModIDsSorted...)
				ctx.PopulateAllModsList()
				return nil
			}
		}

		modID, found := ctx.GetSelectedModIDFromAllModsList()
		if found {
			switch event.Rune() {
			case 'e', 'E':
				ctx.Bisector.ToggleForceEnable(modID)
				ctx.PopulateAllModsList()
				return nil
			case 'd', 'D':
				ctx.Bisector.ToggleForceDisable(modID)
				ctx.PopulateAllModsList()
				return nil
			}
		}
	}
	if event.Key() == tcell.KeyEscape {
		if ctx.ModSearchInput.HasFocus() && ctx.ModSearchInput.GetText() != "" {
			// defer event handling to search input
			return event
		}
		ctx.Pages.SwitchToPage(PageNameBisection)
		ctx.App.SetFocus(ctx.SearchSpaceList) // Default focus for bisection page
		currentStatus := "Press 'S' to start or continue bisection."
		ctx.UpdateInfo(currentStatus, false)
		return nil // Event handled
	}
	return event // Event not handled by this specific handler
}

func HandleModSearchDone(ctx *app.AppContext, key tcell.Key) {
	if key == tcell.KeyEnter {
		if ctx.AllModsList.GetRowCount() > 1 { // Check if list has content rows
			ctx.AllModsList.Select(1, 0) // Select first data row
		}
		ctx.App.SetFocus(ctx.AllModsList)
	} else if key == tcell.KeyEscape {
		if ctx.ModSearchInput.GetText() != "" {
			ctx.ModSearchInput.SetText("")
		}
	}
}
