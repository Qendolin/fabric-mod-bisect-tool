//go:build !windows

package main

// installConsoleCloseHandler is a no-op on platforms that do not deliver a
// distinguishable terminal-close event.
func installConsoleCloseHandler(func()) {}
