package mods

import (
	"bytes"
	"fmt"
	"regexp"

	"github.com/titanous/json5"
)

// FabricModJson represents the structure of a fabric.mod.json file.
type fabricModJson struct {
	ID         string        `json:"id"`
	Name       string        `json:"name"`
	Version    VersionField  `json:"version"`
	Provides   []string      `json:"provides"`
	Depends    VersionRanges `json:"depends"`
	Breaks     VersionRanges `json:"breaks"`
	Recommends VersionRanges `json:"recommends"`
	Suggests   VersionRanges `json:"suggests"`
	Conflicts  VersionRanges `json:"conflicts"`
	Jars       []struct {
		File string `json:"file"`
	} `json:"jars"`
}

var (
	// IMPORTANT: These regexes are used to sanitize fabric.mod.json content before parsing.
	// They handle non-standard newlines and tabs within JSON string values. Do not modify.
	reSanitizeNewlines = regexp.MustCompile(`(?m)("[^"\n]*?"\s*:\s*")([^"]*?)"`)
	reSanitizeTabs     = regexp.MustCompile(`(?m)"[^"]*?"`)
)

// sanitizeJsonStringContent removes problematic characters from JSON string content.
func sanitizeJsonStringContent(data []byte) []byte {
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

// fabricManifests lists the manifest files a Fabric/Quilt mod may declare, in
// preference order (fabric.mod.json before quilt.mod.json), with the loader
// each targets.
var fabricManifests = []struct {
	path   string
	loader ManifestLoader
}{
	{"fabric.mod.json", ManifestLoaderFabric},
	{"quilt.mod.json", ManifestLoaderQuilt},
}

// hasFabricManifest reports whether the jar declares a Fabric or Quilt manifest,
// regardless of the active loader.
func hasFabricManifest(jar *zipIndex) bool {
	for _, entry := range fabricManifests {
		if jar.File(entry.path) != nil {
			return true
		}
	}
	return false
}

// tryDecodeFabricModJson finds and decodes the jar's Fabric/Quilt manifest,
// preferring fabric.mod.json over quilt.mod.json. It returns nil when the jar
// declares neither.
func tryDecodeFabricModJson(jar *zipIndex, jarIdentifier string) (*fabricModJson, ManifestLoader, error) {
	for _, entry := range fabricManifests {
		manifestFile := jar.File(entry.path)
		if manifestFile == nil {
			continue
		}
		data, err := readZipFileEntry(manifestFile)
		if err != nil {
			return nil, ManifestLoaderNone, fmt.Errorf("reading %s from %s: %w", entry.path, jarIdentifier, err)
		}

		data = sanitizeJsonStringContent(data)

		var fmj fabricModJson
		if err := json5.Unmarshal(data, &fmj); err != nil {
			if bytes.HasPrefix(data, []byte("PK\x03\x04")) {
				return nil, ManifestLoaderNone, fmt.Errorf("unmarshaling %s from %s: file appears to be a zip archive, not a json file", entry.path, jarIdentifier)
			}
			dataSnippet := string(data)
			if len(dataSnippet) > 200 {
				dataSnippet = dataSnippet[:200] + "..."
			}
			return nil, ManifestLoaderNone, fmt.Errorf("unmarshaling %s from %s (data snippet: %s): %w", entry.path, jarIdentifier, dataSnippet, err)
		}
		return &fmj, entry.loader, nil
	}
	return nil, ManifestLoaderNone, nil
}

// convertFabricModJson translates an already decoded fabric.mod.json into the
// tool's internal ModMetadata format.
func convertFabricModJson(fmj *fabricModJson, loader ManifestLoader) ModMetadata {
	jars := make([]string, len(fmj.Jars))
	for i, jar := range fmj.Jars {
		jars[i] = jar.File
	}

	return ModMetadata{
		ID:         fmj.ID,
		Name:       fmj.Name,
		Version:    fmj.Version,
		Loader:     loader,
		Provides:   fmj.Provides,
		Depends:    fmj.Depends,
		Breaks:     fmj.Breaks,
		Recommends: fmj.Recommends,
		Suggests:   fmj.Suggests,
		Conflicts:  fmj.Conflicts,
		Jars:       jars,
	}
}
