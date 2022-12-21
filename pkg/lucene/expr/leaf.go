package expr

import (
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
	Term  string
	Value Expression

	IsMust    bool
	IsMustNot bool
}

func (eq Equals) String() string {
	return fmt.Sprintf("%v:%v", eq.Term, eq.Value)
}

// Literal ...
type Literal struct {
	Value any
}

func (l Literal) String() string {
	return fmt.Sprintf("%v", l.Value)
}

// WildLiteral indicates the literal has regex values in it and should be matched as a loose wildcard
type WildLiteral struct{ Literal }

// RegexpLiteral indicates the literal has regex values in it and should be matched as a regex
type RegexpLiteral struct{ Literal }
