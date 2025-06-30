package widgets

import (
	"github.com/Qendolin/fabric-mod-bisect-tool/app/core/sets"
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

// OverviewWidget is a single-row visual representation of mod sets.
type OverviewWidget struct {
	*tview.Box
	allMods      []string
	conflictSet  sets.Set
	clearedSet   sets.Set
	candidateSet sets.Set
	effectiveSet sets.Set
}

// NewOverviewWidget creates a new widget. allMods should be a sorted list.
func NewOverviewWidget(allMods []string) *OverviewWidget {
	if allMods == nil {
		allMods = []string{}
	}
	return &OverviewWidget{
		Box:     tview.NewBox(),
		allMods: allMods,
	}
}

// SetAllMods sets or updates the universe of all mods for the widget.
func (w *OverviewWidget) SetAllMods(allMods []string) {
	w.allMods = allMods
}

// UpdateState provides the widget with the current sets to display.
func (w *OverviewWidget) UpdateState(problemMods, clearedMods, candidateMods, effectiveMods sets.Set) {
	w.conflictSet = problemMods
	w.clearedSet = clearedMods
	w.candidateSet = candidateMods
	w.effectiveSet = effectiveMods
}

// Draw implements tview.Primitive.
func (w *OverviewWidget) Draw(screen tcell.Screen) {
	w.Box.Draw(screen)
	x, y, width, _ := w.GetInnerRect()
	if width <= 0 || len(w.allMods) == 0 {
		return
	}

	splitPointScreenX := w.calculateSplitPointX(x, width)

	for i := 0; i < width; i++ {
		currentScreenX := x + i

		if currentScreenX == splitPointScreenX {
			w.drawSplitLine(screen, currentScreenX, y)
		} else {
			w.drawContentCell(screen, currentScreenX, y, width, i)
		}
	}
}

// calculateSplitPointX determines the screen X-coordinate for the bisection split line.
// Returns -1 if no split line should be drawn.
func (w *OverviewWidget) calculateSplitPointX(drawX, drawWidth int) int {
	if len(w.candidateSet) == 0 {
		return -1 // No candidates, no split.
	}

	// Find the absolute indices of the candidate block in the allMods list.
	candidateStartIndex := -1
	candidateEndIndex := -1
	for i, modID := range w.allMods {
		if _, isCandidate := w.candidateSet[modID]; isCandidate {
			if candidateStartIndex == -1 {
				candidateStartIndex = i
			}
			candidateEndIndex = i
		}
	}

	if candidateStartIndex == -1 {
		return -1 // Should not happen if len(w.candidateMods) > 0, but safety check.
	}

	numCandidates := candidateEndIndex - candidateStartIndex + 1
	// The split index is relative to the start of the candidates.
	// Use `(numCandidates + 1) / 2` to handle odd/even splits.
	splitIndexInCandidates := (numCandidates + 1) / 2
	// This is the absolute index in the `allMods` list.
	splitModIndex := candidateStartIndex + splitIndexInCandidates

	// Convert the mod index to a screen coordinate.
	// `drawWidth / len(w.allMods)` is the mods per screen cell.
	// `splitModIndex * modsPerCell` gives the pixel position.
	splitPointScreenX := drawX + (splitModIndex * drawWidth / len(w.allMods))

	// Check if the visual width of the candidate set is at least 3 cells.
	candidateStartScreenX := drawX + (candidateStartIndex * drawWidth / len(w.allMods))
	candidateEndScreenX := drawX + ((candidateEndIndex + 1) * drawWidth / len(w.allMods))
	candidateScreenWidth := candidateEndScreenX - candidateStartScreenX

	if candidateScreenWidth < 3 {
		return -1 // Not enough space, don't draw the line.
	}

	return splitPointScreenX
}

// drawSplitLine draws the vertical line at the bisection split point.
func (w *OverviewWidget) drawSplitLine(screen tcell.Screen, x, y int) {
	// The split point is always within the candidate set, which is white.
	style := tcell.StyleDefault.Foreground(tcell.ColorBlack).Background(tcell.ColorWhite)
	screen.SetContent(x, y, tview.BoxDrawingsDoubleVertical, nil, style)
}

// drawContentCell draws a single cell of the overview bar, determining its foreground and background colors.
func (w *OverviewWidget) drawContentCell(screen tcell.Screen, x, y, totalWidth, cellIndex int) {
	numTotalMods := len(w.allMods)

	// Determine the mod indices for the left and right halves of this cell.
	start1 := numTotalMods * cellIndex / totalWidth
	end1 := numTotalMods * (cellIndex*2 + 1) / (totalWidth * 2)
	start2 := end1
	end2 := numTotalMods * (cellIndex + 1) / totalWidth

	// Determine the color for each half.
	fgColor := w.determineColor(w.allMods[start1:end1])
	bgColor := w.determineColor(w.allMods[start2:end2])

	// Draw the cell using the half-block character.
	style := tcell.StyleDefault.Foreground(fgColor).Background(bgColor)
	screen.SetContent(x, y, '▌', nil, style)
}

// determineColor finds the dominant color for a slice of mod IDs.
func (w *OverviewWidget) determineColor(modIDs []string) tcell.Color {
	// Priority: 5: Conflict, 4: Cleared, 3: Unused, 2: Candidates, 1: Effective, 0: Rest
	highestPriority := 0

	for _, id := range modIDs {
		// Highest priority first, with early exit.
		if _, ok := w.conflictSet[id]; ok {
			highestPriority = 5
			break
		}

		if _, ok := w.clearedSet[id]; ok {
			if highestPriority < 4 {
				highestPriority = 4
			}
		} else if _, ok := w.candidateSet[id]; ok {
			if highestPriority < 2 {
				highestPriority = 2
			}
		} else if _, ok := w.effectiveSet[id]; ok {
			if highestPriority < 1 {
				highestPriority = 1
			}
		}
	}

	switch highestPriority {
	case 5:
		return tcell.ColorRed
	case 4:
		return tcell.ColorGreen
	case 3:
		fallthrough
	case 2:
		return tcell.ColorWhite
	case 1:
		return tcell.ColorSteelBlue
	default:
		return tcell.ColorGray
	}
}
