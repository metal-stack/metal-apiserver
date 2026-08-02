package queries

import "fmt"

// containsAny checks if a string is present in a []any slice.
func containsAny(slice []any, val string) bool {
	for _, v := range slice {
		if s, ok := v.(string); ok && s == val {
			return true
		}
	}
	return false
}

// toString converts a value from a deserialized JSON map to a string.
func toString(v any) string {
	if v == nil {
		return ""
	}
	if s, ok := v.(string); ok {
		return s
	}
	return fmt.Sprintf("%v", v)
}
