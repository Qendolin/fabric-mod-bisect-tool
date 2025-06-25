package ui

import (
	"fmt"
	"sort"
	"strings"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

// LayoutManager handles the overall visual structure of the application.
type LayoutManager struct {
	root       *tview.Flex
	header     *tview.Flex
	status     *tview.Flex
	statusText *tview.TextView
	footer     *tview.Flex
	pages      *tview.Pages

	errorCounters *tview.TextView
}

// NewLayoutManager creates and initializes the UI layout manager.
func NewLayoutManager() *LayoutManager {
	lm := &LayoutManager{
		pages:         tview.NewPages(),
		root:          tview.NewFlex().SetDirection(tview.FlexRow),
		header:        tview.NewFlex(),
		footer:        tview.NewFlex(),
		errorCounters: tview.NewTextView().SetDynamicColors(true).SetTextAlign(tview.AlignRight),
	}
	lm.setupLayout()
	return lm
}

// RootPrimitive returns the main primitive that should be set as the application's root.
func (lm *LayoutManager) RootPrimitive() tview.Primitive {
	return lm.root
}

// Pages returns the tview.Pages container for content.
func (lm *LayoutManager) Pages() *tview.Pages {
	return lm.pages
}

func (lm *LayoutManager) setupLayout() {
	lm.status = tview.NewFlex().SetDirection(tview.FlexRow)
	lm.SetHeader(nil)

	lm.header.AddItem(lm.status, 0, 1, false).
		AddItem(lm.errorCounters, 0, 1, false)
	lm.header.SetBorderPadding(0, 0, 1, 1)

	lm.root.SetBorder(true).
		SetTitle(" Fabric Mod Bisect Tool ").
		SetTitleAlign(tview.AlignLeft)

	lm.root.AddItem(lm.header, 1, 0, false).
		AddItem(lm.pages, 0, 1, true).
		AddItem(lm.footer, 1, 0, false)
}

// SetErrorCounters updates the error and warning counters.
func (lm *LayoutManager) SetErrorCounters(warnCount, errorCount int) {
	warnBgColor := tcell.ColorYellow
	warnFgColor := tcell.ColorBlack
	errorBgColor := tcell.ColorRed
	errorFgColor := tcell.ColorBlack
	if warnCount == 0 {
		warnBgColor = tcell.ColorBlack
		warnFgColor = tcell.ColorWhite
	}
	if errorCount == 0 {
		errorBgColor = tcell.ColorBlack
		errorFgColor = tcell.ColorWhite
	}
	lm.errorCounters.SetText(fmt.Sprintf("[yellow]Warnings: [%s:%s]%d[-:-:-] [red]Errors: [%s:%s]%d[-:-:-]",
		warnFgColor.Name(), warnBgColor.Name(), warnCount, errorFgColor.Name(), errorBgColor.Name(), errorCount))
}

// SetFooter updates the action hints flexbox.
func (lm *LayoutManager) SetFooter(prompts map[string]string) {
	lm.footer.Clear()
	if prompts == nil {
		return
	}
	globalPrompts := map[string]string{"Ctrl+L": "Logs", "Ctrl+C": "Quit"}
	var allPrompts []string
	for key, desc := range globalPrompts {
		allPrompts = append(allPrompts, fmt.Sprintf("[darkcyan::b]%s[-:-:-]: %s", key, desc))
	}
	var pageKeys []string
	for key := range prompts {
		pageKeys = append(pageKeys, key)
	}
	sort.Strings(pageKeys)
	for _, key := range pageKeys {
		desc := prompts[key]
		allPrompts = append(allPrompts, fmt.Sprintf("[darkcyan::b]%s[-:-:-]: %s", key, desc))
	}
	fullText := " " + strings.Join(allPrompts, " | ")
	lm.footer.AddItem(tview.NewTextView().SetDynamicColors(true).SetText(fullText), 0, 1, false)
}

// SetHeader updates the status bar
func (lm *LayoutManager) SetHeader(p *tview.TextView) {
	if p == nil {
		p = tview.NewTextView().SetDynamicColors(true)
	}
	lm.statusText = p
	lm.status.Clear()
	lm.status.AddItem(p, 0, 1, false)
}

func (lm *LayoutManager) SetStatusText(text string) {
	lm.statusText.SetText(text)
}
