package driver

import (
	"fmt"
	"strings"

	"github.com/grindlemire/go-lucene/pkg/lucene/expr"
)

type renderFN func(left, right string) (string, error)

func literal(left, right string) (string, error) {
	return left, nil
}

func equals(left, right string) (string, error) {
	// at this point the left is considered a column so we should treat it as such
	// and remove the quotes
	left = strings.ReplaceAll(left, "\"", "")
	return fmt.Sprintf("%s = %s", left, right), nil
}

func noop(left, right string) (string, error) {
	return left, nil
}

func like(left, right string) (string, error) {
	// at this point the left is considered a column so we should treat it as such
	// and remove the quotes
	left = strings.ReplaceAll(left, "\"", "")
	return fmt.Sprintf("%s LIKE %s", left, right), nil
}

func rang(left, right string) (string, error) {
	stripped := strings.Replace(strings.Replace(right, "(", "", 1), ")", "", 1)
	rangeSlice := strings.Split(stripped, ",")

	if len(rangeSlice) != 2 {
		return "", fmt.Errorf("the BETWEEN operator needs a two item list in the right hand side, have %s", right)
	}

	return fmt.Sprintf("%s BETWEEN %s AND %s",
			left,
			strings.Trim(rangeSlice[0], " "),
			strings.Trim(rangeSlice[1], " "),
		),
		nil
}

func basicCompound(op expr.Operator) renderFN {
	return func(left, right string) (string, error) {
		return fmt.Sprintf("(%s) %s (%s)", left, op, right), nil
	}
}

func basicWrap(op expr.Operator) renderFN {
	return func(left, right string) (string, error) {
		return fmt.Sprintf("%s(%s)", op, left), nil
	}
}

var shared = map[expr.Operator]renderFN{
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
	expr.Wild:    noop, // this gets handled by the equals
	expr.Regexp:  noop, // this gets handled by the equals
	expr.Like:    like,
}

type base struct {
	renderFNs map[expr.Operator]renderFN
}

// Render will render the expression based on the renderFNs provided by the driver.
func (b base) Render(e *expr.Expression) (s string, err error) {
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

func (b base) serialize(in any) (s string, err error) {
	if in == nil {
		return "", nil
	}

	switch v := in.(type) {
	case *expr.Expression:
		return b.Render(v)
	case *expr.RangeBoundary:
		return fmt.Sprintf("(%s, %s)", v.Min, v.Max), nil
	case expr.Column:
		return fmt.Sprintf("%s", v), nil
	case string:
		return fmt.Sprintf("\"%s\"", v), nil
	default:
		return fmt.Sprintf("%v", v), nil
	}
}
