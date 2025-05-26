package app

import (
	"log"
	"sort"
)

// ProviderInfo and PotentialProvidersMap remain the same as defined previously.
type ProviderInfo struct {
	TopLevelModID         string
	VersionOfProvidedItem string
	IsDirectProvide       bool
	TopLevelModVersion    string
}
type PotentialProvidersMap map[string][]ProviderInfo

// ResolutionInfo stores details about why a mod is included and what it provides.
type ResolutionInfo struct {
	ModID            string        // ID of the top-level mod resolved
	Reason           string        // "Target", "Forced", "Dependency"
	NeededFor        []string      // If "Dependency", which mod ID needed this dependency
	SatisfiedDep     string        // If "Dependency", which dependency ID this mod satisfies
	SelectedProvider *ProviderInfo // Details of how this mod was chosen as a provider (if applicable)
}

// DependencyResolver encapsulates the logic.
type DependencyResolver struct {
	AllMods            map[string]*Mod
	PotentialProviders PotentialProvidersMap
	ForceEnabled       map[string]bool
	ForceDisabled      map[string]bool

	// For tracking during a single ResolveEffectiveSet call
	currentEffectiveSet map[string]bool           // Mods confirmed to be in the set
	resolutionPath      map[string]ResolutionInfo // Tracks why each mod is included
	processing          map[string]bool           // Tracks mods currently in DFS stack to detect cycles
	fullyResolvedDeps   map[string]string         // Tracks which top-level mod satisfied which dependencyID
}

func NewDependencyResolver(
	allMods map[string]*Mod, potentialProviders PotentialProvidersMap,
	forceEnabled, forceDisabled map[string]bool,
) *DependencyResolver {
	return &DependencyResolver{
		AllMods:            allMods,
		PotentialProviders: potentialProviders,
		ForceEnabled:       forceEnabled,
		ForceDisabled:      forceDisabled,
	}
}

// ResolveEffectiveSet calculates the set of active top-level mods and the resolution path.
func (dr *DependencyResolver) ResolveEffectiveSet(targetModIDs []string) (map[string]bool, []ResolutionInfo) {
	dr.currentEffectiveSet = make(map[string]bool)
	dr.resolutionPath = make(map[string]ResolutionInfo)
	dr.processing = make(map[string]bool)
	dr.fullyResolvedDeps = make(map[string]string)

	initialActivationSet := make(map[string]string) // ModID -> reason ("Target" or "Forced")

	for _, modID := range targetModIDs {
		if !dr.ForceDisabled[modID] {
			// If modID is not a top-level mod, it's a provided ID we need to resolve.
			// For now, assume targetModIDs are actual top-level mod IDs for simplicity in initial set.
			// A more robust way would be to treat targetModIDs that aren't in AllMods as initial dependencies to resolve.
			if _, ok := dr.AllMods[modID]; ok {
				initialActivationSet[modID] = "Target"
			} else if !isImplicitMod(modID) { // If it's a provided ID, resolve it
				dr.resolveDependency(modID, "System (Initial Target)", "Initial Target Requirement")
			}
		}
	}
	for modID := range dr.ForceEnabled {
		if !dr.ForceDisabled[modID] {
			if _, ok := dr.AllMods[modID]; ok {
				initialActivationSet[modID] = "Forced"
			} else {
				log.Printf("%sForced-enabled ID '%s' not a known top-level mod. Ignoring for initial set, will try to resolve if it's a dependency.", LogWarningPrefix, modID)
			}
		}
	}

	for modID, reason := range initialActivationSet {
		dr.ensureModActive(modID, "System (Initial Set)", reason)
	}

	// Convert resolutionPath map to slice for ordered output / easier TUI processing
	var pathSlice []ResolutionInfo
	// To maintain some order (e.g., by mod ID), collect and sort keys
	var resolvedModIDs []string
	for modID := range dr.currentEffectiveSet { // Iterate over actual effective set
		resolvedModIDs = append(resolvedModIDs, modID)
	}
	sort.Strings(resolvedModIDs)
	for _, modID := range resolvedModIDs {
		pathSlice = append(pathSlice, dr.resolutionPath[modID])
	}

	return dr.currentEffectiveSet, pathSlice
}

// ensureModActive is the core DFS-like function.
// neededByModID: The ID of the mod that declared the dependency.
// dependencyID: The actual dependency ID being satisfied by activating modToActivateID.
func (dr *DependencyResolver) ensureModActive(modToActivateID, neededByModID, dependencyID string) {
	if dr.ForceDisabled[modToActivateID] {
		log.Printf("%sSkipping activation of '%s': Force-disabled.", LogInfoPrefix, modToActivateID)
		// This dependency might remain unsatisfied, or another provider will be found.
		return
	}
	if dr.currentEffectiveSet[modToActivateID] {
		// Already active, potentially update reason if this is a more direct one (e.g. Target > Dependency)
		// For now, first reason wins.
		return
	}
	if dr.processing[modToActivateID] {
		log.Printf("%sDetected cycle involving mod '%s' while resolving for '%s' needing '%s'. Breaking cycle.",
			LogWarningPrefix, modToActivateID, neededByModID, dependencyID)
		return // Cycle detected
	}

	mod, ok := dr.AllMods[modToActivateID]
	if !ok {
		log.Printf("%sCannot activate mod '%s': Not found in allMods. Dependency '%s' for '%s' might be unfulfilled.",
			LogErrorPrefix, modToActivateID, dependencyID, neededByModID)
		return
	}

	dr.processing[modToActivateID] = true
	log.Printf("%sActivating mod '%s' (v%s) because it's needed by '%s' for dependency '%s'.",
		LogInfoPrefix, mod.FriendlyName(), mod.FabricInfo.Version, neededByModID, dependencyID)

	// Resolve dependencies of modToActivateID *before* marking it fully active.
	for dep := range mod.FabricInfo.Depends {
		if !isImplicitMod(dep) {
			dr.resolveDependency(dep, modToActivateID, dep)
		}
	}

	dr.currentEffectiveSet[modToActivateID] = true
	reason := "Dependency"
	if neededByModID == "System (Initial Set)" { // Special marker for initial targets/forced
		reason = dependencyID // Which is "Target" or "Forced"
	}

	neededByModIDs := []string{}
	if d, ok := dr.resolutionPath[modToActivateID]; ok {
		neededByModIDs = d.NeededFor
	}
	neededByModIDs = append(neededByModIDs, neededByModID)

	dr.resolutionPath[modToActivateID] = ResolutionInfo{
		ModID:        modToActivateID,
		Reason:       reason,
		NeededFor:    neededByModIDs,
		SatisfiedDep: dependencyID,
	}
	delete(dr.processing, modToActivateID)
}

// resolveDependency finds a provider for depID and calls ensureModActive for it.
// neededByModID: The mod that has this dependency.
// originalDepID: The dependency string as declared by neededByModID.
func (dr *DependencyResolver) resolveDependency(originalDepID, neededByModID, depToSatisfy string) {
	if providerModID, ok := dr.fullyResolvedDeps[depToSatisfy]; ok {
		resolutionInfo := dr.resolutionPath[providerModID]
		resolutionInfo.NeededFor = append(resolutionInfo.NeededFor, neededByModID)
		dr.resolutionPath[providerModID] = resolutionInfo

		// This dependency ID has already been satisfied by a specific top-level mod.
		// Ensure that provider mod is active (it should be, but good for consistency).
		if !dr.currentEffectiveSet[providerModID] && !dr.ForceDisabled[providerModID] {
			log.Printf("%sDependency '%s' was previously resolved by '%s', but '%s' is not active. Re-ensuring.", LogInfoPrefix, depToSatisfy, providerModID, providerModID)
			dr.ensureModActive(providerModID, neededByModID, depToSatisfy)
		}
		return
	}

	// Check if any mod already in currentEffectiveSet can provide this depID
	// This prioritizes using already-active mods.
	satisfiedInternally, internalProviderModID := dr.checkInternalSatisfaction(depToSatisfy, dr.currentEffectiveSet)
	if satisfiedInternally {
		log.Printf("%sDependency '%s' for '%s' satisfied internally by already active mod '%s'.",
			LogInfoPrefix, depToSatisfy, neededByModID, internalProviderModID)
		dr.fullyResolvedDeps[depToSatisfy] = internalProviderModID
		// ensureModActive for internalProviderModID was already called or will be if it's an initial target.
		return
	}

	// If not satisfied internally, find the best external provider
	chosenProviderInfo := dr.findBestExternalProvider(depToSatisfy)
	if chosenProviderInfo == nil {
		log.Printf("%sDependency '%s' for mod '%s' cannot be satisfied: No suitable provider found.",
			LogWarningPrefix, depToSatisfy, neededByModID)
		return
	}

	chosenProviderTopLevelID := chosenProviderInfo.TopLevelModID
	dr.fullyResolvedDeps[depToSatisfy] = chosenProviderTopLevelID // Mark as resolved by this provider

	// Update resolution path for the chosen provider IF it's specifically for this dep satisfaction
	// (ensureModActive will also add an entry, but this one might be more specific if it's a "provides" rather than direct ID match)
	// This part can get tricky with multiple reasons. Let ensureModActive handle the primary reason.
	// We mainly need to ensure it gets activated.
	dr.resolutionPath[chosenProviderTopLevelID] = ResolutionInfo{
		ModID:            chosenProviderTopLevelID,
		Reason:           "Dependency",
		NeededFor:        []string{neededByModID},
		SatisfiedDep:     depToSatisfy,
		SelectedProvider: chosenProviderInfo, // Store how it was chosen
	}

	dr.ensureModActive(chosenProviderTopLevelID, neededByModID, depToSatisfy)
}

// checkInternalSatisfaction checks if a dependency can be satisfied by mods already in the effective set
// using the pre-calculated Mod.EffectiveProvides.
func (dr *DependencyResolver) checkInternalSatisfaction(depID string, currentEffectiveSet map[string]bool) (bool, string) {
	var bestProviderModID string
	var bestVersionOfItem string // Version of the depID item itself

	for activeModID := range currentEffectiveSet {
		activeMod, ok := dr.AllMods[activeModID]
		if !ok {
			continue
		}

		// Use the pre-calculated EffectiveProvides map
		if version, provides := activeMod.EffectiveProvides[depID]; provides {
			if compareVersions(version, bestVersionOfItem) > 0 {
				bestVersionOfItem = version
				bestProviderModID = activeModID
			}
		}
	}
	return bestProviderModID != "", bestProviderModID
}

// findBestExternalProvider now iterates a potentially pre-sorted list from PotentialProvidersMap
func (dr *DependencyResolver) findBestExternalProvider(depID string) *ProviderInfo {
	candidates, ok := dr.PotentialProviders[depID]
	if !ok || len(candidates) == 0 {
		return nil
	}

	// If PotentialProvidersMap lists were pre-sorted, we just find the first valid one.
	for i := range candidates {
		candidate := candidates[i] // Take address if ProviderInfo is large, or work with value copy
		if dr.ForceDisabled[candidate.TopLevelModID] {
			continue
		}
		if _, modExists := dr.AllMods[candidate.TopLevelModID]; !modExists {
			continue
		} // Should not happen
		return &candidate // Return the first valid one from the (pre-sorted) list
	}
	return nil // No valid provider found
}
