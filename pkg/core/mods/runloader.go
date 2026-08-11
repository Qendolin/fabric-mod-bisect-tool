package mods

import "fmt"

// RunLoader identifies the mod loader the user runs Minecraft with. It decides
// which mod manifests are recognized and in what priority order. It is distinct
// from ManifestLoader, which describes the loader a single mod manifest targets.
type RunLoader string

const (
	// RunLoaderFabric runs the Fabric loader (with Quilt mods accepted).
	RunLoaderFabric RunLoader = "fabric"
	// RunLoaderNeoForge runs the (Neo)Forge loader.
	RunLoaderNeoForge RunLoader = "neoforge"
	// RunLoaderNeoForgeWithFabric runs (Neo)Forge with Fabric mods via Sinytra
	// Connector. (Neo)Forge manifests are preferred, Fabric is the fallback.
	RunLoaderNeoForgeWithFabric RunLoader = "neoforge-with-fabric"
	// RunLoaderFabricWithNeoForge runs Fabric with (Neo)Forge mods via Kilt.
	// Fabric manifests are preferred, (Neo)Forge is the fallback.
	RunLoaderFabricWithNeoForge RunLoader = "fabric-with-neoforge"
)

// String returns the user-facing label for the loader.
func (l RunLoader) String() string {
	switch l {
	case RunLoaderFabric:
		return "Fabric"
	case RunLoaderNeoForge:
		return "(Neo)Forge"
	case RunLoaderNeoForgeWithFabric:
		return "(Neo)Forge with Fabric (Sinytra Connector)"
	case RunLoaderFabricWithNeoForge:
		return "Fabric with (Neo)Forge (Kilt)"
	default:
		return string(l)
	}
}

// ParseRunLoader parses a command-line value into a RunLoader.
func ParseRunLoader(value string) (RunLoader, error) {
	switch value {
	case string(RunLoaderFabric):
		return RunLoaderFabric, nil
	case string(RunLoaderNeoForge):
		return RunLoaderNeoForge, nil
	case "connector":
		return RunLoaderNeoForgeWithFabric, nil
	case "kilt":
		return RunLoaderFabricWithNeoForge, nil
	default:
		return "", fmt.Errorf("unknown loader %q (expected fabric, neoforge, connector or kilt)", value)
	}
}

// SupportedRunLoaders returns the loaders offered to users, in display order.
// It is shared by the GUI and TUI setup screens.
func SupportedRunLoaders() []RunLoader {
	return []RunLoader{RunLoaderFabric, RunLoaderNeoForge, RunLoaderNeoForgeWithFabric, RunLoaderFabricWithNeoForge}
}

// includesNeoForgeFamily reports whether the loader recognizes (Neo)Forge mods
// (both the modern neoforge.mods.toml and the legacy Forge mods.toml), including
// their nested jar (jarjar) libraries.
func (l RunLoader) includesNeoForgeFamily() bool {
	switch l {
	case RunLoaderNeoForge, RunLoaderNeoForgeWithFabric, RunLoaderFabricWithNeoForge:
		return true
	default:
		return false
	}
}

// toleratesNonModJars reports whether jars without any recognized mod manifest
// are accepted (as (Neo)Forge libraries) rather than rejected.
func (l RunLoader) toleratesNonModJars() bool {
	return l.includesNeoForgeFamily()
}

// manifestOrder returns the manifest loaders to try, in priority order. Within
// the Fabric family, fabric.mod.json is preferred over quilt.mod.json.
func (l RunLoader) manifestOrder() []ManifestLoader {
	switch l {
	case RunLoaderFabric:
		return []ManifestLoader{ManifestLoaderFabric, ManifestLoaderQuilt}
	case RunLoaderNeoForge:
		return []ManifestLoader{ManifestLoaderNeoForge}
	case RunLoaderNeoForgeWithFabric:
		return []ManifestLoader{ManifestLoaderNeoForge, ManifestLoaderFabric, ManifestLoaderQuilt}
	case RunLoaderFabricWithNeoForge:
		return []ManifestLoader{ManifestLoaderFabric, ManifestLoaderQuilt, ManifestLoaderNeoForge}
	default:
		return nil
	}
}
