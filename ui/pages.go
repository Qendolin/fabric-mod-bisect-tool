package ui

import (
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
	ctx.AllModsList = listSetup("All Mods (E/D to toggle Force)")
	ctx.ForceEnabledList = listSetup("Force Enabled")
	ctx.ForceDisabledList = listSetup("Force Disabled")
	ctx.ModSearchInput = tview.NewInputField().SetLabel("Search: ").SetFieldWidth(30)
	ctx.ModSearchInput.SetBorder(true)

	ctx.QuestionModal = tview.NewModal().
		AddButtons([]string{ModalButtonFailure, ModalButtonSuccess, ModalButtonInterrupt})

	ctx.ConfirmQuitModal = tview.NewModal().
		SetText("Are you sure you want to quit?").
		AddButtons([]string{ModalButtonQuitYes, ModalButtonQuitNo}).
		SetDoneFunc(func(buttonIndex int, buttonLabel string) {
			// This DoneFunc will be handled by a new handler in handlers.go
			HandleConfirmQuitModalDone(ctx, buttonIndex, buttonLabel)
		})
}

func SetupPages(ctx *app.AppContext) {
	ctx.Pages = tview.NewPages()
	setupInitialSetupPage(ctx)
	setupLoadingPage(ctx)
	setupBisectionPage(ctx)
	setupModSelectionPage(ctx)
	setupQuestionModalPage(ctx)
	setupConfirmQuitModalPage(ctx)

	ctx.Pages.SwitchToPage(PageNameInitialSetup)
	if f := ctx.SetupForm; f != nil && f.GetFormItemCount() > 0 {
		if item := f.GetFormItem(0); item != nil {
			ctx.App.SetFocus(item)
		}
	}
}

func setupInitialSetupPage(ctx *app.AppContext) {
	ctx.SetupForm = tview.NewForm().
		AddInputField("Mods Folder Path:", "", 60, nil, func(text string) { ctx.SetModsPath(text) }).
		AddTextView("Hint:", "Use Right-Click or Ctrl+V to paste path.", 0, 1, true, false).
		AddButton("Load Mods & Start", func() { HandleLoadModsAndStart(ctx) }).
		AddButton("Quit", func() { ctx.App.Stop() })
	ctx.SetupForm.SetBorder(true).SetTitle("Minecraft Mod Bisect Tool - Setup")

	setupLayout := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(ctx.SetupForm, 9, 0, true). // Form takes proportional space, gets initial focus if flex does
		AddItem(ctx.InfoTextView, 3, 0, false).
		AddItem(ctx.DebugLogView, 0, 2, false)

	// Allow tabbing from DebugLogView back to the form.
	ctx.DebugLogView.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		if event.Key() == tcell.KeyTab || event.Key() == tcell.KeyBacktab {
			ctx.App.SetFocus(ctx.SetupForm)
			return nil
		}
		return event
	})

	// Explicit Tab/Backtab handling from Form elements to DebugLogView
	if quitButton := ctx.SetupForm.GetButton(ctx.SetupForm.GetButtonCount() - 1); quitButton != nil { // Last button
		quitButton.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
			if event.Key() == tcell.KeyTab && event.Modifiers() == 0 { // Tab forward
				ctx.App.SetFocus(ctx.DebugLogView)
				return nil
			}
			return event
		})
	}
	if pathInput, ok := ctx.SetupForm.GetFormItem(0).(*tview.InputField); ok { // First item
		pathInput.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
			if event.Key() == tcell.KeyBacktab { // Shift+Tab
				ctx.App.SetFocus(ctx.DebugLogView)
				return nil
			}
			return event
		})
	}

	ctx.Pages.AddPage(PageNameInitialSetup, setupLayout, true, true)
}

func setupLoadingPage(ctx *app.AppContext) {
	loadingStatusText := tview.NewTextView().
		SetTextAlign(tview.AlignCenter).SetDynamicColors(true).
		SetText("[yellow]Loading mods, please wait...\n\nCheck logs below for progress.")
	loadingLayout := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(loadingStatusText, 3, 0, false).
		AddItem(ctx.DebugLogView, 0, 1, true) // DebugLogView can get initial focus in this flex
	ctx.Pages.AddPage(PageNameLoading, loadingLayout, true, false)
}

func setupBisectionPage(ctx *app.AppContext) {
	searchAndGroupsFlex := tview.NewFlex().
		AddItem(ctx.SearchSpaceList, 0, 1, true).
		AddItem(ctx.GroupAList, 0, 1, false).
		AddItem(ctx.GroupBList, 0, 1, false)

	// InfoTextView is shared, used here for bisection status
	// We'll create a new Flex for the main content area to include a key hint footer
	mainContentFlex := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(ctx.InfoTextView, 5, 0, false).   // Bisection status area, fixed height
		AddItem(searchAndGroupsFlex, 0, 1, false) // Lists take proportional space

	// Key hint text view
	keyHintTextView := tview.NewTextView().
		SetTextAlign(tview.AlignCenter).
		SetDynamicColors(true). // Allow color tags if desired
		SetText("[yellow](S)[white]tep | [yellow](U)[white]ndo | [yellow](M)[white]anage Mods | [yellow](R)[white]eset | [yellow](Q)[white]uit")

	// Overall layout for the bisection page
	bisectionLayout := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(keyHintTextView, 1, 0, false). // Key hint header, fixed height of 1 line
		AddItem(mainContentFlex, 0, 2, false). // Main content (info + lists) takes most space
		AddItem(ctx.DebugLogView, 0, 1, false) // Log view takes proportional space

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

	ctx.SearchSpaceList.SetInputCapture(focusCycleHandler)
	ctx.GroupAList.SetInputCapture(focusCycleHandler)
	ctx.GroupBList.SetInputCapture(focusCycleHandler)
	ctx.DebugLogView.SetInputCapture(focusCycleHandler)

	ctx.Pages.AddPage(PageNameBisection, bisectionLayout, true, false)
}

func setupModSelectionPage(ctx *app.AppContext) {
	ctx.AllModsList.SetSelectedFunc(func(i int, mt, st string, r rune) { HandleModListSelect(ctx) })
	ctx.ModSearchInput.SetChangedFunc(func(text string) { ctx.PopulateAllModsList() })
	ctx.ModSearchInput.SetDoneFunc(func(key tcell.Key) { HandleModSearchDone(ctx, key) })

	forcedListsFlex := tview.NewFlex().
		AddItem(ctx.ForceEnabledList, 0, 1, true).
		AddItem(ctx.ForceDisabledList, 0, 1, true)
	modListAndForcedFlex := tview.NewFlex().
		AddItem(ctx.AllModsList, 0, 2, true).
		AddItem(forcedListsFlex, 0, 1, false) // This flex's children are focusable

	modSelectionLayout := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(ctx.ModSearchInput, 3, 0, true). // Input can get initial focus
		AddItem(modListAndForcedFlex, 0, 1, false)

	modSelectionFrame := tview.NewFrame(modSelectionLayout).
		AddText("Mod Management | E/D on mod in 'All Mods' | Tab/Shift+Tab | Esc to Bisection",
			true, tview.AlignCenter, tcell.ColorYellow)

	modSelectionFocusElements := []tview.Primitive{
		ctx.ModSearchInput, ctx.AllModsList, ctx.ForceEnabledList, ctx.ForceDisabledList,
	}
	// Layout's input capture handles tabbing between major components on this page.
	modSelectionLayout.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		currentFocus := ctx.App.GetFocus()
		isOneOfOurElements := false
		for _, el := range modSelectionFocusElements {
			if currentFocus == el {
				isOneOfOurElements = true
				break
			}
		}
		if isOneOfOurElements { // Only cycle if one of our designated elements has focus
			if event.Key() == tcell.KeyTab {
				cycleFocus(ctx.App, modSelectionFocusElements, false)
				return nil
			} else if event.Key() == tcell.KeyBacktab {
				cycleFocus(ctx.App, modSelectionFocusElements, true)
				return nil
			}
		}
		return event
	})
	ctx.Pages.AddPage(PageNameModSelection, modSelectionFrame, true, false)
}

func setupQuestionModalPage(ctx *app.AppContext) {
	ctx.QuestionModal.SetDoneFunc(func(buttonIndex int, buttonLabel string) {
		HandleQuestionModalDone(ctx, buttonIndex, buttonLabel)
	})
	// Modal is wrapped to be added as a page and centered.
	modalWrapper := tview.NewFlex().
		AddItem(nil, 0, 1, false). // Vertical spacer
		AddItem(tview.NewFlex().SetDirection(tview.FlexRow).
			AddItem(nil, 0, 1, false).               // Horizontal spacer
			AddItem(ctx.QuestionModal, 15, 1, true). // Modal, fixed height, can get initial focus
			AddItem(nil, 0, 1, false),               // Horizontal spacer
						0, 3, true). // Centered group, proportional width
		AddItem(nil, 0, 1, false) // Vertical spacer

	ctx.Pages.AddPage(PageNameQuestionModal, modalWrapper, true, false)
	ctx.Pages.HidePage(PageNameQuestionModal) // Initially hidden
}

func setupConfirmQuitModalPage(ctx *app.AppContext) {
	// Modal is wrapped to be added as a page and centered.
	// We can reuse the same centering logic as the question modal.
	modalWrapper := tview.NewFlex().
		AddItem(nil, 0, 1, false). // Vertical spacer
		AddItem(tview.NewFlex().SetDirection(tview.FlexRow).
			AddItem(nil, 0, 1, false).                  // Horizontal spacer
			AddItem(ctx.ConfirmQuitModal, 10, 1, true). // Modal, fixed height, can get initial focus
			AddItem(nil, 0, 1, false),                  // Horizontal spacer
						0, 3, true). // Centered group, proportional width
		AddItem(nil, 0, 1, false) // Vertical spacer

	ctx.Pages.AddPage(PageNameConfirmQuitModal, modalWrapper, true, false)
	ctx.Pages.HidePage(PageNameConfirmQuitModal) // Initially hidden
}

func cycleFocus(appObj *tview.Application, elements []tview.Primitive, reverse bool) {
	if len(elements) == 0 {
		return
	}
	currentFocusIdx := -1
	for i, el := range elements {
		if el != nil && el.HasFocus() {
			currentFocusIdx = i
			break
		}
	}

	if currentFocusIdx == -1 { // No element in the list currently has focus
		// Attempt to focus the first non-nil element in the specified direction
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
		return // No focusable non-nil element found
	}

	numElements := len(elements)
	for i := 1; i <= numElements; i++ { // Try each element once to find next non-nil
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
