package expr

import (
	"errors"
	"fmt"
	"reflect"
)

// Expression is an interface over all the different types of expressions
// that we can parse out of lucene
type Expression interface {
	Insert(e Expression) (Expression, error)
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
