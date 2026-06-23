package screens

import (
	"fmt"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"github.com/Qendolin/fabric-mod-bisect-tool/pkg/core/sets"
	exlayout "github.com/Qendolin/fabric-mod-bisect-tool/pkg/gui/layout"
)

type MainScreen struct {
	app     App
	content fyne.CanvasObject

	// Normal state widgets
	statusLbl  *widget.Label
	detailsLbl *widget.Label
	btnStep    *widget.Button
	btnUndo    *widget.Button
	normalView *fyne.Container

	// Normal view dashboard components
	progressBar     *widget.ProgressBar
	candidateHeader *widget.Label
	candidateList   *widget.List
	candidatesData  []string

	// Test state components (Replaced custom dialog modal)
	testView      *fyne.Container
	testHeader    *widget.Label
	testDesc      *widget.RichText // Upgraded to RichText
	btnSuccess    *widget.Button
	btnFailure    *widget.Button
	btnCancel     *widget.Button
	testModHeader *widget.Label
	testModList   *widget.List
	testModsData  []string
}

func NewMainScreen(app App) *MainScreen {
	s := &MainScreen{app: app}
	s.build()
	return s
}

func (s *MainScreen) build() {
	// ==========================================
	// 1. Normal View Setup (Idle Between Tests)
	// ==========================================
	s.statusLbl = widget.NewLabelWithStyle("", fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
	s.detailsLbl = widget.NewLabelWithStyle("", fyne.TextAlignLeading, fyne.TextStyle{})
	s.detailsLbl.Wrapping = fyne.TextWrapWord

	s.progressBar = widget.NewProgressBar()
	s.progressBar.TextFormatter = func() string {
		return fmt.Sprintf("%d / %d", int(s.progressBar.Value), int(s.progressBar.Max))
	}

	s.btnStep = widget.NewButton("▶  Next Step", func() {
		s.app.Step()
	})
	s.btnStep.Importance = widget.HighImportance

	s.btnUndo = widget.NewButton("↩  Undo", func() {
		s.app.Undo()
		s.UpdateState()
	})

	btnFlex := exlayout.NewFlexLayout(true, 10)
	btnFlex.Set(s.btnUndo, 1, 0)
	btnFlex.Set(s.btnStep, 1.5, 0)
	btnContainer := container.New(btnFlex, s.btnUndo, s.btnStep)

	leftFlex := exlayout.NewFlexLayout(false, 15)
	leftFlex.Set(s.statusLbl, 0, 0)
	leftFlex.Set(s.detailsLbl, 1, 0) // Let details expand to push content
	leftFlex.Set(s.progressBar, 0, 0)
	leftFlex.Set(btnContainer, 0, 0)

	leftContainer := container.New(leftFlex, s.statusLbl, s.detailsLbl, s.progressBar, btnContainer)

	s.candidateHeader = widget.NewLabelWithStyle("Remaining Candidates", fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
	s.candidatesData = []string{}
	s.candidateList = widget.NewList(
		func() int { return len(s.candidatesData) },
		func() fyne.CanvasObject {
			item := widget.NewLabel("")
			item.Selectable = true
			item.Truncation = fyne.TextTruncateEllipsis
			return item
		},
		func(id widget.ListItemID, item fyne.CanvasObject) {
			item.(*widget.Label).SetText(s.candidatesData[id])
		},
	)

	rightFlex := exlayout.NewFlexLayout(false, 10)
	rightFlex.Set(s.candidateHeader, 0, 0)
	rightFlex.Set(s.candidateList, 1, 0)

	rightContainer := container.New(rightFlex, s.candidateHeader, s.candidateList)

	// Consistent 50/50 screen split
	grid := exlayout.NewGridLayout(
		[]exlayout.GridTrack{{Fraction: 1}},
		[]exlayout.GridTrack{{Fraction: 1}, {Fixed: 320}}, // Left side fills screen, List is fixed to 320px
		25,
	)
	grid.Set(leftContainer, 0, 0)
	grid.Set(rightContainer, 0, 1)

	s.normalView = container.New(grid, leftContainer, rightContainer)

	// ==========================================
	// 2. Active Test View Setup (Inline Screen)
	// ==========================================
	s.testHeader = widget.NewLabelWithStyle("Test Protocol", fyne.TextAlignLeading, fyne.TextStyle{Bold: true})

	// Professional RichText instruction renderer [10]
	s.testDesc = widget.NewRichText()
	s.testDesc.Wrapping = fyne.TextWrapWord

	// Neutral styled buttons with clear colored icons [2, 10]
	s.btnSuccess = widget.NewButtonWithIcon("Success", theme.ConfirmIcon(), nil)
	s.btnSuccess.Importance = widget.SuccessImportance
	s.btnFailure = widget.NewButtonWithIcon("Failure", theme.CancelIcon(), nil)
	s.btnFailure.Importance = widget.DangerImportance
	s.btnCancel = widget.NewButton("Cancel Test", nil)

	// Horizontal action row for Success/Failure
	actionFlex := exlayout.NewFlexLayout(true, 10)
	actionFlex.Set(s.btnSuccess, 1, 0)
	actionFlex.Set(s.btnFailure, 1, 0)
	actionRow := container.New(actionFlex, s.btnSuccess, s.btnFailure)

	// Bottom full-width Cancel row
	cancelFlex := exlayout.NewFlexLayout(true, 0)
	cancelFlex.Set(s.btnCancel, 1, 0)
	cancelRow := container.New(cancelFlex, s.btnCancel)

	testLeftFlex := exlayout.NewFlexLayout(false, 15)
	testLeftFlex.Set(s.testHeader, 0, 0)
	testLeftFlex.Set(widget.NewSeparator(), 0, 0)
	testLeftFlex.Set(s.testDesc, 1, 0) // Grow: 1 expands text layout and pushes buttons downwards [10]
	testLeftFlex.Set(actionRow, 0, 0)
	testLeftFlex.Set(cancelRow, 0, 0)

	testLeftContainer := container.New(testLeftFlex,
		s.testHeader,
		widget.NewSeparator(),
		s.testDesc,
		actionRow,
		cancelRow,
	)

	s.testModHeader = widget.NewLabelWithStyle("Active Mod Set", fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
	s.testModsData = []string{}
	s.testModList = widget.NewList(
		func() int { return len(s.testModsData) },
		func() fyne.CanvasObject { return widget.NewLabel("") },
		func(id widget.ListItemID, item fyne.CanvasObject) {
			item.(*widget.Label).SetText(s.testModsData[id])
		},
	)

	testRightFlex := exlayout.NewFlexLayout(false, 10)
	testRightFlex.Set(s.testModHeader, 0, 0)
	testRightFlex.Set(s.testModList, 1, 0)

	testRightContainer := container.New(testRightFlex, s.testModHeader, s.testModList)

	testGrid := exlayout.NewGridLayout(
		[]exlayout.GridTrack{{Fraction: 1}},
		[]exlayout.GridTrack{{Fraction: 1}, {Fixed: 320}}, // Left side fills screen, List is fixed to 320px
		25,
	)
	testGrid.Set(testLeftContainer, 0, 0)
	testGrid.Set(testRightContainer, 0, 1)

	s.testView = container.New(testGrid, testLeftContainer, testRightContainer)
	s.testView.Hide()

	s.content = container.NewStack(s.normalView, s.testView)
}

func (s *MainScreen) UpdateState() {
	vm := s.app.GetViewModel()
	if !vm.IsReady {
		s.statusLbl.SetText("Initializing...")
		s.detailsLbl.SetText("")
		s.btnStep.Disable()
		s.btnUndo.Disable()
		s.progressBar.Max = 1.0
		s.progressBar.SetValue(0)
		s.candidatesData = []string{}
		s.candidateList.Refresh()
		s.candidateHeader.SetText("Remaining Candidates (0)")
		return
	}

	if vm.CanUndo {
		s.btnUndo.Enable()
	} else {
		s.btnUndo.Disable()
	}

	if vm.IsVerificationStep {
		conflictsSlice := sets.MakeSlice(vm.CurrentConflictSet)
		s.candidatesData = conflictsSlice
		s.candidateList.Refresh()
		s.candidateHeader.SetText("Conflict Set")
		s.progressBar.Max = float64(vm.StepCount + 1)
		s.progressBar.SetValue(float64(vm.StepCount))

		s.btnStep.Enable()
		s.btnStep.SetText("▶  Verification Step")
		s.statusLbl.SetText("Verifying final set...")
		s.detailsLbl.SetText("The next test verifies that the found set of mods is the cause of the issue. If the issue DOES NOT persist, then a new round of tests is started to find the other problematic mods.")
		return
	}

	candidatesSlice := sets.MakeSlice(vm.CandidateSet)
	s.candidatesData = candidatesSlice
	s.candidateList.Refresh()
	s.candidateHeader.SetText(fmt.Sprintf("Remaining Candidates (%d)", len(candidatesSlice)))

	if vm.EstimatedMaxTests > 0 {
		progress := float64(vm.StepCount)
		s.progressBar.Max = float64(vm.EstimatedMaxTests)
		if progress > s.progressBar.Max {
			progress = s.progressBar.Max
		}
		s.progressBar.SetValue(progress)
	} else {
		s.progressBar.Max = 1.0
		s.progressBar.SetValue(0)
	}

	if vm.IsComplete {
		s.statusLbl.SetText("Bisection Complete")
		s.detailsLbl.SetText("The results screen will show what was found.")
		s.btnStep.Disable()
		s.app.SwitchToResultPage()
	} else {
		if vm.StepCount == 0 {
			s.btnStep.SetText("▶  Start Bisection")
			s.statusLbl.SetText("Ready to begin")

			s.detailsLbl.SetText("This tool uses binary search to isolate problematic mods.\n" +
				"Each test halves the candidate set, finding conflicts efficiently.\n" +
				"Be ready to test the game when prompted.")
		} else {
			s.btnStep.SetText("▶  Next Step")
			s.statusLbl.SetText(fmt.Sprintf("Round %d · Iteration %d", vm.Round, vm.Iteration))
			s.detailsLbl.SetText(fmt.Sprintf("Step %d of ~%d estimated tests.", vm.StepCount, vm.EstimatedMaxTests))
		}
		s.btnStep.Enable()
	}
}

func (s *MainScreen) ShowTestPrompt(isVerification bool, onSuccess, onFailure, onCancel func()) {
	var markdown string

	if isVerification {
		s.testHeader.SetText("Verification Test")
		markdown = "Start Minecraft with the current active mod set and verify whether your issue is **still present**.\n\n" +
			"*   **Failure** — The issue is **still there** (confirms the found conflict set is correct).\n" +
			"*   **Success** — The issue is **gone** (suggests the set is incomplete)."
	} else {
		s.testHeader.SetText("Bisection Test")
		markdown = "Start Minecraft with the current active mod set and verify whether your issue is **resolved**.\n\n" +
			"*   **Success** — The game runs fine and the issue is **gone**.\n" +
			"*   **Failure** — The issue is **still present** in the game."
	}

	// Update markdown instructions
	s.testDesc.ParseMarkdown(markdown)

	// Populate mod list data [11]
	vm := s.app.GetViewModel()
	if vm.CurrentTestPlan != nil {
		testSlice := sets.MakeSlice(vm.CurrentTestPlan.ModIDsToTest)
		s.testModsData = testSlice
		s.testModList.Refresh()
		s.testModHeader.SetText(fmt.Sprintf("Active Mod Set (%d mods loaded)", len(testSlice)))
	} else {
		s.testModsData = []string{}
		s.testModList.Refresh()
		s.testModHeader.SetText("Active Mod Set (0)")
	}

	s.btnSuccess.OnTapped = func() {
		s.HideTestPrompt()
		onSuccess()
	}
	s.btnFailure.OnTapped = func() {
		s.HideTestPrompt()
		onFailure()
	}
	s.btnCancel.OnTapped = func() {
		s.HideTestPrompt()
		onCancel()
	}

	s.normalView.Hide()
	s.testView.Show()
}

func (s *MainScreen) HideTestPrompt() {
	s.testView.Hide()
	s.normalView.Show()
	s.UpdateState()
}

func (s *MainScreen) GetContent() fyne.CanvasObject {
	return s.content
}
