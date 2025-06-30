package mods

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"

	"github.com/Qendolin/fabric-mod-bisect-tool/app/logging"
)

// ModLoaderService loads mod information from the filesystem, parses metadata,
// resolves conflicts, and builds dependency provider maps.
type ModLoaderService struct{}

// NewModLoaderService creates a new ModLoaderService.
func NewModLoaderService() *ModLoaderService {
	return &ModLoaderService{}
}

// task to be processed by a worker goroutine.
type processFileTask struct {
	fileEntry    os.DirEntry
	modsDir      string
	progressFunc func(string)
}

// result of a worker goroutine processing a single file.
type processFileResult struct {
	modData      *parsedModFile
	parseError   error
	baseFileName string
}

// intermediate representation of a parsed mod file and its nested modules.
type parsedModFile struct {
	mod    *Mod
	nested []nestedModInfo
}

// holds information about a mod found inside another JAR file.
type nestedModInfo struct {
	fmj       FabricModJson
	pathInJar string
}

// LoadMods discovers mods, parses metadata, resolves basic conflicts, and builds provider maps.
func (s *ModLoaderService) LoadMods(modsDir string, overrides *DependencyOverrides, progressReport func(fileNameBeingProcessed string)) (
	map[string]*Mod, PotentialProvidersMap, []string, error) {

	potentialProviders := make(PotentialProvidersMap)
	addImplicitProvides(potentialProviders)

	diskFiles, err := os.ReadDir(modsDir)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("reading mods directory %s: %w", modsDir, err)
	}

	filesToProcess := filterJarFiles(diskFiles)
	if len(filesToProcess) == 0 {
		logging.Infof("ModLoader: No .jar or .jar.disabled files found in %s", modsDir)
		return make(map[string]*Mod), potentialProviders, []string{}, nil
	}

	parsedFileResults := s.parseJarFilesConcurrently(filesToProcess, modsDir, progressReport)

	allMods := make(map[string]*Mod)
	if err := resolveModConflicts(parsedFileResults, allMods, modsDir); err != nil {
		logging.Errorf("ModLoader: Error during mod conflict resolution: %v. Proceeding with available mods.", err)
	}

	if overrides != nil && len(overrides.Rules) > 0 {
		logging.Info("ModLoader: Applying dependency overrides...")
		s.applyOverridesToLoadedMods(allMods, overrides)
		logging.Info("ModLoader: Dependency overrides applied.")
	}

	populateProviderMaps(allMods, potentialProviders)

	sortedModIDs := make([]string, 0, len(allMods))
	for id := range allMods {
		sortedModIDs = append(sortedModIDs, id)
	}
	sort.Strings(sortedModIDs)

	logging.Infof("ModLoader: Finished loading. Total %d mods loaded. %d potential capabilities provided.", len(allMods), len(potentialProviders))

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
func (s *ModLoaderService) parseJarFilesConcurrently(filesToProcess []os.DirEntry, modsDir string, progressReport func(string)) []*parsedModFile {
	numFiles := len(filesToProcess)
	if numFiles == 0 {
		return nil
	}
	numWorkers := min(numFiles, runtime.NumCPU())

	tasks := make(chan processFileTask, numFiles)
	results := make(chan processFileResult, numFiles)
	var wg sync.WaitGroup

	for i := 0; i < numWorkers; i++ {
		wg.Add(1)
		go s.jarProcessingWorker(&wg, tasks, results)
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
			logging.Warnf("ModLoader: Failed to load mod metadata from file '%s.jar': %v", res.baseFileName, res.parseError)
			continue
		}
		if res.modData != nil {
			s.logParsedFile(res)
			collectedModFileResults = append(collectedModFileResults, res.modData)
		}
	}
	return collectedModFileResults
}

// logParsedFile handles the logging for a single successfully parsed file result.
func (s *ModLoaderService) logParsedFile(res processFileResult) {
	currentMod := res.modData.mod
	nestedMods := res.modData.nested

	logging.Infof("ModLoader: ├─ Mod %s (%s v%s) from file '%s.jar'",
		currentMod.FabricInfo.ID, currentMod.FriendlyName(), currentMod.FabricInfo.Version,
		res.baseFileName)

	for i, nested := range nestedMods {
		treeSymbol := "├"
		if i == len(nestedMods)-1 {
			treeSymbol = "└"
		}
		logging.Infof("ModLoader: │   %s─ Mod %s (%s v%s) provided by %s from '%s'.",
			treeSymbol, nested.fmj.ID, nested.fmj.Name, nested.fmj.Version, currentMod.FabricInfo.ID, nested.pathInJar)
	}
}

// jarProcessingWorker is a goroutine worker that processes file tasks.
func (s *ModLoaderService) jarProcessingWorker(wg *sync.WaitGroup, tasks <-chan processFileTask, results chan<- processFileResult) {
	defer wg.Done()
	for task := range tasks {
		if task.progressFunc != nil {
			task.progressFunc(task.fileEntry.Name())
		}

		fullPath := filepath.Join(task.modsDir, task.fileEntry.Name())
		isJarFile := strings.HasSuffix(strings.ToLower(task.fileEntry.Name()), ".jar")
		baseFilename := strings.TrimSuffix(task.fileEntry.Name(), ".jar.disabled")
		baseFilename = strings.TrimSuffix(baseFilename, ".jar")

		topLevelFmj, nestedMods, err := extractModMetadata(fullPath)
		if err != nil {
			results <- processFileResult{baseFileName: baseFilename, parseError: fmt.Errorf("extracting metadata from %s: %w", task.fileEntry.Name(), err)}
			continue
		}
		currentMod := &Mod{
			Path:              fullPath,
			BaseFilename:      baseFilename,
			FabricInfo:        topLevelFmj,
			IsInitiallyActive: isJarFile,
		}
		results <- processFileResult{
			modData:      &parsedModFile{mod: currentMod, nested: nestedMods},
			baseFileName: baseFilename,
		}
	}
}

// resolveModConflicts handles multiple JAR files providing the same mod ID, choosing a winner.
func resolveModConflicts(parsedFileResults []*parsedModFile, allMods map[string]*Mod, modsDir string) error {
	parsedCandidates := make(map[string][]*parsedModFile)
	for _, modFile := range parsedFileResults {
		modID := modFile.mod.FabricInfo.ID
		parsedCandidates[modID] = append(parsedCandidates[modID], modFile)
	}

	var multiError []string
	var disabledDuplicates []string

	for modID, candidatesData := range parsedCandidates {
		winnerData := determineWinner(modID, candidatesData)
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
				if err := disableDuplicateFile(modsDir, cData.mod.BaseFilename); err != nil {
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
		logging.Infof("ModLoader: Disabled %d non-winning duplicate active files: %s", len(disabledDuplicates), strings.Join(disabledDuplicates, ", "))
	}
	if len(multiError) > 0 {
		return fmt.Errorf("encountered errors during conflict resolution: %s", strings.Join(multiError, "; "))
	}
	return nil
}

// determineWinner selects the preferred mod file from candidates for the same mod ID.
func determineWinner(modID string, candidates []*parsedModFile) *parsedModFile {
	if len(candidates) == 1 {
		return candidates[0]
	}

	logging.Warnf("ModLoader: Found %d conflicting files for mod %s. Determining winner by version...", len(candidates), modID)
	winnerIdx := 0
	for i := 1; i < len(candidates); i++ {
		// Compare versions using the semantic versioning helper.
		if compareVersions(candidates[i].mod.FabricInfo.Version, candidates[winnerIdx].mod.FabricInfo.Version) > 0 {
			winnerIdx = i
		}
	}
	logging.Infof("ModLoader: Winner for mod %s is v%s from file '%s'.",
		modID, candidates[winnerIdx].mod.FabricInfo.Version, filepath.Base(candidates[winnerIdx].mod.Path))
	return candidates[winnerIdx]
}

// disableDuplicateFile renames an active JAR file to its .disabled counterpart.
func disableDuplicateFile(modsDir, baseFilename string) error {
	activePath := filepath.Join(modsDir, baseFilename+".jar")
	disabledPath := filepath.Join(modsDir, baseFilename+".jar.disabled")

	if _, err := os.Stat(activePath); os.IsNotExist(err) {
		return nil // File not found, nothing to do.
	} else if err != nil {
		return fmt.Errorf("stat active file %s: %w", activePath, err)
	}

	// If a .disabled version already exists, remove it to allow rename.
	if _, err := os.Stat(disabledPath); err == nil {
		logging.Warnf("ModLoader: Removing existing disabled file '%s' before disabling '%s'.", filepath.Base(disabledPath), baseFilename)
		if remErr := os.Remove(disabledPath); remErr != nil {
			return fmt.Errorf("remove existing %s: %w", disabledPath, remErr)
		}
	}

	return os.Rename(activePath, disabledPath)
}

// populateProviderMaps populates the potentialProviders map and the EffectiveProvides for each mod.
func populateProviderMaps(allMods map[string]*Mod, potentialProviders PotentialProvidersMap) {
	for _, mod := range allMods {
		mod.EffectiveProvides = make(map[string]string)
		providerInfoBase := ProviderInfo{TopLevelModID: mod.FabricInfo.ID, TopLevelModVersion: mod.FabricInfo.Version}

		// Direct provides (the mod itself)
		addProvider(potentialProviders, mod.EffectiveProvides, mod.FabricInfo.ID, mod.FabricInfo.Version, providerInfoBase, true)

		// Explicit 'provides' array in fabric.mod.json
		for _, p := range mod.FabricInfo.Provides {
			addProvider(potentialProviders, mod.EffectiveProvides, p, mod.FabricInfo.Version, providerInfoBase, true)
		}

		// Provides from nested modules
		for _, nestedFmj := range mod.NestedModules {
			nestedProviderInfo := providerInfoBase
			nestedProviderInfo.VersionOfProvidedItem = nestedFmj.Version
			addProvider(potentialProviders, mod.EffectiveProvides, nestedFmj.ID, nestedFmj.Version, nestedProviderInfo, false)
			for _, p := range nestedFmj.Provides {
				addProvider(potentialProviders, mod.EffectiveProvides, p, nestedFmj.Version, nestedProviderInfo, false)
			}
		}
	}

	sortAndLogProviders(allMods, potentialProviders)
}

// addProvider is a helper to add provider information to the relevant maps.
func addProvider(potentialProviders PotentialProvidersMap, effectiveProvides map[string]string,
	providedID, version string, baseInfo ProviderInfo, isDirect bool) {

	updateEffectiveProvides(effectiveProvides, providedID, version)

	providerInfo := baseInfo
	providerInfo.VersionOfProvidedItem = version
	providerInfo.IsDirectProvide = isDirect
	addSingleProviderInfo(potentialProviders, providedID, providerInfo)
}

// sortAndLogProviders sorts all provider lists for determinism and logs them.
func sortAndLogProviders(allMods map[string]*Mod, potentialProviders PotentialProvidersMap) {
	var providerLogMessages []string
	sortedDepIDs := make([]string, 0, len(potentialProviders))
	for depID := range potentialProviders {
		sortedDepIDs = append(sortedDepIDs, depID)
	}
	sort.Strings(sortedDepIDs)

	for _, depID := range sortedDepIDs {
		infos := potentialProviders[depID]
		sortProviders(infos) // Sort for deterministic resolution
		potentialProviders[depID] = infos

		// Do not log implicit providers
		if IsImplicitMod(depID) {
			continue
		}
		// Do not log simple self-provision (e.g., 'balm' provides 'balm')
		if len(infos) == 1 && infos[0].IsDirectProvide && infos[0].TopLevelModID == depID {
			continue
		}
		providerLogMessages = append(providerLogMessages, formatProviderLog(depID, infos, allMods)...)
	}

	if len(providerLogMessages) > 0 {
		logging.Infof("ModLoader: Populated dependency providers for non-trivial dependencies:\n%s", strings.Join(providerLogMessages, "\n"))
	}
}

// formatProviderLog creates the log message lines for a given dependency and its providers.
func formatProviderLog(depID string, infos []ProviderInfo, allMods map[string]*Mod) []string {
	var messages []string
	if len(infos) == 1 {
		info := infos[0]
		providingMod := allMods[info.TopLevelModID]
		messages = append(messages, fmt.Sprintf("  - Dependency %s provided by %s (at v%s) from '%s'",
			depID, info.TopLevelModID, info.VersionOfProvidedItem, providingMod.BaseFilename+".jar"))
	} else {
		messages = append(messages, fmt.Sprintf("  - Dependency %s provided by:", depID))
		for _, info := range infos {
			providingMod := allMods[info.TopLevelModID]
			messages = append(messages, fmt.Sprintf("      - %s (at v%s) from '%s'",
				info.TopLevelModID, info.VersionOfProvidedItem, providingMod.BaseFilename+".jar"))
		}
	}
	return messages
}

// sortProviders sorts a slice of ProviderInfo for deterministic best-provider selection.
// The sort order is: Direct > Higher Item Version > Higher Top-Level Mod Version.
func sortProviders(infos []ProviderInfo) {
	if len(infos) < 2 {
		return
	}
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
// This runs after all mods are parsed, so we don't need to check for conflicts here.
func addImplicitProvides(potentialProviders PotentialProvidersMap) {
	implicitIDs := []string{"java", "minecraft", "fabricloader"}
	for _, id := range implicitIDs {
		potentialProviders[id] = append(potentialProviders[id], ProviderInfo{
			TopLevelModID: id, VersionOfProvidedItem: "0.0.0",
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

// compareVersions compares two version strings using relaxed semver rules.
func compareVersions(v1Str, v2Str string) int {
	if v1Str == v2Str {
		return 0
	}

	v1, err1 := ParseExtendedSemVer(v1Str)
	v2, err2 := ParseExtendedSemVer(v2Str)

	if err1 == nil && err2 == nil {
		return v1.Compare(v2)
	}

	logging.Warnf("ModLoader: Could not parse one or both versions ('%s', '%s'). Falling back to string comparison.", v1Str, v2Str)
	if err1 == nil {
		return 1
	}
	if err2 == nil {
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

// applyOverridesToLoadedMods applies a final, merged set of override rules.
func (s *ModLoaderService) applyOverridesToLoadedMods(mods map[string]*Mod, overrides *DependencyOverrides) {
	if overrides == nil || len(overrides.Rules) == 0 {
		return
	}

	for _, rule := range overrides.Rules {
		targetMod, exists := mods[rule.Target()]
		if !exists {
			logging.Warnf("ModLoader: Override rule for unknown mod '%s'. Skipping.", rule.Target())
			continue
		}
		rule.Apply(&targetMod.FabricInfo)
	}
}
