package driver

import (
	"fmt"

	"github.com/grindlemire/go-lucene/pkg/lucene/expr"
)

type renderFN func(left, right string) (string, error)

func literal(left, right string) (string, error) {
	return left, nil
}

func equals(left, right string) (string, error) {
	return fmt.Sprintf("%s = %s", left, right), nil
}

var shared = map[expr.Operator]renderFN{
	expr.Literal: literal,
	expr.Equals:  equals,
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
	default:
		return fmt.Sprintf("%v", v), nil
	}
}
