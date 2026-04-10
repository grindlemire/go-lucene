package driver

import (
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/grindlemire/go-lucene/pkg/lucene/expr"
)

// RenderFN is a rendering function. It takes the left and right side of the operator serialized to a string
// and serializes the entire expression
type RenderFN func(left, right string) (string, error)

func literal(left, right string) (string, error) {
	if !utf8.ValidString(left) {
		return "", fmt.Errorf("literal contains invalid utf8: %q", left)
	}
	if strings.ContainsRune(left, 0) {
		return "", fmt.Errorf("literal contains null byte: %q", left)
	}

	return left, nil
}

func equals(left, right string) (string, error) {
	return fmt.Sprintf("%s = %s", left, right), nil
}

func noop(left, right string) (string, error) {
	return left, nil
}

func inFn(left, right string) (string, error) {
	return fmt.Sprintf("%s IN %s", left, right), nil
}

func list(left, right string) (string, error) {
	return fmt.Sprintf("(%s)", left), nil
}

func greater(left, right string) (string, error) {
	return fmt.Sprintf("%s > %s", left, right), nil
}

func less(left, right string) (string, error) {
	return fmt.Sprintf("%s < %s", left, right), nil
}

func greaterEq(left, right string) (string, error) {
	return fmt.Sprintf("%s >= %s", left, right), nil
}

func lessEq(left, right string) (string, error) {
	return fmt.Sprintf("%s <= %s", left, right), nil
}

func basicCompound(op expr.Operator) RenderFN {
	return func(left, right string) (string, error) {
		return fmt.Sprintf("%s %s %s", left, op, right), nil
	}
}

func basicWrap(op expr.Operator) RenderFN {
	return func(left, right string) (string, error) {
		return fmt.Sprintf("%s(%s)", op, left), nil
	}
}
