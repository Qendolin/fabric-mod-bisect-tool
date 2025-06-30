package sets

import "strings"

// Set represents a collection of unique string values.
type Set map[string]struct{}

// OrderedSet represents a slice of strings.
// Note: This is a type alias and does not enforce uniqueness or order at compile time.
// Functions that return an OrderedSet, like MakeSlice, guarantee the slice is sorted and unique.
type OrderedSet []string

// SetFormatter provides a lazy, fmt.Stringer-compliant way to format a Set
// for logging. The conversion to a sorted string only happens when the String()
// method is called by the logging framework.
type SetFormatter struct {
	set Set
}

// FormatSet returns a SetFormatter that can be used in logging statements.
// It delays the expensive work of sorting and joining until it's actually needed.
func FormatSet(set Set) SetFormatter {
	return SetFormatter{set: set}
}

// String implements the fmt.Stringer interface.
func (sf SetFormatter) String() string {
	if len(sf.set) == 0 {
		return "[]"
	}
	// MakeSlice already sorts the slice, providing deterministic output.
	slice := MakeSlice(sf.set)
	return "[" + strings.Join(slice, ", ") + "]"
}
