package main

import (
	"os"
	"os/signal"
	"syscall"

	"github.com/Qendolin/fabric-mod-bisect-tool/app"
	"github.com/Qendolin/fabric-mod-bisect-tool/app/logging"
)

const (
	logPath = "bisect-tool.log"
)

func main() {
	// 1. Setup OS signal trapping
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	// 2. Create the App structure, passing log details
	a := app.NewApp()

	// TODO: I don't like the way logging is initialized
	a.InitLogging(logPath)

	// 3. Goroutine to handle OS signals
	go func() {
		<-sigChan
		a.QueueUpdateDraw(func() {
			a.Dialogs().ShowQuitDialog()
		})
	}()

	// 4. Run the application
	logging.Info("Application starting up.") // This log will go to file and UI TextView
	if err := a.Run(); err != nil {
		logging.Errorf("Application exited with error: %v", err)
		os.Exit(1)
	}
	logging.Info("Application exited gracefully.")
}
