package driver

import (
	"fmt"

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
	expr.Fuzzy:   noop,
	expr.Boost:   noop,
	expr.Wild:    noop, // wildcard expressions can render as literal strings
	expr.Regexp:  noop, // regexp expressions can render as literal strings
	expr.Like:    like,
}

// Base is the base driver that is embedded in each driver
type Base struct {
	renderFNs map[expr.Operator]RenderFN
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

	fn, ok := b.renderFNs[e.Op]
	if !ok {
		return s, fmt.Errorf("unable to render operator [%s] - please file an issue for this", e.Op)
	}

	return fn(left, right)
}

func (b Base) serialize(in any) (s string, err error) {
	if in == nil {
		return "", nil
	}

	switch v := in.(type) {
	case *expr.Expression:
		return b.Render(v)
	case *expr.RangeBoundary:
		if v.Inclusive {
			return fmt.Sprintf("[%s, %s]", v.Min, v.Max), nil
		}
		return fmt.Sprintf("(%s, %s)", v.Min, v.Max), nil

	case expr.Column:
		return fmt.Sprintf("%s", v), nil
	case string:
		return fmt.Sprintf("'%s'", v), nil
	default:
		return fmt.Sprintf("%v", v), nil
	}
}
