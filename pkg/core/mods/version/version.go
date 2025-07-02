package version

import (
	"fmt"
	"regexp"
	"strconv"
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

// orderedOperators ensures we check for ">=" before ">", etc.
var orderedOperators = []VersionComparisonOperator{
	OpGreaterOrEqual, OpLessOrEqual, OpGreater, OpLess, OpCaret, OpTilde, OpEqual,
}

var (
	dotSeparatedIDRegex  = regexp.MustCompile(`^[0-9A-Za-z-]+(\.[0-9A-Za-z-]+)*$`)
	unsignedIntegerRegex = regexp.MustCompile(`^(0|[1-9][0-9]*)$`)
	componentWildcard    = -1
)

// --- Version Types ---

// Version is a generic container for either a SemanticVersion or a StringVersion.
// It embeds fmt.Stringer, the idiomatic Go interface for types that can be stringified.
type Version interface {
	fmt.Stringer
	// Compare returns -1 if the receiver is less than `other`, 0 if they are equal,
	// and 1 if the receiver is greater than `other`.
	Compare(other Version) int
	IsSemantic() bool
}

// SemanticVersion implements the extended SemVer format.
type SemanticVersion struct {
	components    []int
	prerelease    string
	build         string
	hasPrerelease bool
}

// StringVersion is a fallback for non-semantic version strings.
type StringVersion struct {
	version string
}

// --- Version Parsing ---

// Parse parses a string into the most appropriate Version type.
func Parse(s string, storeX bool) (Version, error) {
	if s == "" {
		return nil, fmt.Errorf("version must be a non-empty string")
	}
	sv, err := ParseSemantic(s, storeX)
	if err != nil {
		return &StringVersion{version: s}, nil
	}
	return sv, nil
}

// ParseSemantic strictly parses a string into a SemanticVersion.
func ParseSemantic(s string, storeX bool) (*SemanticVersion, error) {
	if s == "" {
		return nil, fmt.Errorf("version must be a non-empty string")
	}
	v := &SemanticVersion{}
	original := s
	if plusIdx := strings.Index(s, "+"); plusIdx != -1 {
		v.build = s[plusIdx+1:]
		s = s[:plusIdx]
	}
	if dashIdx := strings.Index(s, "-"); dashIdx != -1 {
		v.prerelease = s[dashIdx+1:]
		s = s[:dashIdx]
		v.hasPrerelease = true
		if v.prerelease != "" && !dotSeparatedIDRegex.MatchString(v.prerelease) {
			return nil, fmt.Errorf("invalid prerelease string '%s'", v.prerelease)
		}
	}
	if strings.HasSuffix(s, ".") || strings.HasPrefix(s, ".") {
		return nil, fmt.Errorf("version string must not start or end with a dot: %s", original)
	}
	componentStrings := strings.Split(s, ".")
	if len(componentStrings) == 0 || (len(componentStrings) == 1 && componentStrings[0] == "") {
		return nil, fmt.Errorf("did not provide version numbers: %s", original)
	}
	v.components = make([]int, len(componentStrings))
	firstWildcardIdx := -1
	for i, compStr := range componentStrings {
		if storeX {
			if compStr == "x" || compStr == "X" || compStr == "*" {
				if v.hasPrerelease {
					return nil, fmt.Errorf("pre-release versions are not allowed to use X-ranges")
				}
				if i == 0 {
					return nil, fmt.Errorf("versions of form 'x' or 'X' not allowed")
				}
				v.components[i] = componentWildcard
				if firstWildcardIdx == -1 {
					firstWildcardIdx = i
				}
				continue
			} else if i > 0 && v.components[i-1] == componentWildcard {
				return nil, fmt.Errorf("interjacent wildcards (e.g., 1.x.2) are disallowed")
			}
		}
		if compStr == "x" || compStr == "X" || compStr == "*" {
			return nil, fmt.Errorf("wildcard characters are only allowed when storeX is true")
		}
		if compStr == "" {
			return nil, fmt.Errorf("missing version number component")
		}
		num, err := strconv.Atoi(compStr)
		if err != nil {
			return nil, fmt.Errorf("could not parse version number component '%s': %w", compStr, err)
		}
		if num < 0 {
			return nil, fmt.Errorf("negative version number component '%s'", compStr)
		}
		v.components[i] = num
	}
	if firstWildcardIdx > 0 && len(v.components) > firstWildcardIdx+1 {
		v.components = v.components[:firstWildcardIdx+1]
	}
	return v, nil
}

// --- SemanticVersion Methods ---
func (sv *SemanticVersion) String() string {
	var b strings.Builder
	for i, c := range sv.components {
		if i > 0 {
			b.WriteRune('.')
		}
		if c == componentWildcard {
			b.WriteRune('x')
		} else {
			b.WriteString(strconv.Itoa(c))
		}
	}
	if sv.hasPrerelease {
		b.WriteRune('-')
		b.WriteString(sv.prerelease)
	}
	if sv.build != "" {
		b.WriteRune('+')
		b.WriteString(sv.build)
	}
	return b.String()
}
func (sv *SemanticVersion) IsSemantic() bool { return true }
func (sv *SemanticVersion) HasWildcard() bool {
	for _, c := range sv.components {
		if c == componentWildcard {
			return true
		}
	}
	return false
}
func (sv *SemanticVersion) VersionComponent(pos int) int {
	if pos < 0 {
		panic("tried to access negative version number component")
	}
	if pos >= len(sv.components) {
		if len(sv.components) > 0 && sv.components[len(sv.components)-1] == componentWildcard {
			return componentWildcard
		}
		return 0
	}
	return sv.components[pos]
}
func (sv *SemanticVersion) Compare(other Version) int {
	otherSv, ok := other.(*SemanticVersion)
	if !ok {
		return strings.Compare(sv.String(), other.String())
	}
	maxLen := len(sv.components)
	if len(otherSv.components) > maxLen {
		maxLen = len(otherSv.components)
	}
	for i := 0; i < maxLen; i++ {
		compA := sv.VersionComponent(i)
		compB := otherSv.VersionComponent(i)
		if compA == componentWildcard || compB == componentWildcard {
			continue
		}
		if compA < compB {
			return -1
		}
		if compA > compB {
			return 1
		}
	}
	if sv.hasPrerelease && !otherSv.hasPrerelease {
		return -1
	}
	if !sv.hasPrerelease && otherSv.hasPrerelease {
		return 1
	}
	if !sv.hasPrerelease && !otherSv.hasPrerelease {
		return 0
	}
	partsA := strings.Split(sv.prerelease, ".")
	partsB := strings.Split(otherSv.prerelease, ".")
	if sv.prerelease == "" {
		partsA = []string{}
	}
	if otherSv.prerelease == "" {
		partsB = []string{}
	}
	commonLen := len(partsA)
	if len(partsB) < commonLen {
		commonLen = len(partsB)
	}
	for i := 0; i < commonLen; i++ {
		partA, partB := partsA[i], partsB[i]
		isANumeric := unsignedIntegerRegex.MatchString(partA)
		isBNumeric := unsignedIntegerRegex.MatchString(partB)
		if isANumeric && isBNumeric {
			numA, _ := strconv.Atoi(partA)
			numB, _ := strconv.Atoi(partB)
			if numA < numB {
				return -1
			}
			if numA > numB {
				return 1
			}
		} else if isANumeric {
			return -1
		} else if isBNumeric {
			return 1
		} else {
			if comp := strings.Compare(partA, partB); comp != 0 {
				return comp
			}
		}
	}
	if len(partsA) < len(partsB) {
		return -1
	}
	if len(partsA) > len(partsB) {
		return 1
	}
	return 0
}

// --- StringVersion Methods ---
func (sv *StringVersion) String() string   { return sv.version }
func (sv *StringVersion) IsSemantic() bool { return false }
func (sv *StringVersion) Compare(other Version) int {
	return strings.Compare(sv.String(), other.String())
}

// --- VersionInterval ---

// VersionInterval represents a range of versions, such as [1.0, 2.0).
// Its fields are public to allow for direct inspection.
type VersionInterval struct {
	Min          Version
	MinInclusive bool
	Max          Version
	MaxInclusive bool
}

// String returns the mathematical notation of the interval.
func (vi *VersionInterval) String() string {
	var min, max string
	var minBracket, maxBracket byte

	if vi.Min == nil {
		min = "-∞"
		minBracket = '('
	} else {
		min = vi.Min.String()
		if vi.MinInclusive {
			minBracket = '['
		} else {
			minBracket = '('
		}
	}

	if vi.Max == nil {
		max = "∞"
		maxBracket = ')'
	} else {
		max = vi.Max.String()
		if vi.MaxInclusive {
			maxBracket = ']'
		} else {
			maxBracket = ')'
		}
	}
	return fmt.Sprintf("%c%s,%s%c", minBracket, min, max, maxBracket)
}

// Contains checks if a version is within the interval.
func (vi *VersionInterval) Contains(v Version) bool {
	if vi.Min != nil {
		cmp := v.Compare(vi.Min)
		if cmp < 0 || (cmp == 0 && !vi.MinInclusive) {
			return false
		}
	}
	if vi.Max != nil {
		cmp := v.Compare(vi.Max)
		if cmp > 0 || (cmp == 0 && !vi.MaxInclusive) {
			return false
		}
	}
	return true
}

// And computes the intersection of this interval with another.
func (vi *VersionInterval) And(other *VersionInterval) *VersionInterval {
	if vi == nil || other == nil {
		return nil
	}
	// Handle intersection with infinite interval correctly.
	if vi.Min == nil && vi.Max == nil {
		return other
	}
	if other.Min == nil && other.Max == nil {
		return vi
	}
	if !vi.isSemantic() || !other.isSemantic() {
		if vi.Min != nil && vi.Min.Compare(vi.Max) == 0 && other.Min != nil && other.Min.Compare(other.Max) == 0 && vi.Min.Compare(other.Min) == 0 {
			return vi
		}
		return nil
	}
	var newMin Version
	var newMinInclusive bool
	if compareMin(vi, other) >= 0 {
		newMin, newMinInclusive = vi.Min, vi.MinInclusive
	} else {
		newMin, newMinInclusive = other.Min, other.MinInclusive
	}
	var newMax Version
	var newMaxInclusive bool
	if compareMax(vi, other) <= 0 {
		newMax, newMaxInclusive = vi.Max, vi.MaxInclusive
	} else {
		newMax, newMaxInclusive = other.Max, other.MaxInclusive
	}
	if newMin != nil && newMax != nil {
		cmp := newMin.Compare(newMax)
		if cmp > 0 || (cmp == 0 && (!newMinInclusive || !newMaxInclusive)) {
			return nil
		}
	}
	return &VersionInterval{Min: newMin, MinInclusive: newMinInclusive, Max: newMax, MaxInclusive: newMaxInclusive}
}

// isSemantic is a private helper, as the public API should be on the interval itself.
func (vi *VersionInterval) isSemantic() bool {
	return (vi.Min == nil || vi.Min.IsSemantic()) && (vi.Max == nil || vi.Max.IsSemantic())
}
func compareMin(a, b *VersionInterval) int {
	aMin, bMin := a.Min, b.Min
	if aMin == nil {
		if bMin == nil {
			return 0
		}
		return -1
	}
	if bMin == nil {
		return 1
	}
	cmp := aMin.Compare(bMin)
	if cmp != 0 {
		return cmp
	}
	if a.MinInclusive == b.MinInclusive {
		return 0
	}
	if a.MinInclusive {
		return -1
	}
	return 1
}
func compareMax(a, b *VersionInterval) int {
	aMax, bMax := a.Max, b.Max
	if aMax == nil {
		if bMax == nil {
			return 0
		}
		return 1
	}
	if bMax == nil {
		return -1
	}
	cmp := aMax.Compare(bMax)
	if cmp != 0 {
		return cmp
	}
	if a.MaxInclusive == b.MaxInclusive {
		return 0
	}
	if a.MaxInclusive {
		return 1
	}
	return -1
}

// --- VersionPredicate ---

// PredicateTerm represents a single part of a predicate string, like ">1.0.0".
type PredicateTerm struct {
	Operator VersionComparisonOperator
	Version  Version
}

func (pt *PredicateTerm) String() string {
	opStr := string(pt.Operator)
	// Don't print the operator for default equality.
	if opStr == "=" {
		opStr = ""
	}
	return opStr + pt.Version.String()
}

// VersionPredicate represents one or more AND-ed predicate terms.
type VersionPredicate struct {
	terms []*PredicateTerm
	isAny bool
}

// Any returns a predicate that matches any version.
func Any() *VersionPredicate {
	return &VersionPredicate{isAny: true}
}

// ParseVersionPredicate parses a requirement string like ">=1.0.0 <2.0.0" into a predicate.
func ParseVersionPredicate(s string) (*VersionPredicate, error) {
	s = strings.TrimSpace(s)
	if s == "" || s == "*" {
		return Any(), nil
	}
	p := &VersionPredicate{}
	parts := strings.Fields(s)
	for _, part := range parts {
		op, versionStr := parseOperator(part)
		v, err := Parse(versionStr, true)
		if err != nil {
			return nil, fmt.Errorf("could not parse version string '%s' in predicate '%s'", versionStr, s)
		}
		// Check for invalid operators on non-semantic versions at parse time.
		if !v.IsSemantic() && op != OpEqual {
			return nil, fmt.Errorf("non-semantic version '%s' can only be used with equality operator", v.String())
		}
		p.terms = append(p.terms, &PredicateTerm{Operator: op, Version: v})
	}
	return p, nil
}

// String reconstructs the original predicate string.
func (p *VersionPredicate) String() string {
	if p.isAny {
		return "*"
	}
	var parts []string
	for _, term := range p.terms {
		parts = append(parts, term.String())
	}
	return strings.Join(parts, " ")
}

// Interval calculates the single VersionInterval that this predicate represents.
func (p *VersionPredicate) Interval() *VersionInterval {
	if p.isAny {
		return &VersionInterval{}
	}
	if len(p.terms) == 0 {
		return nil
	}
	currentInterval := &VersionInterval{} // Represents (-inf, inf)
	for _, term := range p.terms {
		termInterval, err := term.interval()
		if err != nil {
			return nil
		}
		currentInterval = currentInterval.And(termInterval)
		if currentInterval == nil {
			return nil
		}
	}
	return currentInterval
}

// Test checks if a version satisfies the predicate.
func (p *VersionPredicate) Test(v Version) bool {
	interval := p.Interval()
	if interval == nil {
		return false
	}
	return interval.Contains(v)
}

// --- Predicate Parsing Helpers ---

func parseOperator(part string) (VersionComparisonOperator, string) {
	for _, op := range orderedOperators {
		if strings.HasPrefix(part, string(op)) {
			return op, part[len(op):]
		}
	}
	return OpEqual, part
}

// interval calculates the VersionInterval for a single predicate term.
func (pt *PredicateTerm) interval() (*VersionInterval, error) {
	op := pt.Operator
	v := pt.Version
	sv, isSemantic := v.(*SemanticVersion)

	if !isSemantic {
		// This is checked at parse time, but included for defensive programming.
		return &VersionInterval{Min: v, MinInclusive: true, Max: v, MaxInclusive: true}, nil
	}

	if sv.HasWildcard() {
		// This block normalizes a wildcard version like "1.x" into a tilde/caret
		// operation on a non-wildcard version like "1.0.0-".
		compCount := 0
		for _, c := range sv.components {
			if c != componentWildcard {
				compCount++
			}
		}

		// Create a new base version, taking only the non-wildcard components.
		// This is the core fix.
		newComps := make([]int, compCount)
		copy(newComps, sv.components[:compCount]) // Correctly copy only non-wildcard parts

		baseVersion := &SemanticVersion{components: newComps, hasPrerelease: true, prerelease: ""}

		if compCount == 1 {
			op = OpCaret
		} else {
			op = OpTilde
		}
		sv = baseVersion
	}

	switch op {
	case OpEqual:
		return &VersionInterval{Min: sv, MinInclusive: true, Max: sv, MaxInclusive: true}, nil
	case OpGreater:
		return &VersionInterval{Min: sv, MinInclusive: false}, nil
	case OpGreaterOrEqual:
		return &VersionInterval{Min: sv, MinInclusive: true}, nil
	case OpLess:
		return &VersionInterval{Max: sv, MaxInclusive: false}, nil
	case OpLessOrEqual:
		return &VersionInterval{Max: sv, MaxInclusive: true}, nil
	case OpTilde:
		minV := sv
		// Normalize versions like ~1.2 to ~1.2.0 for consistent upper bound calculation
		// and correct string representation.
		if len(sv.components) < 3 {
			newComps := make([]int, 3)
			copy(newComps, sv.components)
			minV = &SemanticVersion{components: newComps, hasPrerelease: sv.hasPrerelease, prerelease: sv.prerelease}
		}

		upperComps := make([]int, len(minV.components))
		copy(upperComps, minV.components)
		// This is safe even if len < 2 because of the normalization above.
		upperComps[1]++
		for i := 2; i < len(upperComps); i++ {
			upperComps[i] = 0
		}
		maxV := &SemanticVersion{components: upperComps, hasPrerelease: true}
		return &VersionInterval{Min: minV, MinInclusive: true, Max: maxV, MaxInclusive: false}, nil
	case OpCaret:
		minV := sv
		// Normalize versions like ^1 to ^1.0.0 for correctness.
		if len(sv.components) < 3 {
			newComps := make([]int, 3)
			copy(newComps, sv.components)
			minV = &SemanticVersion{components: newComps, hasPrerelease: sv.hasPrerelease, prerelease: sv.prerelease}
		}

		upperComps := make([]int, len(minV.components))
		copy(upperComps, minV.components)
		upperComps[0]++
		for i := 1; i < len(upperComps); i++ {
			upperComps[i] = 0
		}
		maxV := &SemanticVersion{components: upperComps, hasPrerelease: true}
		return &VersionInterval{Min: minV, MinInclusive: true, Max: maxV, MaxInclusive: false}, nil
	default:
		return nil, fmt.Errorf("unknown operator: %s", op)
	}
}
