// Package version provides tools for parsing, comparing, and evaluating version strings
// and predicates. It supports a format similar to Semantic Versioning but with
// extensions like wildcards (e.g., 1.2.x).
package version

import "fmt"

// VersionInterval represents a range of versions, such as [1.0, 2.0).
// Its fields are public to allow for direct inspection.
type VersionInterval struct {
	Min          Version
	MinInclusive bool
	Max          Version
	MaxInclusive bool
}

// String returns the mathematical notation of the interval, e.g., "[1.0.0,2.0.0)".
func (interval *VersionInterval) String() string {
	var minStr, maxStr string
	var minBracket, maxBracket byte

	if interval.Min == nil {
		minStr = "-∞"
		minBracket = '('
	} else {
		minStr = interval.Min.String()
		minBracket = '('
		if interval.MinInclusive {
			minBracket = '['
		}
	}

	if interval.Max == nil {
		maxStr = "∞"
		maxBracket = ')'
	} else {
		maxStr = interval.Max.String()
		maxBracket = ')'
		if interval.MaxInclusive {
			maxBracket = ']'
		}
	}
	return fmt.Sprintf("%c%s,%s%c", minBracket, minStr, maxStr, maxBracket)
}

// Contains checks if a version is within the interval.
func (interval *VersionInterval) Contains(version Version) bool {
	if interval.Min != nil {
		comparison := version.Compare(interval.Min)
		if comparison < 0 || (comparison == 0 && !interval.MinInclusive) {
			return false
		}
	}
	if interval.Max != nil {
		comparison := version.Compare(interval.Max)
		if comparison > 0 || (comparison == 0 && !interval.MaxInclusive) {
			return false
		}
	}
	return true
}

// And computes the intersection of this interval with another.
// If there is no overlap, it returns nil.
func (interval *VersionInterval) And(other *VersionInterval) *VersionInterval {
	if interval == nil || other == nil {
		return nil
	}
	// An infinite interval intersected with another is just the other interval.
	if interval.Min == nil && interval.Max == nil {
		return other
	}
	if other.Min == nil && other.Max == nil {
		return interval
	}
	// Intersection is not well-defined for non-semantic versions unless they are identical points.
	if !interval.isSemantic() || !other.isSemantic() {
		if interval.Min != nil && interval.Min.Compare(interval.Max) == 0 &&
			other.Min != nil && other.Min.Compare(other.Max) == 0 &&
			interval.Min.Compare(other.Min) == 0 {
			return interval
		}
		return nil
	}

	// The new minimum is the greater (more restrictive) of the two minimums.
	var resultMin Version
	var resultMinInclusive bool
	if compareLowerBounds(interval, other) >= 0 {
		resultMin, resultMinInclusive = interval.Min, interval.MinInclusive
	} else {
		resultMin, resultMinInclusive = other.Min, other.MinInclusive
	}

	// The new maximum is the lesser (more restrictive) of the two maximums.
	var resultMax Version
	var resultMaxInclusive bool
	if compareUpperBounds(interval, other) <= 0 {
		resultMax, resultMaxInclusive = interval.Max, interval.MaxInclusive
	} else {
		resultMax, resultMaxInclusive = other.Max, other.MaxInclusive
	}

	// Check if the resulting interval is valid (min <= max).
	if resultMin != nil && resultMax != nil {
		comparison := resultMin.Compare(resultMax)
		if comparison > 0 || (comparison == 0 && (!resultMinInclusive || !resultMaxInclusive)) {
			return nil // No overlap.
		}
	}
	return &VersionInterval{Min: resultMin, MinInclusive: resultMinInclusive, Max: resultMax, MaxInclusive: resultMaxInclusive}
}

// isSemantic checks if the interval's bounds are semantic versions.
func (interval *VersionInterval) isSemantic() bool {
	return (interval.Min == nil || interval.Min.IsSemantic()) && (interval.Max == nil || interval.Max.IsSemantic())
}

// compareLowerBounds determines which interval has the greater (more restrictive) minimum boundary.
// For example, (1.0) is greater than [1.0].
func compareLowerBounds(intervalA, intervalB *VersionInterval) int {
	minA, minB := intervalA.Min, intervalB.Min
	if minA == nil {
		return -1 // -∞ is always smaller
	}
	if minB == nil {
		return 1
	}

	if comparison := minA.Compare(minB); comparison != 0 {
		return comparison
	}
	// Versions are equal. An exclusive bound is "greater" (more restrictive).
	if intervalA.MinInclusive == intervalB.MinInclusive {
		return 0
	}
	if intervalA.MinInclusive { // [1.0] is a "smaller" bound than (1.0)
		return -1
	}
	return 1
}

// compareUpperBounds determines which interval has the greater (less restrictive) maximum boundary.
// For example, [2.0] is greater than (2.0).
func compareUpperBounds(intervalA, intervalB *VersionInterval) int {
	maxA, maxB := intervalA.Max, intervalB.Max
	if maxA == nil {
		return 1 // +∞ is always larger
	}
	if maxB == nil {
		return -1
	}

	if comparison := maxA.Compare(maxB); comparison != 0 {
		return comparison
	}
	// Versions are equal. An inclusive bound is "greater" (less restrictive).
	if intervalA.MaxInclusive == intervalB.MaxInclusive {
		return 0
	}
	if intervalA.MaxInclusive { // [2.0] is a "greater" bound than (2.0)
		return 1
	}
	return -1
}
