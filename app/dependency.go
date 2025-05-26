package app

import (
	"sort"
)

// GetDependencies resolves all transitive dependencies for a given set of mod IDs.
// Version constraints are IGNORED. Only existence matters.
// Respects forceDisabled map.
func GetDependencies(
	targetModIDs []string,
	allMods map[string]*Mod,
	providesMap map[string]string,
	forceDisabled map[string]bool,
) map[string]bool {
	resolved := make(map[string]bool)
	queue := make([]string, 0, len(targetModIDs))

	for _, id := range targetModIDs {
		actualID, ok := providesMap[id] // Resolve via providesMap first
		if !ok {
			// If not in providesMap, it might be a direct mod ID or something not found
			// For targetModIDs, we assume they are valid primary mod IDs if not in providesMap for some reason.
			// This path usually means 'id' itself is a primary mod_id.
			actualID = id
		}

		if _, modExists := allMods[actualID]; !modExists && !isImplicitMod(actualID) {
			// If the actualID (target) doesn't exist as a known mod and isn't implicit, skip.
			continue
		}

		if forceDisabled[actualID] {
			continue
		}
		if !resolved[actualID] {
			queue = append(queue, actualID)
			resolved[actualID] = true
		}
	}

	head := 0
	for head < len(queue) {
		currentModID := queue[head]
		head++

		mod, ok := allMods[currentModID]
		if !ok { // This mod ID is not in our parsed list (e.g. "java", "minecraft", or a missing dep)
			continue
		}

		for depID := range mod.FabricInfo.Depends {
			if isImplicitMod(depID) { // "java", "minecraft", "fabric-", etc.
				continue
			}

			actualDepID, foundInProvides := providesMap[depID]
			if !foundInProvides {
				// If not in providesMap, it might be a direct mod ID for a mod we don't have,
				// or a typo in fabric.mod.json. We can't resolve it further.
				continue
			}

			if _, modExists := allMods[actualDepID]; !modExists {
				// The provider maps to an ID we don't have info for.
				continue
			}

			if forceDisabled[actualDepID] {
				continue
			}

			if !resolved[actualDepID] {
				resolved[actualDepID] = true
				queue = append(queue, actualDepID)
			}
		}
	}
	return resolved
}

// CheckAllDependencies performs an initial check for missing dependencies across all loaded mods.
// It uses the PotentialProvidersMap to see if a provider exists for each dependency.
// Returns a map of mod IDs to a list of their missing dependency IDs.
func CheckAllDependencies(allMods map[string]*Mod, potentialProviders PotentialProvidersMap) map[string][]string {
	overallMissing := make(map[string][]string)

	for modID, mod := range allMods {
		var currentModMissing []string
		for depID := range mod.FabricInfo.Depends { // Iterate through declared dependencies
			if isImplicitMod(depID) { // isImplicitMod should be defined/accessible
				continue // Skip built-in/fabric API type dependencies
			}

			// A dependency is considered missing if there are no entries for it in potentialProviders.
			// We don't check versions here, just existence of *any* potential provider.
			providers, found := potentialProviders[depID]
			if !found || len(providers) == 0 {
				currentModMissing = append(currentModMissing, depID)
			}
			// Note: This doesn't check if the providers themselves are force-disabled,
			// as this is an initial check of *availability*, not resolvability under current force settings.
		}

		if len(currentModMissing) > 0 {
			sort.Strings(currentModMissing) // Sort for consistent output
			overallMissing[modID] = currentModMissing
		}
	}
	return overallMissing
}

// isImplicitMod checks if a mod ID is considered a built-in or Fabric API mod.
func isImplicitMod(modID string) bool {
	return modID == "java" || modID == "minecraft" || modID == "fabricloader"
}
