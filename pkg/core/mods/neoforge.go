package mods

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"fmt"
	"io"

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

	// Leverage the existing VersionField JSON unmarshaler to parse the version string.
	// The string must be wrapped in quotes to be treated as a valid JSON string.
	modVersionStr, err := version.TranslateMavenVersion(primaryMod.Version)
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
					return FabricModJson{}, fmt.Errorf("parsing translated predicate '%s' (from maven range '%s') for dep '%s' in %s: %w", pStr, dep.VersionRange, dep.ModID, jarIdentifier, err)
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
