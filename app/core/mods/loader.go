package mods

import (
	"archive/zip"
	"bytes"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"sync"

	"github.com/Qendolin/fabric-mod-bisect-tool/app/logging"
	"github.com/titanous/json5"
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
	modData      *parsedModFile
	parseError   error
	baseFileName string
}

type nestedModInfo struct {
	fmj       FabricModJson
	pathInJar string
}

type parsedModFile struct {
	mod    *Mod
	nested []nestedModInfo
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
			logging.Warnf("Loader: Failed to load mod metadata from file '%s.jar': %v", res.baseFileName, res.parseError)
			continue
		}
		if res.modData != nil {
			currentMod := res.modData.mod
			nestedMods := res.modData.nested

			logging.Infof("Loader: ├─ Mod %s (%s v%s) from file '%s.jar'",
				currentMod.FabricInfo.ID, currentMod.FriendlyName(), currentMod.FabricInfo.Version,
				res.baseFileName)

			for i, nested := range nestedMods {
				treeSymbol := "├"
				if i == len(nestedMods)-1 {
					treeSymbol = "└"
				}
				logging.Infof("Loader: │   %s─ Mod %s (%s v%s) provided by %s from '%s'.",
					treeSymbol, nested.fmj.ID, nested.fmj.Name, nested.fmj.Version, currentMod.FabricInfo.ID, nested.pathInJar)
			}

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

		topLevelFmj, nestedMods, err := extractModMetadata(fullPath) // Call package-level function
		if err != nil {
			results <- processFileResult{baseFileName: baseFilename, parseError: fmt.Errorf("extracting metadata from %s: %w", task.fileEntry.Name(), err)}
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
			modData:      &parsedModFile{mod: currentMod, nested: nestedMods},
			baseFileName: baseFilename,
		}
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
		allMods[modID].NestedModules = make([]FabricModJson, len(winnerData.nested))
		for i, nested := range winnerData.nested {
			allMods[modID].NestedModules[i] = nested.fmj
		}

		for _, cData := range candidatesData {
			if cData.mod.Path == winnerData.mod.Path {
				continue
			}
			if cData.mod.IsInitiallyActive { // Only disable active non-winning duplicates
				if err := disableDuplicateFile(modsDir, cData.mod.BaseFilename); err != nil { // Call package-level function
					errMsg := fmt.Sprintf("error disabling non-winning duplicate '%s' (for mod %s): %v", filepath.Base(cData.mod.Path), modID, err)
					logging.Error(errMsg)
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

	logging.Warnf("Loader: Found %d conflicting files for mod %s. Determining winner by version...", len(candidates), modID)
	winnerIdx := 0
	for i := 1; i < len(candidates); i++ {
		// Compare versions using the semantic versioning helper.
		if compareVersions(candidates[i].mod.FabricInfo.Version, candidates[winnerIdx].mod.FabricInfo.Version) > 0 {
			winnerIdx = i
		}
	}
	logging.Infof("Loader: Winner for mod %s is v%s from file '%s'.",
		modID, candidates[winnerIdx].mod.FabricInfo.Version, filepath.Base(candidates[winnerIdx].mod.Path))
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
	var unmetDependencies []string
	var sortedDepIDs []string
	for depID := range potentialProviders {
		sortedDepIDs = append(sortedDepIDs, depID)
	}
	sort.Strings(sortedDepIDs) // Sort top-level depIDs for deterministic output

	for _, depID := range sortedDepIDs {
		infos := potentialProviders[depID]

		// Ensure the providers are sorted for consistent best-provider selection and logging order
		if len(infos) > 1 {
			sort.Slice(infos, func(i, j int) bool {
				// Direct provides are preferred (true comes before false).
				if infos[i].IsDirectProvide != infos[j].IsDirectProvide {
					return infos[i].IsDirectProvide
				}
				// Higher version of the provided item is preferred.
				compItemVer := compareVersions(infos[i].VersionOfProvidedItem, infos[j].VersionOfProvidedItem)
				if compItemVer != 0 {
					return compItemVer > 0
				}
				// Fallback: Higher version of the top-level mod is preferred.
				return compareVersions(infos[i].TopLevelModVersion, infos[j].TopLevelModVersion) > 0
			})
			potentialProviders[depID] = infos
		}

		if len(infos) == 0 {
			unmetDependencies = append(unmetDependencies, depID) // Collect unmet dependencies
		} else if len(infos) == 1 {
			info := infos[0]
			// Skip if it's a direct self-provision (e.g., 'balm' provides 'balm')
			isSelfProvision := info.IsDirectProvide && info.TopLevelModID == depID
			if isSelfProvision {
				continue
			}

			providingMod, ok := allMods[info.TopLevelModID]
			providingFileName := "unknown"
			if ok {
				providingFileName = providingMod.BaseFilename + ".jar"
			}

			providerLogMessages = append(providerLogMessages,
				fmt.Sprintf("  - Dependency %s provided by %s (at v%s) from '%s'.",
					depID, info.TopLevelModID, info.VersionOfProvidedItem, providingFileName))

		} else { // Multiple providers
			providerLogMessages = append(providerLogMessages, fmt.Sprintf("  - Dependency %s provided by:", depID))
			for _, info := range infos {
				providingMod, ok := allMods[info.TopLevelModID]
				providingFileName := "unknown"
				if ok {
					providingFileName = providingMod.BaseFilename + ".jar"
				}
				providerLogMessages = append(providerLogMessages,
					fmt.Sprintf("      - %s (at v%s) from '%s'.",
						info.TopLevelModID, info.VersionOfProvidedItem, providingFileName))
			}
		}
	}

	// Final logging summary
	logging.Infof("Loader: Populated dependency providers for %d unique dependencies:\n%s", len(potentialProviders), strings.Join(providerLogMessages, "\n"))

	// Log unmet dependencies as an error message
	if len(unmetDependencies) > 0 {
		sort.Strings(unmetDependencies)
		logging.Errorf("Loader: Found %d dependencies with no known providers: %s", len(unmetDependencies), strings.Join(unmetDependencies, ", "))
	}
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
func extractModMetadata(jarPath string) (FabricModJson, []nestedModInfo, error) {
	zr, err := zip.OpenReader(jarPath) // Use zip.OpenReader for simplicity
	if err != nil {
		return FabricModJson{}, nil, fmt.Errorf("opening JAR %s as zip: %w", jarPath, err)
	}
	defer zr.Close()

	topLevelFmj, err := parseFabricModJsonFromReader(&zr.Reader, jarPath)
	if err != nil {
		return FabricModJson{}, nil, fmt.Errorf("parsing top-level fabric.mod.json for %s: %w", jarPath, err)
	}

	var allNestedMods []nestedModInfo
	// Iterate through the top-level JAR's nested jar entries to start the recursion.
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

		// Append the entire slice of found mods (from all depths) to the final list.
		allNestedMods = append(allNestedMods, foundMods...)
	}
	return topLevelFmj, allNestedMods, nil
}

// recursivelyParseNestedJar is a helper that parses a nested JAR and any of its own nested JARs.
// It returns a flattened slice of all nested mods found at and below the current level.
func recursivelyParseNestedJar(parentZipReader *zip.Reader, pathInParent string, currentPathPrefix string) ([]nestedModInfo, error) {
	// Find the nested JAR file within the parent zip reader
	var nestedZipFile *zip.File
	for _, f := range parentZipReader.File {
		// ZIP paths always use forward slashes, so we don't need filepath
		if f.Name == pathInParent {
			nestedZipFile = f
			break
		}
	}
	if nestedZipFile == nil {
		return nil, fmt.Errorf("nested JAR '%s' not found in archive", pathInParent)
	}

	// Read the nested JAR into memory to create a new zip reader from its bytes
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

	// --- Core Recursive Logic ---

	// 1. Parse the fabric.mod.json of the *current* nested JAR
	currentFmj, err := parseFabricModJsonFromReader(innerZipReader, pathInParent)
	if err != nil {
		return nil, fmt.Errorf("parsing metadata from '%s': %w", pathInParent, err)
	}

	// 2. The full path is the prefix (path to get here) plus the current file's path
	//    The "path" package is used here to ensure forward slashes.
	fullPathInJar := path.Join(currentPathPrefix, pathInParent)

	// 3. This is our list of discovered mods, starting with the current one.
	allFoundMods := []nestedModInfo{
		{fmj: currentFmj, pathInJar: fullPathInJar},
	}

	// 4. If this JAR contains its own nested JARs, recurse.
	for _, deeperJarEntry := range currentFmj.Jars {
		if deeperJarEntry.File == "" {
			continue // Skip empty entries
		}

		// The new prefix for the deeper call is the full path to the current JAR
		deeperNestedMods, err := recursivelyParseNestedJar(innerZipReader, deeperJarEntry.File, fullPathInJar)
		if err != nil {
			logging.Warnf("Loader: Skipping deeper nested JAR '%s' within '%s': %v", deeperJarEntry.File, fullPathInJar, err)
			continue
		}

		// Add all the mods found in the deeper recursion to our flattened list
		allFoundMods = append(allFoundMods, deeperNestedMods...)
	}

	return allFoundMods, nil
}

// compareVersions compares two version strings using the relaxed Fabric versioning rules.
// It falls back to a simple string comparison if either version fails to parse.
func compareVersions(v1Str, v2Str string) int {
	if v1Str == v2Str {
		return 0
	}

	v1, err1 := ParseExtendedSemVer(v1Str)
	v2, err2 := ParseExtendedSemVer(v2Str)

	// If both parse successfully, use the proper comparison logic.
	if err1 == nil && err2 == nil {
		return v1.Compare(v2)
	}

	// Fallback logic for invalid versions:
	logging.Warnf("Loader: Could not parse one or both versions ('%s', '%s') with Fabric rules. Falling back to string comparison.", v1Str, v2Str)

	// A successfully parsed version is considered "greater" than one that failed.
	if err1 == nil && err2 != nil {
		return 1
	}
	if err1 != nil && err2 == nil {
		return -1
	}

	// If both failed, just compare them as plain strings.
	if v1Str > v2Str {
		return 1
	}
	if v1Str < v2Str {
		return -1
	}
	return 0
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
				dataSnippet := string(fmjData)
				if len(dataSnippet) > 200 {
					dataSnippet = dataSnippet[:200] + "..."
				}
				return FabricModJson{}, fmt.Errorf("unmarshaling fabric.mod.json from %s (data snippet: %s): %w", jarIdentifier, dataSnippet, err)
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
