package app

import "flag"

// CLIArgs holds all command-line arguments passed to the application.
type CLIArgs struct {
	NoEmbeddedOverrides bool
	Verbose             bool
}

// ParseCLIArgs parses the command-line flags and returns a populated CLIArgs struct.
func ParseCLIArgs() *CLIArgs {
	args := &CLIArgs{}

	flag.BoolVar(&args.NoEmbeddedOverrides, "no-embedded-overrides", false, "Disable the built-in dependency overrides for known problematic mods.")
	flag.BoolVar(&args.Verbose, "verbose", false, "Enable verbose (debug) logging.")
	flag.Parse()

	return args
}
