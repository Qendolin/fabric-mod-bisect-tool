package mods

import (
	"archive/zip"
	"bytes"
	"fmt"
	"io"
	"path"
	"path/filepath"
	"regexp"

	"github.com/Qendolin/fabric-mod-bisect-tool/pkg/logging"
	"github.com/titanous/json5"
)

var (
	// IMPORTANT: These regexes are used to sanitize fabric.mod.json content before parsing.
	// They handle non-standard newlines and tabs within JSON string values. Do not modify.
	reSanitizeNewlines = regexp.MustCompile(`(?m)("[^"\n]*?"\s*:\s*")([^"]*?)"`)
	reSanitizeTabs     = regexp.MustCompile(`(?m)"[^"]*?"`)
)

type ModParser struct {
	QuiltParsing    bool
	NeoForgeParsing bool
}

// ExtractModMetadata opens a JAR and extracts its top-level and nested fabric.mod.json files.
func (p *ModParser) ExtractModMetadata(jarPath string, logBuffer *logBuffer) (FabricModJson, []NestedModule, error) {
	zr, err := zip.OpenReader(jarPath)
	if err != nil {
		return FabricModJson{}, nil, fmt.Errorf("opening JAR %s as zip: %w", jarPath, err)
	}
	defer zr.Close()

	topLevelFmj, err := p.parseFabricModJsonFromReader(&zr.Reader, jarPath, logBuffer)
	if err != nil {
		// If standard parsing fails, we only attempt the container fallback if NeoForge parsing is enabled.
		if !p.NeoForgeParsing {
			return FabricModJson{}, nil, fmt.Errorf("parsing top-level metadata for %s: %w", jarPath, err)
		}

		// In NeoForge mode, a parsing failure might just mean it's a container.
		topLevelFmj, err = p.tryParseAsContainer(&zr.Reader, jarPath, err, logBuffer)
		if err != nil {
			// If it's also not a container, then it's a final failure.
			return FabricModJson{}, nil, err
		}
	} else if p.NeoForgeParsing {
		// If it *is* a mod, it might *also* contain jarjar nested JARs. Append them.
		jarJarEntries, _ := p.parseJarJarMetadata(&zr.Reader, jarPath, logBuffer)
		if len(jarJarEntries) > 0 {
			topLevelFmj.Jars = append(topLevelFmj.Jars, jarJarEntries...)
		}
	}

	var allNestedMods []NestedModule
	for _, nestedJarEntry := range topLevelFmj.Jars {
		if nestedJarEntry.File == "" {
			logBuffer.add(logging.LevelWarn, "ModLoader: Top-level mod '%s' has a nested JAR entry with an empty 'file' path. Skipping.", topLevelFmj.ID)
			continue
		}

		if p.NeoForgeParsing {
			isMod, checkErr := p.isNestedJarANeoForgeMod(&zr.Reader, nestedJarEntry.File)
			if checkErr != nil {
				logBuffer.add(logging.LevelWarn, "ModLoader: Failed to check nested JAR '%s' in '%s': %v. Skipping.", nestedJarEntry.File, filepath.Base(jarPath), checkErr)
				continue
			}
			if !isMod {
				logBuffer.add(logging.LevelDebug, "ModLoader: Skipping nested library '%s' in '%s' as it has no neoforge.mods.toml.", nestedJarEntry.File, filepath.Base(jarPath))
				continue
			}
		}

		foundMods, err := p.recursivelyParseNestedJar(&zr.Reader, nestedJarEntry.File, "", logBuffer)
		if err != nil {
			logBuffer.add(logging.LevelWarn, "ModLoader: Failed to process nested JAR '%s' in '%s': %v", nestedJarEntry.File, filepath.Base(jarPath), err)
			continue
		}
		allNestedMods = append(allNestedMods, foundMods...)
	}
	return topLevelFmj, allNestedMods, nil
}

// recursivelyParseNestedJar parses a nested JAR and any of its own nested JARs.
func (p *ModParser) recursivelyParseNestedJar(parentZipReader *zip.Reader, pathInParent, currentPathPrefix string, logBuffer *logBuffer) ([]NestedModule, error) {
	var nestedZipFile *zip.File
	for _, f := range parentZipReader.File {
		if f.Name == pathInParent {
			nestedZipFile = f
			break
		}
	}
	if nestedZipFile == nil {
		return nil, fmt.Errorf("nested JAR '%s' not found in archive", pathInParent)
	}

	rc, err := nestedZipFile.Open()
	if err != nil {
		return nil, fmt.Errorf("opening nested JAR '%s': %w", pathInParent, err)
	}
	defer rc.Close()

	jarBytes, err := io.ReadAll(rc)
	if err != nil {
		return nil, fmt.Errorf("reading nested JAR '%s': %w", pathInParent, err)
	}

	bytesReader := bytes.NewReader(jarBytes)
	innerZipReader, err := zip.NewReader(bytesReader, int64(len(jarBytes)))
	if err != nil {
		return nil, fmt.Errorf("treating nested content '%s' as zip: %w", pathInParent, err)
	}

	currentFmj, err := p.parseFabricModJsonFromReader(innerZipReader, pathInParent, logBuffer)
	if err != nil {
		// Just as with the top level, only attempt the container fallback in NeoForge mode.
		if !p.NeoForgeParsing {
			return nil, fmt.Errorf("parsing metadata from '%s': %w", pathInParent, err)
		}
		currentFmj, err = p.tryParseAsContainer(innerZipReader, pathInParent, err, logBuffer)
		if err != nil {
			return nil, err
		}
	} else if p.NeoForgeParsing {
		// Append jarjar entries if the nested JAR is a real mod.
		jarJarEntries, _ := p.parseJarJarMetadata(innerZipReader, pathInParent, logBuffer)
		if len(jarJarEntries) > 0 {
			currentFmj.Jars = append(currentFmj.Jars, jarJarEntries...)
		}
	}

	fullPathInJar := path.Join(currentPathPrefix, pathInParent)
	allFoundMods := []NestedModule{{Info: currentFmj, PathInJar: fullPathInJar}}

	for _, deeperJarEntry := range currentFmj.Jars {
		if deeperJarEntry.File == "" {
			continue
		}

		if p.NeoForgeParsing {
			isMod, checkErr := p.isNestedJarANeoForgeMod(innerZipReader, deeperJarEntry.File)
			if checkErr != nil {
				logBuffer.add(logging.LevelWarn, "ModLoader: Failed to check deeper nested JAR '%s' within '%s': %v. Skipping.", deeperJarEntry.File, pathInParent, checkErr)
				continue
			}
			if !isMod {
				logBuffer.add(logging.LevelDebug, "ModLoader: Skipping nested library '%s' within '%s' as it has no neoforge.mods.toml.", deeperJarEntry.File, pathInParent)
				continue
			}
		}

		deeperNestedMods, err := p.recursivelyParseNestedJar(innerZipReader, deeperJarEntry.File, fullPathInJar, logBuffer)
		if err != nil {
			logBuffer.add(logging.LevelWarn, "ModLoader: Skipping deeper nested JAR '%s' within '%s': %v", deeperJarEntry.File, pathInParent, err)
			continue
		}
		allFoundMods = append(allFoundMods, deeperNestedMods...)
	}
	return allFoundMods, nil
}

// parseFabricModJsonFromReader searches for and unmarshals fabric.mod.json from a zip reader.
// It also validates that the mod does not provide any reserved/implicit IDs.
func (p *ModParser) parseFabricModJsonFromReader(zipReader *zip.Reader, jarIdentifier string, logBuffer *logBuffer) (FabricModJson, error) {
	// Prioritize NeoForge parsing if the flag is enabled and the file exists.
	if p.NeoForgeParsing {
		if neoForgeFile := p.getZipFileEntry(zipReader, "META-INF/neoforge.mods.toml"); neoForgeFile != nil {
			// Delegate all parsing to the specialist NeoForge function, which handles its own format
			// and its specific method of declaring nested JARs.
			return p.parseNeoForgeModToml(zipReader, neoForgeFile, jarIdentifier, logBuffer)
		} else {
			logBuffer.add(logging.LevelWarn, "ModLoader: NeoForge parsing is enabled but neoforge.mods.toml was not found for %s. Falling back to fabric.mod.json.", jarIdentifier)
		}
	}

	// First, determine which metadata file to use, with quilt taking priority if enabled.
	targetFileName := "fabric.mod.json"
	if p.QuiltParsing {
		if f := p.getZipFileEntry(zipReader, "quilt.mod.json"); f != nil {
			targetFileName = "quilt.mod.json"
			logBuffer.add(logging.LevelDebug, "ModLoader: %s has a quilt.mod.json which takes priority over fabric.mod.json", jarIdentifier)
		}
	}

	// Now, find the actual file entry for the chosen target.
	file := p.getZipFileEntry(zipReader, targetFileName)
	if file == nil {
		return FabricModJson{}, fmt.Errorf("%s not found in %s", targetFileName, jarIdentifier)
	}

	fmjData, err := p.readZipFileEntry(file)
	if err != nil {
		return FabricModJson{}, fmt.Errorf("reading fabric.mod.json from %s: %w", jarIdentifier, err)
	}
	fmjData = p.sanitizeJsonStringContent(fmjData)

	var fmj FabricModJson
	if err := json5.Unmarshal(fmjData, &fmj); err != nil {
		if bytes.HasPrefix(fmjData, []byte("PK\x03\x04")) {
			return FabricModJson{}, fmt.Errorf("unmarshaling %s from %s: file appears to be a zip archive, not a json file", targetFileName, jarIdentifier)
		}
		dataSnippet := string(fmjData)
		if len(dataSnippet) > 200 {
			dataSnippet = dataSnippet[:200] + "..."
		}
		return FabricModJson{}, fmt.Errorf("unmarshaling %s from %s (data snippet: %s): %w", targetFileName, jarIdentifier, dataSnippet, err)
	}

	if fmj.ID == "" {
		return FabricModJson{}, fmt.Errorf("fabric.mod.json from %s has empty mod ID", jarIdentifier)
	}

	if fmj.Version.Version == nil {
		return FabricModJson{}, fmt.Errorf("fabric.mod.json from %s is missing mandatory 'version' field", jarIdentifier)
	}

	if IsImplicitMod(fmj.ID) {
		return FabricModJson{}, fmt.Errorf("mod from '%s' illegally uses reserved ID '%s'", jarIdentifier, fmj.ID)
	}
	for _, providedID := range fmj.Provides {
		if IsImplicitMod(providedID) {
			return FabricModJson{}, fmt.Errorf("mod '%s' from '%s' illegally provides reserved ID '%s'", fmj.ID, jarIdentifier, providedID)
		}
	}

	return fmj, nil
}

func (p *ModParser) getZipFileEntry(r *zip.Reader, name string) *zip.File {
	for _, f := range r.File {
		if f.Name == name {
			return f
		}
	}
	return nil
}

// sanitizeJsonStringContent removes problematic characters from JSON string content.
func (p *ModParser) sanitizeJsonStringContent(data []byte) []byte {
	sanitizedData := reSanitizeNewlines.ReplaceAllFunc(data, func(match []byte) []byte {
		submatches := reSanitizeNewlines.FindSubmatch(match)
		if len(submatches) < 3 {
			return match
		}
		prefix, value := submatches[1], submatches[2]
		escapedValue := bytes.ReplaceAll(value, []byte("\n"), []byte("\\n"))
		escapedValue = bytes.ReplaceAll(escapedValue, []byte("\r"), []byte{})
		return append(append(prefix, escapedValue...), '"')
	})
	sanitizedData = reSanitizeTabs.ReplaceAllFunc(sanitizedData, func(match []byte) []byte {
		if len(match) <= 2 {
			return match
		}
		innerContent := match[1 : len(match)-1]
		escapedInnerContent := bytes.ReplaceAll(innerContent, []byte("\t"), []byte("\\t"))
		return append(append([]byte{'"'}, escapedInnerContent...), '"')
	})
	return sanitizedData
}

// readZipFileEntry reads the content of a file entry within a ZIP archive.
func (p *ModParser) readZipFileEntry(f *zip.File) ([]byte, error) {
	rc, err := f.Open()
	if err != nil {
		return nil, err
	}
	defer rc.Close()
	return io.ReadAll(rc)
}
