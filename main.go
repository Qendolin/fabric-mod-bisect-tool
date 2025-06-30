package main

import (
	"os"
	"os/signal"
	"syscall"

	"github.com/Qendolin/fabric-mod-bisect-tool/app"
	"github.com/Qendolin/fabric-mod-bisect-tool/app/logging"
)

const (
	logPath = "fabric-mod-bisect-tool.log"
)

func main() {
	// 1. Setup logging first.
	mainLogger := logging.NewLogger()
	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0666)
	if err != nil {
		// Can't use logger yet, so print to stderr
		os.Stderr.WriteString("Failed to open log file: " + err.Error())
		os.Exit(1)
	}
	defer logFile.Close()
	mainLogger.SetWriter(logFile)

	// Set this as the default logger for any package-level calls.
	logging.SetDefault(mainLogger)

	// 2. Setup OS signal trapping
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	// 3. Create the App structure, passing the configured logger
	cliArgs := app.ParseCLIArgs()
	a := app.NewApp(mainLogger, cliArgs)

	// 4. Goroutine to handle OS signals
	go func() {
		<-sigChan
		a.QueueUpdateDraw(func() {
			a.Dialogs().ShowQuitDialog()
		})
	}()

	// 5. Run the application
	logging.Infof("Main: Application starting up.")
	if err := a.Run(); err != nil {
		logging.Errorf("Main: Application exited with error: %v", err)
		os.Exit(1)
	}
	logging.Infof("Main: Application exited gracefully.")
}
