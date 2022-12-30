package expr

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
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
	Left  any      `json:"left"`
	Op    Operator `json:"-"`
	Right any      `json:"right,omitempty"`

	// these are operator specific states we have to track
	rangeInclusive bool
	boostPower     float64
	fuzzyDistance  int
}

func (e Expression) String() string {
	if e.Op == Undefined {
		return ""
	}
	return renderers[e.Op](&e, false)
}

// GoString prints a verbose string representation. Useful for debugging exactly
// what types were parsed. You can print this format using %#v
func (e Expression) GoString() string {
	if e.Op == Undefined {
		return ""
	}
	return renderers[e.Op](&e, true)
}

// Lit represents a literal expression
func Lit(in any) *Expression {
	return Expr(in, Literal)
}

// WILD represents a literal wildcard expression
func WILD(in any) *Expression {
	return Expr(in, Wild)
}

// REGEXP represents a literal regular expression
func REGEXP(in any) *Expression {
	return Expr(in, Regexp)
}

// Eq creates a new EQUALS expression
func Eq(a any, b any) *Expression {
	return Expr(a, Equals, b)
}

// LIKE creates a new fuzzy matching LIKE expression
func LIKE(a any, b any) *Expression {
	return Expr(a, Like, b)
}

// AND creates an AND expression
func AND(a, b any) *Expression {
	return Expr(a, And, b)
}

// OR creates a new OR expression
func OR(a, b any) *Expression {
	return Expr(a, Or, b)
}

// Rang creates a new range expression
func Rang(min, max any, inclusive bool) *Expression {
	return Expr(min, Range, max, inclusive)
}

// NOT wraps an expression in a Not
func NOT(e any) *Expression {
	return Expr(e, Not)
}

// MUST wraps an expression in a Must
func MUST(e any) *Expression {
	return Expr(e, Must)
}

// MUSTNOT wraps an expression in a MustNot
func MUSTNOT(e any) *Expression {
	return Expr(e, MustNot)
}

// BOOST wraps an expression in a boost
func BOOST(e any, power ...float64) *Expression {
	if len(power) > 0 {
		return Expr(e, Boost, power[0])
	}
	return Expr(e, Boost)
}

// FUZZY wraps an expression in a fuzzy
func FUZZY(e any, distance ...int) *Expression {
	if len(distance) > 0 {
		return Expr(e, Fuzzy, distance[0])
	}
	return Expr(e, Fuzzy)
}

// IsExpr checks if the input is an expression
func IsExpr(in any) bool {
	_, isExpr := in.(*Expression)
	return isExpr
}

// Validate validates the expression is correctly structured.
func Validate(in any) (err error) {
	e, isExpr := in.(*Expression)
	if !isExpr {
		// if we don't have an expression we must be in a leaf node
		return nil
	}

	fn, found := validators[e.Op]
	if !found {
		return fmt.Errorf("unsupported operator %v", e.Op)
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

// Expr creates a general new expression. The other public functions are just helpers that call this
// function underneath.
func Expr(left any, op Operator, right ...any) *Expression {
	if isLiteral(left) && op != Literal && op != Wild && op != Regexp {
		left = literalToExpr(left)
	}

	e := ptr(empty())
	e.Left = left
	e.Op = op

	// support changing boost power
	if op == Boost {
		e.boostPower = 1.0
		if len(right) == 1 && isFloat(right[0]) {
			e.boostPower = right[0].(float64)
		}
		return e
	}

	// support changing fuzzy distance
	if op == Fuzzy {
		e.fuzzyDistance = 1
		if len(right) == 1 && isInt(right[0]) {
			e.fuzzyDistance = right[0].(int)
		}
		return e
	}

	// support passing a range with inclusivity
	if op == Range && len(right) == 2 && isBool(right[1]) {
		e.rangeInclusive = right[1].(bool)
	}

	// if right is present and non nil then add it to the expression
	if len(right) >= 1 && right[0] != nil {
		if isLiteral(right[0]) {
			right[0] = literalToExpr(right[0])
		}

		e.Right = right[0]
	}

	return e
}

type jsonExpression struct {
	Left     json.RawMessage `json:"left"`
	Operator string          `json:"operator"`
	Right    json.RawMessage `json:"right,omitempty"`

	RangeExclusive *bool    `json:"exclusive,omitempty"`
	FuzzyDistance  *int     `json:"distance,omitempty"`
	BoostPower     *float64 `json:"power,omitempty"`
}

// MarshalJSON is a custom JSON serialization for the Expression
func (e Expression) MarshalJSON() (out []byte, err error) {
	// if we are in a leaf node just marshal the value
	if e.Op == Literal || e.Op == Wild || e.Op == Regexp {
		return json.Marshal(e.Left)
	}

	leftRaw, err := json.Marshal(e.Left)
	if err != nil {
		return out, err
	}

	c := jsonExpression{
		Left:     leftRaw,
		Operator: toString[e.Op],
	}

	// this is dumb but we need it so our "null" is not event given. Otherwise the json serialization
	// will persist a null value.
	if e.Right != nil {
		rightRaw, err := json.Marshal(e.Right)
		if err != nil {
			return out, err
		}
		c.Right = rightRaw
	}

	if e.boostPower != 1.0 {
		c.BoostPower = &e.boostPower
	}

	if e.fuzzyDistance != 1 {
		c.FuzzyDistance = &e.fuzzyDistance
	}

	if !e.rangeInclusive {
		c.RangeExclusive = ptr(true)
	}

	return json.Marshal(c)
}

// UnmarshalJSON is a custom JSON deserialization for the Expression
func (e *Expression) UnmarshalJSON(data []byte) (err error) {
	// initalize our default values, e cannot be nil here.
	*e = empty()
	// if this does not look like an object it must be a literal
	if !isJSONObject(json.RawMessage(data)) {
		Expr, err := unmarshalExpression(json.RawMessage(data))
		// this is required because apparently you can't swap pointers to your receiver mid method
		*e = *Expr
		return err
	}

	var c jsonExpression
	err = json.Unmarshal(data, &c)
	if err != nil {
		return err
	}

	e.Left, err = unmarshalExpression(c.Left)
	if err != nil {
		return err
	}

	if len(c.Right) > 0 {
		e.Right, err = unmarshalExpression(c.Right)
		if err != nil {
			return err
		}
	}

	e.Op = fromString[c.Operator]

	if e.Op == Range {
		e.rangeInclusive = true
		// yes this can be reduced but this is more readble
		if c.RangeExclusive != nil && *c.RangeExclusive {
			e.rangeInclusive = false
		}
	}

	if e.Op == Fuzzy {
		e.fuzzyDistance = 1
		if c.FuzzyDistance != nil {
			e.fuzzyDistance = *c.FuzzyDistance
		}
	}

	if e.Op == Boost {
		e.boostPower = 1.0
		if c.BoostPower != nil {
			e.boostPower = *c.BoostPower
		}
	}

	return nil
}

// unmarshal different edge cases for literals in the expression
func unmarshalExpression(in json.RawMessage) (e *Expression, err error) {
	e = ptr(empty())

	// if it looks like a sub object then parse it as an expression
	if isJSONObject(in) {
		err = json.Unmarshal(in, e)
		if err != nil {
			return e, err
		}
		return e, nil
	}

	// check if it is an int first because all ints can be parsed as floats
	i, err := strconv.Atoi(string(in))
	if err == nil {
		return Lit(i), nil
	}

	// check if it is a float
	f, err := strconv.ParseFloat(string(in), 64)
	if err == nil {
		return Lit(f), nil
	}

	// we know it is some sort of string so decode it
	var s string
	err = json.Unmarshal(in, &s)
	if err != nil {
		return e, err
	}

	return literalToExpr(s), nil
}

func literalToExpr(in any) *Expression {
	s, isStr := in.(string)
	if !isStr {
		return Lit(in)
	}

	// if it has leading and trailing /'s then it probably is a regex.
	// Note this needs to be checked before the wildcard check as a regex
	// can contain * and ?.
	// TODO this should probably check for escaping
	if s[0] == '/' && s[len(s)-1] == '/' {
		return REGEXP(s)
	}

	// if it contains a * or ? then it probably is a wildcard expression
	// TODO this should probably check for escaping
	if strings.ContainsAny(s, "*?") {
		return WILD(s)
	}

	return Lit(s)
}

func isJSONObject(in json.RawMessage) bool {
	trimmed := bytes.TrimSpace(in)
	if len(trimmed) == 0 {
		return false
	}

	return trimmed[0] == '{' && trimmed[len(trimmed)-1] == '}'
}

func empty() Expression {
	return Expression{
		rangeInclusive: true,
		fuzzyDistance:  1,
		boostPower:     1.0,
	}
}

func ptr[T any](in T) *T {
	return &in
}
