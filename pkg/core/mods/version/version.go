// Package version provides tools for parsing, comparing, and evaluating version strings
// and predicates. It supports a format similar to Semantic Versioning but with
// extensions like wildcards (e.g., 1.2.x).
package version

import (
	"cmp"
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"strings"
)

var (
	// dotSeparatedIDRegex validates prerelease and build identifier strings.
	dotSeparatedIDRegex = regexp.MustCompile(`^[0-9A-Za-z-]+(\.[0-9A-Za-z-]+)*$`)
	// unsignedIntegerRegex checks if a string is a valid non-negative integer.
	unsignedIntegerRegex = regexp.MustCompile(`^(0|[1-9][0-9]*)$`)
)

const (
	// componentWildcard is the internal representation of a wildcard component (x, X, *).
	componentWildcard = -1
)

// --- Version Types ---

// Version is the interface for a comparable version. It can be either a
// SemanticVersion or a simple StringVersion for non-standard formats.
type Version interface {
	fmt.Stringer
	// Compare returns -1, 0, or 1 if the receiver is less than, equal to,
	// or greater than `other`, respectively.
	Compare(other Version) int
	// IsSemantic reports whether the version conforms to the semantic structure.
	IsSemantic() bool
}

// SemanticVersion implements a versioning scheme similar to SemVer,
// supporting components, prerelease tags, and build metadata.
type SemanticVersion struct {
	components    []int
	prerelease    string
	build         string
	hasPrerelease bool
}

// StringVersion is a fallback for non-semantic version strings,
// with comparison handled by simple string comparison.
type StringVersion struct {
	version string
}

// --- Version Parsing ---

// Parse parses a string into the most appropriate Version type. It attempts to
// parse as a SemanticVersion first and falls back to a StringVersion.
func Parse(rawVersion string, allowWildcards bool) (Version, error) {
	if rawVersion == "" {
		return nil, fmt.Errorf("version must be a non-empty string")
	}
	semVer, err := ParseSemantic(rawVersion, allowWildcards)
	if err != nil {
		return &StringVersion{version: rawVersion}, nil
	}
	return semVer, nil
}

// ParseSemantic strictly parses a string into a SemanticVersion.
func ParseSemantic(rawVersion string, allowWildcards bool) (*SemanticVersion, error) {
	if rawVersion == "" {
		return nil, fmt.Errorf("version must be a non-empty string")
	}

	core, prerelease, build, hasPrerelease, err := parseBuildAndPrerelease(rawVersion)
	if err != nil {
		return nil, err
	}

	if strings.HasSuffix(core, ".") || strings.HasPrefix(core, ".") {
		return nil, fmt.Errorf("version string must not start or end with a dot: %s", rawVersion)
	}

	componentParts := strings.Split(core, ".")
	components, err := parseComponents(componentParts, hasPrerelease, allowWildcards)
	if err != nil {
		return nil, fmt.Errorf("in version string '%s': %w", rawVersion, err)
	}

	return &SemanticVersion{
		components:    components,
		prerelease:    prerelease,
		build:         build,
		hasPrerelease: hasPrerelease,
	}, nil
}

// parseBuildAndPrerelease splits the version string into the core version, prerelease, and build parts.
func parseBuildAndPrerelease(rawVersion string) (core, prerelease, build string, hasPrerelease bool, err error) {
	core = rawVersion
	if plusIdx := strings.Index(core, "+"); plusIdx != -1 {
		build = core[plusIdx+1:]
		core = core[:plusIdx]
	}
	if dashIdx := strings.Index(core, "-"); dashIdx != -1 {
		prerelease = core[dashIdx+1:]
		core = core[:dashIdx]
		hasPrerelease = true
		if prerelease != "" && !dotSeparatedIDRegex.MatchString(prerelease) {
			err = fmt.Errorf("invalid prerelease string '%s'", prerelease)
			return
		}
	}
	return
}

// parseComponents processes the dot-separated numeric parts of a version string.
func parseComponents(componentParts []string, hasPrerelease, allowWildcards bool) ([]int, error) {
	if len(componentParts) == 0 || (len(componentParts) == 1 && componentParts[0] == "") {
		return nil, fmt.Errorf("did not provide version numbers")
	}

	components := make([]int, len(componentParts))
	firstWildcardIndex := -1

	for i, part := range componentParts {
		isWildcardStr := part == "x" || part == "X" || part == "*"

		if allowWildcards && isWildcardStr {
			if hasPrerelease {
				return nil, fmt.Errorf("pre-release versions are not allowed to use X-ranges")
			}
			if i == 0 {
				return nil, fmt.Errorf("version cannot start with a wildcard")
			}
			components[i] = componentWildcard
			if firstWildcardIndex == -1 {
				firstWildcardIndex = i
			}
		} else {
			if firstWildcardIndex != -1 {
				return nil, fmt.Errorf("interjacent wildcards (e.g., 1.x.2) are disallowed")
			}
			if isWildcardStr { // !allowWildcards but is a wildcard string
				return nil, fmt.Errorf("wildcard characters are only allowed when allowWildcards is true")
			}

			num, err := parseNumericComponent(part)
			if err != nil {
				return nil, err
			}
			components[i] = num
		}
	}

	// Truncate components after the first wildcard, e.g., 1.x.3 becomes 1.x
	if firstWildcardIndex != -1 {
		return components[:firstWildcardIndex+1], nil
	}
	return components, nil
}

// parseNumericComponent converts a string part to a non-negative integer.
func parseNumericComponent(part string) (int, error) {
	if part == "" {
		return 0, fmt.Errorf("missing version number component")
	}
	num, err := strconv.Atoi(part)
	if err != nil {
		return 0, fmt.Errorf("could not parse version number component '%s': %w", part, err)
	}
	if num < 0 {
		return 0, fmt.Errorf("negative version number component '%s'", part)
	}
	return num, nil
}

// --- SemanticVersion Methods ---

func (sv *SemanticVersion) String() string {
	var builder strings.Builder
	for i, component := range sv.components {
		if i > 0 {
			builder.WriteRune('.')
		}
		if component == componentWildcard {
			builder.WriteRune('x')
		} else {
			builder.WriteString(strconv.Itoa(component))
		}
	}
	if sv.hasPrerelease {
		builder.WriteRune('-')
		builder.WriteString(sv.prerelease)
	}
	if sv.build != "" {
		builder.WriteRune('+')
		builder.WriteString(sv.build)
	}
	return builder.String()
}

func (sv *SemanticVersion) IsSemantic() bool { return true }

// HasWildcard returns true if the version contains a wildcard component.
func (sv *SemanticVersion) HasWildcard() bool {
	return slices.Contains(sv.components, componentWildcard)
}

// VersionComponent returns the numeric value of a version component at a given position.
// It returns 0 for positions beyond the defined components, unless a wildcard is present.
func (sv *SemanticVersion) VersionComponent(position int) int {
	if position < 0 {
		panic("tried to access negative version number component")
	}
	if position >= len(sv.components) {
		// If the last component is a wildcard, subsequent components are also wildcards.
		if len(sv.components) > 0 && sv.components[len(sv.components)-1] == componentWildcard {
			return componentWildcard
		}
		return 0 // Implicit zero for missing components.
	}
	return sv.components[position]
}

func (sv *SemanticVersion) Compare(other Version) int {
	otherSemVer, ok := other.(*SemanticVersion)
	if !ok {
		// Fallback to string comparison for non-semantic or mixed types.
		return strings.Compare(sv.String(), other.String())
	}

	if comparison := sv.compareComponents(otherSemVer); comparison != 0 {
		return comparison
	}
	return sv.comparePrerelease(otherSemVer)
}

// compareComponents compares the numeric parts of two semantic versions.
func (sv *SemanticVersion) compareComponents(other *SemanticVersion) int {
	numComponents := max(len(sv.components), len(other.components))
	for i := range numComponents {
		v1Component := sv.VersionComponent(i)
		v2Component := other.VersionComponent(i)

		// Wildcards are not comparable and should be skipped.
		if v1Component == componentWildcard || v2Component == componentWildcard {
			continue
		}
		if comparison := cmp.Compare(v1Component, v2Component); comparison != 0 {
			return comparison
		}
	}
	return 0
}

// comparePrerelease compares the prerelease identifiers of two semantic versions.
func (sv *SemanticVersion) comparePrerelease(other *SemanticVersion) int {
	// A version with a prerelease is lower than one without.
	if sv.hasPrerelease && !other.hasPrerelease {
		return -1
	}
	if !sv.hasPrerelease && other.hasPrerelease {
		return 1
	}
	if !sv.hasPrerelease && !other.hasPrerelease {
		return 0
	}

	// Handle empty prerelease strings which split into [""] instead of [].
	var idsA, idsB []string
	if sv.prerelease != "" {
		idsA = strings.Split(sv.prerelease, ".")
	}
	if other.prerelease != "" {
		idsB = strings.Split(other.prerelease, ".")
	}

	minIDs := min(len(idsA), len(idsB))
	for i := range minIDs {
		if res := comparePrereleaseIdentifiers(idsA[i], idsB[i]); res != 0 {
			return res
		}
	}

	// A version with more prerelease identifiers is greater.
	return cmp.Compare(len(idsA), len(idsB))
}

// comparePrereleaseIdentifiers compares two individual prerelease identifier strings.
func comparePrereleaseIdentifiers(idA, idB string) int {
	isNumA := unsignedIntegerRegex.MatchString(idA)
	isNumB := unsignedIntegerRegex.MatchString(idB)

	if isNumA && isNumB {
		numA, _ := strconv.Atoi(idA) // Error can be ignored due to regex check.
		numB, _ := strconv.Atoi(idB)
		return cmp.Compare(numA, numB)
	}
	if isNumA {
		return -1 // Numeric < Alphanumeric
	}
	if isNumB {
		return 1 // Alphanumeric > Numeric
	}
	return strings.Compare(idA, idB)
}

// --- StringVersion Methods ---

func (sv *StringVersion) String() string   { return sv.version }
func (sv *StringVersion) IsSemantic() bool { return false }
func (sv *StringVersion) Compare(other Version) int {
	return strings.Compare(sv.String(), other.String())
}
