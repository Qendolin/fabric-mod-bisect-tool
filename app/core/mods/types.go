package mods

import "strings"

// ProviderInfo describes a mod that can satisfy a dependency.
type ProviderInfo struct {
	TopLevelModID         string
	VersionOfProvidedItem string
	IsDirectProvide       bool
	TopLevelModVersion    string
}

// PotentialProvidersMap maps a dependency ID to a list of ProviderInfo structs.
type PotentialProvidersMap map[string][]ProviderInfo

// FabricModJson represents the structure of a fabric.mod.json file.
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

// Mod represents a single discovered mod and its metadata.
type Mod struct {
	Path              string
	BaseFilename      string
	FabricInfo        FabricModJson
	IsInitiallyActive bool // Was the mod active (.jar) when first loaded?
	ConfirmedGood     bool // Manually or programmatically marked as not causing issues.
	NestedModules     []FabricModJson
	EffectiveProvides map[string]string // Maps all unique IDs this mod provides to their version.
}

// FriendlyName returns a human-readable name for the mod.
func (m *Mod) FriendlyName() string {
	if m == nil {
		return "Unknown Mod"
	}
	if m.FabricInfo.Name != "" {
		return m.FabricInfo.Name
	}
	return m.FabricInfo.ID
}

// ModStatus represents the current runtime state of a single mod.
type ModStatus struct {
	ID            string
	Mod           *Mod
	ForceEnabled  bool
	ForceDisabled bool
	ManuallyGood  bool
}

// ResolutionInfo stores details about why a mod is included in an effective set.
type ResolutionInfo struct {
	ModID            string        // ID of the top-level mod resolved.
	Reason           string        // "Target", "Forced", or "Dependency".
	NeededFor        []string      // Mod IDs that required this mod.
	SatisfiedDep     string        // The dependency ID this mod satisfies for the needers.
	SelectedProvider *ProviderInfo // How this mod was chosen as a provider, if applicable.
}

// IsImplicitMod checks if a dependency ID is for an implicit (non-mod) dependency.
// These are common implicit dependencies for Fabric mods that don't correspond to actual JARs.
// For example, "java", "minecraft", "fabricloader" are usually provided by the game/environment.
func IsImplicitMod(depID string) bool {
	lowerDepID := strings.ToLower(depID) // Normalize for robustness
	switch lowerDepID {
	case "java", "minecraft", "fabricloader", "forge": // Added "forge" for broader compatibility
		return true
	}
	return false
}

// Replace the dependency override structs with a new, parsed structure
type OverrideAction int

const (
	ActionReplace OverrideAction = iota
	ActionAdd
	ActionRemove
)

type OverrideRule struct {
	TargetModID    string // The mod whose dependencies are being changed
	DependencyID   string // The dependency being changed (e.g., "minecraft")
	VersionMatcher string // The new version string
	Action         OverrideAction
}

// DependencyOverrides now holds a pre-parsed list of rules.
type DependencyOverrides struct {
	Rules []OverrideRule
}

// Struct for initial JSON parsing
type rawDependencyOverrides struct {
	Version   int                                     `json:"version"`
	Overrides map[string]map[string]map[string]string `json:"overrides"`
}
