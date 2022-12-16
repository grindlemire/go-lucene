package expr

import (
	"errors"
	"fmt"
	"reflect"
)

func shouldWrap(e Expression) bool {
	switch e.(type) {
	case *Equals, *Literal, *WildLiteral, *RegexpLiteral, *Range, *Must, *MustNot:
		return true
	default:
		return false
	}
}

// Equals ...
type Equals struct {
	Term  string
	Value Expression

	IsMust    bool
	IsMustNot bool
}

func (eq Equals) String() string {
	return fmt.Sprintf("%v:%v", eq.Term, eq.Value)
}

// Insert ...
func (eq *Equals) Insert(e Expression) (Expression, error) {
	literal, isLiteral := e.(*Literal)
	if eq.Term == "" && !isLiteral {
		return nil, errors.New("an equals expression must have a string as a term")
	}

	if eq.Term == "" && isLiteral {
		str, ok := literal.Value.(string)
		if !ok {
			return nil, fmt.Errorf("unable to insert non string [%v] into equals term", reflect.TypeOf(literal.Value))
		}

		eq.Term = str
		return eq, nil
	}

	// if we are inserting a term into an equals then we are in the implicit boolean case
	if eq.Term != "" && eq.Value != nil {
		return &And{Left: eq, Right: e}, nil
	}

	eq.Value = e
	// this is a hack but idk how to do it otherwise. The must and must nots must only
	// apply to the equals directly following them
	if eq.IsMust {
		eq.IsMust = false
		return &Must{Sub: eq}, nil
	}

	if eq.IsMustNot {
		eq.IsMustNot = false
		return &MustNot{Sub: eq}, nil
	}
	return eq, nil
}

// Literal ...
type Literal struct {
	Value any
}

func (l Literal) String() string {
	return fmt.Sprintf("%v", l.Value)
}

// Insert ...
func (l *Literal) Insert(e Expression) (Expression, error) {
	switch exp := e.(type) {
	case *Equals:
		return exp.Insert(l)
	// if we are inserting a term into a literal then we must be doing an implicit compound
	default:
		return &And{Left: l, Right: e}, nil
		// default:
		// 	return nil, fmt.Errorf("unable to insert [%v] into literal expression", reflect.TypeOf(e)))
	}
}

// WildLiteral indicates the literal has regex values in it and should be matched as a loose wildcard
type WildLiteral struct{ Literal }

// RegexpLiteral indicates the literal has regex values in it and should be matched as a regex
type RegexpLiteral struct{ Literal }
