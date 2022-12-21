package expr

import (
	"encoding/json"
	"fmt"
)

func shouldWrap(e Expression) bool {
	switch e.(type) {
	case *Equals, *Literal, *WildLiteral, *RegexpLiteral, *Range, *Must, *MustNot:
		return true
	default:
		return false
	}
}

// Equals indicates that the term string (aka the column name) should have a value equal to an expression
type Equals struct {
	Term  string     `json:"term"`
	Value Expression `json:"-"`
}

func (eq Equals) String() string {
	return fmt.Sprintf("%v:%v", eq.Term, eq.Value)
}

// UnmarshalJSON ...
func (eq *Equals) UnmarshalJSON(data []byte) error {
	type eqAlias Equals
	tmp := &struct {
		*eqAlias
		Value any `json:"value"`
	}{
		eqAlias: (*eqAlias)(eq),
	}

	if err := json.Unmarshal(data, tmp); err != nil {
		return err
	}

	// TODO handle parsing in a WildLiteral or RegexpLiteral here
	eq.Value = Lit(tmp.Value)
	return nil
}

// MarshalJSON ...
func (eq *Equals) MarshalJSON() ([]byte, error) {
	type eqAlias Equals
	return json.Marshal(&struct {
		*eqAlias
		Operator string `json:"operator"`
		Value    any    `json:"value"`
	}{
		eqAlias:  (*eqAlias)(eq),
		Operator: "EQUALS",
		Value:    eq.Value.String(),
	})
}

// Literal ...
type Literal struct {
	Value any `json:"value"`
}

func (l Literal) String() string {
	return fmt.Sprintf("%v", l.Value)
}

// WildLiteral indicates the literal has regex values in it and should be matched as a loose wildcard
type WildLiteral struct{ Literal }

// RegexpLiteral indicates the literal has regex values in it and should be matched as a regex
type RegexpLiteral struct{ Literal }
