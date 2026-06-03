package mods

import (
	"archive/zip"
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

func (p *ModParser) parseFabricModJson(zipReader *zip.Reader, manifest modManifestResult, jarIdentifier string, logBuffer *logBuffer) (mm ModMetadata, err error) {
	fmjBytes, err := readZipFileEntry(manifest.File)
	if err != nil {
		return mm, fmt.Errorf("reading %s from %s: %w", manifest.Path, jarIdentifier, err)
	}

	fmjBytes = sanitizeJsonStringContent(fmjBytes)

	var fmj fabricModJson
	if err := json5.Unmarshal(fmjBytes, &fmj); err != nil {
		if bytes.HasPrefix(fmjBytes, []byte("PK\x03\x04")) {
			return mm, fmt.Errorf("unmarshaling %s from %s: file appears to be a zip archive, not a json file", manifest.Path, jarIdentifier)
		}
		dataSnippet := string(fmjBytes)
		if len(dataSnippet) > 200 {
			dataSnippet = dataSnippet[:200] + "..."
		}
		return mm, fmt.Errorf("unmarshaling %s from %s (data snippet: %s): %w", manifest.Path, jarIdentifier, dataSnippet, err)
	}

	jars := make([]string, len(fmj.Jars))
	for i, jar := range fmj.Jars {
		jars[i] = jar.File
	}

	return ModMetadata{
		ID:         fmj.ID,
		Name:       fmj.Name,
		Version:    fmj.Version,
		Loader:     manifest.Loader,
		Provides:   fmj.Provides,
		Depends:    fmj.Depends,
		Breaks:     fmj.Breaks,
		Recommends: fmj.Recommends,
		Suggests:   fmj.Suggests,
		Conflicts:  fmj.Conflicts,
		Jars:       jars,
	}, nil
}
