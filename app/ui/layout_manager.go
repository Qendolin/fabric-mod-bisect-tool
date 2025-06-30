package ui

import (
	"fmt"
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

	// Use boxes instead of padding to avoid transparent gap
	lm.header.AddItem(tview.NewBox(), 1, 0, false).
		AddItem(lm.status, 0, 1, false).
		AddItem(tview.NewBox(), 1, 0, false).
		AddItem(lm.errorCounters, 0, 1, false)

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
func (lm *LayoutManager) SetFooter(prompts []ActionPrompt) {
	lm.footer.Clear()
	if prompts == nil {
		return
	}
	globalPrompts := []ActionPrompt{{"Ctrl+C", "Quit"}, {"Ctrl+L", "Logs"}, {"Ctrl+H", "History"}, {"Tab", "Focus"}}
	allPrompts := append(globalPrompts, prompts...)

	var sb strings.Builder
	for i, prompt := range allPrompts {
		sb.WriteString(fmt.Sprintf("[darkcyan::b]%s[-:-:-]: %s", prompt.Input, prompt.Action))
		if i != len(allPrompts)-1 {
			sb.WriteString(" | ")
		}
	}
	lm.footer.AddItem(tview.NewTextView().SetDynamicColors(true).SetText(sb.String()), 0, 1, false)
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
