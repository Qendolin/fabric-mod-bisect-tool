package pages

import (
	"path/filepath"
	"strings"

	"github.com/Qendolin/fabric-mod-bisect-tool/app/core/mods"
	"github.com/Qendolin/fabric-mod-bisect-tool/app/core/sets"
	"github.com/Qendolin/fabric-mod-bisect-tool/app/ui"
	"github.com/Qendolin/fabric-mod-bisect-tool/app/ui/widgets"
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

const PageManageModsID = "manage_mods"

// ManageModsPage allows viewing and changing the state of all mods.
type ManageModsPage struct {
	*tview.Flex
	app     ui.AppInterface
	session *ManagementSession

	modTable          *widgets.SearchableTable
	forceEnabledList  *tview.TextView
	forceDisabledList *tview.TextView
	statusText        *tview.TextView
}

// NewManageModsPage creates a new page for managing mod states.
func NewManageModsPage(app ui.AppInterface) *ManageModsPage {
	p := &ManageModsPage{
		Flex:              tview.NewFlex(),
		app:               app,
		forceEnabledList:  tview.NewTextView().SetDynamicColors(true).SetWordWrap(true),
		forceDisabledList: tview.NewTextView().SetDynamicColors(true).SetWordWrap(true),
		statusText:        tview.NewTextView().SetDynamicColors(true),
	}

	headers := []string{"Status", "ID", "Name", "File"}
	p.modTable = widgets.NewSearchableTable(headers, 1, 2) // Search on ID (col 1) and Name (col 2)
	p.modTable.SetBorderPadding(0, 0, 1, 1)

	p.forceDisabledList.SetBorderPadding(0, 0, 1, 1)
	p.forceEnabledList.SetBorderPadding(0, 0, 1, 1)

	p.setupLayout()
	p.SetInputCapture(p.inputHandler())
	p.RefreshState() // Initial population

	p.statusText.SetText("Manage individual mod states.")
	return p
}

func (p *ManageModsPage) setupLayout() {
	mainListFrame := widgets.NewTitleFrame(p.modTable, "All Mods")
	enabledFrame := widgets.NewTitleFrame(p.forceEnabledList, "Force Enabled")
	disabledFrame := widgets.NewTitleFrame(p.forceDisabledList, "Force Disabled")

	sideBar := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(enabledFrame, 0, 1, false).
		AddItem(disabledFrame, 0, 1, false)

	p.AddItem(mainListFrame, 0, 3, true).
		AddItem(nil, 1, 0, false).
		AddItem(sideBar, 0, 1, false)
}

func (p *ManageModsPage) inputHandler() func(event *tcell.EventKey) *tcell.EventKey {
	return func(event *tcell.EventKey) *tcell.EventKey {
		if _, ok := p.app.GetFocus().(*tview.InputField); ok {
			return event
		}

		if p.modTable.HasFocus() {
			// Handle state changes when table is focused
			if p.handleTableInput(event) == nil {
				return nil
			}
		}

		if event.Key() == tcell.KeyEscape {
			if p.session == nil || !p.session.HasChanges() {
				p.app.Navigation().GoBack() // No changes, just go back.
				return nil
			}

			// There are changes, show the apply/discard dialog.
			p.app.Dialogs().ShowQuestionDialog(
				"You have unsaved changes. Apply them?",
				func() {
					p.commitChanges()
				},
				func() {
					p.app.Navigation().GoBack()
				},
			)
			return nil
		}

		return event
	}
}

// NEW: commitChanges applies the session state to the real StateManager.
func (p *ManageModsPage) commitChanges() {
	if p.session == nil {
		p.app.Navigation().GoBack()
		return
	}

	stateMgr := p.app.GetStateManager()
	enabled, disabled, omitted, normal := p.session.CalculateChanges()

	var pendingAdditionsOnCommit []string
	for _, id := range normal {
		if p.session.isPendingAddition(id) {
			pendingAdditionsOnCommit = append(pendingAdditionsOnCommit, id)
		}
	}

	// Apply all changes in batches.
	if len(enabled) > 0 {
		stateMgr.SetForceEnabledBatch(enabled, true)
	}
	if len(disabled) > 0 {
		stateMgr.SetForceDisabledBatch(disabled, true)
	}
	if len(omitted) > 0 {
		stateMgr.SetOmittedBatch(omitted, true)
	}
	// For mods returning to a normal state, we must explicitly set all flags to false.
	for _, id := range normal {
		stateMgr.SetForceEnabled(id, false)
		stateMgr.SetForceDisabled(id, false)
		stateMgr.SetOmitted(id, false)
	}

	// Now, check if this commit resulted in pending additions and show the info dialog.
	if len(pendingAdditionsOnCommit) > 0 {
		p.app.Dialogs().ShowInfoDialog(
			"Pending Changes",
			"Some mods you have changed will only be added to the search pool at the start of the next bisection iteration.",
			func() {
				// Navigate back only after the user dismisses this second dialog.
				p.app.Navigation().GoBack()
			},
		)
	} else {
		// If there were no pending additions, navigate back immediately.
		p.app.Navigation().GoBack()
	}
}

func (p *ManageModsPage) handleTableInput(event *tcell.EventKey) *tcell.EventKey {
	modState := p.app.GetStateManager()
	if modState == nil {
		return event
	}

	row, _ := p.modTable.GetSelection()
	if row <= 0 { // No selection or header selected
		return event
	}

	cell := p.modTable.GetCell(row, 1)
	if cell == nil {
		return event
	}
	modID := cell.Text
	shift := event.Modifiers()&tcell.ModShift != 0

	switch event.Rune() {
	case 'd', 'D':
		p.session.ToggleForceDisable(modID, shift)
	case 'e', 'E':
		p.session.ToggleForceEnable(modID, shift)
	case 'o', 'O':
		p.session.ToggleOmitted(modID, shift)
	}
	p.RefreshState()

	return event
}

func (p *ManageModsPage) OnPageActivated() {
	vm := p.app.GetViewModel()
	if vm.IsReady {
		// Create a new session with the current true state.
		p.session = NewManagementSession(p.app.GetStateManager())
	} else {
		p.session = nil
	}
	p.RefreshState()
}

// RefreshState updates the lists with the current mod states using the ViewModel.
func (p *ManageModsPage) RefreshState() {
	vm := p.app.GetViewModel()
	if !vm.IsReady || p.session == nil {
		p.modTable.Clear()
		p.forceEnabledList.SetText("")
		p.forceDisabledList.SetText("")
		return
	}
	modState := p.app.GetStateManager()

	row, _ := p.modTable.GetSelection() // Preserve selection

	allIDs := modState.GetAllModIDs()
	allMods := modState.GetAllMods()
	tableData := make([][]string, 0, len(allIDs))
	enabledIDs := []string{}
	disabledIDs := []string{}

	var nextTestSet sets.Set
	if vm.CurrentTestPlan != nil {
		nextTestSet = vm.CurrentTestPlan.ModIDsToTest
	}

	for _, id := range allIDs {
		status := p.session.workingStatuses[id]
		mod := allMods[id]

		_, isGloballyPending := vm.PendingAdditions[id]
		isSessionPending := p.session.isPendingAddition(id)

		var statusStr string
		// Priority: Forced > Omitted > Problem > In Test > Inactive
		if isGloballyPending || isSessionPending {
			statusStr = "[mediumpurple]Pending[-:-:-]"
		} else if status.ForceEnabled {
			statusStr = "[green]Forced[-:-:-]"
			enabledIDs = append(enabledIDs, id)
		} else if status.ForceDisabled {
			statusStr = "[maroon]Disabled[-:-:-]"
			disabledIDs = append(disabledIDs, id)
		} else if status.Omitted {
			statusStr = "[steelblue]Omitted[-:-:-]"
		} else if _, ok := vm.CurrentConflictSet[id]; ok {
			statusStr = "[red::b]Problem[-:-:-]"
		} else if _, ok := nextTestSet[id]; ok {
			statusStr = "[white]In Test[-:-:-]"
		} else {
			statusStr = "[gray]Inactive[-:-:-]"
		}

		name := mod.FriendlyName()
		if len(name) > 35 {
			name = name[:32] + "..."
		}

		rowData := []string{statusStr, id, name, filepath.Base(mod.Path)}
		tableData = append(tableData, rowData)
	}

	p.modTable.SetData(tableData)
	p.forceEnabledList.SetText(strings.Join(enabledIDs, "\n"))
	p.forceDisabledList.SetText(strings.Join(disabledIDs, "\n"))

	if row > 0 && row < p.modTable.GetRowCount() {
		p.modTable.Select(row, 0) // Restore selection
	}
}

// GetActionPrompts returns the key actions for the page.
func (p *ManageModsPage) GetActionPrompts() []ui.ActionPrompt {
	return []ui.ActionPrompt{
		{Input: "E", Action: "Force Enable"},
		{Input: "D", Action: "Force Disable"},
		{Input: "O", Action: "Omit"},
		{Input: "Shift+Key", Action: "Toggle All"},
	}
}

// GetStatusPrimitive returns the tview.Primitive that displays the page's status.
func (p *ManageModsPage) GetStatusPrimitive() *tview.TextView {
	return p.statusText
}

// GetFocusablePrimitives implements the Focusable interface.
func (p *ManageModsPage) GetFocusablePrimitives() []tview.Primitive {
	return []tview.Primitive{
		p.modTable,
		p.forceEnabledList,
		p.forceDisabledList,
	}
}

// RefreshSearchState implements the SearchStateObserver interface.
func (p *ManageModsPage) RefreshSearchState() {
	p.RefreshState()
}

// --- Session Management ---

// ManagementSession holds a temporary, mutable copy of the mod statuses
// for the duration of the user's visit to the ManageModsPage.
type ManagementSession struct {
	workingStatuses  map[string]*mods.ModStatus
	originalStatuses map[string]mods.ModStatus
}

// NewManagementSession creates a new session initialized with the current state.
func NewManagementSession(state *mods.StateManager) *ManagementSession {
	originalSnapshot := state.GetModStatusesSnapshot()
	workingCopy := make(map[string]*mods.ModStatus, len(originalSnapshot))
	for id, status := range originalSnapshot {
		sCopy := status
		workingCopy[id] = &sCopy
	}
	return &ManagementSession{
		originalStatuses: originalSnapshot,
		workingStatuses:  workingCopy,
	}
}

// HasChanges compares the session's state to the original state to see if anything changed.
func (s *ManagementSession) HasChanges() bool {
	for id, workingStatus := range s.workingStatuses {
		originalStatus := s.originalStatuses[id]
		if *workingStatus != originalStatus {
			return true
		}
	}
	return false
}

// CalculateChanges determines which mods changed state and returns them in categorized lists.
func (s *ManagementSession) CalculateChanges() (enabled, disabled, omitted, normal []string) {
	for id, workingStatus := range s.workingStatuses {
		originalStatus := s.originalStatuses[id]
		if *workingStatus != originalStatus {
			if workingStatus.ForceEnabled {
				enabled = append(enabled, id)
			} else if workingStatus.ForceDisabled {
				disabled = append(disabled, id)
			} else if workingStatus.Omitted {
				omitted = append(omitted, id)
			} else {
				normal = append(normal, id)
			}
		}
	}
	return
}

// determineBulkToggleState decides the goal for a bulk operation. If any item is not
// in the target state, the goal is to set all items to that state. If all items
// are already in the target state, the goal is to clear the state for all items.
func (s *ManagementSession) determineBulkToggleState(modIDs []string, hasState func(*mods.ModStatus) bool) bool {
	allHaveState := true
	for _, id := range modIDs {
		if !hasState(s.workingStatuses[id]) {
			allHaveState = false
			break
		}
	}
	return !allHaveState
}

// setStatus sets the state for a single mod, ensuring mutual exclusivity.
func (s *ManagementSession) setStatus(modID string, enabled, disabled, omitted bool) {
	if status, ok := s.workingStatuses[modID]; ok {
		status.ForceEnabled = enabled
		status.ForceDisabled = disabled
		status.Omitted = omitted
	}
}

// ToggleForceEnable toggles the force-enabled state.
func (s *ManagementSession) ToggleForceEnable(modID string, isBulk bool) {
	if _, ok := s.workingStatuses[modID]; !ok {
		return
	}
	if isBulk {
		allIDs := s.getAllIDs()
		shouldEnable := s.determineBulkToggleState(allIDs, func(st *mods.ModStatus) bool { return st.ForceEnabled })
		for _, id := range allIDs {
			s.setStatus(id, shouldEnable, false, false)
		}
	} else {
		if s.workingStatuses[modID].ForceEnabled {
			s.setStatus(modID, false, false, false) // Is enabled -> set to normal
		} else {
			s.setStatus(modID, true, false, false) // Is not enabled -> enable
		}
	}
}

// ToggleForceDisable toggles the force-disabled state.
func (s *ManagementSession) ToggleForceDisable(modID string, isBulk bool) {
	if _, ok := s.workingStatuses[modID]; !ok {
		return
	}
	if isBulk {
		allIDs := s.getAllIDs()
		shouldDisable := s.determineBulkToggleState(allIDs, func(st *mods.ModStatus) bool { return st.ForceDisabled })
		for _, id := range allIDs {
			s.setStatus(id, false, shouldDisable, false)
		}
	} else {
		if s.workingStatuses[modID].ForceDisabled {
			s.setStatus(modID, false, false, false)
		} else {
			s.setStatus(modID, false, true, false)
		}
	}
}

// ToggleOmitted toggles the omitted state.
func (s *ManagementSession) ToggleOmitted(modID string, isBulk bool) {
	if _, ok := s.workingStatuses[modID]; !ok {
		return
	}
	if isBulk {
		allIDs := s.getAllIDs()
		shouldOmit := s.determineBulkToggleState(allIDs, func(st *mods.ModStatus) bool { return st.Omitted })
		for _, id := range allIDs {
			s.setStatus(id, false, false, shouldOmit)
		}
	} else {
		if s.workingStatuses[modID].Omitted {
			s.setStatus(modID, false, false, false)
		} else {
			s.setStatus(modID, false, false, true)
		}
	}
}

func (s *ManagementSession) getAllIDs() []string {
	ids := make([]string, 0, len(s.workingStatuses))
	for id := range s.workingStatuses {
		ids = append(ids, id)
	}
	return ids
}

// isPendingAddition determines if a mod is being transitioned from a special
// state back to a normal state within this session.
func (s *ManagementSession) isPendingAddition(modID string) bool {
	original := s.originalStatuses[modID]
	working := s.workingStatuses[modID]

	// Was it originally "special" (i.e., not a normal candidate)?
	wasSpecial := original.ForceEnabled || original.ForceDisabled || original.Omitted
	// Is its new staged state "normal"?
	isNowNormal := !working.ForceEnabled && !working.ForceDisabled && !working.Omitted

	return wasSpecial && isNowNormal
}
