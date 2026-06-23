package probe

import (
	"archive/zip"
	"os"
	"path/filepath"
	"strings"

	"github.com/Qendolin/fabric-mod-bisect-tool/pkg/logging"
)

// ProbeResult contains the findings of the directory probe.
type ProbeResult struct {
	QuiltSupport    bool
	NeoForgeSupport bool
}

// ProbeModsDirectory scans the .jar files in the given directory to detect
// if Quilt or NeoForge mod files are present.
func ProbeModsDirectory(modsPath string) ProbeResult {
	result := ProbeResult{}

	entries, err := os.ReadDir(modsPath)
	if err != nil {
		logging.Errorf("Probe: failed to read directory %s: %v", modsPath, err)
		return result
	}

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".jar") {
			continue
		}

		jarPath := filepath.Join(modsPath, entry.Name())
		quiltFound, neoForgeFound := probeJar(jarPath)

		if quiltFound {
			result.QuiltSupport = true
		}
		if neoForgeFound {
			result.NeoForgeSupport = true
		}

		// If we found both, we can stop early
		if result.QuiltSupport && result.NeoForgeSupport {
			break
		}
	}

	logging.Infof("Probe: Finished probing %s. QuiltSupport=%v, NeoForgeSupport=%v", modsPath, result.QuiltSupport, result.NeoForgeSupport)
	return result
}

func probeJar(jarPath string) (quiltSupport, neoForgeSupport bool) {
	r, err := zip.OpenReader(jarPath)
	if err != nil {
		return false, false
	}
	defer r.Close()

	for _, f := range r.File {
		if f.Name == "quilt.mod.json" {
			quiltSupport = true
		}
		if f.Name == "META-INF/neoforge.mods.toml" || f.Name == "META-INF/mods.toml" {
			neoForgeSupport = true
		}

		if quiltSupport && neoForgeSupport {
			break
		}
	}

	return quiltSupport, neoForgeSupport
}
