package mods

import (
	"fmt"
	"sort"
	"strings"

	"github.com/Qendolin/fabric-mod-bisect-tool/app/logging"
)

// DependencyResolver calculates the effective set of mods.
type DependencyResolver struct {
	allMods            map[string]*Mod
	potentialProviders PotentialProvidersMap
	forceEnabled       map[string]bool
	forceDisabled      map[string]bool

	// Internal state for a single ResolveEffectiveSet call
	currentEffectiveSet map[string]bool           // Top-level ModIDs successfully activated.
	resolutionPath      map[string]ResolutionInfo // Tracks why each mod is in currentEffectiveSet.
	processing          map[string]bool           // DFS stack tracker for cycle detection.
	dependencySatisfied map[string]string         // Maps a dependencyID to the TopLevelModID that satisfies it.
}

// NewDependencyResolver creates a new DependencyResolver.
func NewDependencyResolver() *DependencyResolver {
	return &DependencyResolver{}
}

// ResolveEffectiveSet calculates the set of active top-level mods based on targets,
// dependencies, and force flags.
func (dr *DependencyResolver) ResolveEffectiveSet(
	targetModIDs []string,
	allMods map[string]*Mod,
	potentialProviders PotentialProvidersMap,
	forceEnabled map[string]bool,
	forceDisabled map[string]bool,
) (map[string]bool, []ResolutionInfo) {
	dr.allMods = allMods
	dr.potentialProviders = potentialProviders
	dr.forceEnabled = forceEnabled
	dr.forceDisabled = forceDisabled

	dr.currentEffectiveSet = make(map[string]bool)
	dr.resolutionPath = make(map[string]ResolutionInfo)
	dr.processing = make(map[string]bool)
	dr.dependencySatisfied = make(map[string]string)

	var initialSetMods []string

	// Process initial targets
	for _, modID := range targetModIDs {
		if _, ok := dr.forceDisabled[modID]; ok {
			logging.Infof("Resolver: Target mod '%s' is force-disabled, skipping.", modID)
			continue
		}
		if _, isDirectMod := dr.allMods[modID]; isDirectMod {
			if dr.ensureModActive(modID, "System (Initial Set)", "Target", modID) {
				initialSetMods = append(initialSetMods, modID)
			}
		} else if !IsImplicitMod(modID) {
			if dr.resolveDependency(modID, "System (Initial Target)", modID) {
				if providerModID, ok := dr.dependencySatisfied[modID]; ok {
					initialSetMods = append(initialSetMods, providerModID)
				}
			}
		}
	}

	// Process force-enabled mods
	for modID := range dr.forceEnabled {
		if _, ok := dr.forceDisabled[modID]; ok {
			logging.Infof("Resolver: Force-enabled mod '%s' is also force-disabled, skipping (force-disabled takes precedence).", modID)
			continue
		}
		if _, isDirectMod := dr.allMods[modID]; isDirectMod {
			if dr.ensureModActive(modID, "System (Initial Set)", "Forced", modID) {
				initialSetMods = append(initialSetMods, modID)
			}
		} else if !IsImplicitMod(modID) {
			if dr.resolveDependency(modID, "System (Forced)", modID) {
				if providerModID, ok := dr.dependencySatisfied[modID]; ok {
					initialSetMods = append(initialSetMods, providerModID)
				}
			}
		}
	}

	if len(initialSetMods) > 0 {
		sort.Strings(initialSetMods) // For consistent logging
		logging.Infof("Resolver: Activated initial set (targets and force-enabled): %v", initialSetMods)
	} else {
		logging.Info("Resolver: No mods activated in initial target/force-enabled set.")
	}

	logging.Infof("Resolver: Dependency resolution complete. Effective mods: %v", mapKeys(dr.currentEffectiveSet))
	return dr.currentEffectiveSet, dr.collectResolutionPath()
}

// ensureModActive attempts to activate a mod and its dependencies.
// Returns true if modToActivateID and all its hard (non-optional, non-implicit)
// dependencies were successfully activated and added to currentEffectiveSet.
func (dr *DependencyResolver) ensureModActive(modToActivateID, neededByModID, reasonForActivation, dependencyIDSatisfied string) bool {
	if _, ok := dr.forceDisabled[modToActivateID]; ok {
		logging.Warnf("Resolver: Mod '%s' is force-disabled, cannot activate (needed by '%s', dependency: '%s').", modToActivateID, neededByModID, dependencyIDSatisfied)
		return false
	}

	if _, ok := dr.currentEffectiveSet[modToActivateID]; ok {
		dr.updateNeededForList(modToActivateID, neededByModID)
		return true
	}

	if _, ok := dr.processing[modToActivateID]; ok {
		logging.Warnf("Resolver: Cycle detected involving '%s' (needed by '%s', dependency: '%s'). Activation path failed.", modToActivateID, neededByModID, dependencyIDSatisfied)
		return false
	}

	mod, exists := dr.allMods[modToActivateID]
	if !exists {
		logging.Errorf("Resolver: Cannot activate '%s': Mod metadata not found (dependency '%s' for '%s' unfulfilled).", modToActivateID, dependencyIDSatisfied, neededByModID)
		return false
	}

	dr.processing[modToActivateID] = true

	allDependenciesSuccessfullyResolved := true
	var unmetDependencies []string
	for depID := range mod.FabricInfo.Depends {
		if IsImplicitMod(depID) {
			continue
		}

		if !dr.resolveDependency(depID, modToActivateID, depID) {
			allDependenciesSuccessfullyResolved = false
			unmetDependencies = append(unmetDependencies, depID)
		}
	}

	delete(dr.processing, modToActivateID)

	if !allDependenciesSuccessfullyResolved {
		sort.Strings(unmetDependencies)
		logging.Warnf("Resolver: Mod '%s' cannot be activated due to unmet dependencies: %v.", modToActivateID, unmetDependencies)
		return false
	}

	dr.currentEffectiveSet[modToActivateID] = true
	dr.updateResolutionPath(modToActivateID, neededByModID, reasonForActivation, dependencyIDSatisfied)
	return true
}

// updateNeededForList adds neededByModID to the ResolutionInfo's NeededFor list if not already present.
func (dr *DependencyResolver) updateNeededForList(modID, neededByModID string) {
	if neededByModID == "System (Initial Set)" {
		return
	}
	info, ok := dr.resolutionPath[modID]
	if !ok {
		logging.Errorf("Resolver Error: Mod '%s' active but no resolution path found to update NeededFor.", modID)
		return
	}

	isNewNeed := true
	for _, existingNeeder := range info.NeededFor {
		if existingNeeder == neededByModID {
			isNewNeed = false
			break
		}
	}
	if isNewNeed {
		info.NeededFor = append(info.NeededFor, neededByModID)
		sort.Strings(info.NeededFor)
		dr.resolutionPath[modID] = info
	}
}

// updateResolutionPath creates or updates the ResolutionInfo for a successfully activated mod.
func (dr *DependencyResolver) updateResolutionPath(modID, neededBy, reason, satisfiedDep string) {
	existingInfo, entryExists := dr.resolutionPath[modID]

	finalReason := reason
	if entryExists && (existingInfo.Reason == "Target" || existingInfo.Reason == "Forced") {
		finalReason = existingInfo.Reason
	}

	neededForSet := make(map[string]struct{})
	if entryExists {
		for _, n := range existingInfo.NeededFor {
			neededForSet[n] = struct{}{}
		}
	}
	neededForSet[neededBy] = struct{}{}

	neededForList := make([]string, 0, len(neededForSet))
	for n := range neededForSet {
		neededForList = append(neededForList, n)
	}
	sort.Strings(neededForList)

	resInfo := ResolutionInfo{
		ModID:        modID,
		Reason:       finalReason,
		NeededFor:    neededForList,
		SatisfiedDep: satisfiedDep,
	}

	if provider, found := dr.getSelectedProviderForDep(satisfiedDep, modID); found {
		resInfo.SelectedProvider = provider
	} else if entryExists && existingInfo.SelectedProvider != nil {
		resInfo.SelectedProvider = existingInfo.SelectedProvider
	}
	dr.resolutionPath[modID] = resInfo
}

// resolveDependency attempts to find a provider for a dependency and ensure it's active.
// Returns true if the dependency was successfully resolved (i.e., a provider was found and activated).
func (dr *DependencyResolver) resolveDependency(dependencyToSatisfy, requiringModID, originalDeclaration string) bool {
	if providerModID, isSatisfied := dr.dependencySatisfied[dependencyToSatisfy]; isSatisfied {
		logging.Infof("Resolver: Dependency '%s' (for '%s') already satisfied by '%s'.", dependencyToSatisfy, requiringModID, providerModID)
		dr.updateNeededForList(providerModID, requiringModID)
		if _, ok := dr.currentEffectiveSet[providerModID]; ok {
			return true
		}
		logging.Errorf("Resolver: Dependency '%s' marked satisfied by '%s', but '%s' is not in effective set (logic error).", dependencyToSatisfy, providerModID, providerModID)
		return false
	}

	chosenProvider := dr.findBestProvider(dependencyToSatisfy)
	if chosenProvider == nil {
		logging.Warnf("Resolver: No provider found for dependency '%s' (required by '%s').", dependencyToSatisfy, requiringModID)
		return false
	}

	providerTopLevelID := chosenProvider.TopLevelModID

	if dr.ensureModActive(providerTopLevelID, requiringModID, "Dependency", dependencyToSatisfy) {
		dr.dependencySatisfied[dependencyToSatisfy] = providerTopLevelID
		info := dr.resolutionPath[providerTopLevelID]
		info.SatisfiedDep = dependencyToSatisfy
		info.SelectedProvider = chosenProvider
		dr.resolutionPath[providerTopLevelID] = info
		logging.Infof("Resolver: Dependency '%s' (for '%s') satisfied by activating '%s'.", dependencyToSatisfy, requiringModID, providerTopLevelID)
		return true
	}

	logging.Warnf("Resolver: Failed to activate provider '%s' for dependency '%s' (required by '%s').", providerTopLevelID, dependencyToSatisfy, requiringModID)
	return false
}

// findBestProvider selects the best available (non-disabled) provider for a given dependencyID.
func (dr *DependencyResolver) findBestProvider(depID string) *ProviderInfo {
	providerCandidates, ok := dr.potentialProviders[depID]
	if !ok || len(providerCandidates) == 0 {
		return nil
	}

	for i := range providerCandidates {
		candidate := providerCandidates[i]
		if _, ok := dr.forceDisabled[candidate.TopLevelModID]; ok {
			continue
		}
		if _, modExists := dr.allMods[candidate.TopLevelModID]; !modExists {
			logging.Errorf("Resolver: Provider '%s' for dependency '%s' not found in allMods map. Skipping.", candidate.TopLevelModID, depID)
			continue
		}
		return &candidate
	}
	return nil
}

// getSelectedProviderForDep checks if modToActivateID is a known provider for dependencyIDSatisfied.
// Used to correctly populate ResolutionInfo.SelectedProvider.
func (dr *DependencyResolver) getSelectedProviderForDep(dependencyIDSatisfied, modToActivateID string) (*ProviderInfo, bool) {
	candidates, ok := dr.potentialProviders[dependencyIDSatisfied]
	if !ok {
		return nil, false
	}

	for i := range candidates {
		if candidates[i].TopLevelModID == modToActivateID {
			if _, ok := dr.forceDisabled[modToActivateID]; !ok {
				return &candidates[i], true
			}
		}
	}
	return nil, false
}

// mapKeys returns a sorted slice of keys from a map[string]bool for consistent logging.
func mapKeys(m map[string]bool) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		if m[k] { // Only include true values if map[string]bool is used as a set
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)
	return keys
}

// collectResolutionPath gathers ResolutionInfo for all mods in the currentEffectiveSet.
func (dr *DependencyResolver) collectResolutionPath() []ResolutionInfo {
	var pathSlice []ResolutionInfo
	var resolvedModIDs []string
	for modID := range dr.currentEffectiveSet {
		resolvedModIDs = append(resolvedModIDs, modID)
	}
	sort.Strings(resolvedModIDs)

	for _, modID := range resolvedModIDs {
		if info, ok := dr.resolutionPath[modID]; ok {
			pathSlice = append(pathSlice, info)
		} else {
			logging.Errorf("Resolver Error: Mod '%s' in effective set but missing resolution path.", modID)
			pathSlice = append(pathSlice, ResolutionInfo{ModID: modID, Reason: "Error: Path Undefined"})
		}
	}

	var depLogMessages []string
	if len(pathSlice) > 0 {
		depLogMessages = append(depLogMessages, "Resolver: Dependency activation paths:")
		for _, info := range pathSlice {
			neededForStr := strings.Join(info.NeededFor, ", ")
			providerStr := ""
			if info.SelectedProvider != nil {
				providerStr = fmt.Sprintf(" (via %s v%s)", info.SelectedProvider.TopLevelModID, info.SelectedProvider.TopLevelModVersion)
			}
			depLogMessages = append(depLogMessages, fmt.Sprintf("  - Mod '%s': Reason: '%s', Satisfies: '%s'%s, Needed for: [%s]",
				info.ModID, info.Reason, info.SatisfiedDep, providerStr, neededForStr))
		}
		logging.Info(strings.Join(depLogMessages, "\n"))
	}
	return pathSlice
}
