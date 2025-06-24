package ui

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/rivo/tview"
)

// LogHandler manages processing log messages for the UI.
type LogHandler struct {
	app         AppInterface
	logTextView *tview.TextView
	logChannel  chan []byte
	logBatch    [][]byte
	shutdownWg  *sync.WaitGroup

	warnCount    int
	errorCount   int
	batchMutex   sync.Mutex
	counterMutex sync.Mutex
	updateTicker *time.Ticker
}

// NewLogHandler creates a new handler for UI log processing.
func NewLogHandler(app AppInterface, logChannel chan []byte, wg *sync.WaitGroup) *LogHandler {
	return &LogHandler{
		app:        app,
		logChannel: logChannel,
		logTextView: tview.NewTextView().
			SetDynamicColors(true).
			SetScrollable(true).
			SetWrap(true),
		shutdownWg: wg,
	}
}

// TextView returns the underlying tview.TextView for the logs.
func (h *LogHandler) TextView() *tview.TextView {
	return h.logTextView
}

// Start begins the log processing loop.
func (h *LogHandler) Start(ctx context.Context) {
	h.updateTicker = time.NewTicker(100 * time.Millisecond)
	h.shutdownWg.Add(1) // Add to waitgroup for this goroutine

	go func() {
		defer h.shutdownWg.Done()
		for {
			select {
			case <-ctx.Done():
				h.updateTicker.Stop()
				return
			case logMsg, ok := <-h.logChannel:
				if !ok {
					return
				}
				h.processLogMessage(logMsg)
			case <-h.updateTicker.C:
				h.flushLogBatch()
			}
		}
	}()
}

// processLogMessage adds a log message to the current batch and updates counters.
func (h *LogHandler) processLogMessage(msg []byte) {
	h.batchMutex.Lock()
	h.logBatch = append(h.logBatch, msg)
	h.batchMutex.Unlock()

	content := string(msg)
	newWarnings := strings.Count(content, "WARN:")
	newErrors := strings.Count(content, "ERROR:")

	if newWarnings > 0 || newErrors > 0 {
		h.counterMutex.Lock()
		h.warnCount += newWarnings
		h.errorCount += newErrors
		h.counterMutex.Unlock()
	}
}

// flushLogBatch writes the entire accumulated batch of logs and updates counters in one UI call.
func (h *LogHandler) flushLogBatch() {
	h.batchMutex.Lock()
	if len(h.logBatch) == 0 {
		h.batchMutex.Unlock()
		return
	}

	var batchContent strings.Builder
	for _, msg := range h.logBatch {
		batchContent.Write(msg)
	}
	h.logBatch = nil
	h.batchMutex.Unlock()

	go h.app.QueueUpdateDraw(func() {
		fmt.Fprint(h.logTextView, batchContent.String())
		h.logTextView.ScrollToEnd()
		h.app.Layout().SetErrorCounters(h.warnCount, h.errorCount)
	})
}
