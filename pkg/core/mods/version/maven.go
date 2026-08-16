package version

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
)

type mavenVersionToken struct {
	value     string
	numeric   bool
	separator byte
}

// TranslateMavenVersion handles concrete versions (no ranges), translating
// Maven qualifier ordering into the SemVer ordering used by this package.
func TranslateMavenVersion(mavenVersion string) (string, error) {
	// In Maven, a plain version like "1.2.3" is a concrete version.
	// No transformation needed.
	if strings.ContainsAny(mavenVersion, "[](),") {
		return "", errors.New("input is not a concrete version, use TranslateMavenVersionRange")
	}

	mavenVersion = strings.TrimSpace(mavenVersion)
	tokens := tokenizeMavenVersion(mavenVersion)
	if len(tokens) == 0 {
		return "0", nil
	}
	tokens = canonicalizeMavenTokens(tokens)

	firstQualifier := 0
	for firstQualifier < len(tokens) && tokens[firstQualifier].numeric {
		firstQualifier++
	}
	if firstQualifier == 0 {
		return mavenVersion, nil
	}
	if firstQualifier == len(tokens) {
		if isPlainNumericMavenVersion(mavenVersion) {
			return mavenVersion, nil
		}
		return numericMavenVersion(tokens), nil
	}

	baseVersion := numericMavenVersion(tokens[:firstQualifier])
	qualifierTokens := tokens[firstQualifier:]
	qualifierName := mavenQualifierName(qualifierTokens)

	// Maven's known release qualifiers compare equal to an unqualified release
	// when they are the final token sequence.
	if len(qualifierTokens) == 1 && (qualifierName == "ga" || qualifierName == "final" || qualifierName == "release") {
		return baseVersion, nil
	}

	qualifier := semverQualifier(qualifierTokens)
	if qualifier == "" {
		return mavenVersion, nil
	}

	// Maven's prerelease qualifiers sort before the base release. Other
	// qualifiers sort after the base release, so add a component between this
	// release and the next numeric Maven version before retaining the qualifier.
	switch qualifierName {
	case "a", "alpha", "b", "beta", "m", "milestone", "rc", "cr", "snapshot":
		return baseVersion + "-" + qualifier, nil
	default:
		return postReleaseBase(baseVersion) + "-" + qualifier, nil
	}
}

func tokenizeMavenVersion(version string) []mavenVersionToken {
	var tokens []mavenVersionToken
	var current strings.Builder
	var separator byte
	flush := func() {
		if current.Len() == 0 {
			return
		}
		value := current.String()
		numeric := true
		for i := range value {
			if value[i] < '0' || value[i] > '9' {
				numeric = false
				break
			}
		}
		tokens = append(tokens, mavenVersionToken{value: value, numeric: numeric, separator: separator})
		current.Reset()
		separator = 0
	}

	for i := 0; i < len(version); i++ {
		c := version[i]
		if c == '.' || c == '-' || c == '_' {
			flush()
			separator = c
			continue
		}
		if current.Len() > 0 {
			previous := current.String()[current.Len()-1]
			if (previous >= '0' && previous <= '9') != (c >= '0' && c <= '9') {
				flush()
				separator = '-'
			}
		}
		current.WriteByte(c)
	}
	flush()
	return tokens
}

func numericMavenVersion(tokens []mavenVersionToken) string {
	if len(tokens) == 0 {
		return "0"
	}
	components := make([]string, 0, len(tokens)*2)
	for i, token := range tokens {
		if i > 0 && token.separator == '-' {
			components = append(components, "0")
		}
		components = append(components, canonicalNumeric(token.value))
	}
	for len(components) > 1 && components[len(components)-1] == "0" {
		components = components[:len(components)-1]
	}
	return strings.Join(components, ".")
}

func canonicalizeMavenTokens(tokens []mavenVersionToken) []mavenVersionToken {
	canonical := append([]mavenVersionToken(nil), tokens...)

	// Maven removes trailing null values, including release aliases.
	for len(canonical) > 1 && isMavenNullToken(canonical[len(canonical)-1]) {
		canonical = canonical[:len(canonical)-1]
	}

	// A release alias before another token is also null. For example, Maven
	// canonicalizes 1-ga-1 to 1-1, while 1-sp-1 retains sp as a qualifier.
	filtered := canonical[:0]
	for i, token := range canonical {
		if i+1 < len(canonical) && canonical[i+1].numeric && !token.numeric && isMavenNullToken(token) {
			continue
		}
		filtered = append(filtered, token)
	}
	canonical = filtered

	// Trim numeric nulls from the numeric portion before each hyphenated
	// section: 1.0.0-foo.0.0 becomes 1-foo.
	for i := len(canonical) - 1; i > 0; i-- {
		if canonical[i].separator != '-' {
			continue
		}
		for j := i - 1; j >= 0 && canonical[j].numeric && canonicalNumeric(canonical[j].value) == "0"; j-- {
			canonical = append(canonical[:j], canonical[j+1:]...)
			i--
		}
	}
	return canonical
}

func isMavenNullToken(token mavenVersionToken) bool {
	if token.numeric {
		return canonicalNumeric(token.value) == "0"
	}
	switch strings.ToLower(token.value) {
	case "", "ga", "final", "release":
		return true
	default:
		return false
	}
}

func canonicalNumeric(value string) string {
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return value
	}
	return strconv.Itoa(parsed)
}

func isPlainNumericMavenVersion(version string) bool {
	if version == "" {
		return false
	}
	for i := range version {
		if (version[i] < '0' || version[i] > '9') && version[i] != '.' {
			return false
		}
	}
	return true
}

func mavenQualifierName(tokens []mavenVersionToken) string {
	if len(tokens) == 0 {
		return ""
	}
	return strings.ToLower(tokens[0].value)
}

func semverQualifier(tokens []mavenVersionToken) string {
	parts := make([]string, 0, len(tokens))
	for _, token := range tokens {
		value := strings.ToLower(token.value)
		if len(parts) == 0 {
			switch value {
			case "a":
				value = "alpha"
			case "b":
				value = "beta"
			case "m":
				value = "milestone"
			case "cr":
				value = "rc"
			case "ga", "final", "release":
				// A release alias followed by a non-numeric token is
				// still ordered before an ordinary qualifier in Maven.
				value = "0"
			}
		}
		if token.numeric {
			value = canonicalNumeric(value)
		}
		parts = append(parts, value)
	}
	for len(parts) > 1 {
		switch parts[len(parts)-1] {
		case "0", "final", "ga", "release":
			parts = parts[:len(parts)-1]
		default:
			return strings.Join(parts, ".")
		}
	}
	return strings.Join(parts, ".")
}

func postReleaseBase(baseVersion string) string {
	components := strings.Split(baseVersion, ".")
	for len(components) < 3 {
		components = append(components, "0")
	}
	// Keep post-release versions below prereleases of the next numeric
	// component as required by Maven's qualifier-vs-number ordering.
	return strings.Join(append(components, "0", "1"), ".")
}

// TranslateMavenVersionRange converts a Maven-style version range into Fabric-style ranges.
func TranslateMavenVersionRange(mavenRange string) (results []string, err error) {
	mavenRange = strings.TrimSpace(mavenRange)

	if mavenRange == "*" || mavenRange == "" {
		return []string{"*"}, nil
	}

	// If the range is just a plain version (no brackets or parens)
	if !strings.ContainsAny(mavenRange, "[]()") {
		// Special Maven meaning: the version is a soft requirement, we just use '*'
		return []string{"*"}, nil
	}

	// Split into top-level ranges (OR)
	parts, err := splitTopLevel(mavenRange)
	if err != nil {
		return nil, err
	}

	for _, part := range parts {
		fabric, err := mavenIntervalToFabric(part)
		if err != nil {
			return nil, err
		}
		results = append(results, strings.Join(fabric, " "))
	}
	return results, nil
}

// splitTopLevel splits a Maven range string by commas that are not nested inside an interval.
func splitTopLevel(s string) ([]string, error) {
	var parts []string
	depth := 0
	last := 0
	for i, r := range s {
		switch r {
		case '[', '(':
			depth++
		case ']', ')':
			if depth == 0 {
				return nil, fmt.Errorf("unbalanced brackets in %q", s)
			}
			depth--
		case ',':
			if depth == 0 {
				// top-level comma: split here
				segment := strings.TrimSpace(s[last:i])
				if segment != "" {
					parts = append(parts, segment)
				}
				last = i + 1
			}
		}
	}
	// add final segment
	if last < len(s) {
		segment := strings.TrimSpace(s[last:])
		if segment != "" {
			parts = append(parts, segment)
		}
	}
	if depth != 0 {
		return nil, fmt.Errorf("unbalanced brackets in %q", s)
	}
	return parts, nil
}

// mavenIntervalToFabric parses a single Maven interval expression.
func mavenIntervalToFabric(interval string) ([]string, error) {
	interval = strings.TrimSpace(interval)
	if interval == "" {
		return nil, errors.New("empty interval")
	}

	// Exact match: [1.0]
	if strings.HasPrefix(interval, "[") && strings.HasSuffix(interval, "]") && !strings.Contains(interval, ",") {
		version := strings.Trim(interval, "[]")
		translated, err := TranslateMavenVersion(strings.TrimSpace(version))
		if err != nil {
			return nil, err
		}
		return []string{"=" + translated}, nil
	}

	// Remove outer [] or ()
	if (strings.HasPrefix(interval, "[") || strings.HasPrefix(interval, "(")) &&
		(strings.HasSuffix(interval, "]") || strings.HasSuffix(interval, ")")) {
		inner := interval[1 : len(interval)-1]
		parts := strings.Split(inner, ",")
		if len(parts) != 2 {
			return nil, fmt.Errorf("expected two bounds: %s", interval)
		}

		low, high := strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1])
		var constraints []string
		var err error
		if low != "" && low != "*" {
			low, err = TranslateMavenVersion(low)
			if err != nil {
				return nil, err
			}
		}
		if high != "" && high != "*" {
			high, err = TranslateMavenVersion(high)
			if err != nil {
				return nil, err
			}
		}

		// Lower bound
		if low != "" && low != "*" {
			if strings.HasPrefix(interval, "[") {
				constraints = append(constraints, ">="+low)
			} else {
				constraints = append(constraints, ">"+low)
			}
		}

		// Upper bound
		if high != "" && high != "*" {
			if strings.HasSuffix(interval, "]") {
				constraints = append(constraints, "<="+high)
			} else {
				constraints = append(constraints, "<"+high)
			}
		}

		// Special case: both sides are *
		if (low == "" || low == "*") && (high == "" || high == "*") {
			return []string{"*"}, nil
		}

		return constraints, nil
	}

	return nil, fmt.Errorf("could not parse interval: %s", interval)
}
