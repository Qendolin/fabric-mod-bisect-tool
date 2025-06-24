package ui

import (
	"context"

	"github.com/Qendolin/fabric-mod-bisect-tool/app/core/mods" // Added import

	"github.com/rivo/tview"
)

// AppInterface defines the necessary methods from the main App struct that UI pages need to interact with.
type AppInterface interface {
	QueueUpdateDraw(f func()) *tview.Application
	SetPageStatus(message string)
	SetFooter(prompts map[string]string)
	ShowPage(pageID string, page Page, resize bool)
	PushPage(pageID string, page Page)
	PopPage()
	ShowErrorDialog(title, message string, onDismiss func())
	ShowQuitDialog()
	GetApplicationContext() context.Context
	GetLogTextView() *tview.TextView
	GetModLoader() mods.ModLoaderService
	OnModsLoaded(allMods map[string]*mods.Mod, potentialProviders mods.PotentialProvidersMap, sortedModIDs []string)
	StartModLoad(path string)
	GetPageManager() PageManager
}

// Page is an interface that all UI pages must implement.
type Page interface {
	Primitive() tview.Primitive
	GetActionPrompts() map[string]string
}

// PageManager defines an interface for a factory that creates UI pages.
type PageManager interface {
	NewLogPage(app AppInterface) Page
	NewSetupPage(app AppInterface) Page
	NewLoadingPage(app AppInterface, modsPath string) Page
}

// uiPageManagerImpl is the concrete implementation of the PageManager interface.
type uiPageManagerImpl struct{}

// NewUIPageManager creates a new instance of the concrete page manager.
func NewUIPageManager() PageManager {
	return &uiPageManagerImpl{}
}

func (pm *uiPageManagerImpl) NewLogPage(app AppInterface) Page {
	return NewLogPage(app)
}

func (pm *uiPageManagerImpl) NewSetupPage(app AppInterface) Page {
	return NewSetupPage(app)
}

func (pm *uiPageManagerImpl) NewLoadingPage(app AppInterface, modsPath string) Page {
	return NewLoadingPage(app, modsPath)
}
