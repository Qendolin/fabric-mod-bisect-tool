package screens

import (
	"fmt"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"

	"github.com/Qendolin/fabric-mod-bisect-tool/pkg/core/sets"
)

type ResultScreen struct {
	app     App
	content fyne.CanvasObject

	lblResult *widget.Label
	lblSets   *widget.Label
}

func NewResultScreen(app App) *ResultScreen {
	s := &ResultScreen{app: app}
	s.build()
	return s
}

func (s *ResultScreen) build() {
	s.lblResult = widget.NewLabelWithStyle("Bisection Complete", fyne.TextAlignCenter, fyne.TextStyle{Bold: true})
	s.lblSets = widget.NewLabelWithStyle("", fyne.TextAlignLeading, fyne.TextStyle{})
	s.lblSets.Wrapping = fyne.TextWrapWord

	btnRestart := widget.NewButton("Restart Bisection", func() {
		s.app.ResetSearch()
		s.app.SwitchToMainPage()
	})

	btnQuit := widget.NewButton("Quit", func() {
		s.app.ShowQuitDialog()
	})

	scrollContainer := container.NewVScroll(s.lblSets)
	// We might need to ensure the scroll container expands, but using a Border layout is better for that
	
	content := container.NewBorder(
		container.NewVBox(s.lblResult, widget.NewSeparator()),
		container.NewVBox(widget.NewSeparator(), container.NewHBox(btnRestart, btnQuit)),
		nil,
		nil,
		scrollContainer,
	)

	s.content = container.NewPadded(content)
}

func (s *ResultScreen) UpdateState() {
	vm := s.app.GetViewModel()

	var problematicMods []string
	if len(vm.CurrentConflictSet) > 0 {
		problematicMods = append(problematicMods, "Current Conflict:")
		problematicMods = append(problematicMods, sets.MakeSlice(vm.CurrentConflictSet)...)
	}

	for i, conflictSet := range vm.AllConflictSets {
		problematicMods = append(problematicMods, fmt.Sprintf("Conflict %d:", i+1))
		problematicMods = append(problematicMods, sets.MakeSlice(conflictSet)...)
	}

	clearedList := sets.MakeSlice(vm.ClearedSet)

	text := ""
	if len(problematicMods) > 0 {
		text += "Problematic Mods:\n" + strings.Join(problematicMods, "\n") + "\n\n"
	}
	text += "Cleared Mods:\n" + strings.Join(clearedList, "\n")

	s.lblSets.SetText(text)
}

func (s *ResultScreen) GetContent() fyne.CanvasObject {
	return s.content
}
