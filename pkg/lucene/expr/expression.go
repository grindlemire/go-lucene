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
	Left  any       `json:"left"`
	Op    Operation `json:"-"`
	Right any       `json:"right,omitempty"`

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

// expr creates a general new expression
func expr(left any, op Operation, right ...any) *Expression {
	e := &Expression{
		Left: left,
		Op:   op,
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
	if op == Range && len(right) == 2 && isBool(right[1]) {
		e.rangeInclusive = right[1].(bool)
	}

	// if right is present and non nil then add it to the expression
	if len(right) >= 1 && right[0] != nil {
		e.Right = right[0]
	}

	return e
}

// MarshalJSON is a custom JSON serialization for the Expression
func (e Expression) MarshalJSON() (out []byte, err error) {
	// if we are in a leaf node just marshal the value
	if e.Op == Literal || e.Op == Wild || e.Op == Regexp {
		return json.Marshal(e.Left)
	}

	// if e.Op == Regexp {
	// 	strRight, ok := e.Right.(string)
	// 	if !ok {
	// 		return out, err
	// 	}

	// 	strRight = "/" + strRight + "/"
	// 	e.Right = strRight
	// }

	leftRaw, err := json.Marshal(e.Left)
	if err != nil {
		return out, err
	}

	rightRaw, err := json.Marshal(e.Right)
	if err != nil {
		return out, err
	}

	serializable := struct {
		Left      json.RawMessage `json:"left"`
		Operation string          `json:"operation"`
		Right     json.RawMessage `json:"right,omitempty"`
	}{
		Left:      leftRaw,
		Operation: toJSON[e.Op],
		Right:     rightRaw,
	}
	return json.Marshal(serializable)
}

// UnmarshalJSON is a custom JSON deserialization for the Expression
func (e *Expression) UnmarshalJSON(data []byte) (err error) {
	type serializable struct {
		Left      json.RawMessage `json:"left"`
		Operation string          `json:"operation"`
		Right     json.RawMessage `json:"right,omitempty"`
	}

	var c serializable
	err = json.Unmarshal(data, &c)
	if err != nil {
		return err
	}

	e.Left, err = unmarshalExpression(c.Left)
	if err != nil {
		return err
	}

	e.Right, err = unmarshalExpression(c.Right)
	if err != nil {
		return err
	}

	e.Op = fromJSON[c.Operation]
	return nil
}

// unmarshal different edge cases for literals in the expression
func unmarshalExpression(in json.RawMessage) (e *Expression, err error) {
	// if it looks like a sub object then parse it as an expression
	if isJSONObject(in) {
		e = &Expression{}
		err = json.Unmarshal(in, e)
		if err != nil {
			return e, err
		}
		return e, nil
	}

	// check if it is a float
	f, err := strconv.ParseFloat(string(in), 64)
	if err == nil {
		return Lit(f), nil
	}

	// check if it is an int
	i, err := strconv.Atoi(string(in))
	if err == nil {
		return Lit(i), nil
	}

	// we know it is some sort of string so decode it
	var s string
	err = json.Unmarshal(in, &s)
	if err != nil {
		return e, err
	}

	fmt.Printf("UNMARSHALLING STRING: %s\n", s)
	// if it has leading and trailing /'s then it probably is a regex.
	// Note this needs to be checked before the wildcard check as a regex
	// can contain * and ?.
	// TODO this should probably check for escaping
	if s[0] == '/' && s[len(s)-1] == '/' {
		return REGEXP(s), nil
	}

	// if it contains a * or ? then it probably is a wildcard expression
	// TODO this should probably check for escaping
	if strings.ContainsAny(s, "*?") {
		return WILD(s), nil
	}

	return Lit(s), nil
}

func isJSONObject(in json.RawMessage) bool {
	trimmed := bytes.TrimSpace(in)
	if len(trimmed) == 0 {
		return false
	}

	return trimmed[0] == '{' && trimmed[len(trimmed)-1] == '}'
}
