package main

import (
	"fmt"
	"log"
	"os" // For potential early exit logging
	"os/signal"
	"syscall"

	"github.com/Qendolin/fabric-mod-bisect-tool/app"
	"github.com/Qendolin/fabric-mod-bisect-tool/ui"
	"github.com/gdamore/tcell/v2"
)

func main() {

	log.SetFlags(log.Ldate | log.Ltime | log.Lshortfile)
	log.Println("Application starting...")

	appCtx := app.NewAppContext()

	// clear screen
	fmt.Print("\033[H\033[2J")

	if err := appCtx.StartLogProcessor("bisect-tool.log"); err != nil {
		log.SetOutput(os.Stderr)
		log.Fatalf("Failed to start log processor: %v", err)
	}
	defer appCtx.StopLogProcessor()

	// Setup signal handling for graceful shutdown on Ctrl+C
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		sig := <-sigChan
		log.Printf("Received signal: %s. Shutting down...", sig)
		appCtx.Bisector.RestoreInitialModState()
		// It's important that app.Stop() is called to allow tview to clean up the screen.
		if appCtx.App != nil {
			appCtx.App.Stop()
		}
	}()

	ui.InitializeTUIPrimitives(appCtx)
	ui.SetupPages(appCtx)

	appCtx.App.SetRoot(appCtx.Pages, true).EnablePaste(true)
	appCtx.App.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		return ui.GlobalInputHandler(appCtx, event)
	})

	log.Println("Starting tview application run loop. Press Ctrl+C to exit.")
	err := appCtx.App.Run()

	// Perform cleanup actions AFTER tview app has stopped.
	// tview's Fini() (called by app.Stop()) should restore the terminal.
	if appCtx.Bisector != nil {
		// This logging might happen after StopLogProcessor if Stop() was from signal
		log.Println("Attempting to restore initial mod states on exit...")
		appCtx.Bisector.RestoreInitialModState()
		log.Println("Initial mod states restoration attempt finished.")
	}

	if err != nil {
		// Log critical error that caused app.Run() to exit abnormally
		log.SetOutput(os.Stderr) // Ensure this error is visible
		log.Fatalf("Application run error: %v", err)
	}

	log.Println("Application ended.")
}
