package ui

import (
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

// HorizontalSeparator is a simple primitive that draws a horizontal line.
type HorizontalSeparator struct {
	*tview.Box
	color tcell.Color
}

// NewHorizontalSeparator creates a new separator with a given color.
func NewHorizontalSeparator(color tcell.Color) *HorizontalSeparator {
	return &HorizontalSeparator{
		Box:   tview.NewBox(),
		color: color,
	}
}

// Draw draws the separator.
func (s *HorizontalSeparator) Draw(screen tcell.Screen) {
	s.Box.Draw(screen)
	x, y, width, _ := s.GetInnerRect()
	style := tcell.StyleDefault.Foreground(s.color)
	for i := 0; i < width; i++ {
		screen.SetContent(x+i, y, tview.BoxDrawingsLightHorizontal, nil, style)
	}
}
