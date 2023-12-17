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
	expr.Range:   rang,
	expr.Must:    noop,                // must doesn't really translate to sql
	expr.MustNot: basicWrap(expr.Not), // must not is really just a negation
	// expr.Fuzzy:     unsupported,
	// expr.Boost:     unsupported,
	expr.Wild:      literal,
	expr.Regexp:    literal,
	expr.Like:      like,
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
}

// Render will render the expression based on the renderFNs provided by the driver.
func (b Base) Render(e *expr.Expression) (s string, err error) {
	if e == nil {
		return "", nil
	}

	left, err := b.serialize(e.Left)
	if err != nil {
		return s, err
	}

	right, err := b.serialize(e.Right)
	if err != nil {
		return s, err
	}

	if e.Op != expr.Range && e.Op != expr.Not && e.Op != expr.List && e.Op != expr.In && e.Op != expr.Literal && e.Op != expr.Must && e.Op != expr.MustNot {
		if !b.isSimple(e.Left) {
			left = "(" + left + ")"
		}
		if !b.isSimple(e.Right) {
			right = "(" + right + ")"
		}
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
	default:
		return fmt.Sprintf("%v", v), nil
	}
}
