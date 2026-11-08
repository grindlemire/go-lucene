package driver

import (
	"fmt"
	"slices"
	"strings"

	"github.com/grindlemire/go-lucene/pkg/lucene/expr"
)

// foldsCase reports whether in is a column or JSON path listed in
// CaseInsensitiveFields. A JSON path is listed by its dotted name, such as
// labels.category.
func (b Base) foldsCase(in any) bool {
	if len(b.CaseInsensitiveFields) == 0 {
		return false
	}
	if e, isExpr := in.(*expr.Expression); isExpr && e.Op == expr.Literal {
		in = e.Left
	}
	var name string
	switch v := in.(type) {
	case expr.Column:
		name = string(v)
	case expr.JSONPath:
		// The plain dotted name, as WithCaseInsensitiveFields lists it.
		// String() quotes a name that is not one word, so it is not used.
		name = string(v.Column) + "." + strings.Join(v.Path, ".")
	default:
		return false
	}
	return slices.Contains(b.CaseInsensitiveFields, name)
}

// foldsCaseCompare reports whether e is an equality or wildcard match on a
// case-insensitive column with a string value. hasPattern is the result of
// likeOperand for a Like: it is false for a regex, which keeps its case.
func (b Base) foldsCaseCompare(e *expr.Expression, hasPattern bool) bool {
	switch e.Op {
	case expr.Equals:
		return isStringTerm(e.Right) && b.foldsCase(e.Left)
	case expr.Like:
		return hasPattern && b.foldsCase(e.Left)
	default:
		return false
	}
}

// isStringTerm reports whether in is one string value: a string, or a literal
// or wildcard that holds one. Only these are wrapped in LOWER(), because
// LOWER() of a number is an error in Postgres.
func isStringTerm(in any) bool {
	switch v := in.(type) {
	case string:
		return true
	case *expr.Expression:
		if v.Op == expr.Literal || v.Op == expr.Wild {
			_, ok := v.Left.(string)
			return ok
		}
	}
	return false
}

func lower(s string) string {
	return "LOWER(" + s + ")"
}

// renderFoldedIn renders an IN list on a case-insensitive column. String
// members are compared with LOWER() on both sides, other members are compared
// as they are, and a null member adds an IS NULL test, all joined with OR.
// ser renders one member. ok is false when the list has no string member; the
// caller then renders it as usual.
func (b Base) renderFoldedIn(
	left string,
	right any,
	ser func(any) (string, []any, error),
) (s string, params []any, ok bool, err error) {
	list, isList := right.(*expr.Expression)
	if !isList || list.Op != expr.List {
		return "", nil, false, nil
	}
	items, isSlice := list.Left.([]*expr.Expression)
	if !isSlice {
		return "", nil, false, nil
	}

	var strs, others []*expr.Expression
	hasNull := false
	for _, item := range items {
		switch {
		case item != nil && item.Op == expr.Null:
			hasNull = true
		case isStringTerm(item):
			strs = append(strs, item)
		default:
			others = append(others, item)
		}
	}
	if len(strs) == 0 {
		return "", nil, false, nil
	}

	alone := len(others) == 0 && !hasNull
	var parts []string
	// compare renders col = v for a single member that is ORed with other
	// tests, and col IN (...) otherwise, the same shapes as an unfolded list.
	compare := func(col string, members []*expr.Expression, wrap func(string) string) error {
		vals := make([]string, 0, len(members))
		for _, m := range members {
			v, p, err := ser(m)
			if err != nil {
				return err
			}
			vals = append(vals, wrap(v))
			params = append(params, p...)
		}
		if len(vals) == 1 && !alone {
			parts = append(parts, fmt.Sprintf("%s = %s", col, vals[0]))
		} else {
			parts = append(parts, fmt.Sprintf("%s IN (%s)", col, strings.Join(vals, ", ")))
		}
		return nil
	}

	if err := compare(lower(left), strs, lower); err != nil {
		return "", nil, true, err
	}
	if len(others) > 0 {
		if err := compare(left, others, func(v string) string { return v }); err != nil {
			return "", nil, true, err
		}
	}
	if hasNull {
		parts = append(parts, fmt.Sprintf("%s IS NULL", left))
	}

	if alone {
		return parts[0], params, true, nil
	}
	return "(" + strings.Join(parts, " OR ") + ")", params, true, nil
}
