package app

import (
	"fmt"
	"log"
	"sort"
	"strings"
	"sync"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

const (
	LogInfoPrefix    = "INFO: "
	LogWarningPrefix = "WARN: "
	LogErrorPrefix   = "ERR:  "
)

// BisectionStrategyType defines the available bisection strategies.
type BisectionStrategyType int

const (
	StrategyFast BisectionStrategyType = iota // Default, current behavior
	StrategyPartial
	StrategyFull
)

// BisectionStrategyTypeStrings provides user-friendly names for strategies.
var BisectionStrategyTypeStrings = map[BisectionStrategyType]string{
	StrategyFast:    "Fast (Only test one half)",
	StrategyPartial: "Partial (Test other half on failure)",
	StrategyFull:    "Full (Always test both halves)",
}

// ModalReturnContext stores information about where to return after a modal.
type ModalReturnContext struct {
	PageName       string
	FocusPrimitive tview.Primitive
}

// AppContext holds all shared application state, TUI components, and logic.
type AppContext struct {
	App               *tview.Application
	Bisector          *Bisector
	BisectionStrategy BisectionStrategyType

	// TUI Primitives (managed by UI package, but stored here for access)
	Pages             *tview.Pages
	InfoTextView      *tview.TextView
	SearchSpaceList   *tview.List
	GroupAList        *tview.List
	GroupBList        *tview.List
	AllModsList       *tview.Table
	ForceEnabledList  *tview.List
	ForceDisabledList *tview.List
	ModSearchInput    *tview.InputField
	DebugLogView      *tview.TextView
	QuestionModal     *tview.Modal
	SetupForm         *tview.Form
	ConfirmQuitModal  *tview.Modal

	// Internal state
	uiLogLines     []string // Stores lines for the TUI DebugLogView
	maxUILogLines  int
	modsPath       string         // Store the mods path from setup form
	logFile        *UILogWriter   // Custom writer for teeing logs
	logRelayStopCh chan struct{}  // To signal log relay goroutine to stop
	logRelayWg     sync.WaitGroup // To wait for log relay goroutine

	// State for AllModsList selection optimization
	currentlyDisplayedModIDsInAllModsList []string

	modalReturnCtx ModalReturnContext
}

// NewAppContext creates and initializes a new application context.
func NewAppContext() *AppContext {
	return &AppContext{
		App:               tview.NewApplication(),
		uiLogLines:        make([]string, 0, 200),
		maxUILogLines:     1000,
		logRelayStopCh:    make(chan struct{}),
		BisectionStrategy: StrategyFast, // Default strategy
	}
}

// UpdateInfo updates the main information text view.
func (ctx *AppContext) UpdateInfo(message string, isError bool) {
	log.Printf("%sUI_INFO (Error: %t): %s", LogInfoPrefix, isError, message)
	color := "white"
	if isError {
		color = "red"
	}
	finalMessage := fmt.Sprintf("[%s]%s", color, tview.Escape(message))

	if ctx.App != nil && ctx.InfoTextView != nil {
		go ctx.App.QueueUpdateDraw(func() {
			ctx.InfoTextView.SetText(finalMessage)
		})
	}
}

// UpdateBisectionLists refreshes the content of lists on the bisection page.
func (ctx *AppContext) UpdateBisectionLists() {
	if ctx.Bisector == nil {
		return
	}

	updateList := func(list *tview.List, titleFmt string, modIDs []string, effectiveCount int) {
		list.Clear()

		if len(modIDs) == 0 {
			if effectiveCount < 0 {
				list.SetTitle(fmt.Sprintf(titleFmt, 0))
			} else {
				list.SetTitle(fmt.Sprintf(titleFmt, 0, 0))
			}
			return
		}

		type modSortInfo struct {
			id   string
			name string
		}

		modsToDisplay := make([]modSortInfo, 0, len(modIDs))
		for _, modID := range modIDs {
			friendlyName := modID
			if mod, ok := ctx.Bisector.AllMods[modID]; ok {
				friendlyName = mod.FriendlyName()
			}
			modsToDisplay = append(modsToDisplay, modSortInfo{id: modID, name: friendlyName})
		}

		sort.Slice(modsToDisplay, func(i, j int) bool {
			return strings.ToLower(modsToDisplay[i].name) < strings.ToLower(modsToDisplay[j].name)
		})

		for _, modInfo := range modsToDisplay {
			list.AddItem(tview.Escape(modInfo.name), "", 0, nil)
		}

		if effectiveCount < 0 {
			list.SetTitle(fmt.Sprintf(titleFmt, len(modsToDisplay)))
		} else {
			list.SetTitle(fmt.Sprintf(titleFmt, len(modsToDisplay), effectiveCount))
		}
	}

	go ctx.App.QueueUpdateDraw(func() {
		updateList(ctx.SearchSpaceList, "Search Space (%d)", ctx.Bisector.CurrentSearchSpace, -1)

		effACount := 0
		if ctx.Bisector.CurrentGroupAEffective != nil {
			effACount = len(ctx.Bisector.CurrentGroupAEffective)
		}
		updateList(ctx.GroupAList, "Group A (Orig: %d, Eff: %d)", ctx.Bisector.CurrentGroupAOriginal, effACount)

		effBCount := 0
		if ctx.Bisector.CurrentGroupBEffective != nil {
			effBCount = len(ctx.Bisector.CurrentGroupBEffective)
		}
		updateList(ctx.GroupBList, "Group B (Orig: %d, Eff: %d)", ctx.Bisector.CurrentGroupBOriginal, effBCount)
	})
}

type ModDisplayInfo struct {
	ID   string
	Name string
	Mod  *Mod
}

// PopulateAllModsList populates the list of all mods for the management screen.
func (ctx *AppContext) PopulateAllModsList() {
	if ctx.Bisector == nil || ctx.AllModsList == nil {
		return
	}

	// Gather all data *before* queueing the UI update.
	searchTerm := ""
	if ctx.ModSearchInput != nil {
		searchTerm = strings.ToLower(ctx.ModSearchInput.GetText())
	}

	// 1. Get all mods and prepare for sorting by name
	displayMods := make([]ModDisplayInfo, 0, len(ctx.Bisector.AllMods))
	for modID, modRef := range ctx.Bisector.AllMods {
		displayMods = append(displayMods, ModDisplayInfo{ID: modID, Name: modRef.FriendlyName(), Mod: modRef})
	}

	// 2. Sort by friendly name
	sort.Slice(displayMods, func(i, j int) bool {
		return strings.ToLower(displayMods[i].Name) < strings.ToLower(displayMods[j].Name)
	})

	// Prepare data for forced lists
	var feNames, fdNames []string
	for id := range ctx.Bisector.ForceEnabled {
		if mod, ok := ctx.Bisector.AllMods[id]; ok {
			feNames = append(feNames, mod.FriendlyName())
		}
	}
	for id := range ctx.Bisector.ForceDisabled {
		if mod, ok := ctx.Bisector.AllMods[id]; ok {
			fdNames = append(fdNames, mod.FriendlyName())
		}
	}
	sort.Strings(feNames)
	sort.Strings(fdNames)

	// Store current selection *before* clearing, to be restored later
	currentSelectedRow := -1
	if ctx.AllModsList.GetRowCount() > 1 {
		r, _ := ctx.AllModsList.GetSelection()
		if r >= 1 {
			currentSelectedRow = r
		}
	}

	go ctx.App.QueueUpdateDraw(func() {
		ctx.AllModsList.Clear()
		ctx.ForceEnabledList.Clear()
		ctx.ForceDisabledList.Clear()

		newlyDisplayedModIDsInThisRun := make([]string, 0, len(displayMods))

		headers := []string{"Status", "Name", "ID", "File"}
		expansions := []int{1, 3, 3, 3}
		for colIdx, headerName := range headers {
			headerCell := tview.NewTableCell(headerName).
				SetTextColor(tcell.ColorYellow).
				SetAlign(tview.AlignLeft).
				SetSelectable(false).SetExpansion(expansions[colIdx])
			ctx.AllModsList.SetCell(0, colIdx, headerCell)
		}
		tableRowIndex := 1

		for _, name := range feNames {
			ctx.ForceEnabledList.AddItem(tview.Escape(name), "", 0, nil)
		}
		for _, name := range fdNames {
			ctx.ForceDisabledList.AddItem(tview.Escape(name), "", 0, nil)
		}

		// Populate AllModsList (Table)
		for _, dispInfo := range displayMods {
			mod := dispInfo.Mod
			modID := dispInfo.ID
			friendlyName := dispInfo.Name

			if searchTerm != "" &&
				!strings.Contains(strings.ToLower(friendlyName), searchTerm) &&
				!strings.Contains(strings.ToLower(modID), searchTerm) {
				continue
			}

			statusText := ""
			statusTextColor := tcell.ColorWhite
			if ctx.Bisector.ForceEnabled[modID] {
				statusText = "Force"
				statusTextColor = tcell.ColorLimeGreen
			} else if ctx.Bisector.ForceDisabled[modID] {
				statusText = "Force"
				statusTextColor = tcell.ColorRed
			} else if mod.ConfirmedGood {
				statusText = "Good "
				statusTextColor = tcell.ColorGreen
			} else if !mod.IsCurrentlyActive {
				statusText = "Off"
				statusTextColor = tcell.ColorGray
			}

			if statusText == "" {
				statusText = "     " // just for spacing
			}

			statusCell := tview.NewTableCell(statusText).
				SetTextColor(statusTextColor).
				SetAlign(tview.AlignLeft).
				SetExpansion(expansions[0])

			nameCell := tview.NewTableCell(tview.Escape(friendlyName)).
				SetTextColor(tcell.ColorWhite).
				SetAlign(tview.AlignLeft).
				SetExpansion(expansions[1]).
				SetMaxWidth(35)

			idCell := tview.NewTableCell(tview.Escape(modID)).
				SetTextColor(tcell.ColorWhite).
				SetAlign(tview.AlignLeft).
				SetExpansion(expansions[2])

			fileText := tview.Escape(mod.BaseFilename) + ".jar"
			fileCell := tview.NewTableCell(fileText).
				SetTextColor(tcell.ColorWhite).
				SetAlign(tview.AlignLeft).
				SetExpansion(expansions[3])

			ctx.AllModsList.SetCell(tableRowIndex, 0, statusCell)
			ctx.AllModsList.SetCell(tableRowIndex, 1, nameCell)
			ctx.AllModsList.SetCell(tableRowIndex, 2, idCell)
			ctx.AllModsList.SetCell(tableRowIndex, 3, fileCell)

			newlyDisplayedModIDsInThisRun = append(newlyDisplayedModIDsInThisRun, modID)
			tableRowIndex++
		}
		ctx.currentlyDisplayedModIDsInAllModsList = newlyDisplayedModIDsInThisRun

		if currentSelectedRow >= 1 {
			if ctx.AllModsList.GetRowCount() > 1 {
				if currentSelectedRow < ctx.AllModsList.GetRowCount() {
					ctx.AllModsList.Select(currentSelectedRow, 0)
				} else {
					ctx.AllModsList.Select(ctx.AllModsList.GetRowCount()-1, 0)
				}
			}
		} else if ctx.AllModsList.GetRowCount() > 1 {
			ctx.AllModsList.Select(1, 0)
		}
	})
}

// GetSelectedModIDFromAllModsList retrieves the mod ID of the currently selected item in AllModsList.
func (ctx *AppContext) GetSelectedModIDFromAllModsList() (string, bool) {
	if ctx.AllModsList == nil || ctx.AllModsList.GetRowCount() <= 1 {
		return "", false
	}
	selectedRow, _ := ctx.AllModsList.GetSelection()
	dataRowIndex := selectedRow - 1

	if dataRowIndex < 0 || dataRowIndex >= len(ctx.currentlyDisplayedModIDsInAllModsList) {
		log.Printf("%sWarning: Selected table row %d (data index %d) is out of bounds for displayed mods list (len %d). No mod selected.", LogWarningPrefix, selectedRow, dataRowIndex, len(ctx.currentlyDisplayedModIDsInAllModsList))
		return "", false
	}
	return ctx.currentlyDisplayedModIDsInAllModsList[dataRowIndex], true
}

// AskBisectionQuestion shows the modal for user feedback.
func (ctx *AppContext) AskBisectionQuestion(question string, returnPageName string, returnFocusPrimitive tview.Primitive) {
	log.Printf("%sAsking bisection question: %s", LogInfoPrefix, strings.ReplaceAll(question, "\n", " "))
	ctx.modalReturnCtx = ModalReturnContext{
		PageName:       returnPageName,
		FocusPrimitive: returnFocusPrimitive,
	}
	go ctx.App.QueueUpdateDraw(func() {
		ctx.QuestionModal.SetText(tview.Escape(question))
		ctx.Pages.ShowPage("questionModal")
		ctx.App.SetFocus(ctx.QuestionModal)
	})
}

// PerformAsyncModLoading handles loading mods in a goroutine.
func (ctx *AppContext) PerformAsyncModLoading(pageNameInitialSetup, pageNameBisection, pageNameLoading string) {
	absModsDir := ctx.modsPath
	log.Printf("%sGoroutine: PerformAsyncModLoading started for directory '%s'.", LogInfoPrefix, absModsDir)

	allMods, providesMap, sortedModIDs, loadErr := LoadMods(absModsDir, func(fileNameBeingProcessed string) {
		log.Printf("%sProcessing: %s", LogInfoPrefix, fileNameBeingProcessed)
	})

	if loadErr != nil {
		ctx.handleModLoadingError(fmt.Sprintf("Error during mod loading from '%s': %v", absModsDir, loadErr), pageNameInitialSetup)
		return
	}
	log.Printf("%sFound %d potential mod files. Parsed information for %d mods.", LogInfoPrefix, len(sortedModIDs), len(allMods))

	if len(allMods) == 0 {
		ctx.handleModLoadingError(fmt.Sprintf("No valid mods found in '%s'.", absModsDir), pageNameInitialSetup)
		return
	}

	ctx.enableInitialMods(allMods)
	ctx.performInitialDependencyChecks(allMods, providesMap)

	log.Printf("%sMods parsed. Initializing bisector logic with strategy: %s...", LogInfoPrefix, BisectionStrategyTypeStrings[ctx.BisectionStrategy])

	var strategy BisectionStrategy
	switch ctx.BisectionStrategy {
	case StrategyFast:
		strategy = NewFastStrategy()
	case StrategyPartial:
		strategy = NewPartialStrategy()
	case StrategyFull:
		strategy = NewFullStrategy()
	default:
		log.Printf("%sUnknown bisection strategy %d, defaulting to Fast.", LogWarningPrefix, ctx.BisectionStrategy)
		strategy = NewFastStrategy() // Fallback
	}

	ctx.Bisector = NewBisector(absModsDir, allMods, sortedModIDs, providesMap, strategy)

	go ctx.App.QueueUpdateDraw(func() {
		if ctx.Bisector.InitialModCount == 0 {
			ctx.UpdateInfo("Initial search space empty. Check logs. Use 'M' to manage mods.", true)
		} else {
			ctx.UpdateInfo(fmt.Sprintf("All %d mods processed. Initial search space: %d. Press 'S' to start.", len(allMods), ctx.Bisector.InitialModCount), false)
		}
		ctx.UpdateBisectionLists()
		ctx.PopulateAllModsList()
		ctx.Pages.SwitchToPage(pageNameBisection)
		if ctx.SearchSpaceList != nil {
			ctx.App.SetFocus(ctx.SearchSpaceList)
		}
		log.Printf("%sInitialization complete. Switched to Bisection view.", LogInfoPrefix)
	})
	log.Printf("%sGoroutine: PerformAsyncModLoading finished.", LogInfoPrefix)
}

// handleModLoadingError is called for errors during the asynchronous loading phase.
func (ctx *AppContext) handleModLoadingError(errMsg string, pageNameInitialSetup string) {
	log.Printf("%s%s", LogErrorPrefix, errMsg)

	go ctx.App.QueueUpdateDraw(func() {
		ctx.Pages.SwitchToPage(pageNameInitialSetup)
		if ctx.SetupForm != nil && ctx.SetupForm.GetFormItemCount() > 0 {
			if item := ctx.SetupForm.GetFormItem(0); item != nil {
				ctx.App.SetFocus(item)
			}
		}
		ctx.UpdateInfo(fmt.Sprintf("Mod loading failed: %s Check path and logs. Try again.", errMsg), true)
	})
}

func (ctx *AppContext) enableInitialMods(allMods map[string]*Mod) {
	log.Printf("%sEnabling all detected mods for initial bisection state...", LogInfoPrefix)
	enabledCount := 0
	for _, mod := range allMods {
		if !mod.IsInitiallyActive {
			err := mod.Enable(ctx.modsPath)
			if err != nil {
				log.Printf("%sFailed to enable mod %s during initial setup: %v", LogWarningPrefix, mod.FriendlyName(), err)
			} else {
				log.Printf("%sInitially enabled mod: %s", LogInfoPrefix, mod.FriendlyName())
				enabledCount++
			}
		}
	}
	if enabledCount > 0 {
		log.Printf("%sEnabled %d previously disabled mods for the start of bisection.", LogInfoPrefix, enabledCount)
	} else {
		log.Printf("%sAll detected mods were already in an enabled state (.jar) or failed to enable.", LogInfoPrefix)
	}
}

func (ctx *AppContext) performInitialDependencyChecks(allMods map[string]*Mod, potentialProviders PotentialProvidersMap) {
	initialMissingDeps := CheckAllDependencies(allMods, potentialProviders)
	if len(initialMissingDeps) > 0 {
		log.Printf("%s--- Initial Dependency Check ---", LogWarningPrefix)
		for modID, missing := range initialMissingDeps {
			modName := modID
			if mod, ok := allMods[modID]; ok {
				modName = mod.FriendlyName()
			}
			log.Printf("%sMod '%s' is missing dependencies: %s", LogWarningPrefix, modName, strings.Join(missing, ", "))
		}
		log.Printf("%s--- These mods might not work correctly or cause issues. ---", LogWarningPrefix)
	} else {
		log.Printf("%sInitial Dependency Check: All direct dependencies appear to be resolvable or are implicit.", LogInfoPrefix)
	}
}

// ReinitializeAppContextForSetup clears bisection-related state for a new session.
func (ctx *AppContext) ReinitializeAppContextForSetup() {
	ctx.Bisector = nil

	if ctx.SearchSpaceList != nil {
		ctx.SearchSpaceList.Clear()
	}
	if ctx.GroupAList != nil {
		ctx.GroupAList.Clear()
	}
	if ctx.GroupBList != nil {
		ctx.GroupBList.Clear()
	}
	if ctx.AllModsList != nil {
		ctx.AllModsList.Clear()
	}
	if ctx.ForceEnabledList != nil {
		ctx.ForceEnabledList.Clear()
	}
	if ctx.ForceDisabledList != nil {
		ctx.ForceDisabledList.Clear()
	}
	if ctx.ModSearchInput != nil {
		ctx.ModSearchInput.SetText("")
	}
	if ctx.InfoTextView != nil {
		ctx.InfoTextView.SetText("")
	}

	ctx.ClearUILogs()
	ctx.modalReturnCtx = ModalReturnContext{}
	// ctx.BisectionStrategy is preserved from last setup or remains default
}

func (ctx *AppContext) SetModsPath(path string)                   { ctx.modsPath = path }
func (ctx *AppContext) GetModsPath() string                       { return ctx.modsPath }
func (ctx *AppContext) GetMaxUILogLines() int                     { return ctx.maxUILogLines }
func (ctx *AppContext) ClearUILogs()                              { ctx.uiLogLines = []string{} }
func (ctx *AppContext) GetModalReturnContext() ModalReturnContext { return ctx.modalReturnCtx }
