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

// Added grammar to be compatible with elastic lucene
// See https://www.elastic.co/guide/en/elasticsearch/reference/8.9/query-dsl-query-string-query.html#query-string-syntax
//      E:>E
//      E:>=E
//      E:<E
//      E:<=E

// Expression is an interface over all the different types of expressions
// that we can parse out of lucene
type Expression struct {
	Left  any      `json:"left"`
	Op    Operator `json:"-"`
	Right any      `json:"right,omitempty"`

	// these are operator specific states we have to track
	boostPower    float64
	fuzzyDistance int
}

// RangeBoundary represents the boundary conditions for a range operator
type RangeBoundary struct {
	Min       any  `json:"min"`
	Max       any  `json:"max"`
	Inclusive bool `json:"inclusive"`
}

func (e Expression) String() string {
	if e.Op == Undefined {
		return ""
	}
	renderer, found := renderers[e.Op]
	if !found {
		return "ERROR: unable to render string for unsupported operator"
	}
	return renderer(&e, false)
}

// GoString prints a verbose string representation. Useful for debugging exactly
// what types were parsed. You can print this format using %#v
func (e Expression) GoString() string {
	if e.Op == Undefined {
		return ""
	}
	renderer, found := renderers[e.Op]
	if !found {
		return "ERROR: unable to render gostring for unsupported operator"
	}
	return renderer(&e, true)
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

func GREATER(a any, b any) *Expression {
	return Expr(a, Greater, b)
}

func LESS(a any, b any) *Expression {
	return Expr(a, Less, b)
}

func GREATEREQ(a any, b any) *Expression {
	return Expr(a, GreaterEq, b)
}

func LESSEQ(a any, b any) *Expression {
	return Expr(a, LessEq, b)
}

// LIKE creates a new fuzzy matching LIKE expression
func LIKE(a any, b any) *Expression {
	return Expr(a, Like, b)
}

func IN(a any, b any) *Expression {
	return Expr(a, In, b)
}

func LIST(a ...any) *Expression {
	return Expr(a, List)
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
func Rang(term any, min, max any, inclusive bool) *Expression {
	return Expr(term, Range, min, max, inclusive)
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

// Column represents a column in sql. It will not be escaped by quotes in the sql rendering
type Column string

// GoString is a debug print for the column type
func (c Column) GoString() string {
	return fmt.Sprintf("COLUMN(%s)", c)
}

// Expr creates a general new expression. The other public functions are just helpers that call this
// function underneath.
func Expr(left any, op Operator, right ...any) *Expression {
	if isStringlike(left) && operatesOnColumn(op) {
		left = wrapInColumn(left)
	}

	if isLiteral(left) && op != Literal && op != Wild && op != Regexp {
		left = literalToExpr(left)
	}

	e := ptr(empty())
	e.Left = left
	e.Op = op

	// support using a like operator with wildcards or regex
	if op == Equals && len(right) == 1 && shouldUseLikeOperator(right[0]) {
		e.Op = Like
		e.Right = right[0].(*Expression)
		return e
	}

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
	if op == Range && len(right) == 3 && isBool(right[2]) {
		e.Right = &RangeBoundary{
			Min:       literalToExpr(right[0]),
			Max:       literalToExpr(right[1]),
			Inclusive: right[2].(bool),
		}
		return e
	}

	// support passing a slice to an IN operator
	if op == In && len(right) > 0 {
		e.Right = right[0].(*Expression)
		return e
	}

	if op == List {
		// super gross but this is how go handles any types that are slices
		slice, isSlice := left.([]any)[0].([]*Expression)
		if isSlice {
			e.Left = slice
			return e
		}

		l := left.([]any)
		vals := []*Expression{}
		for _, v := range l {
			vals = append(vals, v.(*Expression))
		}
		e.Left = vals
		return e
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

	RangeBoundary *RangeBoundary `json:"boundaries,omitempty"`
	FuzzyDistance *int           `json:"distance,omitempty"`
	BoostPower    *float64       `json:"power,omitempty"`
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

	return json.Marshal(c)
}

// UnmarshalJSON is a custom JSON deserialization for the Expression
func (e *Expression) UnmarshalJSON(data []byte) (err error) {
	// initalize our default values, e cannot be nil here.
	*e = empty()
	// if this does not look like an object it must be a literal
	if !isJSONObject(json.RawMessage(data)) {
		Expr, err := unmarshalLiteral(json.RawMessage(data))
		// this is required because apparently you can't swap pointers to your receiver mid method
		*e = *Expr
		return err
	}

	// unmarshal the current layer in the json first, then worry about
	// the left and right hand subobjects
	var c jsonExpression
	err = json.Unmarshal(data, &c)
	if err != nil {
		return err
	}

	// check if it is an array so we can parse it into literals
	if isArray(json.RawMessage(c.Left)) {
		var l []json.RawMessage
		err = json.Unmarshal(c.Left, &l)
		if err != nil {
			return err
		}

		exprs := []*Expression{}
		for _, v := range l {
			parsedExp, err := unmarshalLiteral(v)
			if err != nil {
				return err
			}
			exprs = append(exprs, parsedExp)
		}
		e.Left = exprs
	} else {
		e.Left = ptr(empty())
		err = json.Unmarshal(c.Left, e.Left)
		if err != nil {
			return err
		}
	}

	e.Op = fromString[c.Operator]

	// if the left hand side is a string then it must be a column
	if isStringlike(e.Left) && operatesOnColumn(e.Op) {
		e.Left = wrapInColumn(e.Left)
	}

	if len(c.Right) > 0 && looksLikeRangeBoundary(c.Right) {
		var boundary RangeBoundary
		err = json.Unmarshal(c.Right, &boundary)
		if err != nil {
			return err
		}
		if !IsExpr(boundary.Min) {
			boundary.Min = literalToExpr(toIntIfNecessary(boundary.Min))
		}

		if !IsExpr(boundary.Max) {
			boundary.Max = literalToExpr(toIntIfNecessary(boundary.Max))
		}
		e.Right = &boundary
	} else if len(c.Right) > 0 {
		e.Right = ptr(empty())
		err = json.Unmarshal(c.Right, e.Right)
		if err != nil {
			return err
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

func unmarshalLiteral(in json.RawMessage) (e *Expression, err error) {
	e = ptr(empty())

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

func isArray(in json.RawMessage) bool {
	trimmed := bytes.TrimSpace(in)
	if len(trimmed) == 0 {
		return false
	}

	return trimmed[0] == '[' && trimmed[len(trimmed)-1] == ']'
}

// looksLikeRangeBoundary checks whether the marshalled json has the keys for a range boundary.
// This is a hack but we need to know whether to unmarshal an expression or a range boundary.
func looksLikeRangeBoundary(in json.RawMessage) bool {
	// strip all the whitespace out of the input
	s := strings.Join(strings.Fields(string(in)), "")

	return strings.Contains(s, "\"min\":") &&
		strings.Contains(s, "\"max\":") &&
		!strings.Contains(s, "\"left\":")
}

func literalToExpr(in any) *Expression {
	if IsExpr(in) {
		return in.(*Expression)
	}

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

// isStringLike checks if the input is a string or is a literal wrapping a string
func isStringlike(in any) bool {
	_, isStr := in.(string)
	e, isExpr := in.(*Expression)
	if isExpr {
		_, isStrLiteralExpr := e.Left.(string)
		return isStrLiteralExpr
	}

	return isStr
}

// operatesOnColumn checks if an operator can be applied to a column (the left side of the operator).
// Example: equal can be applied onto a column (e.g. myColumn = 'foo') but Boost (^) cannot.
func operatesOnColumn(op Operator) bool {
	return op == Equals ||
		op == Range ||
		op == Greater ||
		op == Less ||
		op == GreaterEq ||
		op == LessEq ||
		op == In ||
		op == Like
}

// wrapInColumn converts a string to a column and enforces column
// invariants (e.g. if the column name contains a space then it must be quoted)
func wrapInColumn(in any) (out *Expression) {
	s, isStr := in.(string)
	if isStr {
		return Lit(Column(s))
	}

	e, isExpr := in.(*Expression)
	if isExpr {
		s, isStr = e.Left.(string)
		if isStr {
			return Lit(Column(s))
		}
	}
	return e
}

// apparently the json unmarshal only parses float64 values so we check if the float64
// is actually a whole number. If it is then make it an int
func toIntIfNecessary(in any) (out any) {
	f, isFloat := in.(float64)
	if !isFloat {
		return in
	}

	if f == float64(int(f)) {
		return int(f)
	}

	return f
}

func empty() Expression {
	return Expression{
		fuzzyDistance: 1,
		boostPower:    1.0,
	}
}

func ptr[T any](in T) *T {
	return &in
}

func shouldUseLikeOperator(in any) bool {
	expr, isExpr := in.(*Expression)
	if !isExpr {
		return false
	}
	return expr.Op == Wild || expr.Op == Regexp
}
