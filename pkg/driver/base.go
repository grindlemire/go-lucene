package driver

import (
	"fmt"
	"strings"

	"github.com/grindlemire/go-lucene/pkg/lucene/expr"
)

// Shared is the shared set of render functions that can be used as a base and overriden
// for each flavor of sql
var Shared = map[expr.Operator]RenderFN{
	expr.Literal: literal,
	expr.And:     basicCompound(expr.And),
	expr.Or:      basicCompound(expr.Or),
	expr.Not:     basicWrap(expr.Not),
	expr.Equals:  equals,
	expr.Must:    noop,                // must doesn't really translate to sql
	expr.MustNot: basicWrap(expr.Not), // must not is really just a negation
	expr.Wild:    literal,
	expr.Greater:   greater,
	expr.GreaterEq: greaterEq,
	expr.Less:      less,
	expr.LessEq:    lessEq,
	expr.In:        inFn,
	expr.List:      list,
}

// Base is the base driver that is embedded in each driver
type Base struct {
	RenderFNs map[expr.Operator]RenderFN
	// Dialect captures database-specific rendering for Like, Range, standalone
	// wildcard, pattern escaping, and bool literals. If nil, Base falls back to
	// a Postgres-compatible default to preserve backwards compatibility for
	// custom drivers built against the pre-dialect API.
	Dialect Dialect
}

// dialect returns the configured dialect, falling back to defaultDialect if
// the Base was constructed without one (the historical extension API).
func (b Base) dialect() Dialect {
	if b.Dialect == nil {
		return defaultDialect
	}
	return b.Dialect
}

// RenderParam will render the expression into a parameterized query. The returned string will contain placeholders
// and the params will contain the values that should be passed to the query.
func (b Base) RenderParam(e *expr.Expression) (s string, params []any, err error) {
	if e == nil {
		return "", params, nil
	}

	// Standalone Regexp expression: strip /.../ delimiters and return as a
	// parameterized value. This mirrors what serializeParams does for nested
	// Regexp sub-expressions.
	if e.Op == expr.Regexp {
		s, _ := e.Left.(string)
		if len(s) >= 2 && s[0] == '/' && s[len(s)-1] == '/' {
			s = s[1 : len(s)-1]
		}
		return "?", []any{s}, nil
	}

	d := b.dialect()

	left, lparams, err := b.serializeParams(e.Left)
	if err != nil {
		return s, params, err
	}

	right, rparams, err := b.serializeParams(e.Right)
	if err != nil {
		return s, params, err
	}

	// Standalone wildcard on a Like operator: `field:*`. Route through the
	// dialect so each database can decide how to represent "any value".
	if right == "'*'" && e.Op == expr.Like {
		str, err := d.RenderStandaloneWild(left)
		return str, lparams, err
	}

	// Detect regex (Lucene /regex/) vs. wildcard and let the dialect escape
	// the wildcard pattern however it needs to.
	isRegex := false
	if e.Op == expr.Like {
		if rightExpr, ok := e.Right.(*expr.Expression); ok && rightExpr.Op == expr.Regexp {
			isRegex = true
		}
		if !isRegex && len(rparams) > 0 {
			rparams[0] = d.EscapeLikePattern(rparams[0].(string))
		}
	}

	params = append(lparams, rparams...)

	if e.Op != expr.Range &&
		e.Op != expr.Not &&
		e.Op != expr.List &&
		e.Op != expr.In &&
		e.Op != expr.Literal &&
		e.Op != expr.Must &&
		e.Op != expr.MustNot {
		if !b.isSimple(e.Left) {
			left = "(" + left + ")"
		}
		if !b.isSimple(e.Right) {
			right = "(" + right + ")"
		}
	}

	if e.Op == expr.Like {
		str, err := d.RenderLike(left, right, isRegex)
		return str, params, err
	}

	if e.Op == expr.Range {
		str, err := d.RenderRangeParam(left, right, rparams)
		return str, params, err
	}

	fn, ok := b.RenderFNs[e.Op]
	if !ok {
		return s, params, fmt.Errorf("unable to render operator [%s]", e.Op)
	}

	str, err := fn(left, right)
	return str, params, err
}

// Render will render the expression based on the renderFNs provided by the driver.
func (b Base) Render(e *expr.Expression) (s string, err error) {
	if e == nil {
		return "", nil
	}

	// Standalone Regexp expression: strip /.../ delimiters and return as a
	// single-quoted literal. This mirrors what serialize does for nested
	// Regexp sub-expressions.
	if e.Op == expr.Regexp {
		s, _ := e.Left.(string)
		if len(s) >= 2 && s[0] == '/' && s[len(s)-1] == '/' {
			s = s[1 : len(s)-1]
		}
		return fmt.Sprintf("'%s'", strings.ReplaceAll(s, "'", "''")), nil
	}

	d := b.dialect()

	left, err := b.serialize(e.Left)
	if err != nil {
		return s, err
	}

	right, err := b.serialize(e.Right)
	if err != nil {
		return s, err
	}

	// Standalone wildcard on a Like operator: `field:*`. Route through the
	// dialect so each database can decide how to represent "any value".
	// Positioned before paren-wrap to stay symmetric with RenderParam.
	if right == "'*'" && e.Op == expr.Like {
		return d.RenderStandaloneWild(left)
	}

	if e.Op != expr.Range &&
		e.Op != expr.Not &&
		e.Op != expr.List &&
		e.Op != expr.In &&
		e.Op != expr.Literal &&
		e.Op != expr.Must &&
		e.Op != expr.MustNot {
		if !b.isSimple(e.Left) {
			left = "(" + left + ")"
		}
		if !b.isSimple(e.Right) {
			right = "(" + right + ")"
		}
	}

	if e.Op == expr.Like {
		isRegex := false
		if rightExpr, ok := e.Right.(*expr.Expression); ok && rightExpr.Op == expr.Regexp {
			isRegex = true
		}
		if !isRegex && len(right) >= 2 && right[0] == '\'' && right[len(right)-1] == '\'' {
			inner := right[1 : len(right)-1]
			inner = d.EscapeLikePattern(inner)
			right = "'" + inner + "'"
		}
		return d.RenderLike(left, right, isRegex)
	}

	if e.Op == expr.Range {
		return d.RenderRange(left, right)
	}

	fn, ok := b.RenderFNs[e.Op]
	if !ok {
		return s, fmt.Errorf("unable to render operator [%s]", e.Op)
	}

	return fn(left, right)
}

func (b Base) isSimple(in any) bool {
	switch v := in.(type) {
	case *expr.Expression:
		return v.Op == expr.Undefined || v.Op == expr.Literal || v.Op == expr.Regexp || v.Op == expr.Wild
	case expr.Column:
		return true
	case nil:
		return true
	case string, int, float64:
		return true
	default:
		return false
	}
}

func (b Base) serialize(in any) (s string, err error) {
	if in == nil {
		return "", nil
	}

	switch v := in.(type) {
	case *expr.Expression:
		if v.Op == expr.Regexp {
			s, _ := v.Left.(string)
			if len(s) >= 2 && s[0] == '/' && s[len(s)-1] == '/' {
				s = s[1 : len(s)-1]
			}
			return fmt.Sprintf("'%s'", strings.ReplaceAll(s, "'", "''")), nil
		}
		return b.Render(v)
	case []*expr.Expression:
		strs := []string{}
		for _, e := range v {
			s, err = b.Render(e)
			if err != nil {
				return s, err
			}
			strs = append(strs, s)
		}
		return strings.Join(strs, ", "), nil
	case *expr.RangeBoundary:
		min, err := b.serialize(v.Min)
		if err != nil {
			return "", err
		}
		max, err := b.serialize(v.Max)
		if err != nil {
			return "", err
		}

		if v.Inclusive {
			return fmt.Sprintf("[%s, %s]", min, max), nil
		}
		return fmt.Sprintf("(%s, %s)", min, max), nil

	case expr.Column:
		if len(v) == 0 {
			return "", fmt.Errorf("column name is empty")
		}
		if strings.ContainsRune(string(v), '"') {
			return "", fmt.Errorf("column name contains a double quote: %q", v)
		}
		// Always escape column names with double quotes,
		// otherwise we need to know the reserved words
		// which might change in the future.
		return fmt.Sprintf(`"%s"`, string(v)), nil
	case string:
		// escape single quotes with double single quotes
		return fmt.Sprintf("'%s'", strings.ReplaceAll(v, "'", "''")), nil
	case bool:
		return b.dialect().SerializeBool(v), nil
	default:
		return fmt.Sprintf("%v", v), nil
	}
}

func (b Base) serializeParams(in any) (s string, params []any, err error) {
	if in == nil {
		return "", params, nil
	}

	switch v := in.(type) {
	case *expr.Expression:
		if v.Op == expr.Regexp {
			s, _ := v.Left.(string)
			if len(s) >= 2 && s[0] == '/' && s[len(s)-1] == '/' {
				s = s[1 : len(s)-1]
			}
			return "?", []any{s}, nil
		}
		return b.RenderParam(v)
	case []*expr.Expression:
		strs := []string{}
		for _, e := range v {
			s, eparams, err := b.RenderParam(e)
			if err != nil {
				return s, params, err
			}
			strs = append(strs, s)
			params = append(params, eparams...)
		}
		return strings.Join(strs, ", "), params, nil
	case *expr.RangeBoundary:
		min, minParams, err := b.serializeParams(v.Min)
		if err != nil {
			return "", params, err
		}
		max, maxParams, err := b.serializeParams(v.Max)
		if err != nil {
			return "", params, err
		}
		params = append(minParams, maxParams...)

		if v.Inclusive {
			return fmt.Sprintf("[%s, %s]", min, max), params, nil
		}
		return fmt.Sprintf("(%s, %s)", min, max), params, nil

	case expr.Column:
		if len(v) == 0 {
			return "", params, fmt.Errorf("column name is empty")
		}
		if strings.ContainsRune(string(v), '"') {
			return "", params, fmt.Errorf("column name contains a double quote: %q", v)
		}
		// Always escape column names with double quotes,
		// otherwise we need to know the reserved words
		// which might change in the future.
		return fmt.Sprintf(`"%s"`, string(v)), params, nil
	case string:
		// if we have a '*' then we don't want to insert a param since
		// it can be used either in a regexp or a range operator.
		if v == "*" {
			return "'*'", params, nil
		}

		// escape single quotes with double single quotes
		return "?", []any{v}, nil
	default:
		return "?", []any{v}, nil
	}
}
