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
	"runtime"
	"sort"
	"strings"
	"sync"

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

type processFileTask struct {
	fileEntry    os.DirEntry
	modsDir      string
	progressFunc func(string)
}

type processFileResult struct {
	modData    fullModData
	parseError error
	fileName   string
}

type fullModData struct {
	mod        *Mod
	nestedFmjs []FabricModJson
}

// LoadMods orchestrates the loading, parsing, and conflict resolution of mods.
func LoadMods(modsDir string, progressReport func(fileNameBeingProcessed string)) (
	allMods map[string]*Mod, potentialProviders PotentialProvidersMap, sortedModIDs []string, err error) {

	allMods = make(map[string]*Mod)
	potentialProviders = make(PotentialProvidersMap)
	addImplicitProvides(potentialProviders)

	diskFiles, readErr := os.ReadDir(modsDir)
	if readErr != nil {
		return nil, nil, nil, fmt.Errorf("reading mods directory %s: %w", modsDir, readErr)
	}

	filesToProcess := filterJarFiles(diskFiles)
	if len(filesToProcess) == 0 {
		log.Printf("%sNo .jar or .jar.disabled files found in %s", LogInfoPrefix, modsDir)
		return allMods, potentialProviders, sortedModIDs, nil
	}

	// Pass 1: Concurrently parse JAR files
	parsedFileResults := parseJarFilesConcurrently(filesToProcess, modsDir, progressReport)
	// Pass 2: Resolve conflicts and select winning mods
	resolveModConflicts(parsedFileResults, allMods, modsDir)
	// Pass 3: Populate provider maps from winning mods
	populateProviderMaps(allMods, potentialProviders)

	for id := range allMods {
		sortedModIDs = append(sortedModIDs, id)
	}
	sort.Strings(sortedModIDs)

	return allMods, potentialProviders, sortedModIDs, nil
}

// filterJarFiles selects only .jar and .jar.disabled files from directory entries.
func filterJarFiles(diskFiles []os.DirEntry) []os.DirEntry {
	var filesToProcess []os.DirEntry
	for _, file := range diskFiles {
		if file.IsDir() {
			continue
		}
		fileName := file.Name()
		isJar := strings.HasSuffix(strings.ToLower(fileName), ".jar")
		isDisabledJar := strings.HasSuffix(strings.ToLower(fileName), ".jar.disabled")
		if isJar || isDisabledJar {
			filesToProcess = append(filesToProcess, file)
		}
	}
	return filesToProcess
}

// parseJarFilesConcurrently processes JAR files in parallel to extract metadata.
func parseJarFilesConcurrently(filesToProcess []os.DirEntry, modsDir string, progressReport func(string)) []fullModData {
	numWorkers := min(len(filesToProcess), runtime.NumCPU())

	tasks := make(chan processFileTask, len(filesToProcess))
	results := make(chan processFileResult, len(filesToProcess))
	var wg sync.WaitGroup

	for range numWorkers {
		wg.Add(1)
		go jarProcessingWorker(&wg, tasks, results)
	}

	for _, file := range filesToProcess {
		tasks <- processFileTask{fileEntry: file, modsDir: modsDir, progressFunc: progressReport}
	}
	close(tasks)

	go func() {
		wg.Wait()
		close(results)
	}()

	var collectedModFileResults []fullModData
	for res := range results {
		if res.parseError != nil {
			log.Printf("%sSkipping JAR %s: %v", LogWarningPrefix, res.fileName, res.parseError)
			continue
		}
		collectedModFileResults = append(collectedModFileResults, res.modData)
	}
	return collectedModFileResults
}

// jarProcessingWorker is the goroutine function for processing individual JAR files.
func jarProcessingWorker(wg *sync.WaitGroup, tasks <-chan processFileTask, results chan<- processFileResult) {
	defer wg.Done()
	for task := range tasks {
		if task.progressFunc != nil {
			task.progressFunc(task.fileEntry.Name())
		}

		var baseFilenameForRename string
		isJarFile := strings.HasSuffix(strings.ToLower(task.fileEntry.Name()), ".jar")
		if isJarFile {
			baseFilenameForRename = strings.TrimSuffix(task.fileEntry.Name(), ".jar")
		} else {
			baseFilenameForRename = strings.TrimSuffix(task.fileEntry.Name(), ".jar.disabled")
		}
		fullPath := filepath.Join(task.modsDir, task.fileEntry.Name())

		topLevelFmj, nestedFmjs, errExtract := extractModMetadata(fullPath, task.progressFunc)
		if errExtract != nil {
			results <- processFileResult{fileName: task.fileEntry.Name(), parseError: fmt.Errorf("extracting metadata: %w", errExtract)}
			continue
		}
		currentMod := &Mod{
			Path:              fullPath,
			BaseFilename:      baseFilenameForRename,
			FabricInfo:        topLevelFmj,
			IsInitiallyActive: isJarFile,
			IsCurrentlyActive: isJarFile,
		}
		results <- processFileResult{
			modData:  fullModData{mod: currentMod, nestedFmjs: nestedFmjs},
			fileName: task.fileEntry.Name(),
		}
	}
}

// resolveModConflicts processes parsed mod data to select a single "winner" for each mod ID.
// Non-winning duplicates are disabled on disk.
func resolveModConflicts(parsedFileResults []fullModData, allMods map[string]*Mod, modsDir string) {
	parsedCandidates := make(map[string][]fullModData)
	for _, modData := range parsedFileResults {
		modID := modData.mod.FabricInfo.ID
		parsedCandidates[modID] = append(parsedCandidates[modID], modData)
	}

	for modID, candidatesData := range parsedCandidates {
		winnerDataIdx := 0
		if len(candidatesData) > 1 {
			log.Printf("%sConflict for mod ID '%s': Found %d files. Determining winner by version.", LogInfoPrefix, modID, len(candidatesData))
			for i := 1; i < len(candidatesData); i++ {
				if compareVersions(candidatesData[i].mod.FabricInfo.Version, candidatesData[winnerDataIdx].mod.FabricInfo.Version) > 0 {
					winnerDataIdx = i
				}
			}
			log.Printf("%sWinner for mod ID '%s' is '%s' (v%s) from file '%s'.",
				LogInfoPrefix, modID, candidatesData[winnerDataIdx].mod.FriendlyName(),
				candidatesData[winnerDataIdx].mod.FabricInfo.Version, filepath.Base(candidatesData[winnerDataIdx].mod.Path))
		}

		winnerMod := candidatesData[winnerDataIdx].mod
		winnerMod.NestedModules = candidatesData[winnerDataIdx].nestedFmjs
		allMods[modID] = winnerMod

		for i, cData := range candidatesData {
			if i == winnerDataIdx {
				continue
			}
			if cData.mod.IsInitiallyActive {
				log.Printf("%sDisabling non-winning duplicate file '%s' for mod ID '%s'.", LogInfoPrefix, filepath.Base(cData.mod.Path), modID)
				if errDisable := disableDuplicateFile(modsDir, cData.mod.BaseFilename, cData.mod.Path); errDisable != nil {
					log.Printf("%sError disabling non-winning duplicate %s: %v", LogErrorPrefix, filepath.Base(cData.mod.Path), errDisable)
				} else {
					cData.mod.IsCurrentlyActive = false
				}
			}
		}
	}
}

// populateProviderMaps fills PotentialProvidersMap and Mod.EffectiveProvides from the winning mods.
func populateProviderMaps(allMods map[string]*Mod, potentialProviders PotentialProvidersMap) {
	for _, mod := range allMods {
		mod.EffectiveProvides = make(map[string]string)
		providerInfoBase := ProviderInfo{TopLevelModID: mod.FabricInfo.ID, TopLevelModVersion: mod.FabricInfo.Version}

		directProviderInfo := providerInfoBase
		directProviderInfo.IsDirectProvide = true
		directProviderInfo.VersionOfProvidedItem = mod.FabricInfo.Version
		updateEffectiveProvides(mod.EffectiveProvides, mod.FabricInfo.ID, mod.FabricInfo.Version)
		addSingleProviderInfo(potentialProviders, mod.FabricInfo.ID, directProviderInfo)
		for _, p := range mod.FabricInfo.Provides {
			updateEffectiveProvides(mod.EffectiveProvides, p, mod.FabricInfo.Version)
			addSingleProviderInfo(potentialProviders, p, directProviderInfo)
		}

		nestedProviderInfoBase := providerInfoBase
		nestedProviderInfoBase.IsDirectProvide = false
		for _, nestedFmj := range mod.NestedModules {
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

	for _, infos := range potentialProviders {
		sort.Slice(infos, func(i, j int) bool {
			if infos[i].IsDirectProvide != infos[j].IsDirectProvide {
				return infos[i].IsDirectProvide
			}
			compItemVer := compareVersions(infos[i].VersionOfProvidedItem, infos[j].VersionOfProvidedItem)
			if compItemVer != 0 {
				return compItemVer > 0
			}
			return compareVersions(infos[i].TopLevelModVersion, infos[j].TopLevelModVersion) > 0
		})
	}
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

func disableDuplicateFile(modsDir, baseFilename, currentPath string) error {
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
	log.Printf("%sDisabled non-winning duplicate: %s -> %s", LogInfoPrefix, filepath.Base(currentPath), filepath.Base(disabledPath))
	return nil
}

func addImplicitProvides(potentialProviders PotentialProvidersMap) {
	implicitIDs := []string{"java", "minecraft", "fabricloader"}
	for _, id := range implicitIDs {
		potentialProviders[id] = append(potentialProviders[id], ProviderInfo{
			TopLevelModID: id, VersionOfProvidedItem: "0.0.0",
			IsDirectProvide: true, TopLevelModVersion: "0.0.0",
		})
	}
}

func addSingleProviderInfo(potentialProviders PotentialProvidersMap, providedID string, info ProviderInfo) {
	if providedID == "" {
		return
	}
	potentialProviders[providedID] = append(potentialProviders[providedID], info)
}

// extractModMetadata reads fabric.mod.json from a JAR and its nested JARs.
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

	for _, nestedJarEntry := range topLevelFmj.Jars {
		nestedJarPathInZip := nestedJarEntry.File
		if nestedJarPathInZip == "" {
			log.Printf("%sSkipping nested JAR entry with empty file path in %s", LogWarningPrefix, jarPath)
			continue
		}
		nestedJarPathInZip = filepath.ToSlash(nestedJarPathInZip)

		if progressReport != nil {
			progressReport(fmt.Sprintf("  - nested %s (in %s)", nestedJarPathInZip, filepath.Base(jarPath)))
		}

		var nestedZipFile *zip.File
		for _, f := range zr.File {
			if filepath.ToSlash(f.Name) == nestedJarPathInZip {
				nestedZipFile = f
				break
			}
		}

		if nestedZipFile == nil {
			log.Printf("%sNested JAR '%s' specified in %s not found in archive.", LogWarningPrefix, nestedJarPathInZip, jarPath)
			continue
		}

		nestedFmj, errParse := parseNestedJarZipEntry(nestedZipFile, fmt.Sprintf("%s in %s", nestedJarPathInZip, jarPath))
		if errParse != nil {
			log.Printf("%sSkipping nested JAR %s in %s: %v", LogWarningPrefix, nestedJarPathInZip, jarPath, errParse)
			continue
		}
		nestedFmjs = append(nestedFmjs, nestedFmj)
	}
	return topLevelFmj, nestedFmjs, nil
}

// parseNestedJarZipEntry handles the specifics of reading a fabric.mod.json from a JAR that is itself an entry in another ZIP.
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
	if v1Str == v2Str {
		return 0
	}
	if v1Str == "" {
		return -1
	}
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

	log.Printf("%sNon-SemVer version(s): '%s' ('%s') vs '%s' ('%s'). Fallback string comparison.",
		LogWarningPrefix, v1Str, canonV1Str, v2Str, canonV2Str)

	if v1Valid {
		return 1
	}
	if v2Valid {
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

// parseFabricModJsonFromReader finds and parses fabric.mod.json from a zip.Reader.
func parseFabricModJsonFromReader(zipReader *zip.Reader, jarIdentifier string) (FabricModJson, error) {
	var fmj FabricModJson
	foundFmjFile := false
	for _, f := range zipReader.File {
		if strings.EqualFold(f.Name, "fabric.mod.json") {
			fmjData, err := readZipFileEntry(f)
			if err != nil {
				return FabricModJson{}, fmt.Errorf("reading fabric.mod.json from %s: %w", jarIdentifier, err)
			}
			fmjData = sanitizeJsonStringContent(fmjData)
			if err := json5.Unmarshal(fmjData, &fmj); err != nil {
				dataSnippet := string(fmjData)
				if len(dataSnippet) > 200 {
					dataSnippet = dataSnippet[:200] + "..."
				}
				return FabricModJson{}, fmt.Errorf("unmarshaling fabric.mod.json from %s (data snippet: %s): %w", jarIdentifier, dataSnippet, err)
			}
			if fmj.ID == "" {
				return FabricModJson{}, fmt.Errorf("fabric.mod.json from %s parsed but has empty mod ID", jarIdentifier)
			}
			foundFmjFile = true
			break
		}
	}
	if !foundFmjFile {
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
		escapedValue = bytes.ReplaceAll(escapedValue, []byte("\r"), []byte{})
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

// readZipFileEntry reads the content of a single file from a zip archive.
func readZipFileEntry(f *zip.File) ([]byte, error) {
	rc, err := f.Open()
	if err != nil {
		return nil, err
	}
	defer rc.Close()
	return io.ReadAll(rc)
}

// Enable attempts to make the mod active by renaming .jar.disabled to .jar.
func (m *Mod) Enable(modsDir string) error {
	activePath := filepath.Join(modsDir, m.BaseFilename+".jar")
	disabledPath := filepath.Join(modsDir, m.BaseFilename+".jar.disabled")

	if m.IsCurrentlyActive {
		if _, err := os.Stat(activePath); err == nil {
			return nil
		}
		log.Printf("%sMod %s (%s) marked active but %s missing. Attempting to enable from %s.",
			LogWarningPrefix, m.FriendlyName(), m.ModID(), filepath.Base(activePath), filepath.Base(disabledPath))
	}

	if _, err := os.Stat(disabledPath); os.IsNotExist(err) {
		if _, errStatActive := os.Stat(activePath); errStatActive == nil {
			m.IsCurrentlyActive = true
			m.Path = activePath
			log.Printf("%sMod %s (%s) found already active at %s (disabled file %s not found). State corrected.",
				LogInfoPrefix, m.FriendlyName(), m.ModID(), filepath.Base(activePath), filepath.Base(disabledPath))
			return nil
		}
		return fmt.Errorf("cannot enable mod %s (%s): disabled file %s and active file %s not found",
			m.FriendlyName(), m.ModID(), filepath.Base(disabledPath), filepath.Base(activePath))
	}

	err := os.Rename(disabledPath, activePath)
	if err == nil {
		m.IsCurrentlyActive = true
		m.Path = activePath
		return nil
	}
	if os.IsExist(err) || errors.Is(err, os.ErrExist) {
		log.Printf("%sEnable Mod %s (%s): target %s already exists (rename failed). Assuming active.",
			LogInfoPrefix, m.FriendlyName(), m.ModID(), filepath.Base(activePath))
		if _, errStatActive := os.Stat(activePath); errStatActive == nil {
			m.IsCurrentlyActive = true
			m.Path = activePath
			return nil
		}
	}
	return fmt.Errorf("%w: enabling mod %s (%s) by renaming %s to %s: %v",
		ErrRenameFailedSkippable, m.FriendlyName(), m.ModID(), filepath.Base(disabledPath), filepath.Base(activePath), err)
}

// Disable attempts to make the mod inactive by renaming .jar to .jar.disabled.
func (m *Mod) Disable(modsDir string) error {
	activePath := filepath.Join(modsDir, m.BaseFilename+".jar")
	disabledPath := filepath.Join(modsDir, m.BaseFilename+".jar.disabled")

	if !m.IsCurrentlyActive {
		if _, err := os.Stat(disabledPath); err == nil {
			return nil
		}
		log.Printf("%sMod %s (%s) marked inactive but %s missing. Attempting to disable from %s.",
			LogWarningPrefix, m.FriendlyName(), m.ModID(), filepath.Base(disabledPath), filepath.Base(activePath))
	}

	if _, err := os.Stat(activePath); os.IsNotExist(err) {
		if _, errStatDisabled := os.Stat(disabledPath); errStatDisabled == nil {
			m.IsCurrentlyActive = false
			m.Path = disabledPath
			log.Printf("%sMod %s (%s) found already disabled at %s (active file %s not found). State corrected.",
				LogInfoPrefix, m.FriendlyName(), m.ModID(), filepath.Base(disabledPath), filepath.Base(activePath))
			return nil
		}
		return fmt.Errorf("cannot disable mod %s (%s): active file %s and disabled file %s not found",
			m.FriendlyName(), m.ModID(), filepath.Base(activePath), filepath.Base(disabledPath))
	}

	if _, err := os.Stat(disabledPath); err == nil {
		if remErr := os.Remove(disabledPath); remErr != nil {
			log.Printf("%sFailed to remove existing %s before disabling %s: %v. Rename may fail.",
				LogErrorPrefix, filepath.Base(disabledPath), filepath.Base(activePath), remErr)
		}
	}

	err := os.Rename(activePath, disabledPath)
	if err == nil {
		m.IsCurrentlyActive = false
		m.Path = disabledPath
		return nil
	}
	if os.IsExist(err) || errors.Is(err, os.ErrExist) {
		log.Printf("%sDisable Mod %s (%s): target %s already exists (rename failed). Assuming disabled.",
			LogInfoPrefix, m.FriendlyName(), m.ModID(), filepath.Base(disabledPath))
		if _, errStatDisabled := os.Stat(disabledPath); errStatDisabled == nil {
			m.IsCurrentlyActive = false
			m.Path = disabledPath
			return nil
		}
	}
	return fmt.Errorf("%w: disabling mod %s (%s) by renaming %s to %s: %v",
		ErrRenameFailedSkippable, m.FriendlyName(), m.ModID(), filepath.Base(activePath), filepath.Base(disabledPath), err)
}
