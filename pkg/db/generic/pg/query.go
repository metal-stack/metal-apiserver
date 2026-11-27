package pg

import (
	"fmt"
	"reflect"
	"strings"
)

// SelectorPath constructs a JSON path by inspecting struct field names.
// Usage: SelectorPath[UserProfile]("Address", "City") -> "address.city"
func SelectorPath[T any](fieldNames ...string) string {
	var zero T
	currType := reflect.TypeOf(zero)

	if currType.Kind() == reflect.Pointer {
		currType = currType.Elem()
	}

	pathParts := make([]string, 0, len(fieldNames))

	for _, fieldName := range fieldNames {
		if currType.Kind() == reflect.Pointer {
			currType = currType.Elem()
		}

		if currType.Kind() != reflect.Struct {
			panic(fmt.Sprintf("type %s is not a struct", currType.Name()))
		}

		field, ok := currType.FieldByName(fieldName)
		if !ok {
			panic(fmt.Sprintf("field %q not found on struct %s", fieldName, currType.Name()))
		}

		// Read the json tag, falling back to field name
		jsonTag := field.Tag.Get("json")
		if jsonTag != "" && jsonTag != "-" {
			tagParts := strings.Split(jsonTag, ",")
			pathParts = append(pathParts, tagParts[0])
		} else {
			pathParts = append(pathParts, field.Name)
		}

		currType = field.Type
	}

	return strings.Join(pathParts, ".")
}
