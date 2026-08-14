package app

import (
	"flag"

	"github.com/Qendolin/fabric-mod-bisect-tool/pkg/core/mods"
)

const AppCommonName = "mod-bisect-tool"
const AppGuiName = "mod-bisect-gui"
const AppTuiName = "mod-bisect-tui"

const (
	AppDistributionDevelopment   = "development"
	AppDistributionWindowsBinary = "windows-binary"
	AppDistributionLinuxBinary   = "linux-binary"
	AppDistributionLinuxAppImage = "linux-appimage"
	AppDistributionDarwinBinary  = "darwin-binary"
	AppDistributionDarwinApp     = "darwin-app"
)

var AppDistribution = AppDistributionDevelopment

// CLIArgs holds all command-line arguments passed to the application.
type CLIArgs struct {
	NoEmbeddedOverrides bool
	Verbose             bool
	Loader              mods.RunLoader
	LogDir              string
}

// ParseCLIArgs parses the command-line flags and returns a populated CLIArgs struct.
func ParseCLIArgs() *CLIArgs {
	args := &CLIArgs{}

	flag.BoolVar(&args.NoEmbeddedOverrides, "no-embedded-overrides", false, "Disable the built-in dependency overrides for known problematic mods.")
	flag.BoolVar(&args.Verbose, "verbose", false, "Enable verbose (debug) logging.")
	flag.Func("loader", "Mod loader to run with: fabric, neoforge, connector (NeoForge with Fabric) or kilt (Fabric with NeoForge). Defaults to auto-detection.", func(value string) error {
		loader, err := mods.ParseRunLoader(value)
		if err != nil {
			return err
		}
		args.Loader = loader
		return nil
	})
	flag.StringVar(&args.LogDir, "log-dir", ".", "Specifies the directory to store log files.")
	flag.Parse()

	return args
}
