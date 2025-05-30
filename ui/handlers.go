package ui

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

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
	currentPage, _ := ctx.Pages.GetFrontPage()

	if event.Key() == tcell.KeyRune && (event.Rune() == 'q' || event.Rune() == 'Q') {
		focusedPrimitive := ctx.App.GetFocus()
		if _, isInput := focusedPrimitive.(*tview.InputField); isInput {
			return event
		}
		if _, isModal := focusedPrimitive.(*tview.Modal); isModal {
			return event
		}

		if currentPage == PageNameBisection || currentPage == PageNameModSelection || currentPage == PageNameInitialSetup {
			ctx.Pages.ShowPage(PageNameConfirmQuitModal)
			ctx.App.SetFocus(ctx.ConfirmQuitModal)
			return nil
		}
	}

	switch currentPage {
	case PageNameBisection:
		return handleBisectionPageInput(ctx, event)
	case PageNameModSelection:
		return handleModSelectionPageInput(ctx, event)
	case PageNameImportGoodMods:
		return handleImportListPageInput(ctx, event)
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
		ctx.UpdateBisectionLists()

		if done {
			ctx.UpdateInfo(status+"\n\nPress 'R' to Reset, or 'Q' to Quit.", false)
		} else {
			ctx.UpdateInfo(status, false)
			if nextQuestion != "" {
				ctx.AskBisectionQuestion(nextQuestion, PageNameBisection, ctx.SearchSpaceList)
			}
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
	ctx.UpdateInfo("Mod Management: Use E/D/G to toggle Force/Good. Press Esc to return to bisection.", false)
}

func handleReset(ctx *app.AppContext) {
	if ctx.Bisector != nil {
		ctx.Bisector.RestoreInitialModState()
	}
	currentModsPath := ctx.GetModsPath()
	currentStrategy := ctx.BisectionStrategy

	ctx.ReinitializeAppContextForSetup()

	ctx.SetModsPath(currentModsPath)
	ctx.BisectionStrategy = currentStrategy

	go ctx.App.QueueUpdateDraw(func() {
		if setupForm := ctx.SetupForm; setupForm != nil && setupForm.GetFormItemCount() > 0 {
			if pathInput, ok := setupForm.GetFormItem(0).(*tview.InputField); ok {
				pathInput.SetText(ctx.GetModsPath())
			}
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
	if ctx.AllModsList.HasFocus() && event.Key() == tcell.KeyRune {
		if ctx.Bisector == nil {
			ctx.UpdateInfo("Bisector not initialized. Load mods first.", true)
			return nil
		}

		if event.Modifiers()&tcell.ModShift != 0 {
			switch event.Rune() {
			case 'e', 'E':
				ctx.Bisector.ToggleForceEnable(ctx.Bisector.AllModIDsSorted...)
				ctx.PopulateAllModsList()
				ctx.UpdateBisectionLists()
				return nil
			case 'd', 'D':
				ctx.Bisector.ToggleForceDisable(ctx.Bisector.AllModIDsSorted...)
				ctx.PopulateAllModsList()
				ctx.UpdateBisectionLists()
				return nil
			case 'g', 'G':
				ctx.Bisector.ToggleConfirmedGood(ctx.Bisector.AllModIDsSorted...)
				ctx.PopulateAllModsList()
				ctx.UpdateBisectionLists()
				return nil
			}
		} else {
			modID, found := ctx.GetSelectedModIDFromAllModsList()
			if found {
				switch event.Rune() {
				case 'e', 'E':
					ctx.Bisector.ToggleForceEnable(modID)
					ctx.PopulateAllModsList()
					ctx.UpdateBisectionLists()
					return nil
				case 'd', 'D':
					ctx.Bisector.ToggleForceDisable(modID)
					ctx.PopulateAllModsList()
					ctx.UpdateBisectionLists()
					return nil
				case 'g', 'G':
					ctx.Bisector.ToggleConfirmedGood(modID)
					ctx.PopulateAllModsList()
					ctx.UpdateBisectionLists()
					return nil
				}
			}
		}
	}

	if event.Key() == tcell.KeyEscape {
		if ctx.ModSearchInput.HasFocus() && ctx.ModSearchInput.GetText() != "" {
			return event
		}
		ctx.Pages.SwitchToPage(PageNameBisection)
		if ctx.SearchSpaceList != nil {
			ctx.App.SetFocus(ctx.SearchSpaceList)
		}
		currentStatus := "Press 'S' to start or continue bisection."
		if ctx.Bisector != nil && ctx.Bisector.GetIterationCount() > 0 {
			currentStatus = fmt.Sprintf("Iteration %d. Press 'S' to continue.", ctx.Bisector.GetIterationCount())
		} else if ctx.Bisector == nil {
			currentStatus = "Bisector not ready. Load mods first."
		}
		ctx.UpdateInfo(currentStatus, false)
		return nil
	}
	return event
}

func HandleModSearchDone(ctx *app.AppContext, key tcell.Key) {
	if key == tcell.KeyEnter {
		if ctx.AllModsList.GetRowCount() > 1 {
			ctx.AllModsList.Select(1, 0)
		}
		ctx.App.SetFocus(ctx.AllModsList)
	} else if key == tcell.KeyEscape {
		if ctx.ModSearchInput.GetText() != "" {
			ctx.ModSearchInput.SetText("")
		}
	}
}

func handleImportListPageInput(ctx *app.AppContext, event *tcell.EventKey) *tcell.EventKey {
	currentPage, _ := ctx.Pages.GetFrontPage()

	if event.Key() == tcell.KeyEscape {
		if currentPage == PageNameImportGoodMods {
			ctx.Pages.SwitchToPage(PageNameModSelection)
			ctx.App.SetFocus(ctx.ModSearchInput)
			ctx.UpdateInfo("Mod Management: Use E/D/G to toggle. Press Esc to return to bisection.", false)
			return nil
		}
	}

	return event
}

func handleExportGoodModsAction(ctx *app.AppContext) {
	file, err := os.OpenFile("open_mods.txt", os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0644)
	if err != nil {
		log.Printf("%sError opening open_mods.txt for export: %v", app.LogErrorPrefix, err)
		ctx.UpdateInfo(fmt.Sprintf("Error exporting good mods list: %v", err), true)
		return
	}
	file.WriteString("# A list of mod id's that haven't been confirmed good\n")
	for _, modID := range ctx.Bisector.AllModIDsSorted {
		mod := ctx.Bisector.AllMods[modID]
		if !mod.ConfirmedGood {
			file.WriteString(mod.ModID() + "\n")
		}
	}
	if err = file.Close(); err != nil {
		log.Printf("%sError closing open_mods.txt after export: %v", app.LogErrorPrefix, err)
		ctx.UpdateInfo(fmt.Sprintf("Error closing export file: %v", err), true)
		return
	}
	log.Printf("%sSuccessfully exported list of non-good mods to open_mods.txt", app.LogInfoPrefix)
	ctx.UpdateInfo("Successfully exported list of non-good mods to open_mods.txt", false)
}

// handleImportGoodModsAction processes the list of mod identifiers from the import text area.
// markProvidedListAsGood: If true, mods in the list are marked good.
// If false, mods NOT in the list are marked good (limit search space to list).
func handleImportGoodModsAction(ctx *app.AppContext, markProvidedListAsGood bool) {
	if ctx.Bisector == nil {
		ctx.UpdateInfo("Bisector not initialized. Load mods first.", true)
		return
	}
	if ctx.ImportGoodModsTextArea == nil {
		log.Printf("%sImportGoodModsTextArea is nil. Cannot process action.", app.LogErrorPrefix)
		return
	}

	text := ctx.ImportGoodModsTextArea.GetText()
	lines := strings.Split(text, "\n")
	var inputIdentifiers []string
	for _, line := range lines {
		trimmedLine := strings.TrimSpace(line)
		if trimmedLine != "" && !strings.HasPrefix(trimmedLine, "#") {
			inputIdentifiers = append(inputIdentifiers, trimmedLine)
		}
	}

	if len(inputIdentifiers) == 0 {
		ctx.UpdateInfo("No mod identifiers provided in the import list.", true)
		ctx.Pages.SwitchToPage(PageNameModSelection)
		ctx.App.SetFocus(ctx.ModSearchInput)
		return
	}

	var resolvedInputModIDs []string
	fileNameToIDMap := make(map[string]string)
	for modID, mod := range ctx.Bisector.AllMods {
		// Store lowercase for case-insensitive matching of filenames
		fileNameToIDMap[strings.ToLower(mod.BaseFilename+".jar")] = modID
		fileNameToIDMap[strings.ToLower(mod.BaseFilename+".jar.disabled")] = modID
	}

	for _, identifier := range inputIdentifiers {
		lowerIdentifier := strings.ToLower(identifier)
		// Try matching as filename first
		if modID, found := fileNameToIDMap[lowerIdentifier]; found {
			resolvedInputModIDs = append(resolvedInputModIDs, modID)
		} else { // If not a filename, assume it's a mod ID (case-sensitive for mod IDs)
			if _, found := ctx.Bisector.AllMods[identifier]; found {
				resolvedInputModIDs = append(resolvedInputModIDs, identifier)
			} else {
				log.Printf("%sImport: Identifier '%s' not found in loaded mods (checked as filename and mod ID).", app.LogWarningPrefix, identifier)
			}
		}
	}

	if len(resolvedInputModIDs) == 0 {
		ctx.UpdateInfo("None of the provided identifiers matched loaded mods.", true)
		ctx.Pages.SwitchToPage(PageNameModSelection)
		ctx.App.SetFocus(ctx.ModSearchInput)
		return
	}

	var modsToMakeGood []string
	var modsToMakeNotGood []string // Only used if !markProvidedListAsGood
	reason := ""

	if markProvidedListAsGood {
		reason = "Import: Mark provided list as good"
		modsToMakeGood = resolvedInputModIDs // All resolved IDs from the list
		// modsToMakeNotGood remains empty
		log.Printf("%s%s. Input list resolved to %d mod IDs: %v", app.LogInfoPrefix, reason, len(modsToMakeGood), modsToMakeGood)
	} else { // Limit search space to the list (mark everything else as good)
		reason = "Import: Limit search space to provided list"
		inputSet := make(map[string]bool)
		for _, id := range resolvedInputModIDs {
			inputSet[id] = true
		}

		for _, modID := range ctx.Bisector.AllModIDsSorted {
			// Mod is IN the list, should be kept in search space (i.e., NOT good)
			if inputSet[modID] {
				modsToMakeNotGood = append(modsToMakeNotGood, modID)
			} else { // Mod is NOT in the list, should be excluded (i.e., good)
				modsToMakeGood = append(modsToMakeGood, modID)
			}
		}
		log.Printf("%s%s. Input list resolved to %d mod IDs to keep: %v", app.LogInfoPrefix, reason, len(modsToMakeNotGood), modsToMakeNotGood)
	}

	ctx.Bisector.BatchUpdateGoodStatus(modsToMakeGood, modsToMakeNotGood, reason)

	uniqueResolvedCount := 0
	if len(resolvedInputModIDs) > 0 {
		seen := make(map[string]bool)
		for _, id := range resolvedInputModIDs {
			if !seen[id] {
				seen[id] = true
				uniqueResolvedCount++
			}
		}
	}

	ctx.PopulateAllModsList()
	ctx.UpdateBisectionLists()
	ctx.Pages.SwitchToPage(PageNameModSelection)
	ctx.App.SetFocus(ctx.AllModsList)
	ctx.UpdateInfo(fmt.Sprintf("Import action '%s' processed. %d unique mods matched from list.", reason, uniqueResolvedCount), false)
}
