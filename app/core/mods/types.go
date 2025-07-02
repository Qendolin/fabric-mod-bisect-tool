package mods

import "fmt"

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
	ForceEnabled  bool // Is mutually exclusive with ForceDisabled and Omitted
	ForceDisabled bool
	Omitted       bool // Previously called ManuallyGood
	IsMissing     bool // Not exclusive with other states
}

func (s ModStatus) IsSearchCandidate() bool {
	return !s.ForceEnabled && !s.ForceDisabled && !s.Omitted && !s.IsMissing
}

func (s ModStatus) IsActivatable() bool {
	return !s.ForceDisabled && !s.IsMissing
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
	switch depID {
	case "java", "minecraft", "fabricloader", "quilt_loader":
		return true
	}
	return false
}

// OverrideAction defines the type of modification for a rule.
type OverrideAction int

const (
	ActionReplace OverrideAction = iota
	ActionAdd
	ActionRemove
)

func (a OverrideAction) String() string {
	switch a {
	case ActionReplace:
		return "Replace"
	case ActionAdd:
		return "Add"
	case ActionRemove:
		return "Remove"
	default:
		return "Unknown"
	}
}

// OverrideRule is the interface for any dependency or provides override rule.
type OverrideRule interface {
	Apply(fmj *FabricModJson)
	Target() string
	Field() string
	Key() string
	Action() OverrideAction
	Value() string
}

// DependencyOverrides holds a pre-parsed list of polymorphic rules.
type DependencyOverrides struct {
	Rules []OverrideRule
}

// FileMissingError represents an error for a single missing mod file.
// The primary subject is the file path itself.
type FileMissingError struct {
	ModID    string
	FilePath string
}

func (e *FileMissingError) Error() string {
	return fmt.Sprintf("file not found for mod '%s': %s", e.ModID, e.FilePath)
}

// MissingFilesError is a wrapper error that contains one or more FileMissingError instances.
// This allows an operation to report all missing files at once.
type MissingFilesError struct {
	Errors []*FileMissingError
}

func (e *MissingFilesError) Error() string {
	return fmt.Sprintf("found %d missing mod files", len(e.Errors))
}
