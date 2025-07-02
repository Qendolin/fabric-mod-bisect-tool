package mods

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/Qendolin/fabric-mod-bisect-tool/pkg/core/mods/version"
	"github.com/Qendolin/fabric-mod-bisect-tool/pkg/logging"
	"github.com/titanous/json5"
)

// VersionField is a wrapper for version.Version that handles JSON unmarshaling
// from a string, ensuring the version is parsed and valid at load time.
type VersionField struct {
	version.Version
}

func (vf VersionField) String() string {
	if vf.Version == nil {
		return "<invalid>"
	}
	return vf.Version.String()
}

// UnmarshalJSON implements the json.Unmarshaler interface.
func (vf *VersionField) UnmarshalJSON(data []byte) error {
	var versionStr string
	if err := json.Unmarshal(data, &versionStr); err != nil {
		logging.Debugf("VersionField: Unmarshal to string failed: %v", err) // DEBUG
		return fmt.Errorf("version field is not a string: %w", err)
	}

	parsed, err := version.Parse(versionStr, false)
	if err != nil {
		logging.Debugf("VersionField: version.Parse failed for '%s': %v", versionStr, err) // DEBUG
		return fmt.Errorf("parsing version string '%s': %w", versionStr, err)
	}

	logging.Debugf("VersionField: Successfully parsed '%s'", versionStr) // DEBUG
	vf.Version = parsed
	return nil
}

// VersionRanges is a custom type for dependency maps that handles parsing
// of version predicate strings into a slice of VersionPredicate objects.
// A dependency can be satisfied if ANY of the predicates in the slice are met (OR relationship).
type VersionRanges map[string][]*version.VersionPredicate

// UnmarshalJSON implements the json.Unmarshaler interface, allowing us to parse
// the complex "string or array of strings" format for version ranges.
func (vr *VersionRanges) UnmarshalJSON(data []byte) error {
	var raw map[string]interface{}
	if err := json5.Unmarshal(data, &raw); err != nil {
		return fmt.Errorf("parsing dependency block: %w", err)
	}

	parsed := make(VersionRanges)

	for depID, value := range raw {
		var predicateStrings []string

		switch v := value.(type) {
		case string:
			predicateStrings = append(predicateStrings, v)
		case []interface{}:
			for i, item := range v {
				str, ok := item.(string)
				if !ok {
					return fmt.Errorf("dependency '%s' has a non-string element at index %d in its version range array", depID, i)
				}
				predicateStrings = append(predicateStrings, str)
			}
		default:
			return fmt.Errorf("dependency '%s' has an invalid version range format (must be string or array of strings)", depID)
		}

		predicates := make([]*version.VersionPredicate, len(predicateStrings))
		for i, pStr := range predicateStrings {
			p, err := version.ParseVersionPredicate(pStr)
			if err != nil {
				return fmt.Errorf("parsing version predicate '%s' for dependency '%s': %w", pStr, depID, err)
			}
			predicates[i] = p
		}
		parsed[depID] = predicates
	}

	*vr = parsed
	return nil
}

// ProviderInfo describes a mod that can satisfy a dependency.
type ProviderInfo struct {
	TopLevelModID         string
	VersionOfProvidedItem version.Version
	IsDirectProvide       bool
	TopLevelModVersion    version.Version
}

// PotentialProvidersMap maps a dependency ID to a list of ProviderInfo structs.
type PotentialProvidersMap map[string][]ProviderInfo

// FabricModJson represents the structure of a fabric.mod.json file.
type FabricModJson struct {
	ID         string        `json:"id"`
	Name       string        `json:"name"`
	Version    VersionField  `json:"version"`
	Provides   []string      `json:"provides"`
	Depends    VersionRanges `json:"depends"`
	Breaks     VersionRanges `json:"breaks"`
	Recommends VersionRanges `json:"recommends"`
	Suggests   VersionRanges `json:"suggests"`
	Conflicts  VersionRanges `json:"conflicts"`
	Jars       []struct {
		File string `json:"file"`
	} `json:"jars"`
}

// NestedModule holds metadata for a mod found inside another JAR file,
// including its full path within the parent archive.
type NestedModule struct {
	Info      FabricModJson
	PathInJar string
}

// Mod represents a single discovered mod and its metadata.
type Mod struct {
	Path              string
	BaseFilename      string
	FabricInfo        FabricModJson
	IsInitiallyActive bool // Was the mod active (.jar) when first loaded?
	NestedModules     []NestedModule
	EffectiveProvides map[string]version.Version // Maps all unique IDs this mod provides to their version.
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
	ModID            string
	Reason           string
	NeededFor        []string
	SatisfiedDep     string
	SelectedProvider *ProviderInfo
}

// ResolutionPath is a slice of ResolutionInfo that provides a custom string
// representation for logging the dependency activation paths.
type ResolutionPath []ResolutionInfo

// String implements the fmt.Stringer interface for ResolutionPath.
func (rp ResolutionPath) String() string {
	var depLogMessages []string
	for _, info := range rp {
		// We only want to log mods that were pulled in as dependencies.
		if info.Reason != "Dependency" {
			continue
		}

		// Add a header only if we find at least one dependency to log.
		if len(depLogMessages) == 0 {
			depLogMessages = append(depLogMessages, "Dependency activation paths:")
		}

		neededForStr := strings.Join(info.NeededFor, ", ")
		providerStr := ""
		if info.SelectedProvider != nil {
			providerStr = fmt.Sprintf(" (via %s v%s)", info.SelectedProvider.TopLevelModID, info.SelectedProvider.VersionOfProvidedItem)
		}
		depLogMessages = append(depLogMessages, fmt.Sprintf("  - Mod '%s': Satisfies: '%s'%s, Required for: [%s]",
			info.ModID, info.SatisfiedDep, providerStr, neededForStr))
	}

	if len(depLogMessages) == 0 {
		return "No cross-mod dependencies were activated."
	}
	return strings.Join(depLogMessages, "\n")
}

// IsImplicitMod checks if a dependency ID is for an implicit (non-mod) dependency.
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
type FileMissingError struct {
	ModID    string
	FilePath string
}

func (e *FileMissingError) Error() string {
	return fmt.Sprintf("file not found for mod '%s': %s", e.ModID, e.FilePath)
}

// MissingFilesError is a wrapper error that contains one or more FileMissingError instances.
type MissingFilesError struct {
	Errors []*FileMissingError
}

func (e *MissingFilesError) Error() string {
	return fmt.Sprintf("found %d missing mod files", len(e.Errors))
}
