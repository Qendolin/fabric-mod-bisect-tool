package layout

import "fyne.io/fyne/v2"

type GridTrack struct {
	Fraction float32 // e.g., 1 (like 1fr). 0 means it's a fixed size.
	Fixed    float32 // Exact pixel size. If Fraction & Fixed are 0, uses max MinSize of row/col items.
}

type GridCell struct {
	Row, Col int
}

type GridLayout struct {
	Rows  []GridTrack
	Cols  []GridTrack
	Cells map[fyne.CanvasObject]GridCell
	Gap   float32
}

func NewGridLayout(rows, cols []GridTrack, gap float32) *GridLayout {
	return &GridLayout{
		Rows:  rows,
		Cols:  cols,
		Cells: make(map[fyne.CanvasObject]GridCell),
		Gap:   gap,
	}
}

// Set assigns a specific row and column to a CanvasObject
func (l *GridLayout) Set(obj fyne.CanvasObject, row, col int) {
	l.Cells[obj] = GridCell{Row: row, Col: col}
}

func (l *GridLayout) calculateTracks(tracks []GridTrack, totalSize float32, objects []fyne.CanvasObject, isRow bool) []float32 {
	if len(tracks) == 0 {
		return nil
	}

	sizes := make([]float32, len(tracks))
	var totalFixed, totalFraction float32

	for i, t := range tracks {
		if t.Fraction > 0 {
			totalFraction += t.Fraction
		} else if t.Fixed > 0 {
			sizes[i] = t.Fixed
			totalFixed += t.Fixed
		} else {
			var maxMin float32
			for _, obj := range objects {
				if !obj.Visible() {
					continue
				}
				cell, ok := l.Cells[obj]
				if !ok {
					continue
				}
				idx := cell.Col
				if isRow {
					idx = cell.Row
				}

				if idx == i {
					val := obj.MinSize().Width
					if isRow {
						val = obj.MinSize().Height
					}
					if val > maxMin {
						maxMin = val
					}
				}
			}
			sizes[i] = maxMin
			totalFixed += maxMin
		}
	}

	gaps := float32(0)
	if len(tracks) > 1 {
		gaps = float32(len(tracks)-1) * l.Gap
	}
	available := totalSize - totalFixed - gaps
	if available < 0 {
		available = 0
	}

	if totalFraction > 0 {
		for i, t := range tracks {
			if t.Fraction > 0 {
				sizes[i] = available * (t.Fraction / totalFraction)
			}
		}
	}
	return sizes
}

func (l *GridLayout) MinSize(objects []fyne.CanvasObject) fyne.Size {
	// Calculate sizes with 0 totalSize to determine minimum structural footprint
	colSizes := l.calculateTracks(l.Cols, 0, objects, false)
	rowSizes := l.calculateTracks(l.Rows, 0, objects, true)

	var w, h float32
	for _, s := range colSizes {
		w += s
	}
	for _, s := range rowSizes {
		h += s
	}

	if len(l.Cols) > 1 {
		w += float32(len(l.Cols)-1) * l.Gap
	}
	if len(l.Rows) > 1 {
		h += float32(len(l.Rows)-1) * l.Gap
	}

	return fyne.NewSize(w, h)
}

func (l *GridLayout) Layout(objects []fyne.CanvasObject, size fyne.Size) {
	colSizes := l.calculateTracks(l.Cols, size.Width, objects, false)
	rowSizes := l.calculateTracks(l.Rows, size.Height, objects, true)

	colPos, rowPos := make([]float32, len(l.Cols)), make([]float32, len(l.Rows))
	var curX, curY float32
	for i, s := range colSizes {
		colPos[i] = curX
		curX += s + l.Gap
	}
	for i, s := range rowSizes {
		rowPos[i] = curY
		curY += s + l.Gap
	}

	for _, obj := range objects {
		if !obj.Visible() {
			continue
		}
		cell, ok := l.Cells[obj]
		if !ok || cell.Row < 0 || cell.Row >= len(l.Rows) || cell.Col < 0 || cell.Col >= len(l.Cols) {
			continue // Hide or ignore unmapped objects
		}

		obj.Move(fyne.NewPos(colPos[cell.Col], rowPos[cell.Row]))
		obj.Resize(fyne.NewSize(colSizes[cell.Col], rowSizes[cell.Row]))
	}
}
