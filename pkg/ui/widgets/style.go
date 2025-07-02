package widgets

import (
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

var (
	DefaultButtonStlye         = tcell.StyleDefault.Foreground(tcell.ColorBlack).Background(tcell.ColorWhite)
	DefaultButtonActiveStyle   = tcell.StyleDefault.Foreground(tcell.ColorBlack).Background(tcell.ColorBlue).Underline(true)
	DefaultButtonDisabledStyle = tcell.StyleDefault.Foreground(tcell.ColorLightGray).Background(tcell.ColorDarkGray)
)

func DefaultStyleButton(button *tview.Button) {
	button.SetStyle(DefaultButtonStlye)
	button.SetActivatedStyle(DefaultButtonActiveStyle)
	button.SetDisabledStyle(DefaultButtonDisabledStyle)
}
