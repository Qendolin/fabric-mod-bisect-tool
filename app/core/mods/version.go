package mods

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

const (
	// Wildcard represents a wildcard component in a version string (e.g., 'x', 'X', '*').
	Wildcard = -1
)

var (
	// A simple regex to check if a string is a valid unsigned integer.
	unsignedIntegerRegex = regexp.MustCompile(`^([1-9][0-9]*|0)$`)
	// A regex to validate the structure of a prerelease string.
	dotSeparatedIDRegex = regexp.MustCompile(`^[0-9A-Za-z-]+(\.[0-9A-Za-z-]+)*$`)
)

// FabricVersion holds the parsed components of a Fabric-style semantic version.
type FabricVersion struct {
	original   string
	components []int
	prerelease string
	build      string
}

// ParseExtendedSemVer parses a version string according to Fabric Loader's relaxed semver rules.
func ParseExtendedSemVer(versionStr string) (*FabricVersion, error) {
	if versionStr == "" {
		return nil, fmt.Errorf("version string must not be empty")
	}

	v := &FabricVersion{original: versionStr}
	remaining := versionStr

	// 1. Split off build metadata
	if i := strings.Index(remaining, "+"); i != -1 {
		v.build = remaining[i+1:]
		remaining = remaining[:i]
	}

	// 2. Split off prerelease metadata
	if i := strings.Index(remaining, "-"); i != -1 {
		v.prerelease = remaining[i+1:]
		remaining = remaining[:i]
	}

	// 3. Validate prerelease string, if it exists
	if v.prerelease != "" && !dotSeparatedIDRegex.MatchString(v.prerelease) {
		return nil, fmt.Errorf("invalid prerelease string '%s'", v.prerelease)
	}

	// 4. Parse version core components
	if remaining == "" {
		return nil, fmt.Errorf("version string has no version core")
	}
	componentStrings := strings.Split(remaining, ".")
	v.components = make([]int, len(componentStrings))
	firstWildcardIdx := -1

	for i, compStr := range componentStrings {
		if compStr == "" {
			return nil, fmt.Errorf("empty version component found in '%s'", remaining)
		}

		if compStr == "x" || compStr == "X" || compStr == "*" {
			if v.prerelease != "" {
				return nil, fmt.Errorf("pre-release versions are not allowed to use wildcards")
			}
			if i > 0 && v.components[i-1] == Wildcard {
				return nil, fmt.Errorf("interjacent wildcards (e.g., 1.x.2) are disallowed")
			}
			v.components[i] = Wildcard
			if firstWildcardIdx == -1 {
				firstWildcardIdx = i
			}
			continue
		}

		num, err := strconv.Atoi(compStr)
		if err != nil {
			return nil, fmt.Errorf("version component '%s' is not a valid number", compStr)
		}
		if num < 0 {
			return nil, fmt.Errorf("version component '%s' cannot be negative", compStr)
		}
		v.components[i] = num
	}

	if len(v.components) == 1 && v.components[0] == Wildcard {
		return nil, fmt.Errorf("version string 'x' is not allowed")
	}

	// Strip extra wildcards (e.g., 1.x.x -> 1.x)
	if firstWildcardIdx != -1 && len(v.components) > firstWildcardIdx+1 {
		v.components = v.components[:firstWildcardIdx+1]
	}

	return v, nil
}

// String returns the friendly, normalized string representation of the version.
func (v *FabricVersion) String() string {
	var sb strings.Builder
	for i, c := range v.components {
		if i > 0 {
			sb.WriteByte('.')
		}
		if c == Wildcard {
			sb.WriteByte('x')
		} else {
			sb.WriteString(strconv.Itoa(c))
		}
	}
	if v.prerelease != "" {
		sb.WriteByte('-')
		sb.WriteString(v.prerelease)
	}
	if v.build != "" {
		sb.WriteByte('+')
		sb.WriteString(v.build)
	}
	return sb.String()
}

// getComponent returns the version component at a given position.
// It handles out-of-bounds access by returning 0 (or Wildcard if the version ends in one).
func (v *FabricVersion) getComponent(pos int) int {
	if pos >= len(v.components) {
		// If last component was a wildcard, extend it. Otherwise, extend with 0.
		if len(v.components) > 0 && v.components[len(v.components)-1] == Wildcard {
			return Wildcard
		}
		return 0
	}
	return v.components[pos]
}

// isNumeric checks if a string is a valid non-negative integer.
func isNumeric(s string) bool {
	return unsignedIntegerRegex.MatchString(s)
}

// Compare compares this FabricVersion to another.
// It returns -1 if v < other, 0 if v == other, and 1 if v > other.
func (v *FabricVersion) Compare(other *FabricVersion) int {
	// 1. Compare version core components
	maxLen := max(len(v.components), len(other.components))
	for i := 0; i < maxLen; i++ {
		c1 := v.getComponent(i)
		c2 := other.getComponent(i)

		if c1 == Wildcard || c2 == Wildcard {
			continue // Wildcards match anything, so this component is equal
		}
		if c1 < c2 {
			return -1
		}
		if c1 > c2 {
			return 1
		}
	}

	// 2. Compare prerelease identifiers
	hasPrerelease1 := v.prerelease != ""
	hasPrerelease2 := other.prerelease != ""

	if !hasPrerelease1 && hasPrerelease2 {
		return 1 // No prerelease is higher than a prerelease
	}
	if hasPrerelease1 && !hasPrerelease2 {
		return -1 // A prerelease is lower than no prerelease
	}
	if hasPrerelease1 && hasPrerelease2 {
		parts1 := strings.Split(v.prerelease, ".")
		parts2 := strings.Split(other.prerelease, ".")

		maxPreLen := max(len(parts1), len(parts2))
		for i := 0; i < maxPreLen; i++ {
			if i >= len(parts1) {
				return -1 // v1 is shorter, so it's lower
			}
			if i >= len(parts2) {
				return 1 // v2 is shorter, so v1 is higher
			}

			p1 := parts1[i]
			p2 := parts2[i]
			if p1 == p2 {
				continue
			}

			isNum1 := isNumeric(p1)
			isNum2 := isNumeric(p2)

			if isNum1 && isNum2 {
				// Numeric identifiers are compared numerically
				n1, _ := strconv.Atoi(p1)
				n2, _ := strconv.Atoi(p2)
				if n1 < n2 {
					return -1
				}
				if n1 > n2 {
					return 1
				}
			} else if isNum1 {
				return -1 // Numeric identifiers have lower precedence than non-numeric
			} else if isNum2 {
				return 1
			} else {
				// Non-numeric identifiers are compared lexicographically
				if p1 < p2 {
					return -1
				}
				if p1 > p2 {
					return 1
				}
			}
		}
	}

	return 0 // Versions are equal
}
