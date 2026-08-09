package widgets

import (
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

// drawScrollIndicators draws ▲/▼ arrows in the top-right and bottom-right
// corners of a scrollable area to signal that more content exists above or below
// the visible viewport.
func drawScrollIndicators(screen tcell.Screen, x, y, width, height, offsetRow, totalLines int) {
	style := tcell.StyleDefault.Foreground(tcell.ColorWhite)
	if offsetRow > 0 {
		screen.SetContent(x+width-1, y, '▲', nil, style)
	}
	if offsetRow+height < totalLines {
		screen.SetContent(x+width-1, y+height-1, '▼', nil, style)
	}
}

// ScrollTextView is a tview.TextView that draws ▲/▼ scroll indicators in its
// top-right and bottom-right corners whenever its content overflows the visible
// area.
type ScrollTextView struct {
	*tview.TextView
}

// NewScrollTextView creates a new ScrollTextView.
func NewScrollTextView() *ScrollTextView {
	return &ScrollTextView{TextView: tview.NewTextView()}
}

// Draw implements tview.Primitive.
func (s *ScrollTextView) Draw(screen tcell.Screen) {
	s.TextView.Draw(screen)
	x, y, width, height := s.GetInnerRect()
	offsetRow, _ := s.GetScrollOffset()
	totalLines := s.GetWrappedLineCount()
	drawScrollIndicators(screen, x, y, width, height, offsetRow, totalLines)
}
