package postgres

import (
	"fmt"
	"strings"

	"github.com/metal-stack/metal-apiserver/pkg/db/postgres/cond"
)

// buildWhereClause builds a SQL WHERE clause from a list of Where conditions.
// If no conditions are given, returns an empty string (no WHERE clause).
// It renumbers the parameter placeholders to be sequential starting from startParam.
func buildWhereClause(conds []*cond.Where, startParam int) (string, []any) {
	if len(conds) == 0 {
		return "", nil
	}

	var parts []string
	var args []any
	paramIdx := startParam

	for _, c := range conds {
		if c == nil {
			continue
		}
		sql := c.SQL
		for range c.Args {
			// Replace $N with the actual parameter number
			old := fmt.Sprintf("$%d", paramIdx)
			// Only replace first occurrence to handle duplicate patterns
			sql = strings.Replace(sql, fmt.Sprintf("$%d", 1), old, 1)
			paramIdx++
		}
		parts = append(parts, sql)
		args = append(args, c.Args...)
	}

	if len(parts) == 0 {
		return "", nil
	}

	return " WHERE " + strings.Join(parts, " AND "), args
}

func quoteIdent(name string) string {
	return `"` + name + `"`
}

// separateConditions separates query args into Where conditions and in-memory filter funcs.
func separateConditions(args []any) ([]*cond.Where, []func(map[string]any) bool) {
	var conds []*cond.Where
	var filters []func(map[string]any) bool
	for _, a := range args {
		if a == nil {
			continue
		}
		switch v := a.(type) {
		case *cond.Where:
			conds = append(conds, v)
		case func(map[string]any) bool:
			filters = append(filters, v)
		}
	}
	return conds, filters
}
