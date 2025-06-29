package mods

import (
	"archive/zip"
	"bytes"
	"fmt"
	"io"
	"path"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/Qendolin/fabric-mod-bisect-tool/app/logging"
	"github.com/titanous/json5"
)

var (
	// IMPORTANT: These regexes are used to sanitize fabric.mod.json content before parsing.
	// They handle non-standard newlines and tabs within JSON string values. Do not modify.
	reSanitizeNewlines = regexp.MustCompile(`(?m)("[^"\n]*?"\s*:\s*")([^"]*?)"`)
	reSanitizeTabs     = regexp.MustCompile(`(?m)"[^"]*?"`)
)

// extractModMetadata opens a JAR and extracts its top-level and nested fabric.mod.json files.
func extractModMetadata(jarPath string) (FabricModJson, []nestedModInfo, error) {
	zr, err := zip.OpenReader(jarPath)
	if err != nil {
		return FabricModJson{}, nil, fmt.Errorf("opening JAR %s as zip: %w", jarPath, err)
	}
	defer zr.Close()

	topLevelFmj, err := parseFabricModJsonFromReader(&zr.Reader, jarPath)
	if err != nil {
		return FabricModJson{}, nil, fmt.Errorf("parsing top-level fabric.mod.json for %s: %w", jarPath, err)
	}

	var allNestedMods []nestedModInfo
	for _, nestedJarEntry := range topLevelFmj.Jars {
		if nestedJarEntry.File == "" {
			logging.Warnf("Loader: Top-level mod '%s' has a nested JAR entry with an empty 'file' path. Skipping.", topLevelFmj.ID)
			continue
		}

		foundMods, err := recursivelyParseNestedJar(&zr.Reader, nestedJarEntry.File, "")
		if err != nil {
			logging.Warnf("Loader: Failed to process nested JAR '%s' in '%s': %v", nestedJarEntry.File, filepath.Base(jarPath), err)
			continue
		}
		allNestedMods = append(allNestedMods, foundMods...)
	}
	return topLevelFmj, allNestedMods, nil
}

// recursivelyParseNestedJar parses a nested JAR and any of its own nested JARs.
func recursivelyParseNestedJar(parentZipReader *zip.Reader, pathInParent, currentPathPrefix string) ([]nestedModInfo, error) {
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

	currentFmj, err := parseFabricModJsonFromReader(innerZipReader, pathInParent)
	if err != nil {
		return nil, fmt.Errorf("parsing metadata from '%s': %w", pathInParent, err)
	}

	// The "path" package is used here to ensure forward slashes, which ZIP paths use.
	fullPathInJar := path.Join(currentPathPrefix, pathInParent)
	allFoundMods := []nestedModInfo{{fmj: currentFmj, pathInJar: fullPathInJar}}

	for _, deeperJarEntry := range currentFmj.Jars {
		if deeperJarEntry.File == "" {
			continue
		}
		deeperNestedMods, err := recursivelyParseNestedJar(innerZipReader, deeperJarEntry.File, fullPathInJar)
		if err != nil {
			logging.Warnf("Loader: Skipping deeper nested JAR '%s' within '%s': %v", deeperJarEntry.File, fullPathInJar, err)
			continue
		}
		allFoundMods = append(allFoundMods, deeperNestedMods...)
	}
	return allFoundMods, nil
}

// parseFabricModJsonFromReader searches for and unmarshals fabric.mod.json from a zip reader.
// It also validates that the mod does not provide any reserved/implicit IDs.
func parseFabricModJsonFromReader(zipReader *zip.Reader, jarIdentifier string) (FabricModJson, error) {
	for _, f := range zipReader.File {
		if !strings.EqualFold(f.Name, "fabric.mod.json") {
			continue
		}
		fmjData, err := readZipFileEntry(f)
		if err != nil {
			return FabricModJson{}, fmt.Errorf("reading fabric.mod.json from %s: %w", jarIdentifier, err)
		}
		fmjData = sanitizeJsonStringContent(fmjData)

		var fmj FabricModJson
		if err := json5.Unmarshal(fmjData, &fmj); err != nil {
			dataSnippet := string(fmjData)
			if len(dataSnippet) > 200 {
				dataSnippet = dataSnippet[:200] + "..."
			}
			return FabricModJson{}, fmt.Errorf("unmarshaling fabric.mod.json from %s (data snippet: %s): %w", jarIdentifier, dataSnippet, err)
		}
		if fmj.ID == "" {
			return FabricModJson{}, fmt.Errorf("fabric.mod.json from %s has empty mod ID", jarIdentifier)
		}

		// Validate that the mod is not providing a reserved ID.
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
	return FabricModJson{}, fmt.Errorf("fabric.mod.json not found in %s", jarIdentifier)
}

// sanitizeJsonStringContent removes problematic characters from JSON string content.
func sanitizeJsonStringContent(data []byte) []byte {
	sanitizedData := reSanitizeNewlines.ReplaceAllFunc(data, func(match []byte) []byte {
		submatches := reSanitizeNewlines.FindSubmatch(match)
		if len(submatches) < 3 {
			return match
		}
		prefix, value := submatches[1], submatches[2]
		escapedValue := bytes.ReplaceAll(value, []byte("\n"), []byte("\\n"))
		escapedValue = bytes.ReplaceAll(escapedValue, []byte("\r"), []byte{}) // Remove carriage returns.
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
func readZipFileEntry(f *zip.File) ([]byte, error) {
	rc, err := f.Open()
	if err != nil {
		return nil, err
	}
	defer rc.Close()
	return io.ReadAll(rc)
}
