package ui

import (
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

// OverviewWidget is a single-row visual representation of mod sets.
type OverviewWidget struct {
	*tview.Box
	allMods       []string
	problemMods   map[string]struct{}
	goodMods      map[string]struct{}
	c1Mods        map[string]struct{}
	c2Mods        map[string]struct{}
	effectiveMods map[string]struct{}
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
func (w *OverviewWidget) UpdateState(problemMods, goodMods, c1Mods, c2Mods, effectiveMods map[string]struct{}) {
	w.problemMods = problemMods
	w.goodMods = goodMods
	w.c1Mods = c1Mods
	w.c2Mods = c2Mods
	w.effectiveMods = effectiveMods
}

// Draw implements tview.Primitive.
func (w *OverviewWidget) Draw(screen tcell.Screen) {
	w.Box.Draw(screen)
	x, y, width, _ := w.GetInnerRect()
	if width <= 0 || len(w.allMods) == 0 {
		return
	}

	numTotalMods := len(w.allMods)
	numPartitions := width * 2
	modIdx := 0

	for i := 0; i < width; i++ {
		// Calculate mods for the left half of the cell
		start1 := modIdx
		numMods1 := numTotalMods*(i*2+1)/numPartitions - numTotalMods*(i*2)/numPartitions
		end1 := start1 + numMods1
		modIdx = end1

		// Calculate mods for the right half of the cell
		start2 := modIdx
		numMods2 := numTotalMods*(i*2+2)/numPartitions - numTotalMods*(i*2+1)/numPartitions
		end2 := start2 + numMods2
		modIdx = end2

		fgColor := w.determineColor(w.allMods[start1:end1])
		bgColor := w.determineColor(w.allMods[start2:end2])

		style := tcell.StyleDefault.Foreground(fgColor).Background(bgColor)
		screen.SetContent(x+i, y, '▌', nil, style)
	}
}

// determineColor finds the dominant color for a slice of mod IDs.
func (w *OverviewWidget) determineColor(modIDs []string) tcell.Color {
	// Priority: 5: Conflict, 4: Good, 3: C1, 2: C2, 1: Effective, 0: Rest
	highestPriority := 0

	for _, id := range modIDs {
		// Highest priority first, with early exit.
		if _, ok := w.problemMods[id]; ok {
			highestPriority = 5
			break
		}

		if _, ok := w.goodMods[id]; ok {
			if highestPriority < 4 {
				highestPriority = 4
			}
		} else if _, ok := w.c1Mods[id]; ok {
			if highestPriority < 3 {
				highestPriority = 3
			}
		} else if _, ok := w.c2Mods[id]; ok {
			if highestPriority < 2 {
				highestPriority = 2
			}
		} else if _, ok := w.effectiveMods[id]; ok {
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
		return tcell.ColorLightGreen
	case 2:
		return tcell.ColorWhite
	case 1:
		return tcell.ColorSteelBlue
	default:
		return tcell.ColorGray
	}
}
