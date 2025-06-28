package ui

import (
	"path/filepath"
	"strings"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

const PageManageModsID = "manage_mods"

// ManageModsPage allows viewing and changing the state of all mods.
type ManageModsPage struct {
	*tview.Flex
	app AppInterface

	modTable          *SearchableTable
	forceEnabledList  *tview.TextView
	forceDisabledList *tview.TextView
	statusText        *tview.TextView
}

// NewManageModsPage creates a new page for managing mod states.
func NewManageModsPage(app AppInterface) *ManageModsPage {
	p := &ManageModsPage{
		Flex:              tview.NewFlex(),
		app:               app,
		forceEnabledList:  tview.NewTextView().SetDynamicColors(true).SetWordWrap(true),
		forceDisabledList: tview.NewTextView().SetDynamicColors(true).SetWordWrap(true),
		statusText:        tview.NewTextView().SetDynamicColors(true),
	}

	headers := []string{"Status", "ID", "Name", "File"}
	// Search on column 1 (ID) and 2 (Name)
	p.modTable = NewSearchableTable(headers, 1, 2)
	p.modTable.SetBorderPadding(0, 0, 1, 1)

	p.forceDisabledList.SetBorderPadding(0, 0, 1, 1)
	p.forceEnabledList.SetBorderPadding(0, 0, 1, 1)

	p.setupLayout()
	p.SetInputCapture(p.inputHandler())
	p.RefreshState()

	p.statusText.SetText("Manage individual mod states. Press [darkcyan::b]ESC[-:-:-] to return.")
	return p
}

func (p *ManageModsPage) setupLayout() {
	mainListFrame := NewTitleFrame(p.modTable, "All Mods")
	enabledFrame := NewTitleFrame(p.forceEnabledList, "Force Enabled")
	disabledFrame := NewTitleFrame(p.forceDisabledList, "Force Disabled")

	sideBar := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(enabledFrame, 0, 1, false).
		AddItem(disabledFrame, 0, 1, false)

	p.AddItem(mainListFrame, 0, 3, true).
		AddItem(nil, 1, 0, false).
		AddItem(sideBar, 0, 1, false)
}

func (p *ManageModsPage) inputHandler() func(event *tcell.EventKey) *tcell.EventKey {
	return func(event *tcell.EventKey) *tcell.EventKey {
		if p.app.GetFocus() == p.modTable.table {
			// Handle state changes when table is focused
			if p.handleTableInput(event) == nil {
				return nil
			}
		}

		if event.Key() == tcell.KeyEscape {
			p.app.Navigation().GoBack()
			return nil
		}

		return event
	}
}

func (p *ManageModsPage) handleTableInput(event *tcell.EventKey) *tcell.EventKey {
	modState := p.app.GetModState()
	if modState == nil {
		return event
	}

	row, _ := p.modTable.table.GetSelection()
	if row <= 0 { // No selection or header selected
		return event
	}

	// Get the mod ID from the selected row (it's in column 1)
	cell := p.modTable.table.GetCell(row, 1)
	if cell == nil {
		return event
	}
	modID := cell.Text

	shift := event.Modifiers() == tcell.ModShift

	switch event.Rune() {
	case 'd', 'D':
		p.toggleState(modID, shift, "Disabled", modState.SetForceDisabled)
		return nil
	case 'e', 'E':
		p.toggleState(modID, shift, "Enabled", modState.SetForceEnabled)
		return nil
	case 'g', 'G':
		p.toggleState(modID, shift, "Good", modState.SetManuallyGood)
		return nil
	}

	return event
}

// toggleState handles toggling a state for a single mod or all mods.
func (p *ManageModsPage) toggleState(modID string, isBulk bool, stateType string, setter func(string, bool)) {
	modState := p.app.GetModState()

	if isBulk {
		allIDs := modState.GetAllModIDs()
		// Determine if the primary action should be to enable or disable the state for all.
		// We check if *at least one* mod does NOT have the state. If so, our action is to turn it ON for everyone.
		// If ALL mods already have the state, our action is to turn it OFF for everyone.
		allCurrentlyTrue := true
		for _, id := range allIDs {
			status, _ := modState.GetModStatus(id)
			var currentState bool
			switch stateType {
			case "Disabled":
				currentState = status.ForceDisabled
			case "Enabled":
				currentState = status.ForceEnabled
			case "Good":
				currentState = status.ManuallyGood
			}
			if !currentState {
				allCurrentlyTrue = false
				break
			}
		}

		// If all mods are already in the target state, the action is to turn it off for all.
		// Otherwise, the action is to turn it on for all.
		newState := !allCurrentlyTrue
		for _, id := range allIDs {
			setter(id, newState)
		}
	} else {
		// Toggle state for the single selected mod
		status, _ := modState.GetModStatus(modID)
		var currentState bool
		switch stateType {
		case "Disabled":
			currentState = status.ForceDisabled
		case "Enabled":
			currentState = status.ForceEnabled
		case "Good":
			currentState = status.ManuallyGood
		}
		// When toggling a single item, we also need to handle un-setting other states.
		// The StateManager's SetForce... methods already handle this exclusivity.
		setter(modID, !currentState)
	}
	p.RefreshState() // Refresh UI after state change
}

// RefreshState updates the lists with the current mod states.
func (p *ManageModsPage) RefreshState() {
	modState := p.app.GetModState()
	searchProcess := p.app.GetSearchProcess()
	if modState == nil || searchProcess == nil {
		return
	}

	// Preserve selection
	row, _ := p.modTable.table.GetSelection()

	allIDs := modState.GetAllModIDs()
	allMods := modState.GetAllMods()
	tableData := make([][]string, 0, len(allIDs))

	enabledIDs := []string{}
	disabledIDs := []string{}

	// Get the next test set for preview from the idempotent getter
	nextPlan, _ := searchProcess.GetNextTestPlan()
	var nextTestSet map[string]struct{}
	if nextPlan != nil {
		nextTestSet = nextPlan.ModIDsToTest
	}

	for _, id := range allIDs {
		status, _ := modState.GetModStatus(id)
		mod := allMods[id]
		searcherState := searchProcess.GetCurrentState()

		var statusStr string
		if status.ForceEnabled {
			statusStr = "[green]Enabled[-:-:-]"
			enabledIDs = append(enabledIDs, id)
		} else if status.ForceDisabled {
			statusStr = "[red]Disabled[-:-:-]"
			disabledIDs = append(disabledIDs, id)
		} else if status.ManuallyGood {
			statusStr = "[green]Good[-:-:-]"
		} else if _, ok := searcherState.ConflictSet[id]; ok {
			statusStr = "[::b]Problem[-:-:-]"
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

	// Restore selection
	if row > 0 && row < p.modTable.table.GetRowCount() {
		p.modTable.table.Select(row, 0)
	}
}

func (p *ManageModsPage) Primitive() tview.Primitive { return p }

func (p *ManageModsPage) GetActionPrompts() map[string]string {
	return map[string]string{
		"E": "Force Enable", "D": "Force Disable", "G": "Mark Good", "Shift+Key": "Toggle All", "ESC": "Back",
	}
}

// GetStatusPrimitive returns the tview.Primitive that displays the page's status
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

// Implement SearchStateObserver to refresh when search state changes
func (p *ManageModsPage) RefreshSearchState() {
	p.RefreshState()
}
