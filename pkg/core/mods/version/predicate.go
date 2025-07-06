// Package version provides tools for parsing, comparing, and evaluating version strings
// and predicates. It supports a format similar to Semantic Versioning but with
// extensions like wildcards (e.g., 1.2.x).
package version

import (
	"fmt"
	"strings"
)

// --- Enums and Constants ---

// VersionComparisonOperator represents a comparison operation.
type VersionComparisonOperator string

const (
	OpEqual          VersionComparisonOperator = "="
	OpGreaterOrEqual VersionComparisonOperator = ">="
	OpGreater        VersionComparisonOperator = ">"
	OpLessOrEqual    VersionComparisonOperator = "<="
	OpLess           VersionComparisonOperator = "<"
	OpCaret          VersionComparisonOperator = "^" // e.g., ^1.2.3
	OpTilde          VersionComparisonOperator = "~" // e.g., ~1.2.3
)

// orderedOperators ensures we check for multi-character operators first (e.g., ">=" before ">").
var orderedOperators = []VersionComparisonOperator{
	OpGreaterOrEqual, OpLessOrEqual, OpGreater, OpLess, OpCaret, OpTilde, OpEqual,
}

// --- VersionPredicate ---

// PredicateTerm represents a single part of a predicate string, like ">1.0.0".
type PredicateTerm struct {
	Operator VersionComparisonOperator
	Version  Version
}

// String reconstructs the term string, omitting the operator for default equality.
func (term *PredicateTerm) String() string {
	operatorStr := string(term.Operator)
	if operatorStr == "=" {
		operatorStr = ""
	}
	return operatorStr + term.Version.String()
}

// VersionPredicate represents one or more AND-ed predicate terms.
type VersionPredicate struct {
	terms []*PredicateTerm
	isAny bool // Represents a predicate that matches all versions ("*").
}

// Any returns a predicate that matches any version.
func Any() *VersionPredicate {
	return &VersionPredicate{isAny: true}
}

// ParseVersionPredicate parses a requirement string like ">=1.0.0 <2.0.0" into a predicate.
func ParseVersionPredicate(predicateStr string) (*VersionPredicate, error) {
	predicateStr = strings.TrimSpace(predicateStr)
	if predicateStr == "" || predicateStr == "*" {
		return Any(), nil
	}

	predicate := &VersionPredicate{}
	for _, part := range strings.Fields(predicateStr) {
		operator, versionStr := parseOperator(part)
		version, err := Parse(versionStr, true)
		if err != nil {
			return nil, fmt.Errorf("could not parse version string '%s' in predicate '%s'", versionStr, predicateStr)
		}
		// Non-semantic versions can only be used for exact matching.
		if !version.IsSemantic() && operator != OpEqual {
			return nil, fmt.Errorf("non-semantic version '%s' can only be used with equality operator", version.String())
		}
		predicate.terms = append(predicate.terms, &PredicateTerm{Operator: operator, Version: version})
	}
	return predicate, nil
}

// String reconstructs the original predicate string.
func (predicate *VersionPredicate) String() string {
	if predicate.isAny {
		return "*"
	}
	var parts []string
	for _, term := range predicate.terms {
		parts = append(parts, term.String())
	}
	return strings.Join(parts, " ")
}

// Interval calculates the single VersionInterval that this predicate represents.
// It returns nil if the predicate is unsatisfiable (e.g., ">2.0 <1.0").
func (predicate *VersionPredicate) Interval() *VersionInterval {
	if predicate.isAny {
		return &VersionInterval{} // Represents (-∞, ∞)
	}
	if len(predicate.terms) == 0 {
		return nil // An empty predicate has no defined interval.
	}

	// Start with an infinite interval and intersect it with each term's interval.
	currentInterval := &VersionInterval{}
	for _, term := range predicate.terms {
		termInterval, err := term.interval()
		if err != nil {
			return nil // Should not happen with a validly parsed term.
		}
		currentInterval = currentInterval.And(termInterval)
		if currentInterval == nil {
			return nil // Intersection resulted in an empty set.
		}
	}
	return currentInterval
}

// Test checks if a version satisfies the predicate.
func (predicate *VersionPredicate) Test(version Version) bool {
	interval := predicate.Interval()
	if interval == nil {
		return false // Unsatisfiable predicate.
	}
	return interval.Contains(version)
}

// parseOperator extracts the comparison operator and the remaining version string from a part.
func parseOperator(part string) (VersionComparisonOperator, string) {
	for _, operator := range orderedOperators {
		if strings.HasPrefix(part, string(operator)) {
			return operator, part[len(operator):]
		}
	}
	return OpEqual, part
}

// interval calculates the VersionInterval for a single predicate term.
func (term *PredicateTerm) interval() (*VersionInterval, error) {
	semVer, isSemantic := term.Version.(*SemanticVersion)
	if !isSemantic {
		// Non-semantic versions imply an exact match.
		return &VersionInterval{Min: term.Version, MinInclusive: true, Max: term.Version, MaxInclusive: true}, nil
	}

	operator := term.Operator
	if semVer.HasWildcard() {
		// Normalize a wildcard version like "1.x" into an equivalent tilde/caret operation.
		semVer, operator = normalizeWildcard(semVer)
	}

	switch operator {
	case OpEqual:
		return &VersionInterval{Min: semVer, MinInclusive: true, Max: semVer, MaxInclusive: true}, nil
	case OpGreater:
		return &VersionInterval{Min: semVer, MinInclusive: false}, nil
	case OpGreaterOrEqual:
		return &VersionInterval{Min: semVer, MinInclusive: true}, nil
	case OpLess:
		return &VersionInterval{Max: semVer, MaxInclusive: false}, nil
	case OpLessOrEqual:
		return &VersionInterval{Max: semVer, MaxInclusive: true}, nil
	case OpTilde:
		return tildeInterval(semVer), nil
	case OpCaret:
		return caretInterval(semVer), nil
	default:
		return nil, fmt.Errorf("unknown operator: %s", operator)
	}
}

// normalizeWildcard translates a version with a wildcard (e.g., 1.2.x)
// into a base version and an equivalent comparison operator (e.g., 1.2.0- and ~).
func normalizeWildcard(semVer *SemanticVersion) (*SemanticVersion, VersionComparisonOperator) {
	nonWildcardCount := 0
	for _, c := range semVer.components {
		if c != componentWildcard {
			nonWildcardCount++
		} else {
			break // Components after a wildcard are truncated during parsing.
		}
	}

	// Create a new base version from the non-wildcard components.
	baseComponents := make([]int, nonWildcardCount)
	copy(baseComponents, semVer.components[:nonWildcardCount])
	// A prerelease of "" indicates the lowest possible value for this version number.
	baseVersion := &SemanticVersion{components: baseComponents, hasPrerelease: true, prerelease: ""}

	// "1.x" is equivalent to "^1" (major version lock).
	// "1.2.x" is equivalent to "~1.2" (minor version lock).
	operator := OpTilde
	if nonWildcardCount == 1 {
		operator = OpCaret
	}
	return baseVersion, operator
}

// tildeInterval calculates the interval for a tilde (`~`) constraint.
// ~1.2.3 implies >=1.2.3 <1.3.0.
func tildeInterval(minVersion *SemanticVersion) *VersionInterval {
	// Normalize version to at least three components for consistent upper bound calculation.
	if len(minVersion.components) < 3 {
		normalizedComponents := make([]int, 3)
		copy(normalizedComponents, minVersion.components)
		minVersion = &SemanticVersion{components: normalizedComponents, hasPrerelease: minVersion.hasPrerelease, prerelease: minVersion.prerelease}
	}

	// The upper bound is the next minor version.
	upperBoundComponents := make([]int, len(minVersion.components))
	copy(upperBoundComponents, minVersion.components)
	upperBoundComponents[1]++ // Safe since we normalized to 3 components.
	for i := 2; i < len(upperBoundComponents); i++ {
		upperBoundComponents[i] = 0
	}
	// The upper bound is exclusive and has a base prerelease ("-").
	maxVersion := &SemanticVersion{components: upperBoundComponents, hasPrerelease: true}

	return &VersionInterval{Min: minVersion, MinInclusive: true, Max: maxVersion, MaxInclusive: false}
}

// caretInterval calculates the interval for a caret (`^`) constraint.
// ^1.2.3 implies >=1.2.3 <2.0.0.
func caretInterval(minVersion *SemanticVersion) *VersionInterval {
	// Normalize version to at least three components for consistency.
	if len(minVersion.components) < 3 {
		normalizedComponents := make([]int, 3)
		copy(normalizedComponents, minVersion.components)
		minVersion = &SemanticVersion{components: normalizedComponents, hasPrerelease: minVersion.hasPrerelease, prerelease: minVersion.prerelease}
	}

	// The upper bound is the next major version.
	upperBoundComponents := make([]int, len(minVersion.components))
	copy(upperBoundComponents, minVersion.components)
	upperBoundComponents[0]++ // Safe since we normalized to 3 components.
	for i := 1; i < len(upperBoundComponents); i++ {
		upperBoundComponents[i] = 0
	}
	// The upper bound is exclusive and has a base prerelease ("-").
	maxVersion := &SemanticVersion{components: upperBoundComponents, hasPrerelease: true}

	return &VersionInterval{Min: minVersion, MinInclusive: true, Max: maxVersion, MaxInclusive: false}
}
