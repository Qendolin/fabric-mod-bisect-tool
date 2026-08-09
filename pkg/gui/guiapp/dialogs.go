package guiapp

import (
	"fmt"

	"github.com/Qendolin/fabric-mod-bisect-tool/pkg/core/sets"
	"github.com/ncruces/zenity"
)

func (a *App) ShowErrorDialog(title, message string, err error) {
	fullMsg := message
	if err != nil {
		fullMsg += "\n\nDetails: " + err.Error()
	}
	opts := append(a.dialogOptions(), zenity.Title(title))
	_ = zenity.Error(fullMsg, opts...)
}

func (a *App) ShowInfoDialog(title, message, details string) {
	fullMsg := message
	if details != "" {
		fullMsg += "\n\n" + details
	}
	opts := append(a.dialogOptions(), zenity.Title(title))
	_ = zenity.Info(fullMsg, opts...)
}

func (a *App) ShowQuestionDialog(title, message, details string) (ok bool) {
	fullMsg := message
	if details != "" {
		fullMsg += "\n\n" + details
	}
	opts := append(a.dialogOptions(), zenity.Title(title))
	err := zenity.Question(fullMsg, opts...)
	return err == nil
}

// Dialogs (Blocking)
func (a *App) ShowDialogErrorModLoadingGeneric(path string, err error) {
	a.ShowErrorDialog("Mod Loading Error", fmt.Sprintf("Failed to load mods from '%s", path), err)
	a.SetActiveScreen(a.setupScreen)
}

func (a *App) ShowDialogErrorModLoadingNoMods(path string) {
	a.ShowErrorDialog("Mod Loading Error", fmt.Sprintf("No mods were found at '%s'.\nPlease ensure that you've entered the path correctly.", path), nil)
	a.SetActiveScreen(a.setupScreen)
}

func (a *App) ShowDialogErrorBisectionInitialization(err error) {
	a.ShowErrorDialog("Initialization Error", "Failed to initialize the bisection!", err)
	a.SetActiveScreen(a.setupScreen)
}

func (a *App) ShowDialogErrorBisectionCannotContinue(err error) {
	a.ShowErrorDialog("Bisection Error", "Cannot continue the search!", err)
}

func (a *App) ShowDialogErrorBisectionPrepare(err error) {
	a.ShowErrorDialog("Bisection Error", "An error occurred and the next step could not be prepared.\nIf another program, like Minecraft, is currently accessing your mods, please close it.\n\nPlease check the application log for details.", err)
}

func (a *App) ShowDialogInfoBisectionModsMissingExpected(missingMods sets.Set) {
	a.ShowInfoDialog(
		"Known Problematic Mod(s) Removed",
		"The following mod(s), which were part of a known conflict set, have been detected as missing. This is expected. The search will now proceed with the updated mod list.",
		sets.FormatSet(missingMods).String(),
	)
}

func (a *App) ShowDialogInfoBisectionUnresolvableModsDisabled(disabledMods sets.Set) {
	a.ShowInfoDialog(
		"Disabled Mods",
		"The following mods were automatically disabled due to unmet dependencies:",
		sets.FormatSet(disabledMods).String(),
	)
}

func (a *App) ShowDialogQuestionBisectionContinueWithMissingMods(missingMods sets.Set) bool {
	return a.ShowQuestionDialog(
		"Missing Mod Files Detected",
		"The following mod files were unexpectedly missing. Do you want to continue the search without them?",
		sets.FormatSet(missingMods).String(),
	)
}
