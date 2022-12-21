package expr

import (
	"errors"
	"fmt"
	"reflect"
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
type Expression interface {
	String() string
	// Insert(e Expression) (Expression, error)
}

// Eq creates a new equals expression
func Eq(a Expression, b Expression) Expression {
	return &Equals{
		Term:  a.(*Literal).Value.(string),
		Value: b,
	}
}

// AND creates an AND expression
func AND(a, b Expression) Expression {
	return &And{
		Left:  a,
		Right: b,
	}
}

// OR creates a new OR expression
func OR(a, b Expression) Expression {
	return &Or{
		Left:  a,
		Right: b,
	}
}

// Lit creates a new literal
func Lit(val any) Expression {
	return &Literal{
		Value: val,
	}
}

// Wild creates a new literal that contains wildcards
func Wild(val any) Expression {
	return &WildLiteral{
		Literal: Literal{
			Value: val,
		},
	}
}

// Rang creates a new range expression
func Rang(min, max Expression, inclusive bool) Expression {
	lmin, ok := min.(*Literal)
	if !ok {
		wmin, ok := min.(*WildLiteral)
		if !ok {
			panic("must only pass a *Literal or *WildLiteral to the Rang function")
		}
		lmin = &Literal{Value: wmin.Value}
	}

	lmax, ok := max.(*Literal)
	if !ok {
		wmax, ok := max.(*WildLiteral)
		if !ok {
			panic("must only pass a *Literal or *WildLiteral to the Rang function")
		}
		lmax = &Literal{Value: wmax.Value}
	}
	return &Range{
		Inclusive: inclusive,
		Min:       lmin,
		Max:       lmax,
	}
}

// NOT wraps an expression in a Not
func NOT(e Expression) Expression {
	return &Not{
		Sub: e,
	}
}

// MUST wraps an expression in a Must
func MUST(e Expression) Expression {
	return &Must{
		Sub: e,
	}
}

// MUSTNOT wraps an expression in a MustNot
func MUSTNOT(e Expression) Expression {
	return &MustNot{
		Sub: e,
	}
}

// BOOST wraps an expression in a boost
func BOOST(e Expression, power float32) Expression {
	return &Boost{
		Sub:   e,
		Power: power,
	}
}

// FUZZY wraps an expression in a fuzzy
func FUZZY(e Expression, distance int) Expression {
	return &Fuzzy{
		Sub:      e,
		Distance: distance,
	}
}

// REGEXP creates a new regular expression literal
func REGEXP(val any) Expression {
	return &RegexpLiteral{
		Literal: Literal{Value: val},
	}
}

// Validate validates the expression is correctly structured.
func Validate(ex Expression) (err error) {
	switch e := ex.(type) {
	case *Equals:
		if e.Term == "" || e.Value == nil {
			return errors.New("EQUALS operator must have both sides of the expression")
		}
		return Validate(e.Value)
	case *And:
		if e.Left == nil || e.Right == nil {
			return errors.New("AND clause must have two sides")
		}
		err = Validate(e.Left)
		if err != nil {
			return err
		}
		err = Validate(e.Right)
		if err != nil {
			return err
		}
	case *Or:
		if e.Left == nil || e.Right == nil {
			return errors.New("OR clause must have two sides")
		}
		err = Validate(e.Left)
		if err != nil {
			return err
		}
		err = Validate(e.Right)
		if err != nil {
			return err
		}
	case *Not:
		if e.Sub == nil {
			return errors.New("NOT expression must have a sub expression to negate")
		}
		return Validate(e.Sub)
	case *Literal:
		// do nothing
	case *WildLiteral:
		// do nothing
	case *RegexpLiteral:
		// do nothing
	case *Range:
		if e.Min == nil || e.Max == nil {
			return errors.New("range clause must have a min and a max")
		}
		err = Validate(e.Min)
		if err != nil {
			return err
		}
		err = Validate(e.Max)
		if err != nil {
			return err
		}
	case *Must:
		if e.Sub == nil {
			return errors.New("MUST expression must have a sub expression")
		}
		_, isMustNot := e.Sub.(*MustNot)
		_, isMust := e.Sub.(*Must)
		if isMust || isMustNot {
			return errors.New("MUST cannot be repeated with itself or MUST NOT")
		}
		return Validate(e.Sub)
	case *MustNot:
		if e.Sub == nil {
			return errors.New("MUST NOT expression must have a sub expression")
		}
		_, isMustNot := e.Sub.(*MustNot)
		_, isMust := e.Sub.(*Must)
		if isMust || isMustNot {
			return errors.New("MUST NOT cannot be repeated with itself or MUST")
		}
		return Validate(e.Sub)
	case *Boost:
		if e.Sub == nil {
			return errors.New("BOOST expression must have a subexpression")
		}
		return Validate(e.Sub)
	case *Fuzzy:
		if e.Sub == nil {
			return errors.New("FUZZY expression must have a subexpression")
		}
		return Validate(e.Sub)
	default:
		return fmt.Errorf("unable to validate Expression type: %s", reflect.TypeOf(e))
	}

	return nil

}
