package metal_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestExportedStructFieldsHaveJSONTags guards the Postgres storage contract:
// every exported struct field must carry an explicit json tag, otherwise the
// stored JSONB keys would silently follow the Go field name.
func TestExportedStructFieldsHaveJSONTags(t *testing.T) {
	fset := token.NewFileSet()

	entries, err := os.ReadDir(".")
	require.NoError(t, err)

	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".go") || strings.HasSuffix(e.Name(), "_test.go") {
			continue
		}

		f, err := parser.ParseFile(fset, filepath.Join(".", e.Name()), nil, 0)
		require.NoError(t, err)

		ast.Inspect(f, func(n ast.Node) bool {
			st, ok := n.(*ast.StructType)
			if !ok {
				return true
			}
			for _, field := range st.Fields.List {
				for _, name := range field.Names {
					if !ast.IsExported(name.Name) {
						continue
					}
					if field.Tag == nil {
						t.Errorf("%s: field %s has no struct tag", e.Name(), name.Name)
						continue
					}
					tag, ok := reflect.StructTag(strings.Trim(field.Tag.Value, "`")).Lookup("json")
					if !ok || tag == "" {
						t.Errorf("%s: field %s has no json tag", e.Name(), name.Name)
					}
				}
			}
			return true
		})
	}
}

// TestExportedStructFieldsUseSnakeCaseJSONTags verifies the naming convention
// chosen for the Postgres storage keys.
func TestExportedStructFieldsUseSnakeCaseJSONTags(t *testing.T) {
	fset := token.NewFileSet()

	entries, err := os.ReadDir(".")
	require.NoError(t, err)

	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".go") || strings.HasSuffix(e.Name(), "_test.go") {
			continue
		}

		f, err := parser.ParseFile(fset, filepath.Join(".", e.Name()), nil, 0)
		require.NoError(t, err)

		ast.Inspect(f, func(n ast.Node) bool {
			st, ok := n.(*ast.StructType)
			if !ok {
				return true
			}
			for _, field := range st.Fields.List {
				for _, name := range field.Names {
					if !ast.IsExported(name.Name) {
						continue
					}
					if field.Tag == nil {
						continue
					}
					tag, ok := reflect.StructTag(strings.Trim(field.Tag.Value, "`")).Lookup("json")
					if !ok {
						continue
					}
					jsonName, _, _ := strings.Cut(tag, ",")
					if jsonName == "-" {
						continue
					}
					require.Regexp(t, `^[a-z][a-z0-9_]*$`, jsonName,
						"%s: field %s json tag %q is not snake_case", e.Name(), name.Name, jsonName)
				}
			}
			return true
		})
	}
}
