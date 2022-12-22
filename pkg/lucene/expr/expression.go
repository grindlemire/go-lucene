package expr

import (
	"encoding/json"
	"fmt"
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
	return renderers[e.Op](&e)
}

// // MarshalJSON ...
// func (e *Expression) MarshalJSON() ([]byte, error) {
// 	type Alias Expression
// 	return json.Marshal(&struct {
// 		*Alias
// 		Operation string `json:"operation"`
// 	}{
// 		Alias:     (*Alias)(e),
// 		Operation: toJSON[e.Op],
// 	})
// }

// // UnmarshalJSON ...
// func (e *Expression) UnmarshalJSON(data []byte) error {
// 	type Alias Expression
// 	aux := &struct {
// 		*Alias
// 		Operation string `json:"operation"`
// 	}{
// 		Alias: (*Alias)(e),
// 	}

// 	if err := json.Unmarshal(data, aux); err != nil {
// 		return err
// 	}

// 	e.Op = fromJSON[aux.Operation]
// 	return nil
// }

// MarshalJSON is a custom JSON serialization for the Expression
func (e Expression) MarshalJSON() (out []byte, err error) {
	// if we are in a leaf node just marshal the value
	if e.Op == Literal || e.Op == Wild || e.Op == Regexp {
		return json.Marshal(e.Left)
	}

	type custom struct {
		Left      json.RawMessage `json:"left"`
		Operation string          `json:"operation"`
		Right     json.RawMessage `json:"right,omitempty"`
	}

	leftRaw, err := json.Marshal(e.Left)
	if err != nil {
		return out, err
	}

	rightRaw, err := json.Marshal(e.Right)
	if err != nil {
		return out, err
	}

	c := custom{
		Left:      leftRaw,
		Operation: toJSON[e.Op],
		Right:     rightRaw,
	}
	return json.Marshal(c)
}

// UnmarshalJSON is a custom JSON deserialization for the Expression
func (e *Expression) UnmarshalJSON(data []byte) (err error) {
	fmt.Printf("INCOMING: %s\n", data)
	if !strings.Contains(string(data), "{") {
		e = Lit(string(data))
		fmt.Printf("LITERAL E NOW: %s\n", e)
		return nil
	}

	type custom struct {
		Left      json.RawMessage `json:"left"`
		Operation string          `json:"operation"`
		Right     json.RawMessage `json:"right,omitempty"`
	}

	var c custom
	err = json.Unmarshal(data, &c)
	if err != nil {
		return err
	}

	var leftExpr Expression
	err = json.Unmarshal(c.Left, &leftExpr)
	if err != nil {
		return err
	}
	fmt.Printf("LEFT IS [%+v] | PARSED FROM: [%s]\n", leftExpr, c.Left)

	var rightExpr Expression
	err = json.Unmarshal(c.Right, &rightExpr)
	if err != nil {
		return err
	}

	e.Left = leftExpr
	e.Op = fromJSON[c.Operation]
	e.Right = rightExpr
	fmt.Printf("PARSED AT THIS LEVEL: %s\n", e)
	return nil
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
