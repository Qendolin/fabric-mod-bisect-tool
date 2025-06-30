package mods

import (
	"fmt"
	"sort"
	"strings"

	"github.com/Qendolin/fabric-mod-bisect-tool/app/core/sets"
	"github.com/Qendolin/fabric-mod-bisect-tool/app/logging"
)

// DependencyResolver is a long-lived service that holds the static universe of all mods
// and their potential providers. It is safe for concurrent use.
type DependencyResolver struct {
	allMods            map[string]*Mod
	potentialProviders PotentialProvidersMap
}

// resolutionSession holds the state for a single dependency resolution operation.
// It is created for a single call to ResolveEffectiveSet and is not reused.
type resolutionSession struct {
	// Static data from the parent resolver
	allMods            map[string]*Mod
	potentialProviders PotentialProvidersMap

	// Per-call dynamic data
	modStatuses         map[string]ModStatus
	currentEffectiveSet map[string]bool
	resolutionPath      map[string]ResolutionInfo
	processing          map[string]bool   // DFS stack tracker for cycle detection
	dependencySatisfied map[string]string // Maps a dependencyID to the TopLevelModID that satisfies it
}

// NewDependencyResolver creates a new DependencyResolver service.
// It should be initialized once with the complete set of mods and providers.
func NewDependencyResolver(allMods map[string]*Mod, potentialProviders PotentialProvidersMap) *DependencyResolver {
	return &DependencyResolver{
		allMods:            allMods,
		potentialProviders: potentialProviders,
	}
}

// ResolveEffectiveSet calculates the set of active top-level mods based on targets, dependencies, and force flags.
func (dr *DependencyResolver) ResolveEffectiveSet(targetSet sets.Set, modStatuses map[string]ModStatus) (sets.Set, []ResolutionInfo) {
	// Create a new session for this resolution attempt.
	s := &resolutionSession{
		allMods:             dr.allMods,
		potentialProviders:  dr.potentialProviders,
		modStatuses:         modStatuses,
		currentEffectiveSet: make(map[string]bool),
		resolutionPath:      make(map[string]ResolutionInfo),
		processing:          make(map[string]bool),
		dependencySatisfied: make(map[string]string),
	}

	// Process initial targets
	for modID := range targetSet {
		if status, ok := s.modStatuses[modID]; ok && status.ForceDisabled {
			continue
		}
		if _, isDirectMod := s.allMods[modID]; isDirectMod {
			s.ensureModActive(modID, "System (Initial Set)", "Target", modID)
		} else if !IsImplicitMod(modID) {
			s.resolveDependency(modID, "System (Initial Target)")
		}
	}

	// Process force-enabled mods
	for modID, status := range s.modStatuses {
		if status.ForceEnabled {
			if status.ForceDisabled { // Should not happen with StateManager logic, but good practice
				continue
			}
			if _, isDirectMod := s.allMods[modID]; isDirectMod {
				s.ensureModActive(modID, "System (Initial Set)", "Forced", modID)
			} else if !IsImplicitMod(modID) {
				s.resolveDependency(modID, "System (Forced)")
			}
		}
	}

	finalEffectiveSet := make(sets.Set)
	var effectiveModIDs []string // For logging the final set
	for modID, isActive := range s.currentEffectiveSet {
		if isActive {
			finalEffectiveSet[modID] = struct{}{}
			effectiveModIDs = append(effectiveModIDs, modID)
		}
	}
	sort.Strings(effectiveModIDs)
	logging.Infof("Resolver: Resolution complete. Effective set (%d mods): %v", len(effectiveModIDs), effectiveModIDs)

	return finalEffectiveSet, s.collectResolutionPath()
}

// ensureModActive attempts to activate a mod and its dependencies.
// Returns true if modToActivateID and all its hard (non-optional, non-implicit) dependencies were successfully activated.
func (s *resolutionSession) ensureModActive(modToActivateID, neededByModID, reasonForActivation, dependencyIDSatisfied string) bool {
	if status, ok := s.modStatuses[modToActivateID]; ok && status.ForceDisabled {
		logging.Debugf("Resolver: Mod '%s' is force-disabled, cannot activate (needed by '%s', dependency: '%s').", modToActivateID, neededByModID, dependencyIDSatisfied)
		return false
	}

	if _, ok := s.currentEffectiveSet[modToActivateID]; ok {
		logging.Debugf("Resolver: Mod '%s' is already in the effective set. Skipping activation.", modToActivateID)
		s.updateNeededForList(modToActivateID, neededByModID)
		return true
	}

	if _, ok := s.processing[modToActivateID]; ok {
		logging.Warnf("Resolver: Cycle detected involving '%s' (needed by '%s', dependency: '%s').", modToActivateID, neededByModID, dependencyIDSatisfied)
		return false
	}

	mod, exists := s.allMods[modToActivateID]
	if !exists {
		logging.Errorf("Resolver: Cannot activate '%s': Mod not found (dependency '%s' for '%s' unfulfilled).", modToActivateID, dependencyIDSatisfied, neededByModID)
		return false
	}

	s.processing[modToActivateID] = true

	allDependenciesSuccessfullyResolved := true
	var unmetDependencies []string
	for depID := range mod.FabricInfo.Depends {
		if IsImplicitMod(depID) {
			continue
		}

		if !s.resolveDependency(depID, modToActivateID) {
			allDependenciesSuccessfullyResolved = false
			unmetDependencies = append(unmetDependencies, depID)
		}
	}

	delete(s.processing, modToActivateID)

	if !allDependenciesSuccessfullyResolved {
		sort.Strings(unmetDependencies)
		logging.Warnf("Resolver: Mod '%s' cannot be activated due to unmet dependencies: %v.", modToActivateID, unmetDependencies)
		return false
	}

	s.currentEffectiveSet[modToActivateID] = true
	s.updateResolutionPath(modToActivateID, neededByModID, reasonForActivation, dependencyIDSatisfied)
	return true
}

// updateNeededForList adds neededByModID to the ResolutionInfo's NeededFor list if not already present.
func (s *resolutionSession) updateNeededForList(modID, neededByModID string) {
	if neededByModID == "System (Initial Set)" {
		return
	}
	info, ok := s.resolutionPath[modID]
	if !ok {
		logging.Errorf("Resolver: Mod '%s' active but no resolution path found to update NeededFor.", modID)
		return
	}

	for _, existingNeeder := range info.NeededFor {
		if existingNeeder == neededByModID {
			return // Already present
		}
	}

	info.NeededFor = append(info.NeededFor, neededByModID)
	sort.Strings(info.NeededFor)
	s.resolutionPath[modID] = info
}

// updateResolutionPath creates or updates the ResolutionInfo for a successfully activated mod.
func (s *resolutionSession) updateResolutionPath(modID, neededBy, reason, satisfiedDep string) {
	existingInfo, entryExists := s.resolutionPath[modID]

	finalReason := reason
	if entryExists && (existingInfo.Reason == "Target" || existingInfo.Reason == "Forced") {
		finalReason = existingInfo.Reason
	}

	neededForSet := make(sets.Set)
	if entryExists {
		for _, n := range existingInfo.NeededFor {
			neededForSet[n] = struct{}{}
		}
	}
	if neededBy != "System (Initial Set)" {
		neededForSet[neededBy] = struct{}{}
	}

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

	if provider, found := s.getSelectedProviderForDep(satisfiedDep, modID); found {
		resInfo.SelectedProvider = provider
	} else if entryExists && existingInfo.SelectedProvider != nil {
		resInfo.SelectedProvider = existingInfo.SelectedProvider
	}
	s.resolutionPath[modID] = resInfo
}

// resolveDependency attempts to find a provider for a dependency and ensure it's active.
// Returns true if the dependency was successfully resolved.
func (s *resolutionSession) resolveDependency(dependencyToSatisfy, requiringModID string) bool {
	if providerModID, isSatisfied := s.dependencySatisfied[dependencyToSatisfy]; isSatisfied {
		logging.Debugf("Resolver: Dependency '%s' (for '%s') already satisfied by '%s'.", dependencyToSatisfy, requiringModID, providerModID)
		s.updateNeededForList(providerModID, requiringModID)
		if _, ok := s.currentEffectiveSet[providerModID]; ok {
			return true
		}
		// This indicates a logic error where a dependency was marked satisfied by an inactive mod.
		logging.Errorf("Resolver: Dependency '%s' satisfied by '%s' but mod is not active.", dependencyToSatisfy, providerModID)
		return false
	}

	chosenProvider := s.findBestProvider(dependencyToSatisfy)
	if chosenProvider == nil {
		logging.Warnf("Resolver: No provider found for dependency '%s' (required by '%s').", dependencyToSatisfy, requiringModID)
		return false
	}

	providerTopLevelID := chosenProvider.TopLevelModID

	// A mod can satisfy its own dependency requirement, which is not a cycle.
	if providerTopLevelID == requiringModID {
		logging.Debugf("Resolver: Dependency '%s' is self-provided by '%s'.", dependencyToSatisfy, requiringModID)
		return true // The dependency is satisfied contingent on the requiring mod's activation.
	}

	if s.ensureModActive(providerTopLevelID, requiringModID, "Dependency", dependencyToSatisfy) {
		s.dependencySatisfied[dependencyToSatisfy] = providerTopLevelID
		logging.Debugf("Resolver: Dependency '%s' (for '%s') satisfied by activating '%s'.", dependencyToSatisfy, requiringModID, providerTopLevelID)
		return true
	}

	logging.Warnf("Resolver: Failed to activate provider '%s' for dependency '%s' (required by '%s').", providerTopLevelID, dependencyToSatisfy, requiringModID)
	return false
}

// findBestProvider selects the best available provider for a given dependencyID.
// It prioritizes any valid, already-active mod over any inactive mod.
func (s *resolutionSession) findBestProvider(depID string) *ProviderInfo {
	providerCandidates, ok := s.potentialProviders[depID]
	logging.Debugf("Resolver: Finding provider for '%s'. Candidates: %v", depID, providerCandidates)
	if !ok || len(providerCandidates) == 0 {
		return nil
	}

	var firstValidOverallCandidate *ProviderInfo
	var firstValidActiveCandidate *ProviderInfo

	// The providerCandidates list is pre-sorted by version, from best to worst.
	for i := range providerCandidates {
		candidate := &providerCandidates[i]

		if status, ok := s.modStatuses[candidate.TopLevelModID]; ok && status.ForceDisabled {
			continue
		}
		if _, modExists := s.allMods[candidate.TopLevelModID]; !modExists {
			logging.Errorf("Resolver: Provider '%s' for dependency '%s' not found. This indicates an inconsistent state.", candidate.TopLevelModID, depID)
			continue
		}

		// This is the first valid (non-disabled) provider we've encountered in the sorted list.
		if firstValidOverallCandidate == nil {
			firstValidOverallCandidate = candidate
		}

		if _, isActive := s.currentEffectiveSet[candidate.TopLevelModID]; isActive {
			firstValidActiveCandidate = candidate
			break // Active provider found, it's the best choice.
		}
	}

	if firstValidActiveCandidate != nil {
		logging.Debugf("Resolver: Dependency '%s' will be satisfied by already-active mod '%s'.", depID, firstValidActiveCandidate.TopLevelModID)
		return firstValidActiveCandidate
	}

	if firstValidOverallCandidate != nil {
		logging.Debugf("Resolver: Dependency '%s' will be satisfied by activating external mod '%s'.", depID, firstValidOverallCandidate.TopLevelModID)
		return firstValidOverallCandidate
	}

	return nil
}

// getSelectedProviderForDep checks if modToActivateID is a known provider for dependencyIDSatisfied.
// Used to correctly populate ResolutionInfo.SelectedProvider.
func (s *resolutionSession) getSelectedProviderForDep(dependencyIDSatisfied, modToActivateID string) (*ProviderInfo, bool) {
	candidates, ok := s.potentialProviders[dependencyIDSatisfied]
	if !ok {
		return nil, false
	}

	for i := range candidates {
		candidate := &candidates[i]
		if candidate.TopLevelModID == modToActivateID {
			if status, ok := s.modStatuses[modToActivateID]; ok && status.ForceDisabled {
				return nil, false // This provider is disabled
			}
			return candidate, true
		}
	}
	return nil, false
}

// collectResolutionPath gathers ResolutionInfo for all mods in the currentEffectiveSet.
func (s *resolutionSession) collectResolutionPath() []ResolutionInfo {
	resolvedModIDs := make([]string, 0, len(s.currentEffectiveSet))
	for modID := range s.currentEffectiveSet {
		resolvedModIDs = append(resolvedModIDs, modID)
	}
	sort.Strings(resolvedModIDs)

	pathSlice := make([]ResolutionInfo, 0, len(resolvedModIDs))
	for _, modID := range resolvedModIDs {
		if info, ok := s.resolutionPath[modID]; ok {
			pathSlice = append(pathSlice, info)
		} else {
			logging.Errorf("Resolver: Mod '%s' in effective set but missing resolution path.", modID)
			pathSlice = append(pathSlice, ResolutionInfo{ModID: modID, Reason: "Error: Path Undefined"})
		}
	}

	var depLogMessages []string
	if len(pathSlice) > 0 {
		depLogMessages = append(depLogMessages, "Resolver: Dependency activation paths:")
		for _, info := range pathSlice {
			if info.Reason != "Dependency" {
				continue
			}
			neededForStr := strings.Join(info.NeededFor, ", ")
			providerStr := ""
			if info.SelectedProvider != nil {
				providerStr = fmt.Sprintf(" (via %s v%s)", info.SelectedProvider.TopLevelModID, info.SelectedProvider.TopLevelModVersion)
			}
			depLogMessages = append(depLogMessages, fmt.Sprintf("  - Mod '%s': Satisfies: '%s'%s, Required for: [%s]",
				info.ModID, info.SatisfiedDep, providerStr, neededForStr))
		}
		if len(depLogMessages) > 1 { // Only log if there are actual dependency messages
			logging.Info(strings.Join(depLogMessages, "\n"))
		}
	}
	return pathSlice
}

// FindTransitiveDependersOf calculates the complete set of mods that depend,
// directly or indirectly, on any mod in the initial target set.
func (dr *DependencyResolver) FindTransitiveDependersOf(targets sets.Set) sets.Set {
	if len(targets) == 0 {
		return nil
	}

	// This set will grow to include the targets and all found dependers.
	problematicSet := sets.Copy(targets)
	// This set stores only the dependers, not the initial targets.
	dependerSet := make(sets.Set)

	for {
		newlyFound := make(sets.Set)
		for _, mod := range dr.allMods {
			// Skip if this mod is already known to be problematic or is a target.
			if _, isProblematic := problematicSet[mod.FabricInfo.ID]; isProblematic {
				continue
			}

			// Check if this mod depends on any mod in the current problematic set.
			for depID := range mod.FabricInfo.Depends {
				if _, isTargetDep := problematicSet[depID]; isTargetDep {
					newlyFound[mod.FabricInfo.ID] = struct{}{}
					dependerSet[mod.FabricInfo.ID] = struct{}{}
					break // Move to the next mod once a dependency is found.
				}
			}
		}

		// If we didn't find any new dependers in a full pass, we are done.
		if len(newlyFound) == 0 {
			break
		}

		// Add the newly found dependers to the problematic set for the next iteration.
		for id := range newlyFound {
			problematicSet[id] = struct{}{}
		}
	}

	return dependerSet
}
