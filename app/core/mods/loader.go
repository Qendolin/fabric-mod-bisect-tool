package mods

import (
	"archive/zip"
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"sync"

	"github.com/Qendolin/fabric-mod-bisect-tool/app/logging"
	"github.com/titanous/json5"
	sv "golang.org/x/mod/semver"
)

var (
	reSanitizeNewlines = regexp.MustCompile(`(?m)("[^"\n]*?"\s*:\s*")([^"]*?)"`)
	reSanitizeTabs     = regexp.MustCompile(`(?m)"[^"]*?"`)
)

// ModLoaderService defines the interface for loading mod information.
type ModLoaderService interface {
	LoadMods(modsDir string, progressReport func(fileNameBeingProcessed string)) (
		allMods map[string]*Mod,
		potentialProviders PotentialProvidersMap,
		sortedModIDs []string,
		err error,
	)
}

// defaultModLoaderService implements ModLoaderService.
type defaultModLoaderService struct{}

// NewModLoaderService creates a new ModLoaderService.
func NewModLoaderService() ModLoaderService {
	return &defaultModLoaderService{}
}

type processFileTask struct {
	fileEntry    os.DirEntry
	modsDir      string
	progressFunc func(string)
}

type processFileResult struct {
	modData    *parsedModFile
	parseError error
	fileName   string
}

type parsedModFile struct {
	mod        *Mod
	nestedFmjs []FabricModJson
}

// LoadMods discovers mods, parses metadata, resolves basic conflicts, and builds provider maps.
func (s *defaultModLoaderService) LoadMods(modsDir string, progressReport func(fileNameBeingProcessed string)) (
	map[string]*Mod, PotentialProvidersMap, []string, error) {

	potentialProviders := make(PotentialProvidersMap)
	addImplicitProvides(potentialProviders)

	diskFiles, err := os.ReadDir(modsDir)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("reading mods directory %s: %w", modsDir, err)
	}

	filesToProcess := filterJarFiles(diskFiles)
	if len(filesToProcess) == 0 {
		logging.Infof("Loader: No .jar or .jar.disabled files found in %s", modsDir)
		return make(map[string]*Mod), potentialProviders, []string{}, nil
	}

	parsedFileResults := s.parseJarFilesConcurrently(filesToProcess, modsDir, progressReport)

	allMods := make(map[string]*Mod)
	if err := resolveModConflicts(parsedFileResults, allMods, modsDir); err != nil {
		logging.Errorf("Loader: Error during mod conflict resolution: %v. Proceeding with available mods.", err)
	}

	populateProviderMaps(allMods, potentialProviders)

	sortedModIDs := make([]string, 0, len(allMods))
	for id := range allMods {
		sortedModIDs = append(sortedModIDs, id)
	}
	sort.Strings(sortedModIDs)

	logging.Infof("Loader: Finished loading. Total %d mods loaded. %d potential capabilities provided.", len(allMods), len(potentialProviders))

	return allMods, potentialProviders, sortedModIDs, nil
}

// filterJarFiles returns a slice of os.DirEntry for files ending with .jar or .jar.disabled.
func filterJarFiles(diskFiles []os.DirEntry) []os.DirEntry {
	var filesToProcess []os.DirEntry
	for _, file := range diskFiles {
		if file.IsDir() {
			continue
		}
		fileName := file.Name()
		if strings.HasSuffix(strings.ToLower(fileName), ".jar") ||
			strings.HasSuffix(strings.ToLower(fileName), ".jar.disabled") {
			filesToProcess = append(filesToProcess, file)
		}
	}
	return filesToProcess
}

// parseJarFilesConcurrently processes JAR files in parallel to extract mod metadata.
func (s *defaultModLoaderService) parseJarFilesConcurrently(filesToProcess []os.DirEntry, modsDir string, progressReport func(string)) []*parsedModFile {
	numFiles := len(filesToProcess)
	if numFiles == 0 {
		return []*parsedModFile{}
	}
	// Use Go 1.24+ built-in min function.
	numWorkers := min(numFiles, runtime.NumCPU())

	tasks := make(chan processFileTask, numFiles)
	results := make(chan processFileResult, numFiles)
	var wg sync.WaitGroup

	for i := 0; i < numWorkers; i++ {
		wg.Add(1)
		go jarProcessingWorker(&wg, tasks, results) // Call package-level function
	}

	for _, file := range filesToProcess {
		tasks <- processFileTask{fileEntry: file, modsDir: modsDir, progressFunc: progressReport}
	}
	close(tasks)

	go func() {
		wg.Wait()
		close(results)
	}()

	var collectedModFileResults []*parsedModFile
	for res := range results {
		if res.parseError != nil {
			logging.Warnf("Loader: Skipping JAR %s: %v", res.fileName, res.parseError)
			continue
		}
		if res.modData != nil {
			collectedModFileResults = append(collectedModFileResults, res.modData)
		}
	}
	return collectedModFileResults
}

// jarProcessingWorker is a goroutine worker that processes file tasks.
func jarProcessingWorker(wg *sync.WaitGroup, tasks <-chan processFileTask, results chan<- processFileResult) {
	defer wg.Done()
	for task := range tasks {
		if task.progressFunc != nil {
			task.progressFunc(task.fileEntry.Name())
		}

		fullPath := filepath.Join(task.modsDir, task.fileEntry.Name())
		isJarFile := strings.HasSuffix(strings.ToLower(task.fileEntry.Name()), ".jar")
		baseFilename := task.fileEntry.Name()
		if !isJarFile {
			baseFilename = strings.TrimSuffix(baseFilename, ".disabled")
		}
		// Base filename should be just the name without any .jar/.disabled suffix for consistent renaming.
		baseFilename = strings.TrimSuffix(baseFilename, ".jar")

		topLevelFmj, nestedFmjs, err := extractModMetadata(fullPath) // Call package-level function
		if err != nil {
			results <- processFileResult{fileName: task.fileEntry.Name(), parseError: fmt.Errorf("extracting metadata from %s: %w", task.fileEntry.Name(), err)}
			continue
		}
		currentMod := &Mod{
			Path:              fullPath,
			BaseFilename:      baseFilename,
			FabricInfo:        topLevelFmj,
			IsInitiallyActive: isJarFile,
			ConfirmedGood:     false, // Initial state, user can change this.
		}
		results <- processFileResult{
			modData:  &parsedModFile{mod: currentMod, nestedFmjs: nestedFmjs},
			fileName: task.fileEntry.Name(),
		}
		logging.Infof("Loader: Parsed mod '%s' (ID: %s, v%s) from file '%s'. Initially active: %t. Nested jars: %d.",
			currentMod.FriendlyName(), currentMod.FabricInfo.ID, currentMod.FabricInfo.Version, task.fileEntry.Name(), currentMod.IsInitiallyActive, len(nestedFmjs))
	}
}

// resolveModConflicts handles multiple JAR files providing the same mod ID, choosing a winner.
// It also disables non-winning duplicate active files.
func resolveModConflicts(parsedFileResults []*parsedModFile, allMods map[string]*Mod, modsDir string) error {
	parsedCandidates := make(map[string][]*parsedModFile)
	for _, modFile := range parsedFileResults {
		modID := modFile.mod.FabricInfo.ID
		parsedCandidates[modID] = append(parsedCandidates[modID], modFile)
	}

	var multiError []string
	var disabledDuplicates []string

	for modID, candidatesData := range parsedCandidates {
		winnerData := determineWinner(modID, candidatesData) // Call package-level function
		allMods[modID] = winnerData.mod
		allMods[modID].NestedModules = winnerData.nestedFmjs

		for _, cData := range candidatesData {
			if cData.mod.Path == winnerData.mod.Path {
				continue
			}
			if cData.mod.IsInitiallyActive { // Only disable active non-winning duplicates
				if err := disableDuplicateFile(modsDir, cData.mod.BaseFilename); err != nil { // Call package-level function
					errMsg := fmt.Sprintf("error disabling non-winning duplicate '%s' (for mod ID '%s'): %v", filepath.Base(cData.mod.Path), modID, err)
					logging.Errorf("%s", errMsg)
					multiError = append(multiError, errMsg)
				} else {
					disabledDuplicates = append(disabledDuplicates, filepath.Base(cData.mod.Path))
				}
			}
		}
	}
	if len(disabledDuplicates) > 0 {
		logging.Infof("Loader: Disabled %d non-winning duplicate active files: %s", len(disabledDuplicates), strings.Join(disabledDuplicates, ", "))
	}
	if len(multiError) > 0 {
		return fmt.Errorf("encountered errors during conflict resolution: %s", strings.Join(multiError, "; "))
	}
	return nil
}

// determineWinner selects the preferred mod file from a list of candidates for the same mod ID.
// It prioritizes by version using semantic versioning comparison.
func determineWinner(modID string, candidates []*parsedModFile) *parsedModFile {
	if len(candidates) == 1 {
		return candidates[0]
	}

	logging.Infof("Loader: Found %d conflicting files for mod ID '%s'. Determining winner by version...", len(candidates), modID)
	winnerIdx := 0
	for i := 1; i < len(candidates); i++ {
		// Compare versions using the semantic versioning helper.
		if compareVersions(candidates[i].mod.FabricInfo.Version, candidates[winnerIdx].mod.FabricInfo.Version) > 0 {
			winnerIdx = i
		}
	}
	logging.Infof("Loader: Winner for mod ID '%s' is '%s' (v%s) from file '%s'.",
		modID, candidates[winnerIdx].mod.FriendlyName(),
		candidates[winnerIdx].mod.FabricInfo.Version, filepath.Base(candidates[winnerIdx].mod.Path))
	return candidates[winnerIdx]
}

// disableDuplicateFile renames an active JAR file to its .disabled counterpart.
func disableDuplicateFile(modsDir, baseFilename string) error {
	activePath := filepath.Join(modsDir, baseFilename+".jar")
	disabledPath := filepath.Join(modsDir, baseFilename+".jar.disabled")

	// Check if the active file exists. If not, nothing to disable.
	if _, err := os.Stat(activePath); os.IsNotExist(err) {
		return nil // File not found, nothing to do.
	} else if err != nil {
		return fmt.Errorf("stat active file %s: %w", activePath, err)
	}

	// If a .disabled version already exists, remove it to allow rename.
	if _, err := os.Stat(disabledPath); err == nil {
		logging.Warnf("Loader: Removing existing disabled file '%s' before disabling '%s'.", filepath.Base(disabledPath), baseFilename)
		if remErr := os.Remove(disabledPath); remErr != nil {
			return fmt.Errorf("remove existing %s: %w", disabledPath, remErr)
		}
	}

	err := os.Rename(activePath, disabledPath)
	if err != nil {
		return fmt.Errorf("renaming %s to %s: %w", activePath, disabledPath, err)
	}
	return nil
}

// populateProviderMaps populates the potentialProviders map and the EffectiveProvides for each mod.
func populateProviderMaps(allMods map[string]*Mod, potentialProviders PotentialProvidersMap) {
	for _, mod := range allMods {
		mod.EffectiveProvides = make(map[string]string)

		providerInfoBase := ProviderInfo{TopLevelModID: mod.FabricInfo.ID, TopLevelModVersion: mod.FabricInfo.Version}

		// Direct provides (the mod itself)
		directProviderInfo := providerInfoBase
		directProviderInfo.IsDirectProvide = true
		directProviderInfo.VersionOfProvidedItem = mod.FabricInfo.Version
		updateEffectiveProvides(mod.EffectiveProvides, mod.FabricInfo.ID, mod.FabricInfo.Version)
		addSingleProviderInfo(potentialProviders, mod.FabricInfo.ID, directProviderInfo)

		// Explicit 'provides' array in fabric.mod.json
		for _, p := range mod.FabricInfo.Provides {
			updateEffectiveProvides(mod.EffectiveProvides, p, mod.FabricInfo.Version)
			addSingleProviderInfo(potentialProviders, p, directProviderInfo)
		}

		// Provides from nested modules
		nestedProviderInfoBase := providerInfoBase
		nestedProviderInfoBase.IsDirectProvide = false
		for _, nestedFmj := range mod.NestedModules { // Iterate through already parsed nested modules
			nestedProviderInfo := nestedProviderInfoBase
			nestedProviderInfo.VersionOfProvidedItem = nestedFmj.Version
			updateEffectiveProvides(mod.EffectiveProvides, nestedFmj.ID, nestedFmj.Version)
			addSingleProviderInfo(potentialProviders, nestedFmj.ID, nestedProviderInfo)
			for _, p := range nestedFmj.Provides {
				updateEffectiveProvides(mod.EffectiveProvides, p, nestedFmj.Version)
				addSingleProviderInfo(potentialProviders, p, nestedProviderInfo)
			}
		}
	}

	var providerLogMessages []string
	for depID, infos := range potentialProviders {
		// Only sort if there's more than one provider.
		if len(infos) > 1 {
			sort.Slice(infos, func(i, j int) bool {
				// Direct provides are preferred (true comes before false).
				if infos[i].IsDirectProvide != infos[j].IsDirectProvide {
					return infos[i].IsDirectProvide // True comes before false
				}
				// Higher version of the provided item is preferred.
				compItemVer := compareVersions(infos[i].VersionOfProvidedItem, infos[j].VersionOfProvidedItem)
				if compItemVer != 0 {
					return compItemVer > 0 // Greater than 0 means i's version is higher
				}
				// Fallback: Higher version of the top-level mod is preferred.
				return compareVersions(infos[i].TopLevelModVersion, infos[j].TopLevelModVersion) > 0
			})
			potentialProviders[depID] = infos // Update the slice in the map.
			providerLogMessages = append(providerLogMessages, fmt.Sprintf("  - '%s' provided by '%s' (v%s) [top-level mod: '%s' v%s, direct: %t]",
				depID, infos[0].TopLevelModID, infos[0].VersionOfProvidedItem, infos[0].TopLevelModID, infos[0].TopLevelModVersion, infos[0].IsDirectProvide))
		} else if len(infos) == 1 {
			providerLogMessages = append(providerLogMessages, fmt.Sprintf("  - '%s' provided by '%s' (v%s) [top-level mod: '%s' v%s, direct: %t]",
				depID, infos[0].TopLevelModID, infos[0].VersionOfProvidedItem, infos[0].TopLevelModID, infos[0].TopLevelModVersion, infos[0].IsDirectProvide))
		} else {
			providerLogMessages = append(providerLogMessages, fmt.Sprintf("  - No providers found for '%s'", depID))
		}
	}
	sort.Strings(providerLogMessages) // Sort messages for deterministic output.
	logging.Infof("Loader: Populated dependency providers for %d unique capabilities:\n%s", len(potentialProviders), strings.Join(providerLogMessages, "\n"))
}

// updateEffectiveProvides updates the effective provides map for a mod, prioritizing higher versions.
func updateEffectiveProvides(effectiveProvides map[string]string, providedID, version string) {
	if providedID == "" {
		return
	}
	if existingVersion, ok := effectiveProvides[providedID]; !ok || compareVersions(version, existingVersion) > 0 {
		effectiveProvides[providedID] = version
	}
}

// addImplicitProvides adds common implicit dependencies to the potential providers map.
func addImplicitProvides(potentialProviders PotentialProvidersMap) {
	implicitIDs := []string{"java", "minecraft", "fabricloader"}
	for _, id := range implicitIDs {
		// Only add if not already explicitly provided by a mod.
		if existingProviders, ok := potentialProviders[id]; ok && len(existingProviders) > 0 {
			// If there are existing providers, we assume they are sufficient or better.
			continue
		}
		potentialProviders[id] = append(potentialProviders[id], ProviderInfo{
			TopLevelModID: id, VersionOfProvidedItem: "0.0.0", // Use a minimal version
			IsDirectProvide: true, TopLevelModVersion: "0.0.0",
		})
	}
}

// addSingleProviderInfo adds a single ProviderInfo to the potential providers map for a given ID.
func addSingleProviderInfo(potentialProviders PotentialProvidersMap, providedID string, info ProviderInfo) {
	if providedID == "" {
		return
	}
	potentialProviders[providedID] = append(potentialProviders[providedID], info)
}

// extractModMetadata opens a JAR, extracts its top-level fabric.mod.json, and recursively parses nested JARs.
func extractModMetadata(jarPath string) (FabricModJson, []FabricModJson, error) {
	file, err := os.OpenFile(jarPath, os.O_RDONLY, 0)
	if err != nil {
		return FabricModJson{}, nil, fmt.Errorf("opening JAR %s: %w", jarPath, err)
	}
	defer file.Close()

	stat, err := file.Stat()
	if err != nil {
		return FabricModJson{}, nil, fmt.Errorf("statting JAR %s: %w", jarPath, err)
	}

	zr, err := zip.NewReader(file, stat.Size())
	if err != nil {
		return FabricModJson{}, nil, fmt.Errorf("reading JAR %s as zip: %w", jarPath, err)
	}

	topLevelFmj, err := parseFabricModJsonFromReader(zr, jarPath)
	if err != nil {
		return FabricModJson{}, nil, fmt.Errorf("parsing top-level fabric.mod.json for %s: %w", jarPath, err)
	}

	var nestedFmjs []FabricModJson
	// Iterate through `topLevelFmj.Jars` to find paths for nested JARs, then parse them.
	for _, nestedJarEntry := range topLevelFmj.Jars {
		if nestedJarEntry.File == "" {
			logging.Warnf("Loader: Top-level mod '%s' has a nested JAR entry with an empty 'file' path. Skipping.", topLevelFmj.ID)
			continue
		}
		fmj, err := parseNestedJar(zr, nestedJarEntry.File, jarPath) // Call package-level function
		if err != nil {
			logging.Warnf("Loader: Skipping nested JAR '%s' in '%s': %v", nestedJarEntry.File, jarPath, err)
			continue
		}
		nestedFmjs = append(nestedFmjs, fmj)
		logging.Infof("Loader: Parsed nested mod '%s' (ID: %s, v%s) from '%s' within '%s'.",
			fmj.Name, fmj.ID, fmj.Version, nestedJarEntry.File, filepath.Base(jarPath))
	}
	return topLevelFmj, nestedFmjs, nil
}

// parseNestedJar extracts FabricModJson from a nested JAR file within a parent ZIP.
func parseNestedJar(parentZipReader *zip.Reader, nestedJarPathInZip, outerJarPath string) (FabricModJson, error) {
	if nestedJarPathInZip == "" {
		return FabricModJson{}, fmt.Errorf("empty file path for nested JAR in %s", outerJarPath)
	}
	normalizedNestedPath := filepath.ToSlash(nestedJarPathInZip)

	var nestedZipFile *zip.File
	for _, f := range parentZipReader.File {
		if filepath.ToSlash(f.Name) == normalizedNestedPath {
			nestedZipFile = f
			break
		}
	}
	if nestedZipFile == nil {
		return FabricModJson{}, fmt.Errorf("nested JAR '%s' not found in %s", normalizedNestedPath, outerJarPath)
	}

	return parseNestedJarZipEntry(nestedZipFile, fmt.Sprintf("%s in %s", normalizedNestedPath, outerJarPath)) // Call package-level function
}

// parseNestedJarZipEntry reads a nested JAR's content and parses its fabric.mod.json.
func parseNestedJarZipEntry(nestedJarFile *zip.File, identifier string) (FabricModJson, error) {
	rc, err := nestedJarFile.Open()
	if err != nil {
		return FabricModJson{}, fmt.Errorf("opening nested JAR %s: %w", identifier, err)
	}
	defer rc.Close()

	jarBytes, err := io.ReadAll(rc)
	if err != nil {
		return FabricModJson{}, fmt.Errorf("reading nested JAR %s: %w", identifier, err)
	}

	bytesReader := bytes.NewReader(jarBytes)
	zipReader, err := zip.NewReader(bytesReader, int64(len(jarBytes)))
	if err != nil {
		return FabricModJson{}, fmt.Errorf("treating nested JAR %s as zip: %w", identifier, err)
	}
	return parseFabricModJsonFromReader(zipReader, identifier) // Call package-level function
}

// compareVersions compares two version strings using semantic versioning.
// It prepends 'v' if missing for sv.IsValid compatibility.
// Returns >0 if v1Str is greater, <0 if v2Str is greater, 0 if equal.
// Handles invalid semver strings by preferring valid over invalid, and
// then string comparison for two invalid.
func compareVersions(v1Str, v2Str string) int {
	if v1Str == v2Str {
		return 0
	}
	if v1Str == "" { // Empty string is always considered "less"
		return -1
	}
	if v2Str == "" { // Empty string is always considered "less"
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
	if v1Valid {
		return 1 // v1 is valid, v2 is not -> v1 is greater
	}
	if v2Valid {
		return -1 // v2 is valid, v1 is not -> v2 is greater
	}
	// Both are invalid semver, fall back to string comparison for stability.
	if v1Str > v2Str {
		return 1
	}
	if v1Str < v2Str {
		return -1
	}
	return 0 // Should not be reached if v1Str == v2Str is handled earlier.
}

// parseFabricModJsonFromReader searches for and unmarshals fabric.mod.json from a zip reader.
func parseFabricModJsonFromReader(zipReader *zip.Reader, jarIdentifier string) (FabricModJson, error) {
	var fmj FabricModJson
	for _, f := range zipReader.File {
		if strings.EqualFold(f.Name, "fabric.mod.json") {
			fmjData, err := readZipFileEntry(f) // Call package-level function
			if err != nil {
				return FabricModJson{}, fmt.Errorf("reading fabric.mod.json from %s: %w", jarIdentifier, err)
			}
			fmjData = sanitizeJsonStringContent(fmjData) // Call package-level function

			if err := json5.Unmarshal(fmjData, &fmj); err != nil {
				return FabricModJson{}, fmt.Errorf("unmarshaling fabric.mod.json from %s: %w", jarIdentifier, err)
			}
			if fmj.ID == "" {
				return FabricModJson{}, fmt.Errorf("fabric.mod.json from %s has empty mod ID", jarIdentifier)
			}
			return fmj, nil
		}
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
		if len(match) <= 2 { // Empty string or string with just quotes.
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
