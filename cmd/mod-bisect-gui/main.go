package main

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/Qendolin/fabric-mod-bisect-tool/pkg/app"
	"github.com/Qendolin/fabric-mod-bisect-tool/pkg/gui/guiapp"
	"github.com/Qendolin/fabric-mod-bisect-tool/pkg/logging"
)

func main() {
	defer logging.HandlePanic()

	var a *app.App
	var guiApp *guiapp.App

	go func() {
		for p := range logging.PanicChannel {
			if guiApp != nil {
				guiApp.Stop()
			}
			fmt.Fprintf(os.Stderr, "panic: %v\n%s", p.Value, string(p.Stack))
			os.Exit(2)
		}
	}()

	cliArgs := app.ParseCLIArgs()

	mainLogger := logging.NewLogger()
	if err := os.MkdirAll(cliArgs.LogDir, 0755); err != nil {
		os.Stderr.WriteString(fmt.Sprintf("Failed to create log directory: %v\n", err))
		os.Exit(1)
	}
	logFileName := fmt.Sprintf("bisect-gui-%s.log", time.Now().Format("2006-01-02_15-04-05"))
	logPath := filepath.Join(cliArgs.LogDir, logFileName)

	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0666)
	if err != nil {
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

	a = app.NewApp(mainLogger, cliArgs)
	guiApp = guiapp.NewApp(a, mainLogger)
	a.SetView(guiApp)

	logging.Infof("Main: Application starting up.")
	if err := guiApp.Run(); err != nil {
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
