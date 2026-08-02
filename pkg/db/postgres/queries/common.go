package queries

import "fmt"

// enumGetStringValue extracts the string value from a protobuf enum.
func enumGetStringValue(v any) (string, error) {
	if s, ok := v.(interface{ String() string }); ok {
		return s.String(), nil
	}
	return "", fmt.Errorf("cannot convert %T to string", v)
}

// escapeJSONKey escapes a JSON key for use in a quoted string.
func escapeJSONKey(key string) string {
	return key
}

// escapeJSONString escapes a string for use in a JSON literal.
func escapeJSONString(s string) string {
	return s
}
