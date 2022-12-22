package expr

import (
	"errors"
	"fmt"
	"reflect"
)

type validator = func(*Expression) (err error)

var validators = map[Expr]validator{
	Equals:  validateEquals,
	And:     validateAnd,
	Or:      validateOr,
	Not:     validateNot,
	Range:   validateRange,
	Must:    validateMust,
	MustNot: validateMustNot,
	Boost:   validateBoost,
	Fuzzy:   validateFuzzy,
	Literal: validateLiteral,
	Wild:    validateWild,
	Regexp:  validateRegexp,
}

func validateEquals(e *Expression) (err error) {
	if e == nil {
		return nil
	}

	if e.Expr != Equals {
		return errors.New("EQUALS validation error: must have equals operator")
	}

	// _, ok := e.Left.(*Expression)
	// if !ok {
	// 	return fmt.Errorf("EQUALS validation: left value must be an expression not %s", reflect.TypeOf(e.Left))
	// }

	// if !isLiteral(e.Right) {
	// 	return fmt.Errorf("EQUALS validation: right value must be a literal not %s", reflect.TypeOf(e.Left))
	// }

	return nil
}

func validateAnd(e *Expression) (err error) {
	if e == nil {
		return nil
	}

	if e.Left == nil {
		return errors.New("AND validation: left value must not be nil")
	}

	if e.Right == nil {
		return errors.New("AND validation: right value must not be nil")
	}

	return nil
}

func validateOr(e *Expression) (err error) {
	if e == nil {
		return nil
	}

	if e.Left == nil {
		return errors.New("OR validation: left value must not be nil")
	}

	if e.Right == nil {
		return errors.New("OR validation: right value must not be nil")
	}

	return nil
}

func validateNot(e *Expression) (err error) {
	if e == nil {
		return nil
	}

	if e.Left == nil {
		return errors.New("NOT validation: sub expression must not be nil")
	}

	if e.Right != nil {
		return errors.New("NOT validation: must not have two sub expressions")
	}

	return nil
}

func validateRange(e *Expression) (err error) {
	if e == nil {
		return nil
	}

	if e.Left == nil {
		return errors.New("RANGE validation: min value must not be nil")
	}

	if e.Right == nil {
		return errors.New("AND validation: max value must not be nil")
	}

	return nil
}

func validateMust(e *Expression) (err error) {
	if e == nil {
		return nil
	}

	if e.Left == nil {
		return errors.New("MUST validation: sub expression must not be nil")
	}

	if e.Right != nil {
		return errors.New("MUST validation: must not have two sub expressions")
	}

	return nil
}

func validateMustNot(e *Expression) (err error) {
	if e == nil {
		return nil
	}

	if e.Left == nil {
		return errors.New("MUST_NOT validation: sub expression must not be nil")
	}

	if e.Right != nil {
		return errors.New("MUST_NOT validation: must not have two sub expressions")
	}

	return nil
}

func validateBoost(e *Expression) (err error) {
	if e == nil {
		return nil
	}

	if e.Left == nil {
		return errors.New("BOOST validation: sub expression must not be nil")
	}

	if e.Right != nil {
		return errors.New("BOOST validation: must not have two sub expressions")
	}

	return nil
}

func validateFuzzy(e *Expression) (err error) {
	if e == nil {
		return nil
	}

	if e.Left == nil {
		return errors.New("FUZZY validation: sub expression must not be nil")
	}

	if e.Right != nil {
		return errors.New("FUZZY validation: must not have two sub expressions")
	}

	return nil
}

func validateLiteral(e *Expression) (err error) {
	if e == nil {
		return nil
	}

	if e.Left == nil {
		return errors.New("LITERAL validation: value must not be nil")
	}

	if e.Right != nil {
		return errors.New("LITERAL validation: must not have two values")
	}

	if !isLiteral(e.Left) {
		return fmt.Errorf("LITERAL validation: value must be a literal, not %s", reflect.TypeOf(e.Left))
	}

	return nil
}

func validateWild(e *Expression) (err error) {
	if e == nil {
		return nil
	}

	if e.Left == nil {
		return errors.New("WILDCARD validation: value must not be nil")
	}

	if e.Right != nil {
		return errors.New("WILDCARD validation: must not have two values")
	}

	if !isLiteral(e.Left) {
		return fmt.Errorf("WILDCARD validation: value must be a literal, not %s", reflect.TypeOf(e.Left))
	}

	return nil
}

func validateRegexp(e *Expression) (err error) {
	if e == nil {
		return nil
	}

	if e.Left == nil {
		return errors.New("REGEXP validation: value must not be nil")
	}

	if e.Right != nil {
		return errors.New("REGEXP validation: must not have two values")
	}

	if !isLiteral(e.Left) {
		return fmt.Errorf("REGEXP validation: value must be a literal, not %s", reflect.TypeOf(e.Left))
	}

	return nil
}

func isLiteral(in any) bool {
	return isString(in) || isNum(in) || isBool(in)
}

func isString(in any) bool {
	_, is := in.(string)
	return is
}

func isNum(in any) bool {
	return isInt(in) || isFloat(in)
}

func isBool(in any) bool {
	_, is := in.(bool)
	return is
}

func isInt(in any) bool {
	switch in.(type) {
	case int, int32, int64, uint, uint8, uint16, uint32, uint64:
		return true
	default:
		return false
	}
}

func isFloat(in any) bool {
	switch in.(type) {
	case float32, float64:
		return true
	default:
		return false
	}
}
