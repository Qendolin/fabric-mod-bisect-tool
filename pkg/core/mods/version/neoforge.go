package version

import (
	"errors"
	"fmt"
	"strings"
)

// TranslateMavenVersion handles concrete versions (no ranges).
// For now we just return the value unchanged.
func TranslateMavenVersion(mavenVersion string) (string, error) {
	// In Maven, a plain version like "1.2.3" is a concrete version.
	// No transformation needed.
	if strings.ContainsAny(mavenVersion, "[](),") {
		return "", errors.New("input is not a concrete version, use TranslateMavenVersionRange")
	}
	return strings.TrimSpace(mavenVersion), nil
}

// TranslateMavenVersionRange converts a Maven-style version range into Fabric-style ranges.
func TranslateMavenVersionRange(mavenRange string) (results []string, err error) {
	mavenRange = strings.TrimSpace(mavenRange)

	if mavenRange == "*" || mavenRange == "" {
		return []string{"*"}, nil
	}

	// If the range is just a plain version (no brackets or parens)
	if !strings.ContainsAny(mavenRange, "[]()") {
		// Special Maven meaning: treat as minimum version, but actually we just use '*'
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
		return []string{"=" + strings.TrimSpace(version)}, nil
	}

	// Remove outer [] or ()
	if (strings.HasPrefix(interval, "[") || strings.HasPrefix(interval, "(")) &&
		(strings.HasSuffix(interval, "]") || strings.HasSuffix(interval, ")")) {
		inner := interval[1 : len(interval)-1]
		parts := strings.Split(inner, ",")
		if len(parts) == 1 {
			return nil, fmt.Errorf("unexpected single bound: %s", interval)
		}

		low, high := strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1])
		var constraints []string

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
