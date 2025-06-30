package pages

import (
	"fmt"
	"os"
	"strings"

	"github.com/Qendolin/fabric-mod-bisect-tool/app/ui"
	"github.com/Qendolin/fabric-mod-bisect-tool/app/ui/widgets"
	"github.com/rivo/tview"
)

// PageLoadingID is the unique identifier for the LoadingPage.
const PageLoadingID = "loading_page"

// LoadingPage displays progress while mods are being loaded.
type LoadingPage struct {
	*tview.Flex
	app          ui.AppInterface
	progressBar  *tview.TextView
	progressText *tview.TextView
	statusText   *tview.TextView

	totalFiles     int
	processedFiles int
}

// NewLoadingPage creates a new LoadingPage instance.
func NewLoadingPage(app ui.AppInterface) *LoadingPage {
	lp := &LoadingPage{
		Flex:         tview.NewFlex().SetDirection(tview.FlexRow),
		app:          app,
		progressBar:  tview.NewTextView().SetDynamicColors(true),
		progressText: tview.NewTextView().SetDynamicColors(true),
		statusText:   tview.NewTextView().SetDynamicColors(true),
	}

	lp.progressBar.SetTextAlign(tview.AlignCenter).SetBorder(true)

	centeredFlex := tview.NewFlex().
		AddItem(tview.NewBox(), 0, 1, false).
		AddItem(
			tview.NewFlex().SetDirection(tview.FlexRow).
				AddItem(tview.NewBox(), 0, 1, false).
				AddItem(lp.progressBar, 3, 0, false).
				AddItem(lp.progressText, 1, 0, false).
				AddItem(tview.NewBox(), 0, 1, false),
			80, 0, false,
		).
		AddItem(tview.NewBox(), 0, 1, false)

	lp.AddItem(widgets.NewTitleFrame(centeredFlex, "Loading Mods"), 0, 1, true)
	lp.statusText.SetText("Loading mod files from disk...")
	return lp
}

// StartLoading prepares the page for the loading process initiated by the App.
func (lp *LoadingPage) StartLoading(modsPath string) {
	lp.processedFiles = 0
	lp.totalFiles = 0

	files, err := os.ReadDir(modsPath)
	if err != nil {
		// The error will be handled by the App layer which initiated the loading.
		// We just show a generic "starting" message here.
		lp.UpdateProgress("Starting...")
		return
	}

	// Pre-calculate total files for an accurate progress bar.
	for _, file := range files {
		if !file.IsDir() && (strings.HasSuffix(strings.ToLower(file.Name()), ".jar") || strings.HasSuffix(strings.ToLower(file.Name()), ".jar.disabled")) {
			lp.totalFiles++
		}
	}
	lp.UpdateProgress("Starting...") // Initial update
}

// UpdateProgress updates the progress bar and text.
func (lp *LoadingPage) UpdateProgress(currentFile string) {
	lp.processedFiles++

	var progress float32
	if lp.totalFiles > 0 {
		// Cap progress at total to prevent overflow if file count is off
		if lp.processedFiles > lp.totalFiles {
			lp.processedFiles = lp.totalFiles
		}
		progress = float32(lp.processedFiles) / float32(lp.totalFiles)
	}

	_, _, barWidth, _ := lp.progressBar.GetInnerRect()
	if barWidth <= 0 {
		return // Not ready to draw yet
	}

	filledWidth := int(progress * float32(barWidth))
	bar := fmt.Sprintf("[::b][white:blue]%s[-:-]%s[-:-:-]", strings.Repeat(" ", filledWidth), strings.Repeat(" ", barWidth-filledWidth))
	progressText := fmt.Sprintf("%d%%", int(progress*100))

	lp.progressBar.SetText(fmt.Sprintf("%s\n%s", bar, progressText))
	lp.progressText.SetText(fmt.Sprintf("Processing: [yellow]%s[-:-:-]", currentFile))
}

// GetActionPrompts returns the key actions for the loading page.
func (lp *LoadingPage) GetActionPrompts() []ui.ActionPrompt {
	return []ui.ActionPrompt{}
}

// GetStatusPrimitive returns the tview.Primitive that displays the page's status
func (lp *LoadingPage) GetStatusPrimitive() *tview.TextView {
	return lp.statusText
}
