package app

import (
	"fmt"
	"log"
	"sort"
	"strings"
	"sync"

	"github.com/rivo/tview"
)

const (
	LogInfoPrefix    = "INFO: "
	LogWarningPrefix = "WARN: "
	LogErrorPrefix   = "ERR:  "
)

// ModalReturnContext stores information about where to return after a modal.
type ModalReturnContext struct {
	PageName       string
	FocusPrimitive tview.Primitive
}

// AppContext holds all shared application state, TUI components, and logic.
type AppContext struct {
	App      *tview.Application
	Bisector *Bisector

	// TUI Primitives (managed by UI package, but stored here for access)
	Pages             *tview.Pages
	InfoTextView      *tview.TextView
	SearchSpaceList   *tview.List
	GroupAList        *tview.List
	GroupBList        *tview.List
	AllModsList       *tview.List
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
		App:            tview.NewApplication(),
		uiLogLines:     make([]string, 0, 200),
		maxUILogLines:  1000, // Increased max lines for TUI log
		logRelayStopCh: make(chan struct{}),
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

	// updateList is a local helper function to populate a tview.List
	updateList := func(list *tview.List, titleFmt string, modIDs []string, effectiveCount int) {
		list.Clear() // Clear existing items

		if len(modIDs) == 0 { // Handle empty list case early
			if effectiveCount < 0 {
				list.SetTitle(fmt.Sprintf(titleFmt, 0))
			} else {
				list.SetTitle(fmt.Sprintf(titleFmt, 0, 0)) // Assuming effective count is 0 if original is 0
			}
			return
		}

		type modSortInfo struct {
			id   string
			name string
		}

		// Create a slice of modSortInfo to hold IDs and their friendly names for sorting
		modsToDisplay := make([]modSortInfo, 0, len(modIDs))
		for _, modID := range modIDs {
			friendlyName := modID // Default to ID if mod info not found
			if mod, ok := ctx.Bisector.AllMods[modID]; ok {
				friendlyName = mod.FriendlyName()
			}
			modsToDisplay = append(modsToDisplay, modSortInfo{id: modID, name: friendlyName})
		}

		// Sort the slice by friendly name (case-insensitive)
		sort.Slice(modsToDisplay, func(i, j int) bool {
			return strings.ToLower(modsToDisplay[i].name) < strings.ToLower(modsToDisplay[j].name)
		})

		// Add sorted items to the tview.List
		for _, modInfo := range modsToDisplay {
			// The primary text is the friendly name. Secondary text could be the ID if desired, but spec says false.
			list.AddItem(tview.Escape(modInfo.name), "", 0, nil)
		}

		// Update the list title
		if effectiveCount < 0 { // For SearchSpaceList
			list.SetTitle(fmt.Sprintf(titleFmt, len(modsToDisplay)))
		} else { // For GroupA/B lists
			list.SetTitle(fmt.Sprintf(titleFmt, len(modsToDisplay), effectiveCount))
		}
	}

	// The rest of the function correctly calls updateList for each list.
	// The go ctx.App.QueueUpdateDraw is important as you noted.
	go ctx.App.QueueUpdateDraw(func() {
		updateList(ctx.SearchSpaceList, "Search Space (%d)", ctx.Bisector.CurrentSearchSpace, -1)

		effACount := 0
		if ctx.Bisector.CurrentGroupAEffective != nil {
			effACount = len(ctx.Bisector.CurrentGroupAEffective)
		}
		// Pass the original list of mod IDs. updateList will handle fetching names and sorting.
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
	currentAllModsIdx := -1
	if ctx.AllModsList.GetItemCount() > 0 { // Only get index if list is not empty
		currentAllModsIdx = ctx.AllModsList.GetCurrentItem()
	}

	// --- Queue the entire UI update (clear and repopulate) ---
	go ctx.App.QueueUpdateDraw(func() {
		// Clear lists inside the queued function
		ctx.AllModsList.Clear()
		ctx.ForceEnabledList.Clear()
		ctx.ForceDisabledList.Clear()

		// This will store IDs of items *actually added* in this specific update run
		newlyDisplayedModIDsInThisRun := make([]string, 0, len(displayMods))

		// Populate forced lists
		for _, name := range feNames {
			ctx.ForceEnabledList.AddItem(tview.Escape(name), "", 0, nil)
		}
		for _, name := range fdNames {
			ctx.ForceDisabledList.AddItem(tview.Escape(name), "", 0, nil)
		}

		// Populate AllModsList
		for _, dispInfo := range displayMods {
			mod := dispInfo.Mod
			modID := dispInfo.ID
			friendlyName := dispInfo.Name

			if searchTerm != "" &&
				!strings.Contains(strings.ToLower(friendlyName), searchTerm) &&
				!strings.Contains(strings.ToLower(modID), searchTerm) {
				continue
			}

			statusColor := "[white]"
			statusText := ""
			if ctx.Bisector.ForceEnabled[modID] {
				statusText = "(Force)"
				statusColor = "[lime]"
			}
			if ctx.Bisector.ForceDisabled[modID] {
				statusText = "(Force)"
				statusColor = "[red]"
			}
			if mod != nil && mod.ConfirmedGood {
				statusText = "(Good) "
				statusColor = "[green]"
			}
			if mod != nil && !mod.IsCurrentlyActive && statusText == "" {
				statusText = "[gray](Off)  [white]"
			}
			if statusText == "" {
				statusText = "       "
			}

			mainDisplay := fmt.Sprintf("%s%s %s[grey]", statusColor, statusText, tview.Escape(friendlyName))
			mainDisplay += fmt.Sprintf(" / %s / %s.jar", tview.Escape(modID), tview.Escape(mod.BaseFilename))

			ctx.AllModsList.AddItem(mainDisplay, "", 0, nil)
			newlyDisplayedModIDsInThisRun = append(newlyDisplayedModIDsInThisRun, modID)
		}
		// Update the shared list of displayed IDs *after* this run has populated everything
		ctx.currentlyDisplayedModIDsInAllModsList = newlyDisplayedModIDsInThisRun

		// Restore selection logic more carefully
		if currentAllModsIdx >= 0 { // If there was a selection
			if ctx.AllModsList.GetItemCount() > 0 { // And the list is not empty now
				if currentAllModsIdx < ctx.AllModsList.GetItemCount() {
					ctx.AllModsList.SetCurrentItem(currentAllModsIdx)
				} else {
					// If previous index is now out of bounds, select the last item
					ctx.AllModsList.SetCurrentItem(ctx.AllModsList.GetItemCount() - 1)
				}
			}
			// If list became empty, selection is implicitly -1 (no selection)
		} else if ctx.AllModsList.GetItemCount() > 0 { // No previous selection, but list has items
			ctx.AllModsList.SetCurrentItem(0) // Default to first item
		}
	})
}

// GetSelectedModIDFromAllModsList retrieves the mod ID of the currently selected item in AllModsList.
// This relies on currentlyDisplayedModIDsInAllModsList being kept in sync.
func (ctx *AppContext) GetSelectedModIDFromAllModsList() (string, bool) {
	if ctx.AllModsList == nil || ctx.AllModsList.GetItemCount() == 0 {
		return "", false
	}
	selectedIndex := ctx.AllModsList.GetCurrentItem()
	if selectedIndex < 0 || selectedIndex >= len(ctx.currentlyDisplayedModIDsInAllModsList) {
		log.Printf("%sError: Selected index %d is out of bounds for displayed mods list (len %d)", LogErrorPrefix, selectedIndex, len(ctx.currentlyDisplayedModIDsInAllModsList))
		return "", false
	}
	return ctx.currentlyDisplayedModIDsInAllModsList[selectedIndex], true
}

// AskBisectionQuestion shows the modal for user feedback.
// The returnPageName and returnFocusPrimitive define where to go after the modal.
func (ctx *AppContext) AskBisectionQuestion(question string, returnPageName string, returnFocusPrimitive tview.Primitive) {
	log.Printf("%sAsking bisection question: %s", LogInfoPrefix, strings.ReplaceAll(question, "\n", " "))
	ctx.modalReturnCtx = ModalReturnContext{
		PageName:       returnPageName,
		FocusPrimitive: returnFocusPrimitive,
	}
	go ctx.App.QueueUpdateDraw(func() {
		ctx.QuestionModal.SetText(tview.Escape(question))
		// The page name "questionModal" is fixed and known by the ui package.
		ctx.Pages.ShowPage("questionModal")
		ctx.App.SetFocus(ctx.QuestionModal)
	})
}

// PerformAsyncModLoading handles loading mods in a goroutine.
// Assumes ctx.modsPath is an absolute, existing directory path.
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

	log.Printf("%sMods parsed. Initializing bisector logic...", LogInfoPrefix)
	ctx.Bisector = NewBisector(absModsDir, allMods, sortedModIDs, providesMap)

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
		// Provide a summary on the (now active) setup page's info view.
		ctx.UpdateInfo(fmt.Sprintf("Mod loading failed: %s Check path and logs. Try again.", errMsg), true)
	})
}

func (ctx *AppContext) enableInitialMods(allMods map[string]*Mod) {
	log.Printf("%sEnabling all detected mods for initial bisection state...", LogInfoPrefix)
	enabledCount := 0
	for _, mod := range allMods {
		if !mod.IsInitiallyActive { // Mod was e.g. .jar.disabled
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
	// The CheckAllDependencies function will need to be updated to use PotentialProvidersMap
	initialMissingDeps := CheckAllDependencies(allMods, potentialProviders) // Pass the new map
	if len(initialMissingDeps) > 0 {
		log.Printf("%s--- Initial Dependency Check ---", LogWarningPrefix)
		for modID, missing := range initialMissingDeps {
			modName := modID // Fallback if mod not in allMods (should not happen)
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
	ctx.Bisector = nil // This will be recreated on new "Load Mods & Start"

	// Clear TUI lists associated with bisection
	if ctx.SearchSpaceList != nil {
		ctx.SearchSpaceList.Clear().SetTitle("Search Space")
	}
	if ctx.GroupAList != nil {
		ctx.GroupAList.Clear().SetTitle("Group A")
	}
	if ctx.GroupBList != nil {
		ctx.GroupBList.Clear().SetTitle("Group B")
	}
	if ctx.AllModsList != nil {
		ctx.AllModsList.Clear().SetTitle("All Mods")
	}
	if ctx.ForceEnabledList != nil {
		ctx.ForceEnabledList.Clear().SetTitle("Force Enabled")
	}
	if ctx.ForceDisabledList != nil {
		ctx.ForceDisabledList.Clear().SetTitle("Force Disabled")
	}
	if ctx.ModSearchInput != nil {
		ctx.ModSearchInput.SetText("")
	}
	if ctx.InfoTextView != nil {
		ctx.InfoTextView.SetText("")
	}

	ctx.ClearUILogs()
	ctx.modalReturnCtx = ModalReturnContext{}
}

func (ctx *AppContext) SetModsPath(path string)                   { ctx.modsPath = path }
func (ctx *AppContext) GetModsPath() string                       { return ctx.modsPath }
func (ctx *AppContext) GetMaxUILogLines() int                     { return ctx.maxUILogLines }
func (ctx *AppContext) ClearUILogs()                              { ctx.uiLogLines = []string{} }
func (ctx *AppContext) GetModalReturnContext() ModalReturnContext { return ctx.modalReturnCtx }
