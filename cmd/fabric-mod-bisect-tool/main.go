package main

import (
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"runtime/debug"
	"syscall"
	"time"

	"github.com/Qendolin/fabric-mod-bisect-tool/pkg/app"
	"github.com/Qendolin/fabric-mod-bisect-tool/pkg/logging"
)

func main() {
	cliArgs := app.ParseCLIArgs()

	// 1. Setup logging first.
	mainLogger := logging.NewLogger()
	// Create the log directory if it doesn't exist.
	if err := os.MkdirAll(cliArgs.LogDir, 0755); err != nil {
		os.Stderr.WriteString(fmt.Sprintf("Failed to create log directory: %v\n", err))
		os.Exit(1)
	}
	// Create a unique, timestamped log file name.
	logFileName := fmt.Sprintf("bisect-tool-%s.log", time.Now().Format("2006-01-02_15-04-05"))
	logPath := filepath.Join(cliArgs.LogDir, logFileName)

	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0666)
	if err != nil {
		// Can't use logger yet, so print to stderr
		os.Stderr.WriteString("Failed to open log file: " + err.Error())
		os.Exit(1)
	}
	defer logFile.Close()
	mainLogger.SetWriter(logFile)
	logging.SetDefault(mainLogger)

	if cliArgs.Verbose {
		mainLogger.SetDebug(true)
		logging.Infof("Main: Verbose logging enabled.")
	}

	wd, err := os.Getwd()
	if err != nil {
		logging.Errorf("Main: Failed to get current working directory: %v", err)
	} else {
		logging.Infof("Main: Current Working Directory: %s", wd)
	}
	if info, ok := debug.ReadBuildInfo(); ok {
		for _, setting := range info.Settings {
			if setting.Key == "vcs.time" {
				logging.Infof("Main: Build Time: %s", setting.Value)
			}
			if setting.Key == "vcs.revision" {
				logging.Infof("Main: Build Revision: %s", setting.Value)
			}
		}
	}

	// 2. Setup OS signal trapping
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	// 3. Create the App structure, passing the configured logger
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
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	if a.IsBisectionReady() {
		finalReport := app.GenerateLogReport(
			a.GetViewModel(),
			a.GetStateManager(),
		)
		logging.Infof("\n===== Bisection Report =====\n\n%s", finalReport)
	}

	logging.Infof("Main: Application exited gracefully.")
}
