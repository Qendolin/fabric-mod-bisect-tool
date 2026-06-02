package mods

import (
	"archive/zip"
	"bytes"
	"fmt"
	"io"
	"path"

	"github.com/Qendolin/fabric-mod-bisect-tool/pkg/logging"
)

type ModParser struct {
	QuiltParsing    bool
	NeoForgeParsing bool
}

// ExtractModMetadata opens a JAR and extracts its top-level and nested mod files.
func (p *ModParser) ExtractModMetadata(jarPath, jarName string, logBuffer *logBuffer) (ModMetadata, []NestedModule, error) {
	zr, err := zip.OpenReader(jarPath)
	if err != nil {
		return ModMetadata{}, nil, fmt.Errorf("opening JAR %s as zip: %w", jarPath, err)
	}
	defer zr.Close()

	topLevelMetadata, err := p.parseModMetadataFromReader(&zr.Reader, jarName, logBuffer)
	if err != nil {
		return ModMetadata{}, nil, fmt.Errorf("parsing top-level metadata for %s: %w", jarName, err)
	}

	allNestedMods := []NestedModule{}
	for _, nestedJarEntry := range topLevelMetadata.Jars {
		if nestedJarEntry == "" {
			logBuffer.add(logging.LevelWarn, "ModLoader: Top-level mod '%s' has a nested JAR entry with an empty 'file' path. Skipping.", topLevelMetadata.ID)
			continue
		}

		foundMods, err := p.recursivelyParseNestedJar(&zr.Reader, nestedJarEntry, jarName, logBuffer)
		if err != nil {
			if p.NeoForgeParsing {
				logBuffer.add(logging.LevelDebug, "ModLoader: Skipping nested JAR '%s' in '%s' (likely a non-mod library): %v", nestedJarEntry, jarName, err)
			} else {
				logBuffer.add(logging.LevelWarn, "ModLoader: Failed to process nested JAR '%s' in '%s': %v", nestedJarEntry, jarName, err)
			}
			continue
		}
		allNestedMods = append(allNestedMods, foundMods...)
	}
	return topLevelMetadata, allNestedMods, nil
}

// recursivelyParseNestedJar parses a nested JAR and any of its own nested JARs.
func (p *ModParser) recursivelyParseNestedJar(parentZipReader *zip.Reader, pathInParent, currentPathPrefix string, logBuffer *logBuffer) ([]NestedModule, error) {
	fullPathInJar := path.Join(currentPathPrefix, pathInParent)

	var nestedZipFile *zip.File
	for _, f := range parentZipReader.File {
		if f.Name == pathInParent {
			nestedZipFile = f
			break
		}
	}
	if nestedZipFile == nil {
		return nil, fmt.Errorf("nested JAR '%s' not found in archive", fullPathInJar)
	}

	rc, err := nestedZipFile.Open()
	if err != nil {
		return nil, fmt.Errorf("opening nested JAR '%s': %w", fullPathInJar, err)
	}
	defer rc.Close()

	jarBytes, err := io.ReadAll(rc)
	if err != nil {
		return nil, fmt.Errorf("reading nested JAR '%s': %w", fullPathInJar, err)
	}

	bytesReader := bytes.NewReader(jarBytes)
	innerZipReader, err := zip.NewReader(bytesReader, int64(len(jarBytes)))
	if err != nil {
		return nil, fmt.Errorf("reading nested content '%s' as zip: %w", fullPathInJar, err)
	}

	currentModMetadata, err := p.parseModMetadataFromReader(innerZipReader, fullPathInJar, logBuffer)
	if err != nil {
		return nil, fmt.Errorf("parsing metadata from '%s': %w", fullPathInJar, err)
	}

	allFoundMods := []NestedModule{{Info: currentModMetadata, PathInJar: fullPathInJar}}

	for _, deeperJarEntry := range currentModMetadata.Jars {
		if deeperJarEntry == "" {
			continue
		}

		deeperNestedMods, err := p.recursivelyParseNestedJar(innerZipReader, deeperJarEntry, fullPathInJar, logBuffer)
		if err != nil {
			logBuffer.add(logging.LevelWarn, "ModLoader: Skipping deeper nested JAR '%s' within '%s': %v", deeperJarEntry, pathInParent, err)
			continue
		}
		allFoundMods = append(allFoundMods, deeperNestedMods...)
	}
	return allFoundMods, nil
}

type modManifestResult struct {
	File   *zip.File
	Path   string
	Loader Loader
}

func (p *ModParser) findModManifest(zipReader *zip.Reader) modManifestResult {
	// Prioritize NeoForge parsing if the flag is enabled
	if p.NeoForgeParsing {
		if file := getZipFileEntry(zipReader, "META-INF/neoforge.mods.toml"); file != nil {
			return modManifestResult{
				File:   file,
				Path:   "META-INF/neoforge.mods.toml",
				Loader: LoaderNeoForge,
			}
		}

		// Legacy NeoForge 1.20.1 (uses Forge toml)
		if file := getZipFileEntry(zipReader, "META-INF/mods.toml"); file != nil {
			return modManifestResult{
				File:   file,
				Path:   "META-INF/mods.toml",
				Loader: LoaderNeoForge,
			}
		}
	}

	if p.QuiltParsing {
		if file := getZipFileEntry(zipReader, "quilt.mod.json"); file != nil {
			return modManifestResult{
				File:   file,
				Path:   "quilt.mod.json",
				Loader: LoaderQuilt,
			}
		}
	}

	// TODO: In the future, add `if (p.FabricParsing || p.QuiltParsing)`
	if file := getZipFileEntry(zipReader, "fabric.mod.json"); file != nil {
		return modManifestResult{
			File:   file,
			Path:   "fabric.mod.json",
			Loader: LoaderFabric,
		}
	}

	return modManifestResult{}
}

// parseModMetadataFromReader parses the metadata of a mod JAR file.
// It attempts to find and parse the appropriate manifest file (fabric.mod.json, quilt.mod.json, or neoforge.mods.toml)
// based on the enabled parsing flags. If the JAR is not a mod but a container (containing nested mods via jarjar/metadata.json),
// it creates a synthetic ModMetadata object for the container.
func (p *ModParser) parseModMetadataFromReader(zipReader *zip.Reader, jarIdentifier string, logBuffer *logBuffer) (mm ModMetadata, err error) {
	manifest := p.findModManifest(zipReader)

	if !p.NeoForgeParsing && manifest.Loader == LoaderNone {
		// Non-mod jars are valid for NF, but not Fabric and Quilt
		return mm, fmt.Errorf("no mod manifest found in %s", jarIdentifier)
	}

	if p.NeoForgeParsing && (manifest.Loader == LoaderFabric || manifest.Loader == LoaderQuilt) {
		logBuffer.add(logging.LevelWarn, "ModLoader: NeoForge parsing is enabled but %s is missing a neoforge.mods.toml and will fall back to %s parsing.", jarIdentifier, manifest.Loader)
	} else if p.QuiltParsing && manifest.Loader == LoaderFabric {
		logBuffer.add(logging.LevelWarn, "ModLoader: Quilt parsing is enabled but %s is missing a quilt.mod.json and will fall back to Fabric parsing.", jarIdentifier)
	}

	if manifest.Loader == LoaderNeoForge {
		mm, err = p.parseNeoForgeModToml(zipReader, manifest, jarIdentifier, logBuffer)
		if err != nil {
			return
		}
	}

	if manifest.Loader == LoaderFabric || manifest.Loader == LoaderQuilt {
		mm, err = p.parseFabricModJson(zipReader, manifest, jarIdentifier, logBuffer)
		if err != nil {
			return
		}
	}

	if p.NeoForgeParsing {
		// Unlike fabric mods, nested jars from META-INF/jarjar/metadata.json are loaded here
		err = p.parseNeoForgeNestedJars(zipReader, &mm, jarIdentifier, logBuffer)
		if err != nil {
			return
		}
	}

	// At this point, mm is guaranteed to be populated because:
	// - If manifest.Loader is NeoForge/Fabric/Quilt, the respective parser filled mm
	// - If manifest.Loader is LoaderNone, we either returned an error or created a synthetic container

	// Validation
	if mm.ID == "" {
		return mm, fmt.Errorf("%s from %s has a missing mod ID", manifest.Path, jarIdentifier)
	}

	if mm.Version.Version == nil {
		return mm, fmt.Errorf("%s from %s is missing mandatory 'version' entry", manifest.Path, jarIdentifier)
	}

	if IsImplicitMod(mm.ID) {
		return mm, fmt.Errorf("mod from '%s' illegally uses reserved ID '%s'", jarIdentifier, mm.ID)
	}
	for _, providedID := range mm.Provides {
		if IsImplicitMod(providedID) {
			return mm, fmt.Errorf("mod '%s' from '%s' illegally provides reserved ID '%s'", mm.ID, jarIdentifier, providedID)
		}
	}

	return mm, nil
}

func readZipFileEntry(f *zip.File) ([]byte, error) {
	rc, err := f.Open()
	if err != nil {
		return nil, err
	}
	defer rc.Close()
	return io.ReadAll(rc)
}

func getZipFileEntry(r *zip.Reader, name string) *zip.File {
	for _, f := range r.File {
		if f.Name == name {
			return f
		}
	}
	return nil
}
