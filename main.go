package main

import (
	"bytes"
	"fmt"
	"os"

	"github.com/Qendolin/fabric-mod-bisect-tool/app"
	"github.com/Qendolin/fabric-mod-bisect-tool/app/logging"
)

const (
	logDir      = "_logs"
	logFileName = "imcs_app.log"
)

func main() {
	// 1. Capture initial logs before UI is set up
	initialLogBuffer := &bytes.Buffer{}
	captureWriter := &logging.CaptureWriter{Buffer: initialLogBuffer}
	if err := logging.Init(logDir, logFileName, captureWriter); err != nil {
		fmt.Fprintf(os.Stderr, "Fatal: Failed to initialize pre-UI logging: %v\n", err)
		os.Exit(1)
	}
	logging.Info("Pre-UI logging captured.")

	// 2. Create the App structure, which no longer needs a factory
	a := app.NewApp()

	// 3. Initialize logging again, now passing the UI writer and initial logs
	if err := a.InitLogging(logDir, logFileName, initialLogBuffer); err != nil {
		fmt.Fprintf(os.Stderr, "Fatal: Failed to initialize UI logging: %v\n", err)
		os.Exit(1)
	}
	defer logging.Close()

	logging.Info("Application started.")

	// 4. Run the application
	if err := a.Run(); err != nil {
		logging.Errorf("Application exited with error: %v", err)
		os.Exit(1)
	}
	logging.Info("Application exited gracefully.")
}
