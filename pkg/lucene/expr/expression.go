package expr

import (
	"fmt"
)

// Lucene Grammar:
// E ->
// 		E:E
// 		(E)
// 		+E
// 		-E
// 		E~E
// 		E^E
// 		NOT E
//      E AND E
// 		E OR E
// 		id
// 		[id TO id]

// Expression is an interface over all the different types of expressions
// that we can parse out of lucene
type Expression struct {
	Left  any  `json:"left"`
	Expr  Expr `json:"op"`
	Right any  `json:"right"`

	// these are operator specific states we have to track
	rangeInclusive bool    `json:"-"`
	boostPower     float64 `json:"-"`
	fuzzyDistance  int     `json:"-"`
}

func (e Expression) String() string {
	return renderers[e.Expr](&e)
}

// expr creates a general new expression
func expr(left any, op Expr, right ...any) *Expression {
	e := &Expression{
		Left: left,
		Expr: op,
	}
	// if right is present and non nil then add it to the expression
	if len(right) == 1 && right[0] != nil {
		e.Right = right[0]
	}

	// support passing a range with inclusivity
	if op == Range && len(right) == 2 && isBool(right[1]) {
		e.rangeInclusive = right[1].(bool)
	}

	// support changing boost power
	if op == Boost {
		e.boostPower = 1.0
		if len(right) == 1 && isFloat(right[0]) {
			e.boostPower = right[0].(float64)
		}
	}

	// support changing fuzzy distance
	if op == Fuzzy {
		e.fuzzyDistance = 1
		if len(right) == 1 && isInt(right[0]) {
			e.fuzzyDistance = right[0].(int)
		}
	}
	return e
}

// Lit represents a literal expression
func Lit(in any) *Expression {
	return expr(in, Literal)
}

// WILD represents a literal wildcard expression
func WILD(in any) *Expression {
	return expr(in, Wild)
}

// REGEXP represents a literal regular expression
func REGEXP(in any) *Expression {
	return expr(in, Regexp)
}

// Eq creates a new EQUALS expression
func Eq(a any, b any) *Expression {
	return expr(a, Equals, b)
}

// AND creates an AND expression
func AND(a, b any) *Expression {
	return expr(a, And, b)
}

// OR creates a new OR expression
func OR(a, b any) *Expression {
	return expr(a, Or, b)
}

// Rang creates a new range expression
func Rang(min, max any, inclusive bool) *Expression {
	return expr(min, Range, max, inclusive)
}

// NOT wraps an expression in a Not
func NOT(e any) *Expression {
	return expr(e, Not)
}

// MUST wraps an expression in a Must
func MUST(e any) *Expression {
	return expr(e, Must)
}

// MUSTNOT wraps an expression in a MustNot
func MUSTNOT(e any) *Expression {
	return expr(e, MustNot)
}

// BOOST wraps an expression in a boost
func BOOST(e any, power float64) *Expression {
	return expr(e, Boost, power)
}

// FUZZY wraps an expression in a fuzzy
func FUZZY(e any, distance int) *Expression {
	return expr(e, Fuzzy, distance)
}

// Validate validates the expression is correctly structured.
func Validate(in any) (err error) {
	e, isExpr := in.(*Expression)
	if !isExpr {
		// if we don't have an expression we must be in a leaf node
		return nil
	}

	fn, found := validators[e.Expr]
	if !found {
		return fmt.Errorf("unsupported operator %v", e.Expr)
	}
	err = fn(e)
	if err != nil {
		return err
	}

	err = Validate(e.Left)
	if err != nil {
		return err
	}

	return Validate(e.Right)
}
