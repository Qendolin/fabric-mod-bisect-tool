package widgets

import (
	"fmt"
	"strings"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

// ExtendedTable combines a search input field with a tview.Table.
type ExtendedTable struct {
	*tview.Flex
	Table           *tview.Table
	Search          *tview.InputField
	headers         []string
	rawData         [][]string // Stores all data rows for filtering. Each inner slice is a row.
	searchColumns   []int
	columnWidths    []int
	maxColumnWidths []int
	cellClicked     func(int, int) bool
}

// NewExtendedTable creates a new ExtendedTable.
func NewExtendedTable(headers []string, search bool) *ExtendedTable {
	et := &ExtendedTable{
		Flex:          tview.NewFlex().SetDirection(tview.FlexRow),
		Table:         tview.NewTable().SetSelectable(true, false).SetFixed(1, 0),
		headers:       headers,
		searchColumns: []int{},
	}

	et.Table.SetEvaluateAllRows(false).SetBorder(false)

	if search {
		et.Search = tview.NewInputField().SetPlaceholder("Search...")
		et.AddItem(et.Search, 1, 0, true)
	}
	et.AddItem(et.Table, 0, 1, false)

	et.maxColumnWidths = make([]int, len(headers))
	et.calculateColumnWidths()
	et.populateHeaders()

	// --- Event and Style Handling ---
	if search {
		et.Search.SetChangedFunc(func(text string) {
			et.Filter(text)
		})

		searchFocusedStyle := et.Search.GetFieldStyle().Foreground(tcell.ColorBlack)
		searchBlurredStyle := searchFocusedStyle.Background(tcell.ColorDarkSlateGray)

		et.Search.SetFocusFunc(func() {
			et.Search.SetFieldStyle(searchFocusedStyle)
			et.Search.SetPlaceholderStyle(searchFocusedStyle)
			et.updateFocusWithin()
		})
		et.Search.SetBlurFunc(func() {
			et.Search.SetFieldStyle(searchBlurredStyle)
			et.Search.SetPlaceholderStyle(searchBlurredStyle)
			et.updateFocusWithin()
		})
		et.Search.Blur() // Start blurred
	}

	et.Table.SetFocusFunc(func() {
		et.updateFocusWithin()
		et.Table.SetSelectable(true, false) // Ensure selectable on focus
	})
	et.Table.SetBlurFunc(func() {
		et.updateFocusWithin()
	})
	et.Table.Blur() // Start blurred

	et.updateFocusWithin()

	return et
}

func (et *ExtendedTable) SetSearchColumns(searchColumns ...int) {
	et.searchColumns = searchColumns
}

func (et *ExtendedTable) SetClickedHandler(handler func(row int, column int) bool) {
	et.cellClicked = handler
}

func (et *ExtendedTable) SetMaxColumnWidths(maxWidths ...int) {
	if len(maxWidths) != len(et.headers) {
		panic("max column width value count must match column count")
	}
	et.maxColumnWidths = maxWidths
	et.calculateColumnWidths()
}

// updateFocusWithin changes styles based on whether the widget has focus.
func (et *ExtendedTable) updateFocusWithin() {
	if et.HasFocus() {
		et.Table.SetSelectedStyle(tcell.StyleDefault.Foreground(tcell.ColorBlack).Background(tcell.ColorBlue))
	} else {
		et.Table.SetSelectedStyle(tcell.StyleDefault.Foreground(tcell.ColorWhite).Background(tcell.ColorDarkSlateGray))
	}
}

// Blur is called when this primitive loses focus.
func (et *ExtendedTable) Blur() {
	et.Flex.Blur()
	if et.Search != nil {
		et.Search.Blur()
	}
	et.Table.Blur()
	et.updateFocusWithin()
}

// Focus delegates focus to the search field by default.
func (et *ExtendedTable) Focus(delegate func(p tview.Primitive)) {
	if et.Search != nil {
		et.Search.SetDoneFunc(func(key tcell.Key) {
			if key == tcell.KeyEnter || key == tcell.KeyDown {
				if et.Table.GetRowCount() > 1 { // More than just the header
					delegate(et.Table)
				}
			}
		})
		delegate(et.Search)
	} else {
		delegate(et.Table)
	}
	et.updateFocusWithin()
}

func (et *ExtendedTable) SetData(data [][]string) {
	et.rawData = data
	et.calculateColumnWidths() // Calculate widths once
	if et.Search != nil {
		et.Filter(et.Search.GetText())
	} else {
		et.Filter("")
	}
}

func (et *ExtendedTable) GetData(row int, column int) *string {
	if row < 0 || row >= len(et.rawData) || column < 0 || column >= len(et.rawData[row]) {
		return nil
	}
	return &et.rawData[row][column]
}

// Clear implements a custom Clear method that targets the inner table,
// preventing the Flex layout from being destroyed.
func (et *ExtendedTable) Clear() {
	et.Table.Clear()
	et.rawData = nil
	et.columnWidths = nil
}

// GetSelection returns the currently selected row and column.
func (et *ExtendedTable) GetSelection() (row, column int) {
	return et.Table.GetSelection()
}

// GetCell returns the cell at the specified row and column.
func (et *ExtendedTable) GetCell(row, column int) *tview.TableCell {
	return et.Table.GetCell(row, column)
}

// GetRowCount returns the number of rows in the table, including headers.
func (et *ExtendedTable) GetRowCount() int {
	return et.Table.GetRowCount()
}

// Select sets the currently selected cell by row and column.
func (et *ExtendedTable) Select(row, column int) {
	et.Table.Select(row, column)
}

// Filter re-populates the table based on the search query.
// Replace the Filter method to use pre-calculated widths
func (et *ExtendedTable) Filter(query string) {
	// Preserve selection logic
	selectedRow, _ := et.Table.GetSelection()
	var selectedRef string
	if selectedRow > 0 && selectedRow < et.Table.GetRowCount() {
		selectedRef = et.Table.GetCell(selectedRow, 1).Text
	}

	et.Table.Clear()
	et.populateHeaders() // Headers also use the new width logic

	query = strings.ToLower(query)
	currentRow := 1
	newSelectedRow := 0

	for _, rowData := range et.rawData {
		matches := query == ""
		if !matches {
			for _, colIndex := range et.searchColumns {
				if colIndex < len(rowData) && strings.Contains(strings.ToLower(rowData[colIndex]), query) {
					matches = true
					break
				}
			}
		}

		if matches {
			for col, cellData := range rowData {
				var cell *tview.TableCell
				if col == len(rowData)-1 {
					// expand last column
					cell = tview.NewTableCell(cellData).
						SetAlign(tview.AlignLeft).
						SetExpansion(1)
				} else {
					cell = tview.NewTableCell(cellData).
						SetAlign(tview.AlignLeft).
						SetMaxWidth(et.columnWidths[col]). // Set fixed width
						SetExpansion(0)                    // Crucial: Set expansion to 0 for fixed width
				}

				et.Table.SetCell(currentRow, col, cell)
			}
			if selectedRef != "" && rowData[1] == selectedRef {
				newSelectedRow = currentRow
			}
			currentRow++
		}
	}

	// Restore selection logic
	if newSelectedRow > 0 {
		et.Table.Select(newSelectedRow, 0)
	} else if et.Table.GetRowCount() > 1 {
		et.Table.Select(1, 0)
	}
}

// Add the new calculateColumnWidths method
func (et *ExtendedTable) calculateColumnWidths() {
	if len(et.rawData) == 0 {
		et.columnWidths = make([]int, len(et.headers))
		return
	}

	widths := make([]int, len(et.headers))
	// Initialize with header widths
	for i, h := range et.headers {
		widths[i] = len(h)
	}

	// Find max width for each column from data
	for _, row := range et.rawData {
		for i, cellData := range row {
			// Strip color tags before calculating length
			width := tview.TaggedStringWidth(cellData)
			widths[i] = max(widths[i], width)
			if et.maxColumnWidths[i] > 0 {
				widths[i] = min(widths[i], et.maxColumnWidths[i])
			}
		}
	}

	et.columnWidths = widths
}

func (et *ExtendedTable) populateHeaders() {
	for i, header := range et.headers {
		paddedHeader := fmt.Sprintf("%-*s", et.columnWidths[i], header)
		cell := tview.NewTableCell(paddedHeader).
			SetSelectable(false).
			SetTextColor(tcell.ColorYellow).
			SetAttributes(tcell.AttrBold).
			SetAlign(tview.AlignLeft).
			SetMaxWidth(et.columnWidths[i]). // Set fixed width for header
			SetExpansion(0)                  // Set expansion to 0 for fixed width

		et.Table.SetCell(0, i, cell)
	}
}

// GetFocusablePrimitives implements the Focusable interface.
func (et *ExtendedTable) GetFocusablePrimitives() []tview.Primitive {
	if et.Search != nil {
		return []tview.Primitive{et.Search, et.Table}
	}
	return []tview.Primitive{et.Table}
}

// MouseHandler returns the mouse handler for this primitive.
func (et *ExtendedTable) MouseHandler() func(action tview.MouseAction, event *tcell.EventMouse, setFocus func(p tview.Primitive)) (consumed bool, capture tview.Primitive) {
	return et.WrapMouseHandler(func(action tview.MouseAction, event *tcell.EventMouse, setFocus func(p tview.Primitive)) (consumed bool, capture tview.Primitive) {
		x, y := event.Position()
		if !et.InRect(x, y) {
			return false, nil
		}

		if action == tview.MouseLeftClick {
			row, column := et.Table.CellAt(x, y)
			if et.cellClicked != nil && et.cellClicked(row, column) {
				return true, nil
			}
		}

		return et.Table.MouseHandler()(action, event, setFocus)
	})
}
