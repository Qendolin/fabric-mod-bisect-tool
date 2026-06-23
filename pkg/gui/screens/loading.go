package screens

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"

	exlayout "github.com/Qendolin/fabric-mod-bisect-tool/pkg/gui/layout"
)

type LoadingScreen struct {
	app         App
	content     fyne.CanvasObject
	loadingLbl  *widget.Label
	progressBar *widget.ProgressBar
}

func NewLoadingScreen(app App) *LoadingScreen {
	s := &LoadingScreen{app: app}
	s.build()
	return s
}

func (s *LoadingScreen) build() {
	// --- Centered Title ---
	title := widget.NewLabelWithStyle(
		"Loading Mods...",
		fyne.TextAlignCenter,
		fyne.TextStyle{Bold: true},
	)

	// --- Subtitle Message ---
	msg := widget.NewLabelWithStyle(
		"This shouldn't take much longer than it takes you to read this.",
		fyne.TextAlignCenter,
		fyne.TextStyle{},
	)
	msg.Wrapping = fyne.TextWrapWord

	// --- Progress Bar ---
	s.progressBar = widget.NewProgressBar()

	// --- Left-aligned Filename Label ---
	s.loadingLbl = widget.NewLabel("")
	s.loadingLbl.Alignment = fyne.TextAlignLeading
	s.loadingLbl.Wrapping = fyne.TextWrapWord // Prevent long filename overlaps

	// Group the central loading widgets vertically with modern spacing
	formFlex := exlayout.NewFlexLayout(false, 15) // Vertical, 15px gaps
	formFlex.Set(title, 0, 0)
	formFlex.Set(msg, 0, 0)
	formFlex.Set(s.progressBar, 0, 0)
	formFlex.Set(s.loadingLbl, 0, 0)

	centerContent := container.New(formFlex, title, msg, s.progressBar, s.loadingLbl)

	// Outer grid to vertically center the form and constrain its width [11]
	rows := []exlayout.GridTrack{
		{Fraction: 1},   // Top spacer
		{Fixed: 0},      // Form content Row (sizes dynamically to centerContent MinSize)
		{Fraction: 1.2}, // Bottom spacer (visually balanced slightly upward)
	}
	cols := []exlayout.GridTrack{
		{Fraction: 1}, // Left spacer
		{Fixed: 450},  // Form column width (locks in with Setup & Main Screen)
		{Fraction: 1}, // Right spacer
	}

	grid := exlayout.NewGridLayout(rows, cols, 0)
	grid.Set(centerContent, 1, 1)

	s.content = container.New(grid, centerContent)
}

func (s *LoadingScreen) UpdateProgress(fileName string, i, count int) {
	fyne.Do(func() {
		s.loadingLbl.SetText(fileName)
		s.progressBar.Max = float64(count)
		s.progressBar.SetValue(float64(i))
	})
}

func (s *LoadingScreen) GetContent() fyne.CanvasObject {
	return s.content
}
