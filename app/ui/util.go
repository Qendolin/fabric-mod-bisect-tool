package ui

// subtractSet returns a new set containing elements from set 'a' that are not present in set 'b'.
func subtractSet(a, b map[string]struct{}) map[string]struct{} {
	result := make(map[string]struct{})
	for k := range a {
		if _, found := b[k]; !found {
			result[k] = struct{}{}
		}
	}
	return result
}
