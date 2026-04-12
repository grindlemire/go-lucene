package driver

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/grindlemire/go-lucene/pkg/lucene/expr"
)

// stripRegexpDelimiters removes surrounding /.../ delimiters from a Lucene
// regexp literal, returning the inner pattern.
func stripRegexpDelimiters(s string) string {
	if len(s) >= 2 && s[0] == '/' && s[len(s)-1] == '/' {
		return s[1 : len(s)-1]
	}
	return s
}

// Shared is the shared set of render functions that can be used as a base and overriden
// for each flavor of sql
var Shared = map[expr.Operator]RenderFN{
	expr.Literal:   literal,
	expr.And:       basicCompound(expr.And),
	expr.Or:        basicCompound(expr.Or),
	expr.Not:       basicWrap(expr.Not),
	expr.Equals:    equals,
	expr.Must:      noop,                // must doesn't really translate to sql
	expr.MustNot:   basicWrap(expr.Not), // must not is really just a negation
	expr.Wild:      literal,
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
		return "?", []any{stripRegexpDelimiters(s)}, nil
	}

	d := b.dialect()

	left, lparams, err := b.serializeParams(e.Left)
	if err != nil {
		return s, params, err
	}

	// Range: access typed boundary directly, skip serializing right side
	if e.Op == expr.Range {
		boundary, ok := e.Right.(*expr.RangeBoundary)
		if !ok {
			return "", nil, fmt.Errorf("range operator requires *expr.RangeBoundary, got %T", e.Right)
		}
		str, rangeParams, err := b.renderRangeParam(left, boundary)
		return str, append(lparams, rangeParams...), err
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

	if e.Op != expr.Not &&
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
		s = stripRegexpDelimiters(s)
		return fmt.Sprintf("'%s'", strings.ReplaceAll(s, "'", "''")), nil
	}

	d := b.dialect()

	left, err := b.serialize(e.Left)
	if err != nil {
		return s, err
	}

	// Range: access typed boundary directly, skip serializing right side
	if e.Op == expr.Range {
		boundary, ok := e.Right.(*expr.RangeBoundary)
		if !ok {
			return "", fmt.Errorf("range operator requires *expr.RangeBoundary, got %T", e.Right)
		}
		return b.renderRange(left, boundary)
	}

	right, err := b.serialize(e.Right)
	if err != nil {
		return s, err
	}

	// Standalone wildcard on a Like operator: `field:*`. Route through the
	// dialect so each database can decide how to represent "any value".
	if right == "'*'" && e.Op == expr.Like {
		return d.RenderStandaloneWild(left)
	}

	// Detect regex (Lucene /regex/) vs. wildcard and let the dialect escape
	// the wildcard pattern however it needs to. Positioned before paren-wrap
	// to stay symmetric with RenderParam.
	isRegex := false
	if e.Op == expr.Like {
		if rightExpr, ok := e.Right.(*expr.Expression); ok && rightExpr.Op == expr.Regexp {
			isRegex = true
		}
		if !isRegex && len(right) >= 2 && right[0] == '\'' && right[len(right)-1] == '\'' {
			inner := right[1 : len(right)-1]
			inner = d.EscapeLikePattern(inner)
			right = "'" + inner + "'"
		}
	}

	if e.Op != expr.Not &&
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
		return d.RenderLike(left, right, isRegex)
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
			s = stripRegexpDelimiters(s)
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
	case expr.Column:
		if len(v) == 0 {
			return "", fmt.Errorf("column name is empty")
		}
		return b.dialect().QuoteColumn(string(v))
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
			return "?", []any{stripRegexpDelimiters(s)}, nil
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
	case expr.Column:
		if len(v) == 0 {
			return "", params, fmt.Errorf("column name is empty")
		}
		quoted, err := b.dialect().QuoteColumn(string(v))
		if err != nil {
			return "", params, err
		}
		return quoted, params, nil
	case bool:
		return "?", []any{b.dialect().BoolParam(v)}, nil
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

// extractBoundValue unwraps a range boundary value from its Expression wrapper.
// Returns the raw Go value (int, float64, string) and whether the bound is unbounded (*).
func extractBoundValue(bound any) (val any, unbounded bool) {
	e, ok := bound.(*expr.Expression)
	if !ok {
		return bound, false
	}
	if e.Op == expr.Wild {
		return nil, true
	}
	return e.Left, false
}

// formatRangeValue renders a range bound value as a SQL literal.
func (b Base) formatRangeValue(val any) (string, error) {
	switch v := val.(type) {
	case int:
		return strconv.Itoa(v), nil
	case float64:
		return strconv.FormatFloat(v, 'f', -1, 64), nil
	case string:
		return fmt.Sprintf("'%s'", strings.ReplaceAll(v, "'", "''")), nil
	case expr.Column:
		return b.dialect().QuoteColumn(string(v))
	default:
		return fmt.Sprintf("%v", v), nil
	}
}

// isNumericBound checks whether a range bound value is numeric.
func isNumericBound(val any) bool {
	switch val.(type) {
	case int, float64:
		return true
	default:
		return false
	}
}

func (b Base) renderRange(left string, boundary *expr.RangeBoundary) (string, error) {
	minVal, minUnbounded := extractBoundValue(boundary.Min)
	maxVal, maxUnbounded := extractBoundValue(boundary.Max)
	inclusive := boundary.Inclusive

	if minUnbounded && maxUnbounded {
		return "1=1", nil
	}

	if minUnbounded {
		maxStr, err := b.formatRangeValue(maxVal)
		if err != nil {
			return "", err
		}
		if inclusive {
			return fmt.Sprintf("%s <= %s", left, maxStr), nil
		}
		return fmt.Sprintf("%s < %s", left, maxStr), nil
	}

	if maxUnbounded {
		minStr, err := b.formatRangeValue(minVal)
		if err != nil {
			return "", err
		}
		if inclusive {
			return fmt.Sprintf("%s >= %s", left, minStr), nil
		}
		return fmt.Sprintf("%s > %s", left, minStr), nil
	}

	minStr, err := b.formatRangeValue(minVal)
	if err != nil {
		return "", err
	}
	maxStr, err := b.formatRangeValue(maxVal)
	if err != nil {
		return "", err
	}

	if isNumericBound(minVal) || isNumericBound(maxVal) {
		if inclusive {
			return fmt.Sprintf("%s >= %s AND %s <= %s", left, minStr, left, maxStr), nil
		}
		return fmt.Sprintf("%s > %s AND %s < %s", left, minStr, left, maxStr), nil
	}

	if inclusive {
		return fmt.Sprintf("%s BETWEEN %s AND %s", left, minStr, maxStr), nil
	}
	return fmt.Sprintf("%s > %s AND %s < %s", left, minStr, left, maxStr), nil
}

func (b Base) renderRangeParam(left string, boundary *expr.RangeBoundary) (string, []any, error) {
	minVal, minUnbounded := extractBoundValue(boundary.Min)
	maxVal, maxUnbounded := extractBoundValue(boundary.Max)
	inclusive := boundary.Inclusive

	if minUnbounded && maxUnbounded {
		return "1=1", nil, nil
	}

	if minUnbounded {
		if inclusive {
			return fmt.Sprintf("%s <= ?", left), []any{maxVal}, nil
		}
		return fmt.Sprintf("%s < ?", left), []any{maxVal}, nil
	}

	if maxUnbounded {
		if inclusive {
			return fmt.Sprintf("%s >= ?", left), []any{minVal}, nil
		}
		return fmt.Sprintf("%s > ?", left), []any{minVal}, nil
	}

	if isNumericBound(minVal) || isNumericBound(maxVal) {
		if inclusive {
			return fmt.Sprintf("%s >= ? AND %s <= ?", left, left), []any{minVal, maxVal}, nil
		}
		return fmt.Sprintf("%s > ? AND %s < ?", left, left), []any{minVal, maxVal}, nil
	}

	if inclusive {
		return fmt.Sprintf("%s BETWEEN ? AND ?", left), []any{minVal, maxVal}, nil
	}
	return fmt.Sprintf("%s > ? AND %s < ?", left, left), []any{minVal, maxVal}, nil
}
