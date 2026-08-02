package cond

import "fmt"

// Where is a condition that can be used in a JSONB query.
// It builds a WHERE clause using PostgreSQL JSONB operators.
type Where struct {
	SQL  string
	Args []any
}

// And combines multiple conditions with AND.
func And(conds ...*Where) *Where {
	if len(conds) == 0 {
		return nil
	}
	if len(conds) == 1 {
		return conds[0]
	}
	var parts []string
	var args []any
	for _, c := range conds {
		if c == nil {
			continue
		}
		parts = append(parts, c.SQL)
		args = append(args, c.Args...)
	}
	return &Where{
		SQL:  "(" + joinWithAND(parts) + ")",
		Args: args,
	}
}

func joinWithAND(parts []string) string {
	result := parts[0]
	for i := 1; i < len(parts); i++ {
		result += " AND " + parts[i]
	}
	return result
}

// FieldEq returns a WHERE clause for data->>'field' = $N.
func FieldEq(field, value string) *Where {
	return &Where{
		SQL:  fmt.Sprintf("data->>'%s' = $%d", field, 1),
		Args: []any{value},
	}
}

// NestedFieldEq returns a WHERE clause for data->parent->>'field' = $N.
func NestedFieldEq(parent, field, value string) *Where {
	return &Where{
		SQL:  fmt.Sprintf("data->'%s'->>'%s' = $%d", parent, field, 1),
		Args: []any{value},
	}
}

// FieldContains returns a WHERE clause for data @> $N::jsonb.
// This checks if the document contains the given JSON subset.
func FieldContains(subsetJSON string) *Where {
	return &Where{
		SQL:  fmt.Sprintf("data @> $%d::jsonb", 1),
		Args: []any{subsetJSON},
	}
}

// NestedArrayFieldEq checks for a value inside a JSON array of objects:
// EXISTS (SELECT 1 FROM jsonb_array_elements(data->'parent') elem WHERE elem->>'field' = $N)
func NestedArrayFieldEq(parent, field, value string) *Where {
	return &Where{
		SQL:  fmt.Sprintf("EXISTS (SELECT 1 FROM jsonb_array_elements(data->'%s') elem WHERE elem->>'%s' = $%d)", parent, field, 1),
		Args: []any{value},
	}
}

// NestedArrayCheck checks for a nested condition inside an array element:
// EXISTS (SELECT 1 FROM jsonb_array_elements(data->'parent') elem WHERE elem @> $N::jsonb)
func NestedArrayCheck(parent string, subsetJSON string) *Where {
	return &Where{
		SQL:  fmt.Sprintf("EXISTS (SELECT 1 FROM jsonb_array_elements(data->'%s') elem WHERE elem @> $%d::jsonb)", parent, 1),
		Args: []any{subsetJSON},
	}
}

// FieldHasKey checks if the JSONB document has the specified key.
func FieldHasKey(field string) *Where {
	return &Where{
		SQL:  fmt.Sprintf("data ? '%s'", field),
		Args: nil,
	}
}

// NestedFieldHasKey checks if a nested object has the specified key.
func NestedFieldHasKey(parent, field string) *Where {
	return &Where{
		SQL:  fmt.Sprintf("data->'%s' ? '%s'", parent, field),
		Args: nil,
	}
}

// FieldIsNull checks if the JSONB value at the given key is null.
func FieldIsNull(field string) *Where {
	return &Where{
		SQL:  fmt.Sprintf("data->>'%s' IS NULL", field),
		Args: nil,
	}
}

// FieldIsNotNull checks if the JSONB key exists.
func FieldIsNotNull(field string) *Where {
	return &Where{
		SQL:  fmt.Sprintf("data ? '%s'", field),
		Args: nil,
	}
}

// FieldEqFloat compares a JSONB value to a float.
func FieldEqFloat(field string, value float64) *Where {
	return &Where{
		SQL:  fmt.Sprintf("(data->>'%s')::float8 = $%d", field, 1),
		Args: []any{value},
	}
}

// FieldEqInt compares a JSONB value to an integer.
func FieldEqInt(field string, value int) *Where {
	return &Where{
		SQL:  fmt.Sprintf("(data->>'%s')::int = $%d", field, 1),
		Args: []any{value},
	}
}

// TagInSlice checks if a tag string is contained in a JSONB text array field.
// Uses: EXISTS (SELECT 1 FROM jsonb_array_elements_text(data->'Tags') tag WHERE tag = $N)
func TagInSlice(value string) *Where {
	return &Where{
		SQL:  fmt.Sprintf("EXISTS (SELECT 1 FROM jsonb_array_elements_text(data->'Tags') tag WHERE tag = $%d)", 1),
		Args: []any{value},
	}
}
