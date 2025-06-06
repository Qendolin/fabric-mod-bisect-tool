package ui

import (
	"log"

	"github.com/Qendolin/fabric-mod-bisect-tool/app"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

const (
	PageNameInitialSetup     = "initialSetup"
	PageNameLoading          = "loading"
	PageNameBisection        = "bisection"
	PageNameModSelection     = "modSelection"
	PageNameQuestionModal    = "questionModal"
	PageNameConfirmQuitModal = "confirmQuitModal"
	PageNameImportGoodMods   = "importGoodMods"
)

const (
	ModalButtonFailure   = "Failure (Issue Persists)"
	ModalButtonSuccess   = "Success (Issue Gone)"
	ModalButtonInterrupt = "Interrupt (Manage Mods)"
	ModalButtonQuitYes   = "Yes, Quit"
	ModalButtonQuitNo    = "No, Continue"
)

func InitializeTUIPrimitives(ctx *app.AppContext) {
	ctx.DebugLogView = tview.NewTextView().
		SetMaxLines(ctx.GetMaxUILogLines() * 2).
		SetScrollable(true).SetRegions(true).SetDynamicColors(true).
		SetChangedFunc(func() {
			if ctx.DebugLogView != nil {
				ctx.DebugLogView.ScrollToEnd()
			}
		})
	ctx.DebugLogView.SetBorder(true).SetTitle("Logs (Scroll: PgUp/PgDn/Arrows, Cycle Focus: Tab/Shift+Tab)")

	ctx.InfoTextView = tview.NewTextView().
		SetDynamicColors(true).SetScrollable(true).SetWordWrap(true).SetTextAlign(tview.AlignCenter)
	ctx.InfoTextView.SetBorder(true).SetTitle("Status / Instructions")

	listSetup := func(title string) *tview.List {
		l := tview.NewList().ShowSecondaryText(false)
		l.SetBorder(true).SetTitle(title)
		return l
	}
	ctx.SearchSpaceList = listSetup("Search Space")
	ctx.GroupAList = listSetup("Group A")
	ctx.GroupBList = listSetup("Group B")
	ctx.AllModsList = tview.NewTable().
		SetBorders(false).
		SetFixed(1, 0).
		SetEvaluateAllRows(true).
		SetSelectable(true, false)
	ctx.ForceEnabledList = listSetup("Force Enabled")
	ctx.ForceDisabledList = listSetup("Force Disabled")

	ctx.ModSearchInput = tview.NewInputField().SetLabel("Search: ").SetFieldWidth(0)

	ctx.QuestionModal = tview.NewModal().
		AddButtons([]string{ModalButtonFailure, ModalButtonSuccess, ModalButtonInterrupt})

	ctx.ConfirmQuitModal = tview.NewModal().
		SetText("Are you sure you want to quit?").
		AddButtons([]string{ModalButtonQuitYes, ModalButtonQuitNo}).
		SetDoneFunc(func(buttonIndex int, buttonLabel string) {
			HandleConfirmQuitModalDone(ctx, buttonIndex, buttonLabel)
		})

	ctx.ImportGoodModsTextArea = tview.NewTextArea().
		SetPlaceholder("Paste list of mod IDs or filenames here, one per line.")
}

func SetupPages(ctx *app.AppContext) {
	ctx.Pages = tview.NewPages()
	setupInitialSetupPage(ctx)
	setupLoadingPage(ctx)
	setupBisectionPage(ctx)
	setupModSelectionPage(ctx)
	setupQuestionModalPage(ctx)
	setupConfirmQuitModalPage(ctx)
	setupImportGoodModsPage(ctx)

	ctx.Pages.SwitchToPage(PageNameInitialSetup)
	ctx.UpdateInfo("Enter mod folder path. Use Tab/Shift+Tab to navigate, Enter to interact.", false)
}

func setupInitialSetupPage(ctx *app.AppContext) {
	pathInput := tview.NewInputField().SetLabel("Mods Folder Path:").SetFieldWidth(60).
		SetChangedFunc(func(text string) { ctx.SetModsPath(text) })

	strategyOptionStrings := []string{
		app.BisectionStrategyTypeStrings[app.StrategyFast],
		app.BisectionStrategyTypeStrings[app.StrategyPartial],
		app.BisectionStrategyTypeStrings[app.StrategyFull],
	}

	strategyDropDown := tview.NewDropDown().
		SetLabel("Bisection Strategy:").
		SetOptions(strategyOptionStrings, func(text string, index int) {
			ctx.BisectionStrategy = app.BisectionStrategyType(index)
			log.Printf("%sBisection strategy set to: %s", app.LogInfoPrefix, text)
		})

	strategyDropDown.SetCurrentOption(int(ctx.BisectionStrategy))

	ctx.SetupForm = tview.NewForm().
		AddFormItem(pathInput).
		AddTextView("Hint:", "Use Right-Click or Ctrl+V to paste the path.", 0, 1, true, false).
		AddFormItem(strategyDropDown).
		AddButton("Load Mods & Start", func() { HandleLoadModsAndStart(ctx) }).
		AddButton("Quit", func() { ctx.App.Stop() })
	ctx.SetupForm.SetBorder(true).SetTitle(" Qendolin's Fabric Mod Bisect Tool ")

	setupLayout := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(ctx.SetupForm, 11, 0, true).
		AddItem(ctx.InfoTextView, 3, 0, false).
		AddItem(ctx.DebugLogView, 0, 2, false)

	ctx.DebugLogView.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		if event.Key() == tcell.KeyTab || event.Key() == tcell.KeyBacktab {
			if ctx.SetupForm.GetFormItemCount() > 0 {
				ctx.App.SetFocus(ctx.SetupForm.GetFormItem(0))
			} else {
				ctx.App.SetFocus(ctx.SetupForm)
			}
			return nil
		}
		return event
	})

	if quitButton := ctx.SetupForm.GetButton(ctx.SetupForm.GetButtonCount() - 1); quitButton != nil {
		quitButton.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
			if event.Key() == tcell.KeyTab && event.Modifiers() == 0 {
				ctx.App.SetFocus(ctx.DebugLogView)
				return nil
			}
			return event
		})
	}
	pathInput.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		if event.Key() == tcell.KeyBacktab {
			ctx.App.SetFocus(ctx.DebugLogView)
			return nil
		}
		return event
	})

	ctx.Pages.AddPage(PageNameInitialSetup, setupLayout, true, true)
}

func setupLoadingPage(ctx *app.AppContext) {
	loadingStatusText := tview.NewTextView().
		SetTextAlign(tview.AlignCenter).SetDynamicColors(true).
		SetText("[yellow]Loading mods, please wait...\n\nCheck logs below for progress.")
	loadingLayout := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(loadingStatusText, 3, 0, false).
		AddItem(ctx.DebugLogView, 0, 1, true)
	ctx.Pages.AddPage(PageNameLoading, loadingLayout, true, false)
}

func setupBisectionPage(ctx *app.AppContext) {
	searchAndGroupsFlex := tview.NewFlex().
		AddItem(ctx.SearchSpaceList, 0, 1, true).
		AddItem(ctx.GroupAList, 0, 1, false).
		AddItem(ctx.GroupBList, 0, 1, false)

	mainContentFlex := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(ctx.InfoTextView, 5, 0, false).
		AddItem(searchAndGroupsFlex, 0, 1, false)

	keyHintTextView := tview.NewTextView().
		SetTextAlign(tview.AlignCenter).
		SetDynamicColors(true).
		SetText("[yellow](S)[white]tep | [yellow](U)[white]ndo | [yellow](M)[white]anage Mods | [yellow](R)[white]eset | [yellow](Q)[white]uit")

	bisectionLayout := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(keyHintTextView, 1, 0, false).
		AddItem(mainContentFlex, 0, 2, false).
		AddItem(ctx.DebugLogView, 0, 1, false)

	bisectionFocusElements := []tview.Primitive{
		ctx.SearchSpaceList, ctx.GroupAList, ctx.GroupBList, ctx.DebugLogView,
	}

	focusCycleHandler := func(event *tcell.EventKey) *tcell.EventKey {
		if event.Key() == tcell.KeyTab {
			cycleFocus(ctx.App, bisectionFocusElements, false)
			return nil
		} else if event.Key() == tcell.KeyBacktab {
			cycleFocus(ctx.App, bisectionFocusElements, true)
			return nil
		}
		return event
	}

	for _, prim := range bisectionFocusElements {
		if box, ok := prim.(*tview.Box); ok {
			box.SetInputCapture(focusCycleHandler)
		}
	}
	ctx.Pages.AddPage(PageNameBisection, bisectionLayout, true, false)
}

func setupModSelectionPage(ctx *app.AppContext) {
	ctx.ModSearchInput.SetChangedFunc(func(text string) { ctx.PopulateAllModsList() })
	ctx.ModSearchInput.SetDoneFunc(func(key tcell.Key) { HandleModSearchDone(ctx, key) })

	importButton := tview.NewButton("Import").SetSelectedFunc(func() {
		if ctx.Bisector == nil {
			ctx.UpdateInfo("Bisector not initialized. Load mods first.", true)
			return
		}
		if ctx.ImportGoodModsTextArea != nil {
			ctx.ImportGoodModsTextArea.SetText("", true)
		}
		ctx.Pages.SwitchToPage(PageNameImportGoodMods)
		ctx.App.SetFocus(ctx.ImportGoodModsTextArea)
	})

	exportButton := tview.NewButton("Export").SetSelectedFunc(func() {
		if ctx.Bisector == nil {
			ctx.UpdateInfo("Bisector not initialized. Load mods first.", true)
			return
		}
		handleExportGoodModsAction(ctx)
	})

	buttonRowFlex := tview.NewFlex().
		AddItem(importButton, len(importButton.GetLabel())+4, 0, false).
		AddItem(nil, 1, 0, false).
		AddItem(exportButton, len(exportButton.GetLabel())+4, 0, false)

	searchInputAndButtonFlex := tview.NewFlex().
		AddItem(ctx.ModSearchInput, 0, 2, true).
		AddItem(nil, 2, 0, false).
		AddItem(buttonRowFlex, 0, 1, false)

	forcedListsFlex := tview.NewFlex().
		AddItem(ctx.ForceEnabledList, 0, 1, false).
		AddItem(ctx.ForceDisabledList, 0, 1, false)

	ctx.AllModsList.SetTitle("All Mods | [yellow](E)[white]nable | [yellow](D)[white]isable | [yellow](G)[white]ood | [yellow](+ Shift)[white] All").SetBorder(true)

	modListAndForcedFlex := tview.NewFlex().
		AddItem(ctx.AllModsList, 0, 2, true).
		AddItem(forcedListsFlex, 0, 1, false)

	modSelectionLayout := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(ctx.InfoTextView, 3, 0, false).        // Info / Status text
		AddItem(searchInputAndButtonFlex, 1, 0, true). // Search and import button
		AddItem(modListAndForcedFlex, 0, 1, false)     // Main list and side lists

	modSelectionFrame := tview.NewFrame(modSelectionLayout).
		AddText("Mod Management | Tab/Shift+Tab | Esc to close",
			true, tview.AlignCenter, tcell.ColorYellow)

	modSelectionFocusElements := []tview.Primitive{
		ctx.ModSearchInput, ctx.AllModsList, ctx.ForceEnabledList, ctx.ForceDisabledList, importButton, exportButton,
	}
	// Input capture for Tab/Shift+Tab cycling within the ModSelectionPage
	// This ensures that focus cycles correctly between search, import button, and lists.
	modSelectionLayout.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		currentFocus := ctx.App.GetFocus()
		isOneOfOurElements := false
		for _, el := range modSelectionFocusElements {
			if currentFocus == el {
				isOneOfOurElements = true
				break
			}
		}

		if isOneOfOurElements {
			if event.Key() == tcell.KeyTab {
				cycleFocus(ctx.App, modSelectionFocusElements, false)
				return nil
			} else if event.Key() == tcell.KeyBacktab {
				cycleFocus(ctx.App, modSelectionFocusElements, true)
				return nil
			}
		}
		// If focus is not on one of our managed elements, let specific handlers (like AllModsList's own) work.
		return event
	})

	ctx.Pages.AddPage(PageNameModSelection, modSelectionFrame, true, false)
}

func setupQuestionModalPage(ctx *app.AppContext) {
	ctx.QuestionModal.SetDoneFunc(func(buttonIndex int, buttonLabel string) {
		HandleQuestionModalDone(ctx, buttonIndex, buttonLabel)
	})
	modalWrapper := tview.NewFlex().
		AddItem(nil, 0, 1, false).
		AddItem(tview.NewFlex().SetDirection(tview.FlexRow).
			AddItem(nil, 0, 1, false).
			AddItem(ctx.QuestionModal, 15, 1, true).
			AddItem(nil, 0, 1, false),
			0, 3, true).
		AddItem(nil, 0, 1, false)

	ctx.Pages.AddPage(PageNameQuestionModal, modalWrapper, true, false)
	ctx.Pages.HidePage(PageNameQuestionModal)
}

func setupConfirmQuitModalPage(ctx *app.AppContext) {
	modalWrapper := tview.NewFlex().
		AddItem(nil, 0, 1, false).
		AddItem(tview.NewFlex().SetDirection(tview.FlexRow).
			AddItem(nil, 0, 1, false).
			AddItem(ctx.ConfirmQuitModal, 10, 1, true).
			AddItem(nil, 0, 1, false),
			0, 3, true).
		AddItem(nil, 0, 1, false)

	ctx.Pages.AddPage(PageNameConfirmQuitModal, modalWrapper, true, false)
	ctx.Pages.HidePage(PageNameConfirmQuitModal)
}

func setupImportGoodModsPage(ctx *app.AppContext) {

	excludeButton := tview.NewButton("Exclude List from Search").SetSelectedFunc(func() {
		handleImportGoodModsAction(ctx, true)
	})

	limitButton := tview.NewButton("Limit Search to List").SetSelectedFunc(func() {
		handleImportGoodModsAction(ctx, false)
	})

	buttonLayout := tview.NewFlex().
		AddItem(nil, 0, 1, false).
		AddItem(excludeButton, 0, 1, true).
		AddItem(nil, 3, 0, false).
		AddItem(limitButton, 0, 1, true).
		AddItem(nil, 0, 1, false)

	buttonLayout.SetBorder(true)

	// Layout for the import page
	importLayout := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(ctx.ImportGoodModsTextArea, 0, 1, true).
		AddItem(buttonLayout, 3, 0, false)

	importLayout.SetBorder(true).
		SetTitle("Import Good Mods List | Esc to Cancel")

	importFocusElements := []tview.Primitive{ctx.ImportGoodModsTextArea, excludeButton, limitButton}

	importLayout.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		if event.Key() == tcell.KeyTab {
			cycleFocus(ctx.App, importFocusElements, false)
			return nil
		} else if event.Key() == tcell.KeyBacktab {
			cycleFocus(ctx.App, importFocusElements, true)
			return nil
		}
		return event
	})

	ctx.Pages.AddPage(PageNameImportGoodMods, importLayout, true, false)
}

func cycleFocus(appObj *tview.Application, elements []tview.Primitive, reverse bool) {
	if len(elements) == 0 {
		return
	}
	currentFocusIdx := -1
	for i, el := range elements {
		if el.HasFocus() {
			currentFocusIdx = i
			break
		}
	}

	if currentFocusIdx == -1 {
		startIndex := 0
		if reverse {
			startIndex = len(elements) - 1
		}
		for i := 0; i < len(elements); i++ {
			var checkIdx int
			if reverse {
				checkIdx = (startIndex - i + len(elements)) % len(elements)
			} else {
				checkIdx = (startIndex + i) % len(elements)
			}
			if elements[checkIdx] != nil {
				appObj.SetFocus(elements[checkIdx])
				return
			}
		}
		return
	}

	numElements := len(elements)
	for i := 1; i <= numElements; i++ {
		var nextIdx int
		if reverse {
			nextIdx = (currentFocusIdx - i + numElements) % numElements
		} else {
			nextIdx = (currentFocusIdx + i) % numElements
		}
		if elements[nextIdx] != nil {
			appObj.SetFocus(elements[nextIdx])
			return
		}
	}
}
