package tui

import (
	"os"
	"runtime"
	"strings"
)

// UnicodeGlyphsSupported reports whether the terminal is likely to render
// non-ASCII glyphs such as ✓ (U+2713) and ✗ (U+2717). Terminals do not expose
// their font's glyph coverage, so this is a best-effort environment heuristic.
// It errs on the side of "false" (ASCII fallback) for unknown environments,
// which is safe for the legacy Windows console whose raster font lacks them.
func UnicodeGlyphsSupported() bool {
	if runtime.GOOS == "windows" {
		return windowsGlyphsSupported()
	}
	// Unix terminals generally ship glyph-capable fonts; only a "dumb" TERM
	// (e.g. an editor buffer or a serial line) cannot be trusted.
	return strings.ToLower(os.Getenv("TERM")) != "dumb"
}

func windowsGlyphsSupported() bool {
	// Windows Terminal.
	if os.Getenv("WT_SESSION") != "" {
		return true
	}
	// ConEmu / Cmder sets this when it hosts the console.
	if os.Getenv("ConEmuANSI") != "" {
		return true
	}
	// Terminals that identify themselves explicitly, e.g. VS Code's
	// integrated terminal (TERM_PROGRAM=vscode), WezTerm, Apple Terminal.
	if os.Getenv("TERM_PROGRAM") != "" {
		return true
	}
	if os.Getenv("TERMINAL_EMULATOR") != "" {
		return true
	}
	// A set TERM on Windows indicates a POSIX-style terminal (mintty,
	// git-bash, cygwin) rather than the legacy console host.
	term := strings.ToLower(os.Getenv("TERM"))
	if term != "" && term != "dumb" {
		return true
	}
	return false
}
