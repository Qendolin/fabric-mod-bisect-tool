package widgets

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
		Box:   tview.NewBox().SetBackgroundColor(tview.Styles.PrimitiveBackgroundColor),
		color: color,
	}
}

// Draw draws the separator.
func (s *HorizontalSeparator) Draw(screen tcell.Screen) {
	s.Box.Draw(screen)
	x, y, width, _ := s.GetInnerRect()
	style := tcell.StyleDefault.Background(s.GetBackgroundColor()).Foreground(s.color)
	for i := 0; i < width; i++ {
		screen.SetContent(x+i, y, tview.BoxDrawingsLightHorizontal, nil, style)
	}
}

// VerticalSeparator is a simple primitive that draws a vertical line.
type VerticalSeparator struct {
	*tview.Box
	color tcell.Color
}

// NewVerticalSeparator creates a new separator with a given color.
func NewVerticalSeparator(color tcell.Color) *VerticalSeparator {
	return &VerticalSeparator{
		Box:   tview.NewBox().SetBackgroundColor(tview.Styles.PrimitiveBackgroundColor),
		color: color,
	}
}

// Draw draws the separator.
func (s *VerticalSeparator) Draw(screen tcell.Screen) {
	s.Box.Draw(screen)
	x, y, _, height := s.GetInnerRect()
	style := tcell.StyleDefault.Background(s.GetBackgroundColor()).Foreground(s.color)
	for i := 0; i < height; i++ {
		screen.SetContent(x, y+i, tview.BoxDrawingsLightVertical, nil, style)
	}
}
