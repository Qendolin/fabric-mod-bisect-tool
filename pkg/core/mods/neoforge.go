package mods

import (
	"archive/zip"
	"bufio"
	"encoding/json"
	"fmt"
	"path/filepath"
	"slices"
	"strings"

	"github.com/Qendolin/fabric-mod-bisect-tool/pkg/core/mods/version"
	"github.com/Qendolin/fabric-mod-bisect-tool/pkg/logging"
	"github.com/pelletier/go-toml/v2"
)

// neoForgeModsToml defines the structure for unmarshaling neoforge.mods.toml.
type neoForgeModsToml struct {
	Mods         []neoForgeMod                   `toml:"mods"`
	Dependencies map[string][]neoForgeDependency `toml:"dependencies"`
}

// neoForgeMod represents a single [[mods]] entry.
type neoForgeMod struct {
	ModID       string   `toml:"modId"`
	Version     string   `toml:"version"`
	DisplayName string   `toml:"displayName"`
	Provides    []string `toml:"provides"`
}

// neoForgeDependency represents a single dependency entry.
type neoForgeDependency struct {
	ModID        string `toml:"modId"`
	Type         string `toml:"type"`
	VersionRange string `toml:"versionRange"`
}

// jarJarMetadata defines the structure for unmarshaling META-INF/jarjar/metadata.json.
type jarJarMetadata struct {
	Jars []struct {
		Path string `json:"path"`
	} `json:"jars"`
}

// parseJarJarMetadata finds and parses a META-INF/jarjar/metadata.json file from a zip archive.
// It returns a slice of internal file paths
func (p *ModParser) parseJarJarMetadata(zipReader *zip.Reader, jarIdentifier string, logBuffer *logBuffer) ([]string, error) {
	jarJarFile := getZipFileEntry(zipReader, "META-INF/jarjar/metadata.json")
	if jarJarFile == nil {
		return nil, nil // Not an error, the file is optional.
	}

	jarJarData, err := readZipFileEntry(jarJarFile)
	if err != nil {
		return nil, fmt.Errorf("failed to read META-INF/jarjar/metadata.json in %s: %w", jarIdentifier, err)
	}

	var metadata jarJarMetadata
	if err := json.Unmarshal(jarJarData, &metadata); err != nil {
		return nil, fmt.Errorf("failed to parse META-INF/jarjar/metadata.json in %s: %w", jarIdentifier, err)
	}

	jars := make([]string, len(metadata.Jars))
	for i, jarEntry := range metadata.Jars {
		jars[i] = jarEntry.Path
	}
	return jars, nil
}

// parseNeoForgeNestedJars handles the NeoForge-specific container logic for JARs that are not mods
// but contain nested mods via jarjar metadata. It creates a synthetic ModMetadata object for the container.
// Returns an error if the JAR is not a valid container (no jarjar metadata and not a mod).
// Returns nil error if the JAR is a library (no jarjar metadata) or a valid container.
func (p *ModParser) parseNeoForgeNestedJars(zipReader *zip.Reader, mm *ModMetadata, jarIdentifier string, logBuffer *logBuffer) error {
	jarJarEntries, jarJarErr := p.parseJarJarMetadata(zipReader, jarIdentifier, logBuffer)

	// Jar is not a mod, it's either a container or just a java library
	if mm.Loader == LoaderNone {
		// Treat the jar as a container. Create a synthetic metadata object for it.
		// The placeholder version is required for the struct, but will not be used in dependency resolution.
		placeholderVersion, _ := version.Parse("0.0.0-synthetic", false)
		*mm = ModMetadata{
			ID:            fmt.Sprintf("library-%s", filepath.Base(jarIdentifier)),
			Version:       VersionField{Version: placeholderVersion},
			Name:          "Library",
			Jars:          jarJarEntries,
			Loader:        LoaderNone,
			IsJavaLibrary: true,
		}

		if len(jarJarEntries) == 0 {
			if jarJarErr != nil {
				return fmt.Errorf("parsing metadata for %s failed and it is not a valid container: %w", jarIdentifier, jarJarErr)
			} else {
				// This is not an error
				logBuffer.add(logging.LevelDebug, "ModLoader: %s is not a mod and not a container, probably just a library", jarIdentifier)
			}
		} else {
			logBuffer.add(logging.LevelDebug, "ModLoader: %s is not a mod, but is a container for %d nested JAR(s).", jarIdentifier, len(jarJarEntries))
		}
	} else {
		// Jar is a mod, Doesn't matter if it's NF or Fabric/Quilt mod, we need to add the nested jars
		if jarJarErr != nil {
			logBuffer.add(logging.LevelWarn, "ModLoader: Jar file is a mod, but loading nested jars failed: %v", jarJarErr)
		}

		lenBefore := len(mm.Jars)
		for _, e := range jarJarEntries {
			// Deduplicate. Not efficient, but whatever
			if !slices.Contains(mm.Jars[:lenBefore], e) {
				mm.Jars = append(mm.Jars, e)
			}
		}
	}

	return nil
}

// readVersionFromManifest finds and parses the META-INF/MANIFEST.MF file within a JAR
// to extract the value of the "Implementation-Version" attribute.
func (p *ModParser) readVersionFromManifest(zipReader *zip.Reader, jarIdentifier string) (string, error) {
	manifestFile := getZipFileEntry(zipReader, "META-INF/MANIFEST.MF")
	if manifestFile == nil {
		return "", fmt.Errorf("mod '%s' specifies version=${file.jarVersion} but META-INF/MANIFEST.MF was not found", jarIdentifier)
	}

	rc, err := manifestFile.Open()
	if err != nil {
		return "", fmt.Errorf("could not open META-INF/MANIFEST.MF for '%s': %w", jarIdentifier, err)
	}
	defer rc.Close()

	scanner := bufio.NewScanner(rc)
	for scanner.Scan() {
		line := scanner.Text()
		// Check for the specific key. A simple prefix check is sufficient and robust.
		if strings.HasPrefix(line, "Implementation-Version:") {
			// Split the line at the first colon and trim whitespace from the value.
			parts := strings.SplitN(line, ":", 2)
			if len(parts) == 2 {
				version := strings.TrimSpace(parts[1])
				if version != "" {
					return version, nil
				}
			}
		}
	}

	if err := scanner.Err(); err != nil {
		return "", fmt.Errorf("error reading META-INF/MANIFEST.MF for '%s': %w", jarIdentifier, err)
	}

	return "", fmt.Errorf("mod '%s' specifies version=${file.jarVersion} but 'Implementation-Version' was not found in META-INF/MANIFEST.MF", jarIdentifier)
}

// parseNeoForgeModToml reads a neoforge.mods.toml file and translates its contents
// into the tool's internal ModMetadata format.
func (p *ModParser) parseNeoForgeModToml(zipReader *zip.Reader, manifest modManifestResult, jarIdentifier string, logBuffer *logBuffer) (mm ModMetadata, err error) {
	tomlBytes, err := readZipFileEntry(manifest.File)
	if err != nil {
		return mm, fmt.Errorf("reading %s from %s: %w", manifest.Path, jarIdentifier, err)
	}

	var tomlData neoForgeModsToml
	if err := toml.Unmarshal(tomlBytes, &tomlData); err != nil {
		return mm, fmt.Errorf("unmarshaling %s from %s: %w", manifest.Path, jarIdentifier, err)
	}

	if len(tomlData.Mods) == 0 {
		return mm, fmt.Errorf("%s from %s contains no [[mods]] entries", manifest.Path, jarIdentifier)
	}

	// Translate the primary mod identity and "provides" list.
	// The first [[mods]] entry is considered the primary mod.
	primaryMod := tomlData.Mods[0]
	mm = ModMetadata{
		ID:       primaryMod.ModID,
		Name:     primaryMod.DisplayName,
		Loader:   manifest.Loader,
		Provides: primaryMod.Provides,
	}

	mavenModVersionStr := primaryMod.Version
	if mavenModVersionStr == "${file.jarVersion}" {
		versionFromManifest, err := p.readVersionFromManifest(zipReader, jarIdentifier)
		if err != nil {
			logBuffer.add(logging.LevelWarn, "ModLoader: %v", err)
			versionFromManifest = "0.0.0"
		}
		logBuffer.add(logging.LevelDebug, "ModLoader: Resolved dynamic version for %s from MANIFEST.MF as %s", primaryMod.ModID, versionFromManifest)
		mavenModVersionStr = versionFromManifest
	}

	// Leverage the existing VersionField JSON unmarshaler to parse the version string.
	// The string must be wrapped in quotes to be treated as a valid JSON string.
	modVersionStr, err := version.TranslateMavenVersion(mavenModVersionStr)
	if err != nil {
		return mm, fmt.Errorf("translating version '%s' for mod '%s' in %s: %w", primaryMod.Version, primaryMod.ModID, jarIdentifier, err)
	}
	if err := mm.Version.UnmarshalJSON([]byte(fmt.Sprintf(`"%s"`, modVersionStr))); err != nil {
		return mm, fmt.Errorf("parsing version '%s' for mod '%s' in %s: %w", primaryMod.Version, primaryMod.ModID, jarIdentifier, err)
	}

	// Subsequent [[mods]] blocks are treated as "provides".
	if len(tomlData.Mods) > 1 {
		for _, providedMod := range tomlData.Mods[1:] {
			// Add the mod ID itself
			mm.Provides = append(mm.Provides, providedMod.ModID)
			// Also add any additional provides from the block
			mm.Provides = append(mm.Provides, providedMod.Provides...)
		}
	}

	// Translate dependencies.
	mm.Depends = make(VersionRanges)
	mm.Recommends = make(VersionRanges)
	mm.Breaks = make(VersionRanges)
	mm.Conflicts = make(VersionRanges)

	if modDependencies, ok := tomlData.Dependencies[primaryMod.ModID]; ok {
		for _, dep := range modDependencies {
			if dep.ModID == "" {
				continue // Skip malformed dependencies with no ID.
			}

			// A single Maven range string can translate to multiple Fabric predicate strings (OR relationship).
			predicateStrings, err := version.TranslateMavenVersionRange(dep.VersionRange)
			if err != nil {
				return mm, fmt.Errorf("translating maven version range '%s' for dep '%s' in %s: %w", dep.VersionRange, dep.ModID, jarIdentifier, err)
			}

			// Parse each translated predicate string into a VersionPredicate object.
			predicates := make([]*version.VersionPredicate, 0, len(predicateStrings))
			for _, pStr := range predicateStrings {
				pred, err := version.ParseVersionPredicate(pStr)
				if err != nil {
					logBuffer.add(logging.LevelWarn, "ModLoader: Failed to parse translated predicate '%s' (from maven range '%s') for dep '%s' in %s: %v", pStr, dep.VersionRange, dep.ModID, jarIdentifier, err)
					pred = version.Any()
				}
				predicates = append(predicates, pred)
			}

			depType := dep.Type
			if depType == "" {
				depType = "required"
			}

			switch depType {
			case "required":
				mm.Depends[dep.ModID] = predicates
			case "optional":
				mm.Recommends[dep.ModID] = predicates
			case "incompatible":
				mm.Breaks[dep.ModID] = predicates
			case "discouraged":
				mm.Conflicts[dep.ModID] = predicates
			}
		}
	}

	return mm, nil
}
