package widgets

import (
	"fmt"
	"strings"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

// SearchableTable combines a search input field with a tview.Table.
type SearchableTable struct {
	*tview.Flex
	table         *tview.Table
	searchField   *tview.InputField
	headers       []string
	rawData       [][]string // Stores all data rows for filtering. Each inner slice is a row.
	searchColumns []int
	columnWidths  []int
}

// NewSearchableTable creates a new SearchableTable.
func NewSearchableTable(headers []string, searchColumns ...int) *SearchableTable {
	st := &SearchableTable{
		Flex:          tview.NewFlex().SetDirection(tview.FlexRow),
		table:         tview.NewTable().SetSelectable(true, false).SetFixed(1, 0),
		searchField:   tview.NewInputField().SetPlaceholder("Search..."),
		headers:       headers,
		searchColumns: searchColumns,
	}

	st.table.SetEvaluateAllRows(false).SetBorder(false)

	st.AddItem(st.searchField, 1, 0, true).
		AddItem(st.table, 0, 1, false)

	st.calculateColumnWidths()
	st.populateHeaders()

	// --- Event and Style Handling ---
	st.searchField.SetChangedFunc(func(text string) {
		st.Filter(text)
	})

	searchFocusedStyle := st.searchField.GetFieldStyle().Foreground(tcell.ColorBlack)
	searchBlurredStyle := searchFocusedStyle.Background(tcell.ColorDarkSlateGray)

	st.searchField.SetFocusFunc(func() {
		st.searchField.SetFieldStyle(searchFocusedStyle)
		st.searchField.SetPlaceholderStyle(searchFocusedStyle)
		st.updateFocusWithin()
	})
	st.searchField.SetBlurFunc(func() {
		st.searchField.SetFieldStyle(searchBlurredStyle)
		st.searchField.SetPlaceholderStyle(searchBlurredStyle)
		st.updateFocusWithin()
	})
	st.searchField.Blur() // Start blurred

	st.table.SetFocusFunc(func() {
		st.updateFocusWithin()
		st.table.SetSelectable(true, false) // Ensure selectable on focus
	})
	st.table.SetBlurFunc(func() {
		st.updateFocusWithin()
		// Optional: Make it unselectable on blur to avoid confusion
		// st.table.SetSelectable(false, false)
	})
	st.table.Blur() // Start blurred

	st.updateFocusWithin()

	return st
}

// updateFocusWithin changes styles based on whether the widget has focus.
func (st *SearchableTable) updateFocusWithin() {
	if st.HasFocus() {
		st.table.SetSelectedStyle(tcell.StyleDefault.Foreground(tcell.ColorBlack).Background(tcell.ColorBlue))
	} else {
		st.table.SetSelectedStyle(tcell.StyleDefault.Foreground(tcell.ColorWhite).Background(tcell.ColorDarkSlateGray))
	}
}

// Blur is called when this primitive loses focus.
func (st *SearchableTable) Blur() {
	st.Flex.Blur()
	st.searchField.Blur()
	st.table.Blur()
	st.updateFocusWithin()
}

// Focus delegates focus to the search field by default.
func (st *SearchableTable) Focus(delegate func(p tview.Primitive)) {
	st.searchField.SetDoneFunc(func(key tcell.Key) {
		if key == tcell.KeyEnter || key == tcell.KeyDown {
			if st.table.GetRowCount() > 1 { // More than just the header
				delegate(st.table)
			}
		}
	})
	delegate(st.searchField)
	st.updateFocusWithin()
}

// Replace the SetData method
func (st *SearchableTable) SetData(data [][]string) {
	st.rawData = data
	st.calculateColumnWidths() // Calculate widths once
	st.Filter(st.searchField.GetText())
}

// Clear implements a custom Clear method that targets the inner table,
// preventing the Flex layout from being destroyed.
func (st *SearchableTable) Clear() {
	st.table.Clear()
	st.rawData = nil
	st.columnWidths = nil
}

// GetSelection returns the currently selected row and column.
func (st *SearchableTable) GetSelection() (row, column int) {
	return st.table.GetSelection()
}

// GetCell returns the cell at the specified row and column.
func (st *SearchableTable) GetCell(row, column int) *tview.TableCell {
	return st.table.GetCell(row, column)
}

// GetRowCount returns the number of rows in the table, including headers.
func (st *SearchableTable) GetRowCount() int {
	return st.table.GetRowCount()
}

// Select sets the currently selected cell by row and column.
func (st *SearchableTable) Select(row, column int) {
	st.table.Select(row, column)
}

// Filter re-populates the table based on the search query.
// Replace the Filter method to use pre-calculated widths
func (st *SearchableTable) Filter(query string) {
	// Preserve selection logic (unchanged)
	selectedRow, _ := st.table.GetSelection()
	var selectedRef string
	if selectedRow > 0 && selectedRow < st.table.GetRowCount() {
		selectedRef = st.table.GetCell(selectedRow, 1).Text
	}

	st.table.Clear()
	st.populateHeaders() // Headers also use the new width logic

	query = strings.ToLower(query)
	currentRow := 1
	newSelectedRow := 0

	for _, rowData := range st.rawData {
		matches := query == ""
		if !matches {
			for _, colIndex := range st.searchColumns {
				if colIndex < len(rowData) && strings.Contains(strings.ToLower(rowData[colIndex]), query) {
					matches = true
					break
				}
			}
		}

		if matches {
			for col, cellData := range rowData {
				cell := tview.NewTableCell(cellData).
					SetAlign(tview.AlignLeft).
					SetMaxWidth(st.columnWidths[col]). // Set fixed width
					SetExpansion(0)                    // Crucial: Set expansion to 0 for fixed width

				// Special handling for Status and Name columns as before
				if col == 0 {
					cell.SetTextColor(tcell.ColorGray) // Default color for status text
				}
				if col == 2 {
					cell.SetMaxWidth(35) // Enforce max width for name column
				}
				st.table.SetCell(currentRow, col, cell)
			}
			if selectedRef != "" && rowData[1] == selectedRef {
				newSelectedRow = currentRow
			}
			currentRow++
		}
	}

	// Restore selection logic (unchanged)
	if newSelectedRow > 0 {
		st.table.Select(newSelectedRow, 0)
	} else if st.table.GetRowCount() > 1 {
		st.table.Select(1, 0)
	}
}

// Add the new calculateColumnWidths method
func (st *SearchableTable) calculateColumnWidths() {
	if len(st.rawData) == 0 {
		st.columnWidths = make([]int, len(st.headers))
		return
	}

	widths := make([]int, len(st.headers))
	// Initialize with header widths
	for i, h := range st.headers {
		widths[i] = len(h)
	}

	// Find max width for each column from data
	for _, row := range st.rawData {
		for i, cellData := range row {
			// Strip color tags before calculating length
			width := tview.TaggedStringWidth(cellData)
			if width > widths[i] {
				widths[i] = width
			}
		}
	}

	st.columnWidths = widths
}

func (st *SearchableTable) populateHeaders() {
	for i, header := range st.headers {
		paddedHeader := fmt.Sprintf("%-*s", st.columnWidths[i], header)
		cell := tview.NewTableCell(paddedHeader).
			SetSelectable(false).
			SetTextColor(tcell.ColorYellow).
			SetAttributes(tcell.AttrBold).
			SetAlign(tview.AlignLeft).
			SetMaxWidth(st.columnWidths[i]). // Set fixed width for header
			SetExpansion(0)                  // Set expansion to 0 for fixed width

		if i == 2 {
			cell.SetMaxWidth(35) // Enforce max width for name column
		}
		st.table.SetCell(0, i, cell)
	}
}

// GetFocusablePrimitives implements the Focusable interface.
func (st *SearchableTable) GetFocusablePrimitives() []tview.Primitive {
	return []tview.Primitive{st.searchField, st.table}
}
