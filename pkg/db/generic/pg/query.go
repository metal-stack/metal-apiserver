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

// PathOf accepts a generic type T and field selectors pointing to nested fields.
// Example: PathOf[UserProfile](func(u *UserProfile) any { return &u.Address.City })
func PathOf[T any](selectors ...func(instance *T) any) string {
	var dummy T
	dummyVal := reflect.ValueOf(&dummy).Elem()

	var pathParts []string

	for _, selector := range selectors {
		// Populate nested pointer/struct fields to prevent nil dereference panics
		ensureInitialized(dummyVal)

		// Execute selector function to get pointer to target field
		targetPtr := selector(&dummy)

		// Map the returned pointer back to its JSON tag
		jsonTag := resolveJSONTag(dummyVal, targetPtr)
		if jsonTag == "" {
			panic("field path could not be resolved or lacks a valid struct tag")
		}
		pathParts = append(pathParts, jsonTag)
	}

	return strings.Join(pathParts, ".")
}

// Recursively inspects the struct and finds the field matching targetPtr's address
func resolveJSONTag(val reflect.Value, targetPtr any) string {
	if val.Kind() == reflect.Pointer {
		if val.IsNil() {
			return ""
		}
		val = val.Elem()
	}

	if val.Kind() != reflect.Struct {
		return ""
	}

	targetAddr := reflect.ValueOf(targetPtr).Pointer()

	for i := 0; i < val.NumField(); i++ {
		fieldVal := val.Field(i)
		fieldType := val.Type().Field(i)

		// Check pointer address matching
		if fieldVal.CanAddr() {
			fieldAddr := fieldVal.Addr().Pointer()
			if fieldAddr == targetAddr {
				return getJSONName(fieldType)
			}
		}

		// Recurse into nested structs
		if fieldVal.Kind() == reflect.Struct || (fieldVal.Kind() == reflect.Pointer && !fieldVal.IsNil()) {
			if tag := resolveJSONTag(fieldVal, targetPtr); tag != "" {
				return getJSONName(fieldType) + "." + tag
			}
		}
	}

	return ""
}

func ensureInitialized(val reflect.Value) {
	if val.Kind() == reflect.Pointer && val.IsNil() && val.CanSet() {
		val.Set(reflect.New(val.Type().Elem()))
	}
	if val.Kind() == reflect.Pointer {
		val = val.Elem()
	}
	if val.Kind() == reflect.Struct {
		for _, field := range val.Fields() {
			ensureInitialized(field)
		}
	}
}

func getJSONName(field reflect.StructField) string {
	tag := field.Tag.Get("json")
	if tag != "" && tag != "-" {
		return strings.Split(tag, ",")[0]
	}
	return field.Name
}
