package app

import (
	"fmt"
	"log"
	"os"
	"strings"
	"time"
)

// UILogWriter is an io.Writer that tees writes to a log file and a channel for TUI updates.
type UILogWriter struct {
	logFile     *os.File
	uiLogChan   chan<- string
	logPrefixer func(string) string // Optional function to prefix messages for UI
}

// Write implements io.Writer. It expects p to be a single log entry.
func (w *UILogWriter) Write(p []byte) (n int, err error) {
	if w.logFile != nil {
		n, err = w.logFile.Write(p)
		if err != nil {
			// Fallback to stderr if file write fails
			fmt.Fprintf(os.Stderr, "Error writing to log file: %v\nOriginal log: %s", err, string(p))
		}
	} else {
		n = len(p) // Pretend we wrote if no file
	}

	// Send to UI log channel. Standard logger often adds a newline.
	msg := strings.TrimSpace(string(p))
	if w.logPrefixer != nil {
		msg = w.logPrefixer(msg)
	}

	// Non-blocking send or drop if channel is full to prevent deadlocks.
	// However, a buffered channel should handle typical load.
	// For critical logs, ensure channel has enough buffer or handle differently.
	select {
	case w.uiLogChan <- msg:
	default:
		// Log dropped for UI, should be rare with adequate buffer.
		// Can log this occurrence to stderr if needed for debugging logger itself.
		fmt.Fprintf(os.Stderr, "UI log channel full, dropped message: %s\n", msg)
	}

	return n, err // Return results from file write
}

// StartLogProcessor initializes logging to file and TUI.
// It redirects the standard log output.
func (ctx *AppContext) StartLogProcessor(logFilePath string) error {
	logFile, err := os.OpenFile(logFilePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		return fmt.Errorf("failed to open log file %s: %w", logFilePath, err)
	}

	// Channel for relaying messages from standard logger to TUI updater
	// Buffer size should be generous enough for bursts.
	logRelayChan := make(chan string, 512)

	ctx.logFile = &UILogWriter{
		logFile:   logFile,
		uiLogChan: logRelayChan,
	}

	// Redirect standard Go logger
	log.SetOutput(ctx.logFile)
	log.Println(LogInfoPrefix + "Standard log output redirected to file and TUI.")

	ctx.logRelayWg.Add(1)
	go func() {
		defer ctx.logRelayWg.Done()
		log.Println(LogInfoPrefix + "TUI Log Relay Goroutine: Started.")
		ticker := time.NewTicker(250 * time.Millisecond) // Update TUI periodically
		defer ticker.Stop()

		var batch []string
		for {
			select {
			case msg, ok := <-logRelayChan:
				if !ok { // Channel closed
					ctx.flushLogBatchToUI(batch)
					batch = nil
					log.Println(LogInfoPrefix + "TUI Log Relay Goroutine: Channel closed, exiting.")
					return
				}
				// Standard log messages already have timestamps if LstdFlags is set.
				batch = append(batch, msg)

			case <-ticker.C:
				if len(batch) > 0 {
					ctx.flushLogBatchToUI(batch)
					batch = nil // Reset batch
				}
			case <-ctx.logRelayStopCh:
				ctx.flushLogBatchToUI(batch)
				batch = nil
				log.Println(LogInfoPrefix + "TUI Log Relay Goroutine: Stop signal received, exiting.")
				return
			}
		}
	}()
	return nil
}

func (ctx *AppContext) flushLogBatchToUI(batch []string) {
	if len(batch) == 0 {
		return
	}
	ctx.uiLogLines = append(ctx.uiLogLines, batch...)
	if len(ctx.uiLogLines) > ctx.maxUILogLines {
		ctx.uiLogLines = ctx.uiLogLines[len(ctx.uiLogLines)-ctx.maxUILogLines:]
	}

	if ctx.App != nil && ctx.DebugLogView != nil {
		currentLogText := strings.Join(ctx.uiLogLines, "\n")
		go ctx.App.QueueUpdateDraw(func() {
			ctx.DebugLogView.SetText(currentLogText)
			// ScrollToEnd is handled by SetChangedFunc of DebugLogView in ui/pages.go
		})
	}
}

// StopLogProcessor signals the log processor to stop and waits for it.
func (ctx *AppContext) StopLogProcessor() {
	log.Println(LogInfoPrefix + "Attempting to stop log processor...")

	// Signal the relay goroutine to stop
	close(ctx.logRelayStopCh)
	ctx.logRelayWg.Wait() // Wait for relay goroutine to finish processing remaining messages

	// Close the underlying log relay channel AFTER the goroutine has exited
	// to ensure all messages sent before stop signal are processed.
	// The UILogWriter's uiLogChan is unbuffered for select, but logRelayChan (internal to this file) is buffered.
	// Actually, uiLogChan is the one passed to UILogWriter, which is logRelayChan.
	// It's better to close it before Wait, so the Write method won't block/panic if called after StopLogProcessor started.
	// However, standard log might still try to write.
	// The safest is to reset log output first.

	log.SetOutput(os.Stderr) // Reset standard logger before closing file

	if ctx.logFile != nil && ctx.logFile.logFile != nil {
		if err := ctx.logFile.logFile.Close(); err != nil {
			log.Printf(LogErrorPrefix+"Error closing log file: %v", err)
		}
		log.Println(LogInfoPrefix + "Log file closed.")
	}
	log.Println(LogInfoPrefix + "Log processor stopped.")
}
