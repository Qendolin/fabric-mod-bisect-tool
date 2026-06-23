package screens

import (
	"errors"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	exlayout "github.com/Qendolin/fabric-mod-bisect-tool/pkg/gui/layout"
	"github.com/Qendolin/fabric-mod-bisect-tool/pkg/gui/probe"

	"github.com/ncruces/zenity"
)

type SetupScreen struct {
	app     App
	content fyne.CanvasObject

	// Title
	titleLabel *widget.Label

	// Instruction
	instructionLabel *widget.Label

	// Path input
	pathEntry *widget.Entry
	browseBtn *widget.Button

	// Start button
	startBtn *widget.Button
}

func NewSetupScreen(app App) *SetupScreen {
	s := &SetupScreen{app: app}
	s.build()
	return s
}

type dialogResizeLayout struct {
	dialog dialog.Dialog
}

func (d *dialogResizeLayout) Layout(objects []fyne.CanvasObject, size fyne.Size) {
	if d.dialog != nil {
		d.dialog.Resize(size)
	}
}

func (d *dialogResizeLayout) MinSize(objects []fyne.CanvasObject) fyne.Size {
	return fyne.NewSize(0, 0)
}

func (s *SetupScreen) build() {
	// --- Title ---
	s.titleLabel = widget.NewLabelWithStyle("Fabric Mod Bisect Tool", fyne.TextAlignCenter, fyne.TextStyle{Bold: true})

	// --- Instruction ---
	s.instructionLabel = widget.NewLabel("Select your mods folder to begin.")
	s.instructionLabel.Alignment = fyne.TextAlignCenter

	// --- Path input ---
	s.pathEntry = widget.NewEntry()
	s.pathEntry.SetPlaceHolder("Enter or browse for mods folder path...")

	s.browseBtn = widget.NewButtonWithIcon("", theme.FolderIcon(), func() {
		path, err := zenity.SelectFile(zenity.Title("Select Mods Folder"), zenity.Directory(), zenity.Modal(), zenity.Filename(s.pathEntry.Text))
		if err == nil && path != "" {
			s.pathEntry.SetText(path)
		}
	})

	// Path Input Row: Entry grows, browse button uses MinSize
	pathFlex := exlayout.NewFlexLayout(true, 10)
	pathFlex.Set(s.pathEntry, 1, 0)
	pathFlex.Set(s.browseBtn, 0, 0)
	pathInputContainer := container.New(pathFlex, s.pathEntry, s.browseBtn)

	// --- Start button ---
	s.startBtn = widget.NewButton("Start Bisection", func() {
		path := s.pathEntry.Text
		if path == "" {
			dialog.ShowError(errors.New("please select a mods folder"), s.app.GetWindow())
			return
		}

		res := probe.ProbeModsDirectory(path)
		s.app.StartLoadingProcess(path, res.QuiltSupport, res.NeoForgeSupport)
	})
	s.startBtn.Importance = widget.HighImportance

	// Right-align the button using a horizontal FlexLayout.
	// The spacer gets Grow: 1 to consume all left-side space, pushing the button to the right.
	btnFlex := exlayout.NewFlexLayout(true, 0)
	spacer := layout.NewSpacer()
	btnFlex.Set(spacer, 1, 0)
	btnFlex.Set(s.startBtn, 0, 0)
	// btnContainer := container.New(btnFlex, spacer, s.startBtn)

	// Add the startBtn directly to the vertical flex instead of btnContainer
	formFlex := exlayout.NewFlexLayout(false, 20)
	formFlex.Set(s.titleLabel, 0, 0)
	formFlex.Set(s.instructionLabel, 0, 0)
	formFlex.Set(pathInputContainer, 0, 0)
	formFlex.Set(s.startBtn, 0, 0) // Stretches button horizontally to 450px

	centerContent := container.New(formFlex,
		s.titleLabel,
		s.instructionLabel,
		pathInputContainer,
		s.startBtn,
	)

	// Main Outer Layout (Centers the form with a fixed width of 450px)
	rows := []exlayout.GridTrack{
		{Fraction: 1},   // Top spacer
		{Fixed: 0},      // Form Row
		{Fraction: 1.5}, // Bottom spacer
	}
	cols := []exlayout.GridTrack{
		{Fraction: 1}, // Left spacer
		{Fixed: 450},  // Center Form width
		{Fraction: 1}, // Right spacer
	}

	grid := exlayout.NewGridLayout(rows, cols, 0)
	grid.Set(centerContent, 1, 1)

	s.content = container.New(grid, centerContent)
}

func (s *SetupScreen) GetContent() fyne.CanvasObject {
	return s.content
}
