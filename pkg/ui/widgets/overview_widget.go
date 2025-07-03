package widgets

import (
	"github.com/Qendolin/fabric-mod-bisect-tool/pkg/core/sets"
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

	lastModIndex := 0
	for i := 0; i < width; i++ {
		currentScreenX := x + i

		endModIndex := len(w.allMods) * (i + 1) / width
		if currentScreenX == splitPointScreenX {
			w.drawSplitLine(screen, currentScreenX, y, lastModIndex, endModIndex)
		} else {
			w.drawContentCell(screen, currentScreenX, y, lastModIndex, endModIndex)
		}
		lastModIndex = endModIndex
	}
}

// calculateSplitPointX determines the screen X-coordinate for the bisection split line.
// Returns -1 if no split line should be drawn.
func (w *OverviewWidget) calculateSplitPointX(drawX, drawWidth int) int {
	// The split line is only meaningful if there are enough candidates to form two groups.
	if len(w.candidateSet) < 2 {
		return -1
	}

	// 1. Get an ordered list of the actual candidate mod IDs. This is the list the bisection algorithm operates on.
	candidateMods := sets.MakeSlice(w.candidateSet)
	numCandidates := len(candidateMods)

	// Don't draw the line if the candidate set is too small to be visually useful.
	if numCandidates < 3 {
		return -1
	}

	// 2. Find the logical split point within the candidate list.
	// This gives us the index of the first mod in the second partition (C2).
	splitIndexInCandidates := sets.GetSplitIndex(numCandidates)
	if splitIndexInCandidates >= len(candidateMods) {
		return -1 // Safety check, should not happen.
	}
	splitModID := candidateMods[splitIndexInCandidates]

	// 3. Find the absolute index of this specific mod within the `w.allMods` list, which dictates the visual layout.
	splitModIndexInAllMods := -1
	for i, modID := range w.allMods {
		if modID == splitModID {
			splitModIndexInAllMods = i
			break
		}
	}

	if splitModIndexInAllMods == -1 {
		// This can happen if the widget's state is inconsistent. Do not draw.
		return -1
	}

	// 4. Convert the absolute mod index to a screen coordinate.
	return drawX + (splitModIndexInAllMods * drawWidth / len(w.allMods))
}

// drawSplitLine draws the vertical line at the bisection split point.
func (w *OverviewWidget) drawSplitLine(screen tcell.Screen, x, y, startModIndex, endModIndex int) {
	color := w.determineColor(w.allMods[startModIndex:endModIndex])
	style := tcell.StyleDefault.Foreground(tcell.ColorBlack).Background(color)
	screen.SetContent(x, y, tview.BoxDrawingsDoubleVertical, nil, style)
}

// drawContentCell draws a single cell of the overview bar, determining its foreground and background colors.
func (w *OverviewWidget) drawContentCell(screen tcell.Screen, x, y, startModIndex, endModIndex int) {
	// If start and end are the same, this cell represents no mods, so draw nothing.
	if startModIndex >= endModIndex {
		return
	}

	midModIndex := startModIndex + (endModIndex-startModIndex)/2

	// Determine the color for each half of this specific cell.
	fgColor := w.determineColor(w.allMods[startModIndex : midModIndex+1])
	bgColor := w.determineColor(w.allMods[midModIndex+1 : endModIndex])

	if startModIndex == midModIndex+1 {
		fgColor = bgColor
	}
	if midModIndex+1 == endModIndex {
		bgColor = fgColor
	}

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
