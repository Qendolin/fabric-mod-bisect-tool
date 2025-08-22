package mods

import (
	"archive/zip"
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"path/filepath"
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
	ModID       string `toml:"modId"`
	Version     string `toml:"version"`
	DisplayName string `toml:"displayName"`
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

// isNestedJarANeoForgeMod checks if a JAR file embedded within another ZIP archive
// contains a neoforge.mods.toml file, indicating it is a parsable mod.
func (p *ModParser) isNestedJarANeoForgeMod(parentZipReader *zip.Reader, pathInParent string) (bool, error) {
	// Find the nested JAR file entry within the parent archive.
	var nestedJarFile *zip.File
	for _, f := range parentZipReader.File {
		if f.Name == pathInParent {
			nestedJarFile = f
			break
		}
	}
	if nestedJarFile == nil {
		return false, fmt.Errorf("nested JAR '%s' not found in archive", pathInParent)
	}

	// Read the entire nested JAR into memory to treat it as a new zip archive.
	rc, err := nestedJarFile.Open()
	if err != nil {
		return false, fmt.Errorf("opening nested JAR '%s': %w", pathInParent, err)
	}
	defer rc.Close()

	jarBytes, err := io.ReadAll(rc)
	if err != nil {
		return false, fmt.Errorf("reading nested JAR '%s': %w", pathInParent, err)
	}

	// Create a new zip reader for the in-memory JAR data.
	bytesReader := bytes.NewReader(jarBytes)
	innerZipReader, err := zip.NewReader(bytesReader, int64(len(jarBytes)))
	if err != nil {
		return false, fmt.Errorf("treating nested content '%s' as zip: %w", pathInParent, err)
	}

	// Search for the metadata file within the nested JAR.
	for _, f := range innerZipReader.File {
		if f.Name == "META-INF/neoforge.mods.toml" {
			return true, nil // Found it, this is a valid mod.
		}
	}

	return false, nil // Did not find it, this is likely a library.
}

// parseJarJarMetadata finds and parses a META-INF/jarjar/metadata.json file from a zip archive.
// It returns a slice of file paths suitable for populating the FabricModJson.Jars field.
func (p *ModParser) parseJarJarMetadata(zipReader *zip.Reader, jarIdentifier string, logBuffer *logBuffer) ([]struct {
	File string `json:"file"`
}, error) {
	jarJarFile := p.getZipFileEntry(zipReader, "META-INF/jarjar/metadata.json")
	if jarJarFile == nil {
		return nil, nil // Not an error, the file is optional.
	}

	jarJarData, err := p.readZipFileEntry(jarJarFile)
	if err != nil {
		logBuffer.add(logging.LevelWarn, "ModLoader: Failed to read META-INF/jarjar/metadata.json in %s: %v", jarIdentifier, err)
		return nil, err
	}

	var metadata jarJarMetadata
	if err := json.Unmarshal(jarJarData, &metadata); err != nil {
		logBuffer.add(logging.LevelWarn, "ModLoader: Failed to parse META-INF/jarjar/metadata.json in %s: %v", jarIdentifier, err)
		return nil, err
	}

	logBuffer.add(logging.LevelDebug, "ModLoader: Found %d nested JAR(s) in jarjar/metadata.json for %s", len(metadata.Jars), jarIdentifier)
	jarEntries := make([]struct {
		File string `json:"file"`
	}, len(metadata.Jars))
	for i, jarEntry := range metadata.Jars {
		jarEntries[i].File = jarEntry.Path
	}
	return jarEntries, nil
}

// tryParseAsContainer is a fallback mechanism used when a JAR file fails standard mod metadata parsing.
// It checks if the JAR contains nested mods via a jarjar/metadata.json file. If so, it creates
// a synthetic "container" FabricModJson object to allow the recursive parsing process to continue.
// If not, it returns the original parsing error.
func (p *ModParser) tryParseAsContainer(zipReader *zip.Reader, jarIdentifier string, originalErr error, logBuffer *logBuffer) (FabricModJson, error) {
	// This logic is only applicable when NeoForge support is enabled.
	if !p.NeoForgeParsing {
		return FabricModJson{}, originalErr
	}

	// Attempt to find and parse a jarjar/metadata.json file.
	jarJarEntries, jarJarErr := p.parseJarJarMetadata(zipReader, jarIdentifier, logBuffer)

	// If there's an error finding nested JARs, or if none are found, then this is a definitive failure.
	// We wrap the original parsing error to provide full context.
	if jarJarErr != nil || len(jarJarEntries) == 0 {
		return FabricModJson{}, fmt.Errorf("parsing metadata for %s failed and it is not a valid container: %w", jarIdentifier, originalErr)
	}

	// Success! The JAR is a container. Create a synthetic metadata object for it.
	logBuffer.add(logging.LevelDebug, "ModLoader: %s is not a mod, but is a container for %d nested JAR(s).", jarIdentifier, len(jarJarEntries))

	// The placeholder version is required for the struct, but will not be used in dependency resolution.
	placeholderVersion, _ := version.Parse("0.0.0-container", false)
	containerFmj := FabricModJson{
		// Create a unique, identifiable ID for the container.
		ID:      fmt.Sprintf("container-%s", filepath.Base(jarIdentifier)),
		Version: VersionField{Version: placeholderVersion},
		Jars:    jarJarEntries,
	}

	return containerFmj, nil
}

// readVersionFromManifest finds and parses the META-INF/MANIFEST.MF file within a JAR
// to extract the value of the "Implementation-Version" attribute.
func (p *ModParser) readVersionFromManifest(zipReader *zip.Reader, jarIdentifier string) (string, error) {
	manifestFile := p.getZipFileEntry(zipReader, "META-INF/MANIFEST.MF")
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
// into the tool's internal FabricModJson format. It also looks for and parses
// a corresponding jarjar/metadata.json for nested JARs.
func (p *ModParser) parseNeoForgeModToml(zipReader *zip.Reader, tomlFile *zip.File, jarIdentifier string, logBuffer *logBuffer) (FabricModJson, error) {
	// 1. Read and parse the main TOML file
	tomlData, err := p.readZipFileEntry(tomlFile)
	if err != nil {
		return FabricModJson{}, fmt.Errorf("reading neoforge.mods.toml from %s: %w", jarIdentifier, err)
	}

	var rawToml neoForgeModsToml
	if err := toml.Unmarshal(tomlData, &rawToml); err != nil {
		return FabricModJson{}, fmt.Errorf("unmarshaling neoforge.mods.toml from %s: %w", jarIdentifier, err)
	}

	if len(rawToml.Mods) == 0 {
		return FabricModJson{}, fmt.Errorf("neoforge.mods.toml from %s contains no [[mods]] entries", jarIdentifier)
	}

	// 2. Translate the primary mod identity and "provides" list.
	// The first [[mods]] entry is considered the primary mod.
	primaryMod := rawToml.Mods[0]
	fmj := FabricModJson{
		ID:   primaryMod.ModID,
		Name: primaryMod.DisplayName,
	}

	mavenModVersionStr := primaryMod.Version
	if mavenModVersionStr == "${file.jarVersion}" {
		logBuffer.add(logging.LevelDebug, "ModLoader: Resolving dynamic version for %s from MANIFEST.MF", primaryMod.ModID)
		versionFromManifest, err := p.readVersionFromManifest(zipReader, jarIdentifier)
		if err != nil {
			logBuffer.add(logging.LevelWarn, "ModLoader: %v", err)
			versionFromManifest = "0.0.0"
		}
		mavenModVersionStr = versionFromManifest
	}

	// Leverage the existing VersionField JSON unmarshaler to parse the version string.
	// The string must be wrapped in quotes to be treated as a valid JSON string.
	modVersionStr, err := version.TranslateMavenVersion(mavenModVersionStr)
	if err != nil {
		return FabricModJson{}, fmt.Errorf("translating version '%s' for mod '%s' in %s: %w", primaryMod.Version, primaryMod.ModID, jarIdentifier, err)
	}
	if err := fmj.Version.UnmarshalJSON([]byte(fmt.Sprintf(`"%s"`, modVersionStr))); err != nil {
		return FabricModJson{}, fmt.Errorf("parsing version '%s' for mod '%s' in %s: %w", primaryMod.Version, primaryMod.ModID, jarIdentifier, err)
	}

	// Subsequent [[mods]] blocks are treated as "provides".
	if len(rawToml.Mods) > 1 {
		fmj.Provides = make([]string, 0, len(rawToml.Mods)-1)
		for _, providedMod := range rawToml.Mods[1:] {
			fmj.Provides = append(fmj.Provides, providedMod.ModID)
		}
	}

	// 3. Translate dependencies.
	fmj.Depends = make(VersionRanges)
	fmj.Recommends = make(VersionRanges)
	fmj.Breaks = make(VersionRanges)
	fmj.Conflicts = make(VersionRanges)

	if modDependencies, ok := rawToml.Dependencies[primaryMod.ModID]; ok {
		for _, dep := range modDependencies {
			if dep.ModID == "" {
				continue // Skip malformed dependencies with no ID.
			}

			// A single Maven range string can translate to multiple Fabric predicate strings (OR relationship).
			predicateStrings, err := version.TranslateMavenVersionRange(dep.VersionRange)
			if err != nil {
				return FabricModJson{}, fmt.Errorf("translating maven version range '%s' for dep '%s' in %s: %w", dep.VersionRange, dep.ModID, jarIdentifier, err)
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
				fmj.Depends[dep.ModID] = predicates
			case "optional":
				fmj.Recommends[dep.ModID] = predicates
			case "incompatible":
				fmj.Breaks[dep.ModID] = predicates
			case "discouraged":
				fmj.Conflicts[dep.ModID] = predicates
			}
		}
	}

	// 4. Look for and parse nested JARs from jarjar/metadata.json.
	// This is an optional file; failure to read or parse it should not halt mod loading.
	if jarJarFile := p.getZipFileEntry(zipReader, "META-INF/jarjar/metadata.json"); jarJarFile != nil {
		jarJarData, err := p.readZipFileEntry(jarJarFile)
		if err != nil {
			logBuffer.add(logging.LevelWarn, "ModLoader: Failed to read META-INF/jarjar/metadata.json in %s: %v", jarIdentifier, err)
		} else {
			var metadata jarJarMetadata
			if err := json.Unmarshal(jarJarData, &metadata); err != nil {
				logBuffer.add(logging.LevelWarn, "ModLoader: Failed to parse META-INF/jarjar/metadata.json in %s: %v", jarIdentifier, err)
			} else {
				// Pre-filter the JARs to only include those that are actual mods.
				validJars := make([]struct {
					File string `json:"file"`
				}, 0)
				for _, jarEntry := range metadata.Jars {
					isMod, err := p.isNestedJarANeoForgeMod(zipReader, jarEntry.Path)
					if err != nil {
						logBuffer.add(logging.LevelWarn, "ModLoader: Could not check nested JAR '%s' in '%s': %v", jarEntry.Path, jarIdentifier, err)
						continue
					}
					if isMod {
						validJars = append(validJars, struct {
							File string `json:"file"`
						}{File: jarEntry.Path})
					} else {
						logBuffer.add(logging.LevelDebug, "ModLoader: Skipping nested library '%s' in '%s' as it is not a NeoForge mod.", jarEntry.Path, jarIdentifier)
					}
				}
				logBuffer.add(logging.LevelDebug, "ModLoader: Found %d parsable nested mod(s) in jarjar/metadata.json for %s", len(validJars), jarIdentifier)
				fmj.Jars = validJars
			}
		}
	}

	// 5. Perform final validation, similar to the main parser.
	if fmj.ID == "" {
		return FabricModJson{}, fmt.Errorf("neoforge.mods.toml from %s has an empty mod ID in its primary [[mods]] entry", jarIdentifier)
	}

	return fmj, nil
}
