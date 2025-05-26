package app

import (
	"archive/zip"
	"bytes"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/titanous/json5"
	sv "golang.org/x/mod/semver"
)

var ErrRenameFailedSkippable = errors.New("rename failed, potentially skippable (e.g., file in use)")

type FabricModJson struct {
	ID       string                 `json:"id"`
	Name     string                 `json:"name"`
	Version  string                 `json:"version"`
	Provides []string               `json:"provides"`
	Depends  map[string]interface{} `json:"depends"`
	Jars     []struct {
		File string `json:"file"`
	} `json:"jars"`
}

type Mod struct {
	Path              string
	BaseFilename      string
	FabricInfo        FabricModJson
	IsInitiallyActive bool
	IsCurrentlyActive bool
	ConfirmedGood     bool
	NestedModules     []FabricModJson   // Metadata of JARs nested within this Mod
	EffectiveProvides map[string]string // All IDs this Mod provides (directly or via nesting) mapped to their version
}

func (m *Mod) ModID() string {
	return m.FabricInfo.ID
}

func (m *Mod) FriendlyName() string {
	if m.FabricInfo.Name != "" {
		return m.FabricInfo.Name
	}
	return m.FabricInfo.ID
}

func LoadMods(modsDir string, progressReport func(fileNameBeingProcessed string)) (
	allMods map[string]*Mod, potentialProviders PotentialProvidersMap, sortedModIDs []string, err error) {

	allMods = make(map[string]*Mod)
	potentialProviders = make(PotentialProvidersMap)

	addImplicitProvides(potentialProviders)

	files, readErr := os.ReadDir(modsDir)
	if readErr != nil {
		return nil, nil, nil, fmt.Errorf("reading mods directory %s: %w", modsDir, readErr)
	}

	// Pass 1: Parse all JARs, gather candidates, and store full metadata (including nested)
	type fullModData struct {
		mod        *Mod
		nestedFmjs []FabricModJson
	}
	parsedCandidates := make(map[string][]fullModData) // modID -> list of fullModData

	for _, file := range files {
		if file.IsDir() {
			continue
		}
		fileName := file.Name()
		isJar := strings.HasSuffix(strings.ToLower(fileName), ".jar")
		isDisabledJar := strings.HasSuffix(strings.ToLower(fileName), ".jar.disabled")
		if !isJar && !isDisabledJar {
			continue
		}

		if progressReport != nil {
			progressReport(fileName)
		}
		fullPath := filepath.Join(modsDir, fileName)
		var baseFilenameForRename string
		if isJar {
			baseFilenameForRename = strings.TrimSuffix(fileName, ".jar")
		} else {
			baseFilenameForRename = strings.TrimSuffix(fileName, ".jar.disabled")
		}

		topLevelFmj, nestedFmjs, errExtract := extractModMetadata(fullPath, progressReport) // Pass progressReport here
		if errExtract != nil {
			log.Printf("%sSkipping JAR %s: error extracting metadata: %v", LogWarningPrefix, fileName, errExtract)
			continue
		}
		if topLevelFmj.ID == "" {
			log.Printf("%sSkipping JAR %s: top-level mod ID is empty.", LogWarningPrefix, fileName)
			continue
		}
		currentMod := &Mod{
			Path: fullPath, BaseFilename: baseFilenameForRename, FabricInfo: topLevelFmj,
			IsInitiallyActive: isJar, IsCurrentlyActive: isJar,
			// NestedModules and EffectiveProvides will be set later for the winner
		}
		parsedCandidates[currentMod.ModID()] = append(parsedCandidates[currentMod.ModID()], fullModData{mod: currentMod, nestedFmjs: nestedFmjs})
	}

	// Pass 2: Resolve conflicts for top-level mod IDs and populate allMods with winners
	for modID, candidatesData := range parsedCandidates {
		if len(candidatesData) == 0 {
			continue
		}

		winnerData := candidatesData[0]
		if len(candidatesData) > 1 {
			// Log conflict and determine winner (compareVersions is in resolver.go, needs to be accessible or passed)
			for i := 1; i < len(candidatesData); i++ {
				if compareVersions(candidatesData[i].mod.FabricInfo.Version, winnerData.mod.FabricInfo.Version) > 0 {
					winnerData = candidatesData[i]
				}
			}
			// Log winner
		}

		// Store winner, and its fully parsed nested modules
		winnerMod := winnerData.mod
		winnerMod.NestedModules = winnerData.nestedFmjs // Store parsed nested data on the winner
		allMods[modID] = winnerMod

		for _, cData := range candidatesData { // Disable non-winners
			if cData.mod.Path != winnerMod.Path && cData.mod.IsInitiallyActive {
				if errDisable := disablePhysicalFile(modsDir, cData.mod.BaseFilename, cData.mod.Path); errDisable != nil {
					log.Printf("%sError disabling non-winning duplicate %s: %v", LogErrorPrefix, cData.mod.Path, errDisable)
				}
			}
		}
	}

	// Pass 3: Populate PotentialProvidersMap and Mod.EffectiveProvides using ONLY winning mods
	for _, mod := range allMods {
		mod.EffectiveProvides = make(map[string]string)

		// From top-level mod itself
		updateEffectiveProvides(mod.EffectiveProvides, mod.FabricInfo.ID, mod.FabricInfo.Version)
		for _, p := range mod.FabricInfo.Provides {
			updateEffectiveProvides(mod.EffectiveProvides, p, mod.FabricInfo.Version)
		}
		addSingleProviderInfo(potentialProviders, mod.FabricInfo.ID, ProviderInfo{
			TopLevelModID: mod.FabricInfo.ID, VersionOfProvidedItem: mod.FabricInfo.Version,
			IsDirectProvide: true, TopLevelModVersion: mod.FabricInfo.Version,
		})
		for _, providedItem := range mod.FabricInfo.Provides {
			addSingleProviderInfo(potentialProviders, providedItem, ProviderInfo{
				TopLevelModID: mod.FabricInfo.ID, VersionOfProvidedItem: mod.FabricInfo.Version,
				IsDirectProvide: true, TopLevelModVersion: mod.FabricInfo.Version,
			})
		}

		// From its nested modules (already parsed and stored in mod.NestedModules)
		for _, nestedFmj := range mod.NestedModules {
			updateEffectiveProvides(mod.EffectiveProvides, nestedFmj.ID, nestedFmj.Version)
			for _, p := range nestedFmj.Provides {
				updateEffectiveProvides(mod.EffectiveProvides, p, nestedFmj.Version)
			}
			addSingleProviderInfo(potentialProviders, nestedFmj.ID, ProviderInfo{
				TopLevelModID: mod.FabricInfo.ID, VersionOfProvidedItem: nestedFmj.Version,
				IsDirectProvide: false, TopLevelModVersion: mod.FabricInfo.Version,
			})
			for _, providedItem := range nestedFmj.Provides {
				addSingleProviderInfo(potentialProviders, providedItem, ProviderInfo{
					TopLevelModID: mod.FabricInfo.ID, VersionOfProvidedItem: nestedFmj.Version,
					IsDirectProvide: false, TopLevelModVersion: mod.FabricInfo.Version,
				})
			}
		}
	}

	// Optional: Pre-sort PotentialProvidersMap lists here (Optimization C)
	for _, infos := range potentialProviders {
		sort.Slice(infos, func(i, j int) bool {
			// Sort by: IsDirectProvide (true first), VersionOfProvidedItem (desc), TopLevelModVersion (desc)
			if infos[i].IsDirectProvide != infos[j].IsDirectProvide {
				return infos[i].IsDirectProvide // True (direct) comes before false (nested)
			}
			compItemVer := compareVersions(infos[i].VersionOfProvidedItem, infos[j].VersionOfProvidedItem)
			if compItemVer != 0 {
				return compItemVer > 0 // Higher item version first
			}
			return compareVersions(infos[i].TopLevelModVersion, infos[j].TopLevelModVersion) > 0 // Higher top-level mod version first
		})
	}

	for id := range allMods {
		sortedModIDs = append(sortedModIDs, id)
	}
	sort.Strings(sortedModIDs)
	return allMods, potentialProviders, sortedModIDs, nil
}

// updateEffectiveProvides adds/updates an entry in a mod's EffectiveProvides map.
// It ensures that for any given providedID, the highest version is stored.
func updateEffectiveProvides(effectiveProvides map[string]string, providedID, version string) {
	if providedID == "" {
		return
	}
	if existingVersion, ok := effectiveProvides[providedID]; ok {
		if compareVersions(version, existingVersion) > 0 {
			effectiveProvides[providedID] = version
		}
	} else {
		effectiveProvides[providedID] = version
	}
}

func disablePhysicalFile(modsDir, baseFilename, currentPath string) error {
	if !strings.HasSuffix(strings.ToLower(currentPath), ".jar") {
		return nil
	}
	disabledPath := filepath.Join(modsDir, baseFilename+".jar.disabled")

	if _, err := os.Stat(currentPath); os.IsNotExist(err) {
		return fmt.Errorf("cannot disable non-winner: active file %s does not exist", currentPath)
	}

	if _, err := os.Stat(disabledPath); err == nil {
		if remErr := os.Remove(disabledPath); remErr != nil {
			log.Printf("%sFailed to remove existing .jar.disabled file %s: %v. Rename may fail.", LogErrorPrefix, disabledPath, remErr)
		}
	}

	err := os.Rename(currentPath, disabledPath)
	if err != nil {
		return fmt.Errorf("%w: disabling non-winning duplicate %s: %v", ErrRenameFailedSkippable, baseFilename, err)
	}
	log.Printf("%sDisabled non-winning duplicate: %s -> %s", LogInfoPrefix, currentPath, disabledPath)
	return nil
}

func addImplicitProvides(potentialProviders PotentialProvidersMap) {
	implicitIDs := []string{"java", "minecraft", "fabricloader"}
	for _, id := range implicitIDs {
		potentialProviders[id] = append(potentialProviders[id], ProviderInfo{
			TopLevelModID:         id,      // Provided by itself (conceptually)
			VersionOfProvidedItem: "0.0.0", // Arbitrary version for environment provides
			IsDirectProvide:       true,
			TopLevelModVersion:    "0.0.0",
		})
	}
}

// populatePotentialProvidersForMod extracts metadata and adds all provides (direct and nested)
// from a single top-level mod to the PotentialProvidersMap.
func populatePotentialProvidersForMod(
	mod *Mod, // The winning top-level mod
	potentialProviders PotentialProvidersMap,
	progressReport func(string),
) {
	topLevelFmj, nestedFmjs, err := extractModMetadata(mod.Path, progressReport)
	if err != nil {
		log.Printf("%sError extracting metadata for mod '%s' (%s) to populate providers: %v",
			LogErrorPrefix, mod.FabricInfo.ID, mod.Path, err)
		return
	}

	// Add provides from the top-level mod itself
	addSingleProviderInfo(potentialProviders, topLevelFmj.ID, ProviderInfo{
		TopLevelModID:         topLevelFmj.ID,
		VersionOfProvidedItem: topLevelFmj.Version,
		IsDirectProvide:       true,
		TopLevelModVersion:    topLevelFmj.Version,
	})
	for _, providedItem := range topLevelFmj.Provides {
		addSingleProviderInfo(potentialProviders, providedItem, ProviderInfo{
			TopLevelModID:         topLevelFmj.ID,
			VersionOfProvidedItem: topLevelFmj.Version, // Version of provider for this item
			IsDirectProvide:       true,
			TopLevelModVersion:    topLevelFmj.Version,
		})
	}

	// Add provides from nested mods
	for _, nestedFmj := range nestedFmjs {
		// Nested mod provides its own ID, fulfilled by the top-level mod
		addSingleProviderInfo(potentialProviders, nestedFmj.ID, ProviderInfo{
			TopLevelModID:         topLevelFmj.ID,    // Fulfilled by this top-level mod
			VersionOfProvidedItem: nestedFmj.Version, // Version of the nested item itself
			IsDirectProvide:       false,             // It's a nested (indirect) provide from top-level POV
			TopLevelModVersion:    topLevelFmj.Version,
		})
		for _, providedItem := range nestedFmj.Provides {
			addSingleProviderInfo(potentialProviders, providedItem, ProviderInfo{
				TopLevelModID:         topLevelFmj.ID,
				VersionOfProvidedItem: nestedFmj.Version, // Version of the nested mod providing this
				IsDirectProvide:       false,
				TopLevelModVersion:    topLevelFmj.Version,
			})
		}
	}
}

// addSingleProviderInfo appends a ProviderInfo to the list for a given providedID.
func addSingleProviderInfo(potentialProviders PotentialProvidersMap, providedID string, info ProviderInfo) {
	if providedID == "" {
		return
	} // Do not add empty provided IDs
	potentialProviders[providedID] = append(potentialProviders[providedID], info)
}

func extractModMetadata(jarPath string, progressReport func(fileNameBeingProcessed string)) (FabricModJson, []FabricModJson, error) {
	var topLevelFmj FabricModJson
	var nestedFmjs []FabricModJson

	zr, err := zip.OpenReader(jarPath)
	if err != nil {
		return topLevelFmj, nestedFmjs, fmt.Errorf("opening JAR %s: %w", jarPath, err)
	}
	defer zr.Close()

	topLevelFmj, err = parseFabricModJsonFromReader(&zr.Reader, jarPath)
	if err != nil {
		return topLevelFmj, nestedFmjs, fmt.Errorf("parsing top-level fabric.mod.json for %s: %w", jarPath, err)
	}
	if topLevelFmj.ID == "" {
		return topLevelFmj, nestedFmjs, fmt.Errorf("top-level fabric.mod.json for %s has empty mod ID", jarPath)
	}

	// Process nested JARs specified in the "jars" field of the topLevelFmj
	if len(topLevelFmj.Jars) > 0 {
		for _, nestedJarEntry := range topLevelFmj.Jars {
			nestedJarPathInZip := nestedJarEntry.File
			if nestedJarPathInZip == "" {
				log.Printf("%sSkipping nested JAR entry with empty file path in %s", LogWarningPrefix, jarPath)
				continue
			}
			// Normalize path separators for zip searching
			nestedJarPathInZip = filepath.ToSlash(nestedJarPathInZip)

			if progressReport != nil {
				progressReport(fmt.Sprintf("  - nested %s (in %s)", nestedJarPathInZip, jarPath))
			}

			// Find the zip.File for the nested JAR
			var nestedZipFile *zip.File
			for _, f := range zr.File {
				if filepath.ToSlash(f.Name) == nestedJarPathInZip {
					nestedZipFile = f
					break
				}
			}

			if nestedZipFile == nil {
				log.Printf("%sNested JAR path '%s' specified in %s's fabric.mod.json not found in the archive.",
					LogWarningPrefix, nestedJarPathInZip, jarPath)
				continue
			}

			// Now that we have the zip.File, parse it
			nestedFmj, errParse := parseNestedJarZipEntry(nestedZipFile, fmt.Sprintf("%s in %s", nestedJarPathInZip, jarPath))
			if errParse != nil {
				log.Printf("%sSkipping nested JAR %s in %s due to error: %v", LogWarningPrefix, nestedJarPathInZip, jarPath, errParse)
				continue
			}
			if nestedFmj.ID != "" {
				nestedFmjs = append(nestedFmjs, nestedFmj)
			} else {
				log.Printf("%sSkipping nested JAR %s in %s: mod ID is empty.", LogWarningPrefix, nestedJarPathInZip, jarPath)
			}
		}
	}

	return topLevelFmj, nestedFmjs, nil
}

func parseNestedJarZipEntry(nestedJarFile *zip.File, identifier string) (FabricModJson, error) {
	var fmj FabricModJson
	rc, errOpen := nestedJarFile.Open()
	if errOpen != nil {
		return fmj, fmt.Errorf("opening nested JAR %s: %w", identifier, errOpen)
	}
	defer rc.Close()

	jarBytes, errRead := io.ReadAll(rc)
	if errRead != nil {
		return fmj, fmt.Errorf("reading nested JAR %s: %w", identifier, errRead)
	}

	bytesReader := bytes.NewReader(jarBytes)
	zipReader, errZip := zip.NewReader(bytesReader, int64(len(jarBytes)))
	if errZip != nil {
		return fmj, fmt.Errorf("treating nested JAR %s as zip: %w", identifier, errZip)
	}

	return parseFabricModJsonFromReader(zipReader, identifier)
}

func compareVersions(v1Str, v2Str string) int {
	if v1Str == "" && v2Str == "" {
		return 0
	}
	if v1Str == "" {
		return -1
	} // Prefer a versioned item over unversioned
	if v2Str == "" {
		return 1
	}

	canonV1Str := v1Str
	if !strings.HasPrefix(v1Str, "v") {
		canonV1Str = "v" + v1Str
	}
	canonV2Str := v2Str
	if !strings.HasPrefix(v2Str, "v") {
		canonV2Str = "v" + v2Str
	}
	v1Valid := sv.IsValid(canonV1Str)
	v2Valid := sv.IsValid(canonV2Str)

	if v1Valid && v2Valid {
		return sv.Compare(canonV1Str, canonV2Str)
	}

	log.Printf("%sNon-SemVer version(s): '%s' cmp '%s'. Fallback comparison.", LogWarningPrefix, v1Str, v2Str)

	if v1Valid && !v2Valid {
		return 1
	}
	if !v1Valid && v2Valid {
		return -1
	}
	if v1Str > v2Str {
		return 1
	}
	if v1Str < v2Str {
		return -1
	}
	return 0
}

func parseFabricModJsonFromReader(zipReader *zip.Reader, jarIdentifier string) (FabricModJson, error) {
	var fmj FabricModJson
	foundFmj := false
	for _, f := range zipReader.File {
		if f.Name == "fabric.mod.json" {
			fmjData, err := readZipFileEntry(f)
			if err != nil {
				return FabricModJson{}, fmt.Errorf("reading fabric.mod.json from %s: %w", jarIdentifier, err)
			}
			fmjData = sanitizeJsonStringContent(fmjData)
			if err := json5.Unmarshal(fmjData, &fmj); err != nil {
				return FabricModJson{}, fmt.Errorf("parsing fabric.mod.json from %s: %w", jarIdentifier, err)
			}
			foundFmj = true
			break
		}
	}
	if !foundFmj {
		return FabricModJson{}, fmt.Errorf("fabric.mod.json not found in %s", jarIdentifier)
	}
	return fmj, nil
}

// Sanitize JSON string content to handle literal newlines and tabs within strings,
// which are invalid in standard JSON/JSON5 but sometimes occur in fabric.mod.json.
func sanitizeJsonStringContent(data []byte) []byte {
	// Step 1: Replace actual newlines inside strings
	reNewlines := regexp.MustCompile(`(?m)(^\s*"[^"\n]*?"\s*:\s*")([^"]*?)"`)
	data = reNewlines.ReplaceAllFunc(data, func(match []byte) []byte {
		submatches := reNewlines.FindSubmatch(match)
		if len(submatches) < 3 {
			return match
		}
		prefix := submatches[1]
		value := submatches[2]
		escapedValue := bytes.ReplaceAll(value, []byte("\n"), []byte(`\\n`))
		return append(append(prefix, escapedValue...), '"')
	})

	// Step 2: Replace actual tab characters inside all string values
	reTabs := regexp.MustCompile(`(?m)"[^"]*?"`)
	data = reTabs.ReplaceAllFunc(data, func(match []byte) []byte {
		if len(match) <= 2 {
			return match
		}
		inner := match[1 : len(match)-1]
		escaped := bytes.ReplaceAll(inner, []byte("\t"), []byte(`\\t`))
		return append(append([]byte{'"'}, escaped...), '"')
	})
	return data
}

func readZipFileEntry(f *zip.File) ([]byte, error) {
	rc, err := f.Open()
	if err != nil {
		return nil, err
	}
	defer rc.Close()
	return io.ReadAll(rc)
}

func (m *Mod) Enable(modsDir string) error {
	if m.IsCurrentlyActive {
		activePath := filepath.Join(modsDir, m.BaseFilename+".jar")
		if _, err := os.Stat(activePath); err == nil {
			return nil
		}
	}
	disabledPath := filepath.Join(modsDir, m.BaseFilename+".jar.disabled")
	activePath := filepath.Join(modsDir, m.BaseFilename+".jar")
	if _, err := os.Stat(disabledPath); os.IsNotExist(err) {
		if _, errStatActive := os.Stat(activePath); errStatActive == nil {
			m.IsCurrentlyActive = true
			m.Path = activePath
			return nil
		}
		return fmt.Errorf("cannot enable mod %s: disabled file %s does not exist, and active file %s also not found", m.ModID(), disabledPath, activePath)
	}
	err := os.Rename(disabledPath, activePath)
	if err == nil {
		m.IsCurrentlyActive = true
		m.Path = activePath
		return nil
	}
	if os.IsExist(err) {
		m.IsCurrentlyActive = true
		m.Path = activePath
		return nil
	}
	return fmt.Errorf("%w: enabling mod %s (renaming %s to %s): %v", ErrRenameFailedSkippable, m.ModID(), disabledPath, activePath, err)
}

func (m *Mod) Disable(modsDir string) error {
	if !m.IsCurrentlyActive {
		disabledPath := filepath.Join(modsDir, m.BaseFilename+".jar.disabled")
		if _, err := os.Stat(disabledPath); err == nil {
			return nil
		}
	}
	activePath := filepath.Join(modsDir, m.BaseFilename+".jar")
	disabledPath := filepath.Join(modsDir, m.BaseFilename+".jar.disabled")
	if _, err := os.Stat(activePath); os.IsNotExist(err) {
		if _, errStatDisabled := os.Stat(disabledPath); errStatDisabled == nil {
			m.IsCurrentlyActive = false
			m.Path = disabledPath
			return nil
		}
		return fmt.Errorf("cannot disable mod %s: active file %s does not exist, and disabled file %s also not found", m.ModID(), activePath, disabledPath)
	}
	err := os.Rename(activePath, disabledPath)
	if err == nil {
		m.IsCurrentlyActive = false
		m.Path = disabledPath
		return nil
	}
	return fmt.Errorf("%w: disabling mod %s (renaming %s to %s): %v", ErrRenameFailedSkippable, m.ModID(), activePath, disabledPath, err)
}
