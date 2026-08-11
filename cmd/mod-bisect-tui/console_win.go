//go:build windows

package main

import (
	"syscall"
)

var consoleCloseCh = make(chan struct{}, 1)

// installConsoleCloseHandler arranges for fn to be called when the console
// window is closed. Closing the window delivers CTRL_CLOSE_EVENT to the
// process; nobody can answer a dialog at that point, so fn should clean up and
// exit. The event is swallowed so the Go runtime does not turn it into SIGTERM
// (which would otherwise pop the quit dialog and freeze until Windows kills the
// process).
func installConsoleCloseHandler(fn func()) {
	go func() {
		<-consoleCloseCh
		fn()
	}()

	proc := syscall.NewLazyDLL("kernel32.dll").NewProc("SetConsoleCtrlHandler")
	proc.Call(syscall.NewCallback(consoleCtrlHandler), 1)
}

// consoleCtrlHandler is invoked by the OS when a console control event arrives.
// Handlers are called in last-in, first-out order, so this runs before the Go
// runtime's handler.
func consoleCtrlHandler(ctrlType uint32) uintptr {
	switch ctrlType {
	case syscall.CTRL_CLOSE_EVENT, syscall.CTRL_LOGOFF_EVENT, syscall.CTRL_SHUTDOWN_EVENT:
		// The window is closing: no one can answer a dialog. Signal cleanup and
		// swallow the event so the runtime does not deliver SIGTERM.
		select {
		case consoleCloseCh <- struct{}{}:
		default:
		}
		// Do not return: Windows terminates the process once the handler
		// returns. Blocking keeps the close sequence pending so the cleanup
		// goroutine has its full grace period (the Go runtime does the same
		// for SIGTERM). The cleanup ends with os.Exit, so this callback never
		// needs to unwind.
		select {}
	default:
		// Ctrl+C / Ctrl+Break: leave to the runtime, which delivers SIGINT and
		// shows the interactive quit dialog as usual.
		return 0
	}
}
