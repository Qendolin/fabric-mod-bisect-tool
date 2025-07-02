package mods

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/Qendolin/fabric-mod-bisect-tool/pkg/core/mods/version"
	"github.com/Qendolin/fabric-mod-bisect-tool/pkg/core/sets"
	"github.com/Qendolin/fabric-mod-bisect-tool/pkg/logging"
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
	modStatuses      map[string]ModStatus
	effectiveSet     map[string]*Mod
	resolutionPath   map[string]ResolutionInfo
	dfsStack         map[string]bool
	cachedProviders  map[string]*ProviderInfo
	unresolvableDeps map[string]bool
	resolutionFailed bool
	failureReason    string
}

// NewDependencyResolver creates a new DependencyResolver service.
func NewDependencyResolver(allMods map[string]*Mod, potentialProviders PotentialProvidersMap) *DependencyResolver {
	return &DependencyResolver{
		allMods:            allMods,
		potentialProviders: potentialProviders,
	}
}

// ResolveEffectiveSet calculates the set of active top-level mods based on targets, dependencies, and force flags.
func (dr *DependencyResolver) ResolveEffectiveSet(targetSet sets.Set, modStatuses map[string]ModStatus) (sets.Set, []ResolutionInfo) {
	startTime := time.Now()
	logging.Info("Resolver: Starting dependency resolution...")

	s := &resolutionSession{
		allMods:            dr.allMods,
		potentialProviders: dr.potentialProviders,
		modStatuses:        modStatuses,
		effectiveSet:       make(map[string]*Mod),
		resolutionPath:     make(map[string]ResolutionInfo),
		dfsStack:           make(map[string]bool),
		cachedProviders:    make(map[string]*ProviderInfo),
		unresolvableDeps:   make(map[string]bool),
	}

	initialActivationSet := sets.Copy(targetSet)
	for modID, status := range s.modStatuses {
		if status.ForceEnabled {
			initialActivationSet[modID] = struct{}{}
		}
	}

	for modID := range initialActivationSet {
		// If a previous activation attempt resulted in a fatal error, stop processing.
		if s.resolutionFailed {
			break
		}
		status := s.modStatuses[modID]
		reason := "Target"
		if status.ForceEnabled {
			reason = "Forced"
		}
		s.ensureModActive(modID, "System (Initial Set)", reason, modID)
	}
	duration := time.Since(startTime)
	if s.resolutionFailed {
		logging.Warnf("Resolver: Resolution failed in %v. Reason: %s", duration, s.failureReason)
		return make(sets.Set), nil
	}

	if err := s.validateBreaks(); err != nil {
		s.resolutionFailed = true
		s.failureReason = err.Error()
		logging.Warnf("Resolver: Resolution failed 'breaks' validation in %v. Reason: %s", duration, s.failureReason)
		return make(sets.Set), nil
	}

	finalSet := make(sets.Set, len(s.effectiveSet))
	effectiveIDs := make([]string, 0, len(s.effectiveSet))
	for id := range s.effectiveSet {
		finalSet[id] = struct{}{}
		effectiveIDs = append(effectiveIDs, id)
	}
	sort.Strings(effectiveIDs)
	logging.Infof("Resolver: Resolution complete in %v. Effective set (%d mods): %v", duration, len(effectiveIDs), effectiveIDs)

	return finalSet, s.collectResolutionPath()
}

// ensureModActive is the main recursive function. It attempts to activate a mod and its dependencies.
func (s *resolutionSession) ensureModActive(modID, neededBy, reason, satisfiedDep string) bool {
	if s.resolutionFailed {
		return false
	}
	if _, isActive := s.effectiveSet[modID]; isActive {
		s.updateNeededForList(modID, neededBy)
		return true
	}
	// The IsActivatable check correctly uses the global modStatuses map, which is what we want.
	// It allows the resolver to pull in any mod that isn't explicitly force-disabled by the user.
	if status, ok := s.modStatuses[modID]; ok && !status.IsActivatable() {
		return false
	}
	if s.dfsStack[modID] {
		s.resolutionFailed = true
		s.failureReason = fmt.Sprintf("circular dependency detected involving '%s'", modID)
		logging.Error("Resolver: " + s.failureReason)
		return false
	}

	mod, exists := s.allMods[modID]
	if !exists {
		// This can happen if a dependency points to a modID that doesn't exist in any file.
		return false
	}

	s.dfsStack[modID] = true
	logging.Debugf("Resolver: > Attempting to activate '%s' (for '%s')", modID, neededBy)

	// Tentatively add the mod to the set for this recursive path.
	originalState := s.copyState()
	s.effectiveSet[modID] = mod

	allDepsOK := true
	for depID, predicates := range mod.FabricInfo.Depends {
		if IsImplicitMod(depID) {
			continue
		}
		if !s.resolveDependency(depID, predicates, modID) {
			allDepsOK = false
			break
		}
	}

	s.dfsStack[modID] = false // Pop from the virtual stack.

	if allDepsOK {
		logging.Debugf("Resolver: < Successfully activated '%s'", modID)
		s.updateResolutionPath(modID, neededBy, reason, satisfiedDep)
		return true
	} else {
		// Backtrack: The dependencies for this mod could not be resolved.
		// Restore the state to before we tried activating this mod.
		s.restoreState(originalState)
		// Refined log message for clarity during backtracking.
		logging.Debugf("Resolver: < Backtracking from '%s'; could not satisfy its dependencies.", modID)
		return false
	}
}

// resolveDependency finds a valid provider for a dependency and activates it. This is the heart of the backtracking logic.
func (s *resolutionSession) resolveDependency(depID string, predicates []*version.VersionPredicate, requiringModID string) bool {
	if s.unresolvableDeps[depID] {
		return false
	}

	if cachedProvider, isCached := s.cachedProviders[depID]; isCached {
		for _, p := range predicates {
			if !p.Test(cachedProvider.VersionOfProvidedItem) {
				s.resolutionFailed = true
				s.failureReason = fmt.Sprintf("dependency conflict for '%s'. Mod '%s' requires '%s', but mod '%s' (v%s) was already chosen.",
					depID, requiringModID, p, cachedProvider.TopLevelModID, cachedProvider.VersionOfProvidedItem)
				logging.Warn("Resolver: " + s.failureReason)
				return false
			}
		}
		// The cached provider is compatible, ensure it's active. This will handle cycles correctly.
		return s.ensureModActive(cachedProvider.TopLevelModID, requiringModID, "Dependency", depID)
	}

	candidates := s.findBestProviders(depID, predicates)
	predicateStr := formatPredicates(predicates)

	if logging.IsDebugEnabled() {
		var candidateNames []string
		for _, c := range candidates {
			candidateNames = append(candidateNames, fmt.Sprintf("%s v%s", c.TopLevelModID, c.VersionOfProvidedItem))
		}
		logging.Debugf("Resolver:  ? Trying to satisfy '%s %s' for '%s'. Found %d valid candidates: %v", depID, predicateStr, requiringModID, len(candidates), candidateNames)
	}

	originalState := s.copyState()
	for _, provider := range candidates {
		logging.Debugf("Resolver:    - Trying candidate: %s (v%s)", provider.TopLevelModID, provider.VersionOfProvidedItem)
		if s.ensureModActive(provider.TopLevelModID, requiringModID, "Dependency", depID) {
			s.cachedProviders[depID] = provider
			return true
		}

		if s.resolutionFailed {
			return false
		}

		logging.Debugf("Resolver:    - Candidate %s failed. Backtracking.", provider.TopLevelModID)
		s.restoreState(originalState)
	}

	s.unresolvableDeps[depID] = true
	// Only set the failure reason if one hasn't already been set by a deeper, more specific error.
	if !s.resolutionFailed {
		s.failureReason = fmt.Sprintf("failed to resolve dependency '%s %s' for mod '%s'", depID, predicateStr, requiringModID)
	}
	logging.Errorf("Resolver: Could not resolve dependency '%s %s' for mod '%s': no valid providers could be activated.", depID, predicateStr, requiringModID)
	return false
}

// findBestProviders finds all activatable mods that provide a given dependency and satisfy the version predicates.
func (s *resolutionSession) findBestProviders(depID string, predicates []*version.VersionPredicate) []*ProviderInfo {
	providerCandidates, ok := s.potentialProviders[depID]
	if !ok || len(providerCandidates) == 0 {
		return nil
	}

	// First, filter the list to only providers that are activatable and meet the version requirements.
	var validProviders []*ProviderInfo
	for i := range providerCandidates {
		candidate := &providerCandidates[i]

		if status, ok := s.modStatuses[candidate.TopLevelModID]; ok && !status.IsActivatable() {
			continue
		}

		versionIsSatisfactory := true
		for _, p := range predicates {
			if !p.Test(candidate.VersionOfProvidedItem) {
				versionIsSatisfactory = false
				break
			}
		}
		if !versionIsSatisfactory {
			continue
		}

		validProviders = append(validProviders, candidate)
	}

	// Second, sort the list of valid providers by priority. This is crucial for making
	// the best choices first during the backtracking search.
	sort.Slice(validProviders, func(i, j int) bool {
		a := validProviders[i]
		b := validProviders[j]

		// Priority 1: Higher version of the provided item is better.
		versionCmp := a.VersionOfProvidedItem.Compare(b.VersionOfProvidedItem)
		if versionCmp != 0 {
			return versionCmp > 0 // descending order
		}

		// Priority 2: A direct provide is better than a nested provide.
		if a.IsDirectProvide != b.IsDirectProvide {
			return a.IsDirectProvide // true comes before false
		}

		// Priority 3 (Tie-breaker): Alphabetical ID for determinism.
		return a.TopLevelModID < b.TopLevelModID
	})

	return validProviders
}

// validateBreaks performs a final check on the successful resolution set.
func (s *resolutionSession) validateBreaks() error {
	for modID, mod := range s.effectiveSet {
		if mod.FabricInfo.Breaks == nil {
			continue
		}
		for brokenDepID, predicates := range mod.FabricInfo.Breaks {
			provider, isProvided := s.cachedProviders[brokenDepID]
			if !isProvided {
				continue
			}
			for _, p := range predicates {
				if p.Test(provider.VersionOfProvidedItem) {
					predicateStr := formatPredicates(predicates)
					return fmt.Errorf("mod '%s' (v%s) breaks '%s' (provided by '%s' v%s) due to rule '%s %s'",
						modID, mod.FabricInfo.Version.Version, brokenDepID, provider.TopLevelModID, provider.VersionOfProvidedItem, brokenDepID, predicateStr)
				}
			}
		}
	}
	return nil
}

// --- State Management and Logging Helpers ---

// formatPredicates creates a readable string representation of version predicates.
func formatPredicates(predicates []*version.VersionPredicate) string {
	if len(predicates) == 0 {
		return "*"
	}
	var parts []string
	for _, p := range predicates {
		parts = append(parts, p.String())
	}
	return strings.Join(parts, ", ")
}

func (s *resolutionSession) copyState() map[string]*Mod {
	stateCopy := make(map[string]*Mod, len(s.effectiveSet))
	for k, v := range s.effectiveSet {
		stateCopy[k] = v
	}
	return stateCopy
}

func (s *resolutionSession) restoreState(state map[string]*Mod) {
	s.effectiveSet = state
}

func (s *resolutionSession) updateNeededForList(modID, neededByModID string) {
	if neededByModID == "System (Initial Set)" {
		return
	}
	info, ok := s.resolutionPath[modID]
	if !ok {
		return
	}
	for _, existingNeeder := range info.NeededFor {
		if existingNeeder == neededByModID {
			return
		}
	}
	info.NeededFor = append(info.NeededFor, neededByModID)
	sort.Strings(info.NeededFor)
	s.resolutionPath[modID] = info
}

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
	neededForList := sets.MakeSlice(neededForSet)

	s.resolutionPath[modID] = ResolutionInfo{
		ModID:            modID,
		Reason:           finalReason,
		NeededFor:        neededForList,
		SatisfiedDep:     satisfiedDep,
		SelectedProvider: s.cachedProviders[satisfiedDep],
	}
}

func (s *resolutionSession) collectResolutionPath() []ResolutionInfo {
	pathSlice := make([]ResolutionInfo, 0, len(s.effectiveSet))
	for _, mod := range s.effectiveSet {
		if info, ok := s.resolutionPath[mod.FabricInfo.ID]; ok {
			pathSlice = append(pathSlice, info)
		} else {
			logging.Errorf("Resolver: Mod '%s' in effective set but missing resolution path.", mod.FabricInfo.ID)
			pathSlice = append(pathSlice, ResolutionInfo{ModID: mod.FabricInfo.ID, Reason: "Error: Path Undefined"})
		}
	}
	sort.Slice(pathSlice, func(i, j int) bool {
		return pathSlice[i].ModID < pathSlice[j].ModID
	})

	var depLogMessages []string
	for _, info := range pathSlice {
		if info.Reason != "Dependency" {
			continue
		}
		if len(depLogMessages) == 0 {
			depLogMessages = append(depLogMessages, "Resolver: Dependency activation paths:")
		}
		neededForStr := strings.Join(info.NeededFor, ", ")
		providerStr := ""
		if info.SelectedProvider != nil {
			providerStr = fmt.Sprintf(" (via %s v%s)", info.SelectedProvider.TopLevelModID, info.SelectedProvider.VersionOfProvidedItem)
		}
		depLogMessages = append(depLogMessages, fmt.Sprintf("  - Mod '%s': Satisfies: '%s'%s, Required for: [%s]",
			info.ModID, info.SatisfiedDep, providerStr, neededForStr))
	}
	if len(depLogMessages) > 0 {
		logging.Info(strings.Join(depLogMessages, "\n"))
	}
	return pathSlice
}

// FindTransitiveDependersOf calculates the complete set of mods that depend,
// directly or indirectly, on any mod in the initial target set.
// This function operates on dependency IDs and does not evaluate versions.
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

// FindAllUnresolvableMods iteratively calculates the complete set of unresolvable mods
// from an initial set of candidates. It finds not only mods with missing direct
// dependencies but also mods whose dependencies become unavailable transitively.
func (dr *DependencyResolver) CalculateTransitivelyUnresolvableMods(initialCandidates sets.Set) sets.Set {
	// Start with the assumption that all candidates are available.
	currentlyAvailable := sets.Copy(initialCandidates)
	// This will accumulate all mods we discover are unresolvable.
	totalUnresolvable := make(sets.Set)

	for {
		// Find the next layer of unresolvable mods based on the currently available set.
		newlyFoundUnresolvable := dr.CalculateDirectlyUnresolvableMods(currentlyAvailable)

		// If this pass found no new problems, the set is stable and we are done.
		if len(newlyFoundUnresolvable) == 0 {
			break
		}

		// Add the newly found unresolvable mods to our final result set.
		for modID := range newlyFoundUnresolvable {
			totalUnresolvable[modID] = struct{}{}
		}

		// Remove the newly found unresolvable mods from the available pool
		// for the next iteration of the check.
		currentlyAvailable = sets.SubtractInPlace(currentlyAvailable, newlyFoundUnresolvable)
	}

	return totalUnresolvable
}

// CalculateDirectlyUnresolvableMods performs a single, first-degree check to determine
// which mods from a given set of candidates are immediately unresolvable.
//
// A mod is considered directly unresolvable if any of its immediate 'depends' entries
// cannot be satisfied by a version-compatible provider that is also present within
// the `availableMods` set.
//
// This function does not calculate transitive unresolvability. For example, if mod A
// depends on mod B, and mod B is found to be unresolvable, this function will not flag
// mod A as unresolvable due to B's issue. For the full transitive check, see
// FindAllUnresolvableMods, which uses this function iteratively.
func (dr *DependencyResolver) CalculateDirectlyUnresolvableMods(availableMods sets.Set) sets.Set {
	unresolvable := make(sets.Set)
	sortedCandidates := sets.MakeSlice(availableMods)

	for _, modID := range sortedCandidates {
		mod, ok := dr.allMods[modID]
		if !ok {
			continue // Should not happen with a correctly constructed set
		}

		for depID, predicates := range mod.FabricInfo.Depends {
			if IsImplicitMod(depID) {
				continue
			}

			// Use the helper method to check for a valid provider.
			if !dr.findValidProviderInSet(depID, predicates, availableMods) {
				unresolvable[modID] = struct{}{}
				break // This mod is unresolvable; no need to check its other dependencies.
			}
		}
	}

	return unresolvable
}

// findValidProviderInSet searches for at least one provider that satisfies the dependency,
// is available in the given set, and matches all version predicates.
func (dr *DependencyResolver) findValidProviderInSet(depID string, predicates []*version.VersionPredicate, availableMods sets.Set) bool {
	providerCandidates, found := dr.potentialProviders[depID]
	if !found {
		return false // No potential providers exist for this dependency at all.
	}

	for _, provider := range providerCandidates {
		// Condition 1: The provider's top-level mod must be in our available set.
		if _, providerIsAvailable := availableMods[provider.TopLevelModID]; !providerIsAvailable {
			continue
		}

		// Condition 2: The provider's version must satisfy all predicates for this dependency.
		if checkAllPredicatesSatisfied(predicates, provider.VersionOfProvidedItem) {
			return true // Found a valid provider.
		}
	}

	// Scanned all potential providers and none were suitable.
	return false
}

// checkAllPredicatesSatisfied is a helper that returns true only if the version
// satisfies every predicate in the slice.
func checkAllPredicatesSatisfied(predicates []*version.VersionPredicate, v version.Version) bool {
	for _, p := range predicates {
		if !p.Test(v) {
			return false // Early exit on first failure.
		}
	}
	return true // All predicates were satisfied.
}
