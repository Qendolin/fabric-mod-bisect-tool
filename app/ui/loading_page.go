package ui

import (
	"fmt"
	"os"
	"strings"

	"github.com/rivo/tview"
)

// PageLoadingID is the unique identifier for the LoadingPage.
const PageLoadingID = "loading_page"

// LoadingPage displays progress while mods are being loaded.
type LoadingPage struct {
	*tview.Flex
	app          AppInterface
	progressBar  *tview.TextView
	progressText *tview.TextView
	modsPath     string
	progress     float32
	statusText   *tview.TextView
}

// NewLoadingPage creates a new LoadingPage instance.
func NewLoadingPage(app AppInterface) *LoadingPage {
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

	lp.AddItem(NewTitleFrame(centeredFlex, "Loading Mods"), 0, 1, true)
	lp.statusText.SetText("Loading mod files from disk...")
	return lp
}

// StartLoading begins the asynchronous mod loading process.
// This should be called after the page is shown.
func (lp *LoadingPage) StartLoading(modsPath string) {
	lp.modsPath = modsPath
	go func() {
		// First, count the files for an accurate progress bar
		files, err := os.ReadDir(lp.modsPath)
		if err != nil {
			go lp.app.QueueUpdateDraw(func() {
				lp.app.Dialogs().ShowErrorDialog("Loading Error", fmt.Sprintf("Failed to read mods directory: %v", err), func() {
					lp.app.Navigation().SwitchTo(PageSetupID)
				})
			})
			return
		}

		totalFiles := 0
		for _, file := range files {
			if !file.IsDir() && (strings.HasSuffix(strings.ToLower(file.Name()), ".jar") || strings.HasSuffix(strings.ToLower(file.Name()), ".jar.disabled")) {
				totalFiles++
			}
		}

		processedFiles := 0
		loader := lp.app.GetModLoader()
		// TODO: Dependency overrides
		allMods, potentialProviders, sortedModIDs, err := loader.LoadMods(lp.modsPath, nil, func(fileName string) {
			processedFiles++
			lp.UpdateProgress(totalFiles, processedFiles, fileName)
		})

		// After loading, update UI on the main thread
		go lp.app.QueueUpdateDraw(func() {
			if err != nil {
				lp.app.Dialogs().ShowErrorDialog("Loading Error", fmt.Sprintf("Failed to load mods: %v", err), func() {
					lp.app.Navigation().SwitchTo(PageSetupID)
				})
				return
			}
			if len(allMods) == 0 {
				lp.app.Dialogs().ShowErrorDialog("Information", "No mods were found in the specified directory.", func() {
					lp.app.Navigation().SwitchTo(PageSetupID)
				})
				return
			}
			lp.app.OnModsLoaded(lp.modsPath, allMods, potentialProviders, sortedModIDs)
		})
	}()
}

// GetActionPrompts returns the key actions for the loading page.
func (lp *LoadingPage) GetActionPrompts() map[string]string {
	return map[string]string{}
}

// GetStatusPrimitive returns the tview.Primitive that displays the page's status
func (lp *LoadingPage) GetStatusPrimitive() *tview.TextView {
	return lp.statusText
}

// UpdateProgress updates the progress bar and text.
func (lp *LoadingPage) UpdateProgress(totalFiles, processedFiles int, currentFile string) {
	var progress float32
	if totalFiles > 0 {
		progress = float32(processedFiles) / float32(totalFiles)
	}

	if progress <= lp.progress {
		return
	}
	lp.progress = progress

	_, _, barWidth, _ := lp.progressBar.Box.GetInnerRect()
	filledWidth := int(progress * float32(barWidth))
	bar := fmt.Sprintf("[::b][white:blue]%s[-:-]%s[-:-:-]", strings.Repeat(" ", filledWidth), strings.Repeat(" ", barWidth-filledWidth))
	progressText := fmt.Sprintf("%d%%", int(progress*100))

	lp.progressBar.SetText(fmt.Sprintf("%s\n%s", bar, progressText))

	lp.progressText.SetText(fmt.Sprintf("Processing: [yellow]%s[-:-:-]", currentFile))
}
