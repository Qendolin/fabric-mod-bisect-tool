package mods

import (
	"archive/zip"
	"bytes"
	"fmt"
	"io"
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
	QuiltParsing bool
}

// ExtractModMetadata opens a JAR and extracts its top-level and nested fabric.mod.json files.
func (p *ModParser) ExtractModMetadata(jarPath string, logBuffer *logBuffer) (FabricModJson, []FabricModJson, error) {
	zr, err := zip.OpenReader(jarPath)
	if err != nil {
		return FabricModJson{}, nil, fmt.Errorf("opening JAR %s as zip: %w", jarPath, err)
	}
	defer zr.Close()

	topLevelFmj, err := p.parseFabricModJsonFromReader(&zr.Reader, jarPath, logBuffer)
	if err != nil {
		return FabricModJson{}, nil, fmt.Errorf("parsing top-level fabric.mod.json for %s: %w", jarPath, err)
	}

	var allNestedMods []FabricModJson
	for _, nestedJarEntry := range topLevelFmj.Jars {
		if nestedJarEntry.File == "" {
			logBuffer.add(logging.LevelWarn, "ModLoader: Top-level mod '%s' has a nested JAR entry with an empty 'file' path. Skipping.", topLevelFmj.ID)
			continue
		}

		foundMods, err := p.recursivelyParseNestedJar(&zr.Reader, nestedJarEntry.File, logBuffer)
		if err != nil {
			logBuffer.add(logging.LevelWarn, "ModLoader: Failed to process nested JAR '%s' in '%s': %v", nestedJarEntry.File, filepath.Base(jarPath), err)
			continue
		}
		allNestedMods = append(allNestedMods, foundMods...)
	}
	return topLevelFmj, allNestedMods, nil
}

// recursivelyParseNestedJar parses a nested JAR and any of its own nested JARs.
func (p *ModParser) recursivelyParseNestedJar(parentZipReader *zip.Reader, pathInParent string, logBuffer *logBuffer) ([]FabricModJson, error) {
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
		return nil, fmt.Errorf("parsing metadata from '%s': %w", pathInParent, err)
	}

	allFoundMods := []FabricModJson{currentFmj}

	for _, deeperJarEntry := range currentFmj.Jars {
		if deeperJarEntry.File == "" {
			continue
		}
		deeperNestedMods, err := p.recursivelyParseNestedJar(innerZipReader, deeperJarEntry.File, logBuffer)
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
