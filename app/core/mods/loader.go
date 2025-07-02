package mods

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"

	"github.com/Qendolin/fabric-mod-bisect-tool/app/core/mods/version"
	"github.com/Qendolin/fabric-mod-bisect-tool/app/core/sets"
	"github.com/Qendolin/fabric-mod-bisect-tool/app/logging"
)

// ModLoader loads mod information from the filesystem, parses metadata,
// resolves conflicts, and builds dependency provider maps.
type ModLoader struct {
	ModParser
}

// bufferedLog represents a log message captured by a worker for later printing.
// It is defined here as it is an internal implementation detail of the concurrent loader.
type bufferedLog struct {
	Level   logging.LogLevel
	Message string
}

// logBuffer is a slice of bufferedLogs with a helper method for appending.
// This makes the calling code in the parser much cleaner.
type logBuffer []bufferedLog

// add formats and appends a new log entry to the buffer.
func (b *logBuffer) add(level logging.LogLevel, format string, v ...interface{}) {
	*b = append(*b, bufferedLog{Level: level, Message: fmt.Sprintf(format, v...)})
}

// task to be processed by a worker goroutine.
type processFileTask struct {
	fileEntry    os.DirEntry
	modsDir      string
	progressFunc func(string)
}

// result of a worker goroutine processing a single file.
type processFileResult struct {
	mod          *Mod
	parseError   error
	baseFileName string
	logs         logBuffer // Use the new logBuffer type.
}

// LoadMods discovers mods, parses metadata, resolves basic conflicts, and builds provider maps.
func (ml *ModLoader) LoadMods(modsDir string, overrides *DependencyOverrides, progressReport func(fileNameBeingProcessed string)) (
	map[string]*Mod, PotentialProvidersMap, []string, error,
) {
	if ml.QuiltParsing {
		logging.Info("ModLoader: Loading mods with Quilt support. Please note that log messages with fabric.mod.json might refer to quilt.mod.json too.")
	}

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

	parsedFileResults := ml.parseJarFilesConcurrently(filesToProcess, modsDir, progressReport)

	allMods := make(map[string]*Mod)
	if err := resolveModConflicts(parsedFileResults, allMods, modsDir); err != nil {
		logging.Errorf("ModLoader: Error during mod conflict resolution: %v. Proceeding with available mods.", err)
	}

	if overrides != nil && len(overrides.Rules) > 0 {
		logging.Info("ModLoader: Applying dependency overrides...")
		ml.applyOverridesToLoadedMods(allMods, overrides)
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
func (ml *ModLoader) parseJarFilesConcurrently(filesToProcess []os.DirEntry, modsDir string, progressReport func(string)) []*processFileResult {
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
		go ml.jarProcessingWorker(&wg, tasks, results)
	}

	for _, file := range filesToProcess {
		tasks <- processFileTask{fileEntry: file, modsDir: modsDir, progressFunc: progressReport}
	}
	close(tasks)

	go func() {
		wg.Wait()
		close(results)
	}()

	var collectedResults []*processFileResult
	for res := range results {
		// Drain the log buffer from the worker first. This ensures log messages
		// appear before the final status message for that file.
		for _, logEntry := range res.logs {
			switch logEntry.Level {
			case logging.LevelDebug:
				logging.Debug(logEntry.Message)
			case logging.LevelInfo:
				logging.Info(logEntry.Message)
			case logging.LevelWarn:
				logging.Warn(logEntry.Message)
			case logging.LevelError:
				logging.Error(logEntry.Message)
			}
		}

		if res.parseError != nil {
			logging.Warnf("ModLoader: Failed to load mod metadata from file '%s.jar': %v", res.baseFileName, res.parseError)
			continue
		}
		if res.mod != nil {
			ml.logParsedFile(res)
			collectedResults = append(collectedResults, &res)
		}
	}
	return collectedResults
}

// logParsedFile handles the logging for a single successfully parsed file result.
func (ml *ModLoader) logParsedFile(res processFileResult) {
	currentMod := res.mod
	nestedMods := res.mod.NestedModules

	logging.Infof("ModLoader: ├─ Mod %s (%s v%s) from file '%s.jar'",
		currentMod.FabricInfo.ID, currentMod.FriendlyName(), currentMod.FabricInfo.Version.Version,
		res.baseFileName)

	for i, nested := range nestedMods {
		treeSymbol := "├"
		if i == len(nestedMods)-1 {
			treeSymbol = "└"
		}
		logging.Infof("ModLoader: │   %s─ Mod %s (%s v%s) provided by %s.",
			treeSymbol, nested.ID, nested.Name, nested.Version.Version, currentMod.FabricInfo.ID)
	}
}

// jarProcessingWorker is a goroutine worker that processes file tasks.
func (ml *ModLoader) jarProcessingWorker(wg *sync.WaitGroup, tasks <-chan processFileTask, results chan<- processFileResult) {
	defer wg.Done()
	for task := range tasks {
		if task.progressFunc != nil {
			task.progressFunc(task.fileEntry.Name())
		}

		fullPath := filepath.Join(task.modsDir, task.fileEntry.Name())
		isJarFile := strings.HasSuffix(strings.ToLower(task.fileEntry.Name()), ".jar")
		baseFilename := strings.TrimSuffix(task.fileEntry.Name(), ".jar.disabled")
		baseFilename = strings.TrimSuffix(baseFilename, ".jar")

		// Create a log buffer for this specific task.
		var logBuffer logBuffer
		topLevelFmj, nestedFmjs, err := ml.ExtractModMetadata(fullPath, &logBuffer)
		if err != nil {
			results <- processFileResult{baseFileName: baseFilename, parseError: fmt.Errorf("extracting metadata from %s: %w", task.fileEntry.Name(), err), logs: logBuffer}
			continue
		}

		currentMod := &Mod{
			Path:              fullPath,
			BaseFilename:      baseFilename,
			FabricInfo:        topLevelFmj,
			IsInitiallyActive: isJarFile,
			NestedModules:     nestedFmjs,
		}
		results <- processFileResult{
			mod:          currentMod,
			baseFileName: baseFilename,
			logs:         logBuffer,
		}
	}
}

// resolveModConflicts handles multiple JAR files providing the same mod ID, choosing a winner.
func resolveModConflicts(parsedFileResults []*processFileResult, allMods map[string]*Mod, modsDir string) error {
	// Group all parsed results by the primary mod ID they represent.
	candidatesByID := make(map[string][]*Mod)
	for _, res := range parsedFileResults {
		modID := res.mod.FabricInfo.ID
		candidatesByID[modID] = append(candidatesByID[modID], res.mod)
	}

	var multiError []string
	var disabledDuplicates []string

	for modID, candidates := range candidatesByID {
		winner := determineWinner(modID, candidates)
		allMods[modID] = winner // Add ONLY the winner to the final mod map.

		// Now, handle disabling files for the losers.
		for _, loser := range candidates {
			if loser.Path == winner.Path {
				continue // Don't disable the winner.
			}
			// Only disable files that are currently active.
			if loser.IsInitiallyActive {
				if err := disableDuplicateFile(modsDir, loser.BaseFilename); err != nil {
					errMsg := fmt.Sprintf("error disabling non-winning duplicate '%s' (for mod %s): %v", loser.BaseFilename, modID, err)
					logging.Error(errMsg)
					multiError = append(multiError, errMsg)
				} else {
					disabledDuplicates = append(disabledDuplicates, loser.BaseFilename+".jar")
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

// determineWinner sorts candidates for the same mod ID and selects the best one.
// The priority is: Higher Version > Alphabetical Filename (as a stable tie-breaker).
func determineWinner(modID string, candidates []*Mod) *Mod {
	// This function is called when multiple files provide the same top-level mod ID.
	if len(candidates) == 1 {
		return candidates[0]
	}

	logging.Warnf("ModLoader: Found %d conflicting files for mod %s. Determining winner by version...", len(candidates), modID)

	// Sort the candidates slice in-place to find the best one.
	sort.Slice(candidates, func(i, j int) bool {
		// Rule 1: Higher version is higher priority.
		v1 := candidates[i].FabricInfo.Version.Version
		v2 := candidates[j].FabricInfo.Version.Version
		versionCmp := v1.Compare(v2)
		if versionCmp != 0 {
			return versionCmp > 0 // true if i > j, resulting in descending order.
		}

		// Rule 2 (Tie-breaker): Alphabetical base filename for deterministic order.
		return candidates[i].BaseFilename < candidates[j].BaseFilename
	})

	winner := candidates[0]
	logging.Infof("ModLoader: Winner for mod %s is v%s from file '%s'.",
		modID, winner.FabricInfo.Version.Version, winner.BaseFilename+".jar")

	return winner
}

// disableDuplicateFile renames an active JAR file to its .disabled counterpart.
func disableDuplicateFile(modsDir, baseFilename string) error {
	activePath := filepath.Join(modsDir, baseFilename+".jar")
	disabledPath := filepath.Join(modsDir, baseFilename+".jar.disabled")

	if _, err := os.Stat(activePath); os.IsNotExist(err) {
		return nil
	} else if err != nil {
		return fmt.Errorf("stat active file %s: %w", activePath, err)
	}

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
		mod.EffectiveProvides = make(map[string]version.Version)
		providerInfoBase := ProviderInfo{TopLevelModID: mod.FabricInfo.ID, TopLevelModVersion: mod.FabricInfo.Version.Version}

		addProvider(potentialProviders, mod.EffectiveProvides, mod.FabricInfo.ID, mod.FabricInfo.Version.Version, providerInfoBase, true)

		for _, p := range mod.FabricInfo.Provides {
			addProvider(potentialProviders, mod.EffectiveProvides, p, mod.FabricInfo.Version.Version, providerInfoBase, true)
		}

		for _, nested := range mod.NestedModules {
			nestedProviderInfo := providerInfoBase
			nestedProviderInfo.VersionOfProvidedItem = nested.Version.Version
			addProvider(potentialProviders, mod.EffectiveProvides, nested.ID, nested.Version.Version, nestedProviderInfo, false)
			for _, p := range nested.Provides {
				addProvider(potentialProviders, mod.EffectiveProvides, p, nested.Version.Version, nestedProviderInfo, false)
			}
		}
	}
	sortAndLogProviders(allMods, potentialProviders)
}

// addProvider is a helper to add provider information to the relevant maps.
func addProvider(potentialProviders PotentialProvidersMap, effectiveProvides map[string]version.Version,
	providedID string, ver version.Version, baseInfo ProviderInfo, isDirect bool) {
	if ver == nil {
		return
	}
	updateEffectiveProvides(effectiveProvides, providedID, ver)

	providerInfo := baseInfo
	providerInfo.VersionOfProvidedItem = ver
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
		sortProviders(infos)
		potentialProviders[depID] = infos

		if IsImplicitMod(depID) {
			continue
		}
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
func sortProviders(infos []ProviderInfo) {
	if len(infos) < 2 {
		return
	}
	sort.Slice(infos, func(i, j int) bool {
		if infos[i].IsDirectProvide != infos[j].IsDirectProvide {
			return infos[i].IsDirectProvide
		}
		compItemVer := infos[i].VersionOfProvidedItem.Compare(infos[j].VersionOfProvidedItem)
		if compItemVer != 0 {
			return compItemVer > 0
		}
		return infos[i].TopLevelModVersion.Compare(infos[j].TopLevelModVersion) > 0
	})
}

// updateEffectiveProvides updates the effective provides map for a mod, prioritizing higher versions.
func updateEffectiveProvides(effectiveProvides map[string]version.Version, providedID string, ver version.Version) {
	if providedID == "" {
		return
	}
	if existingVersion, ok := effectiveProvides[providedID]; !ok || ver.Compare(existingVersion) > 0 {
		effectiveProvides[providedID] = ver
	}
}

// addImplicitProvides adds common implicit dependencies to the potential providers map.
func addImplicitProvides(potentialProviders PotentialProvidersMap) {
	implicitIDs := []string{"java", "minecraft", "fabricloader", "quilt_loader"}
	placeholderVersion, _ := version.Parse("0.0.0", false)

	for _, id := range implicitIDs {
		potentialProviders[id] = append(potentialProviders[id], ProviderInfo{
			TopLevelModID:         id,
			VersionOfProvidedItem: placeholderVersion,
			IsDirectProvide:       true,
			TopLevelModVersion:    placeholderVersion,
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

// applyOverridesToLoadedMods applies a final, merged set of override rules.
func (ml *ModLoader) applyOverridesToLoadedMods(mods map[string]*Mod, overrides *DependencyOverrides) {
	if overrides == nil || len(overrides.Rules) == 0 {
		return
	}

	rulesByModID := make(map[string][]OverrideRule)
	allRuleTargets := make(map[string]struct{})
	for _, rule := range overrides.Rules {
		targetID := rule.Target()
		rulesByModID[targetID] = append(rulesByModID[targetID], rule)
		allRuleTargets[targetID] = struct{}{}
	}

	foundTargets := make(map[string]struct{})

	for topLevelID, mod := range mods {
		if rules, ok := rulesByModID[topLevelID]; ok {
			logging.Infof("ModLoader: Applying %d override rule(s) to top-level mod %s.", len(rules), topLevelID)
			for _, rule := range rules {
				rule.Apply(&mod.FabricInfo)
				logging.Debugf("ModLoader:   - Applied rule: Target='%s', Field='%s', Key='%s', Action='%s', Value='%s'",
					rule.Target(), rule.Field(), rule.Key(), rule.Action().String(), rule.Value())
			}
			foundTargets[topLevelID] = struct{}{}
		}

		for i := range mod.NestedModules {
			nestedMod := &mod.NestedModules[i]
			if rules, ok := rulesByModID[nestedMod.ID]; ok {
				logging.Infof("ModLoader: Applying %d override rule(s) to nested mod %s (within %s).", len(rules), nestedMod.ID, topLevelID)
				for _, rule := range rules {
					rule.Apply(nestedMod)
					logging.Debugf("ModLoader:   - Applied rule: Target='%s', Field='%s', Key='%s', Action='%s', Value='%s'",
						rule.Target(), rule.Field(), rule.Key(), rule.Action().String(), rule.Value())
				}
				foundTargets[nestedMod.ID] = struct{}{}
			}
		}
	}

	unappliedTargets := sets.Subtract(allRuleTargets, foundTargets)
	if len(unappliedTargets) > 0 {
		logging.Warnf("ModLoader: Skipping override rule(s) for unknown mod(s) not found in any top-level or nested JAR: %v", sets.FormatSet(unappliedTargets))
	}
}
