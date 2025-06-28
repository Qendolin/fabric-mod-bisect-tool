package ui

import (
	"fmt"
	"strings"
	"time"

	"github.com/Qendolin/fabric-mod-bisect-tool/app/logging"
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

const PageLogID = "log_page"

// LogPage displays application logs with filtering capabilities.
type LogPage struct {
	*tview.Flex
	app               AppInterface
	logView           *tview.TextView
	statusText        *tview.TextView
	currentFilter     logging.LogLevel
	lastLogCount      int
	stopPolling       chan struct{}
	isWordWrapEnabled bool
}

// NewLogPage creates a new LogPage instance.
func NewLogPage(app AppInterface) *LogPage {
	logView := tview.NewTextView().
		SetDynamicColors(true).
		SetScrollable(true).
		SetRegions(true).
		SetWordWrap(true).
		SetWrap(true)

	wrapper := tview.NewFlex().SetDirection(tview.FlexRow)
	frame := NewTitleFrame(logView, "Log")
	wrapper.AddItem(frame, 0, 1, true)

	page := &LogPage{
		Flex:              wrapper,
		app:               app,
		logView:           logView,
		statusText:        tview.NewTextView().SetDynamicColors(true),
		currentFilter:     logging.LevelInfo, // Default filter
		stopPolling:       make(chan struct{}),
		isWordWrapEnabled: true,
	}

	page.setKeybindings()
	page.refreshLogs(true) // Initial population of the log view
	page.startPolling()

	return page
}

// startPolling starts a goroutine that checks for new logs periodically.
func (p *LogPage) startPolling() {
	ticker := time.NewTicker(250 * time.Millisecond)

	go func() {
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				allLogs := p.app.GetLogger().Store().GetAll()
				if len(allLogs) != p.lastLogCount {
					p.app.QueueUpdateDraw(func() {
						p.refreshLogs(false)
					})
				}
			case <-p.stopPolling:
				return
			}
		}
	}()
}

// setKeybindings configures the input handling for the log page.
func (p *LogPage) setKeybindings() {
	p.Flex.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		// Handle filter changes first.
		newFilter := p.currentFilter
		switch event.Rune() {
		case 'a', 'A':
			newFilter = logging.LevelDebug
		case 'i', 'I':
			newFilter = logging.LevelInfo
		case 'w', 'W':
			newFilter = logging.LevelWarn
		case 'e', 'E':
			newFilter = logging.LevelError
		case 'r', 'R':
			p.isWordWrapEnabled = !p.isWordWrapEnabled
			p.logView.SetWrap(p.isWordWrapEnabled)
			p.updateStatus() // Update status to show the new state
			return nil
		}

		if newFilter != p.currentFilter {
			p.currentFilter = newFilter
			p.refreshLogs(true) // Force a full redraw and scroll
			return nil
		}

		// Handle navigation keys.
		if event.Key() == tcell.KeyEscape || (event.Key() == tcell.KeyCtrlL && event.Modifiers()&tcell.ModCtrl != 0) {
			close(p.stopPolling) // Stop the poller goroutine
			go p.app.QueueUpdateDraw(p.app.Navigation().GoBack)
			return nil
		}

		return event
	})
}

// refreshLogs re-renders the log view based on the current filter.
func (p *LogPage) refreshLogs(forceScrollToEnd bool) {
	p.updateStatus()

	allEntries := p.app.GetLogger().Store().GetAll()
	p.lastLogCount = len(allEntries)

	var builder strings.Builder
	const maxPrefixLength = 30
	for _, entry := range allEntries {
		if entry.Level >= p.currentFilter {
			var levelColor string
			switch entry.Level {
			case logging.LevelError:
				levelColor = "red"
			case logging.LevelWarn:
				levelColor = "yellow"
			default:
				levelColor = "white"
			}

			// Check for a component prefix like "Component: "
			prefixEndIndex := strings.Index(entry.Message, ": ")

			if prefixEndIndex > 0 && prefixEndIndex < maxPrefixLength {
				// A prefix was found. Color it separately.
				prefix := entry.Message[:prefixEndIndex+1]  // e.g., "Resolver:"
				message := entry.Message[prefixEndIndex+2:] // The rest of the message

				builder.WriteString(fmt.Sprintf(
					"[%s::b]%-5s[-:-:-] [darkcyan::b]%s[-:-:-] %s\n",
					levelColor,
					entry.Level.String(),
					tview.Escape(prefix),
					tview.Escape(message),
				))
			} else {
				// No prefix found, use the standard format.
				builder.WriteString(fmt.Sprintf(
					"[%s::b]%-5s[-:-:-] %s\n",
					levelColor,
					entry.Level.String(),
					tview.Escape(entry.Message),
				))
			}
		}
	}

	// To avoid flickering, only update if the text has changed.
	currentText := p.logView.GetText(false)
	newText := builder.String()
	if currentText != newText {
		p.logView.SetText(newText)
	}

	if forceScrollToEnd {
		p.logView.ScrollToEnd()
	}
}

// updateStatus updates the page's status text with the current filter.
func (p *LogPage) updateStatus() {
	wrapStatus := "Off"
	if p.isWordWrapEnabled {
		wrapStatus = "On"
	}
	p.statusText.SetText(
		fmt.Sprintf("Filter: [yellow]%s[-] | Wrap: [yellow]%s[-]", p.currentFilter.String(), wrapStatus),
	)
}

// GetActionPrompts returns the key actions for the log page.
func (p *LogPage) GetActionPrompts() map[string]string {
	return map[string]string{
		"ESC/Ctrl+L": "Close",
		"A/I/W/E":    "Filter All/Info/Warn/Error",
		"R":          "Toggle Wrap",
		"↑/↓":        "Scroll",
	}
}

// GetStatusPrimitive returns the tview.Primitive that displays the page's status.
func (p *LogPage) GetStatusPrimitive() *tview.TextView {
	return p.statusText
}
