package ui

import (
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/Qendolin/fabric-mod-bisect-tool/app" // Use your module path
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
	log.Printf("%sSwitched to loading page. Starting async mod processing for: %s", app.LogInfoPrefix, absModsDir)
	go ctx.PerformAsyncModLoading(PageNameInitialSetup, PageNameBisection, PageNameLoading)
}

func GlobalInputHandler(ctx *app.AppContext, event *tcell.EventKey) *tcell.EventKey {
	if event.Key() == tcell.KeyRune && (event.Rune() == 'q' || event.Rune() == 'Q') {
		focusedPrimitive := ctx.App.GetFocus()
		// Allow 'q' if an input field or modal has focus (they might use 'q' for input)
		if _, isInput := focusedPrimitive.(*tview.InputField); isInput {
			return event
		}
		if _, isModal := focusedPrimitive.(*tview.Modal); isModal {
			return event
		}

		currentPage, _ := ctx.Pages.GetFrontPage()
		// Only quit if on a main page, not e.g. loading screen where q might be confusing
		if currentPage == PageNameBisection || currentPage == PageNameModSelection || currentPage == PageNameInitialSetup {
			// Show Confirm Quit Modal instead of stopping directly
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
		// Other pages (InitialSetup, Loading, QuestionModal) primarily handle input via their primitives
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
	} // Default focus

	ctx.Pages.HidePage(PageNameQuestionModal) // Always hide modal first
	ctx.Pages.SwitchToPage(targetPage)        // Then switch page
	if targetFocus != nil {
		ctx.App.SetFocus(targetFocus)
	} // Then set focus

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
		ctx.UpdateBisectionLists()

		if done {
			ctx.UpdateInfo(status+"\n\nPress 'R' to Reset, or 'Q' to Quit.", false)
		} else {
			// ProcessUserFeedback returns the next question and current status.
			ctx.UpdateInfo(status, false) // Display current iteration status
			ctx.AskBisectionQuestion(nextQuestion, PageNameBisection, ctx.SearchSpaceList)
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
	ctx.Pages.HidePage(PageNameConfirmQuitModal) // Always hide the modal first

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
	default: // Fallback if on an unexpected page
		if ctx.SetupForm != nil && ctx.SetupForm.GetFormItemCount() > 0 {
			defaultFocus = ctx.SetupForm.GetFormItem(0)
		} else {
			defaultFocus = ctx.Pages
		}

	}
	if defaultFocus != nil { // Check if defaultFocus is not nil before setting
		ctx.App.SetFocus(defaultFocus)
	}

	if buttonLabel == ModalButtonQuitYes {
		log.Printf("%sUser confirmed quit.", app.LogInfoPrefix)
		ctx.App.Stop() // Proceed to quit
	} else {
		log.Printf("%sUser cancelled quit.", app.LogInfoPrefix)
		// Do nothing, user remains in the application, focus restored above.
	}
}

func handleBisectionPageInput(ctx *app.AppContext, event *tcell.EventKey) *tcell.EventKey {
	if event.Key() != tcell.KeyRune {
		return event
	} // Handle only rune keys here
	switch event.Rune() {
	case 's', 'S':
		handleStartBisectionStep(ctx)
	case 'u', 'U':
		handleUndo(ctx)
	case 'm', 'M':
		handleManageMods(ctx) // 'M' for Manage/Interrupt
	case 'r', 'R':
		handleReset(ctx)
	// 'q' for Quit is handled by GlobalInputHandler
	default:
		return event
	}
	return nil // Event was handled
}

func handleStartBisectionStep(ctx *app.AppContext) {
	if ctx.Bisector == nil {
		ctx.UpdateInfo("Bisector not initialized. Please load mods first.", true)
		return
	}

	var question, status string
	done := false

	// If not actively testing A or B, prepare the next iteration.
	// Otherwise, re-prompt the current question.
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

	ctx.UpdateBisectionLists() // Update lists before showing status or question
	ctx.UpdateInfo(status, false)

	if done {
		// 'status' from PrepareNextTestOrConclude is the final conclusion message.
		ctx.UpdateInfo(status+"\n\nPress 'R' to Reset, or 'Q' to Quit.", false)
	} else if question != "" { // If not done, there should be a question
		ctx.AskBisectionQuestion(question, PageNameBisection, ctx.SearchSpaceList)
	} else {
		log.Printf("%shandleStartBisectionStep: Not done, but no question generated. Phase: %d", app.LogErrorPrefix, currentPhase)
		ctx.UpdateInfo("Internal error: Failed to determine next bisection step.", true)
	}
}

func handleUndo(ctx *app.AppContext) {
	if ctx.Bisector == nil {
		ctx.UpdateInfo("Bisector not initialized.", true)
		return
	}
	possible, msg := ctx.Bisector.UndoLastStep()
	ctx.UpdateInfo(msg, !possible) // Show error if not possible
	if possible {
		ctx.UpdateBisectionLists()
		ctx.PopulateAllModsList() // Forced lists might have been reverted
		// Info message from UndoLastStep already guides user.
	}
}

func handleManageMods(ctx *app.AppContext) { // 'M' key
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
	ctx.ReinitializeAppContextForSetup() // Resets bisector and UI elements related to bisection

	go ctx.App.QueueUpdateDraw(func() {
		// Ensure form path field is updated if modsPath was kept or reset
		if setupForm := ctx.SetupForm; setupForm != nil && setupForm.GetFormItemCount() > 0 {
			if pathInput, ok := setupForm.GetFormItem(0).(*tview.InputField); ok {
				pathInput.SetText(ctx.GetModsPath()) // Get potentially persisted or default path
			}
			ctx.App.SetFocus(setupForm.GetFormItem(0)) // Focus first form item
		}
		ctx.Pages.SwitchToPage(PageNameInitialSetup)
	})
	log.Printf("%sBisection reset. Returned to Initial Setup.", app.LogInfoPrefix)
	ctx.UpdateInfo("Bisection has been reset. Please enter mods folder path.", false)
}

func handleModSelectionPageInput(ctx *app.AppContext, event *tcell.EventKey) *tcell.EventKey {
	if ctx.AllModsList.HasFocus() && event.Key() == tcell.KeyRune {
		modID, found := ctx.GetSelectedModIDFromAllModsList()
		if found {
			switch event.Rune() {
			case 'e', 'E':
				ctx.Bisector.ToggleForceEnable(modID)
				ctx.PopulateAllModsList() // Refresh lists to show status change
				return nil                // Event handled
			case 'd', 'D':
				ctx.Bisector.ToggleForceDisable(modID)
				ctx.PopulateAllModsList()
				return nil // Event handled
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

func HandleModSearchDone(ctx *app.AppContext, key tcell.Key) { // Called on Enter/Esc in ModSearchInput
	if key == tcell.KeyEnter {
		ctx.App.SetFocus(ctx.AllModsList) // Move focus from search to the list
	} else if key == tcell.KeyEscape {
		if ctx.ModSearchInput.GetText() != "" {
			ctx.ModSearchInput.SetText("")
		}
	}
}
