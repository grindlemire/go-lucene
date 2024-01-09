package expr

import (
	"errors"
	"fmt"
	"reflect"
)

type validator = func(*Expression) (err error)

var validators = map[Operator]validator{
	Equals:    validateEquals,
	And:       validateAnd,
	Or:        validateOr,
	Not:       validateNot,
	Range:     validateRange,
	Must:      validateMust,
	MustNot:   validateMustNot,
	Boost:     validateBoost,
	Fuzzy:     validateFuzzy,
	Literal:   validateLiteral,
	Wild:      validateWild,
	Regexp:    validateRegexp,
	Greater:   validateCompare,
	Less:      validateCompare,
	GreaterEq: validateCompare,
	LessEq:    validateCompare,
	Like:      validateLike,
	In:        validateIn,
	List:      validateList,
}

func validateEquals(e *Expression) (err error) {
	if e == nil {
		return nil
	}

	if e.Op != Equals {
		return errors.New("EQUALS validation error: must have equals operator")
	}

	if !isLiteralExpr(e.Left) {
		return errors.New("EQUALS validation: left value must be a literal expression")
	}

	return nil
}

func validateCompare(e *Expression) (err error) {
	if e == nil {
		return nil
	}

	if e.Op != Greater && e.Op != Less && e.Op != GreaterEq && e.Op != LessEq {
		return errors.New("COMPARE validation error: must have comparison operator operator")
	}

	if !isLiteralExpr(e.Left) {
		return errors.New("COMPARE validation: left value must be a literal expression")
	}

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
		return errors.New("RANGE validation: term value must not be nil")
	}

	if e.Right == nil {
		return errors.New("RANGE validation: boundary value must not be nil")
	}

	if !isLiteralExpr(e.Left) {
		return errors.New("RANGE validation: term value must be a literal")
	}

	boundary, isBoundary := e.Right.(*RangeBoundary)
	if !isBoundary {
		return fmt.Errorf("RANGE validation: invalid range boundary - incorrect type [%s]", reflect.TypeOf(e.Right))
	}

	if boundary == nil {
		return errors.New("RANGE validation: range boundary must not be nil")
	}

	if boundary.Min == nil {
		return errors.New("RANGE validation: range boundary must have a minimum")
	}

	if boundary.Max == nil {
		return errors.New("RANGE validation: range boundary must have a maximum")
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

func validateLike(e *Expression) (err error) {
	if e == nil {
		return nil
	}

	if e.Left == nil {
		return errors.New("LIKE validation: column must not be nil")
	}

	if !isLiteralExpr(e.Left) {
		return fmt.Errorf("LIKE validation: value must be a literal, not %s", reflect.TypeOf(e.Left))
	}

	if e.Right == nil {
		return errors.New("LIKE validation: must have two values")
	}

	right, ok := e.Right.(*Expression)
	if !ok {
		return fmt.Errorf("LIKE validation: right side must be an expression, not %s", reflect.TypeOf(e.Right))
	}

	if right.Op != Wild && right.Op != Regexp {
		return fmt.Errorf("LIKE validation: right side must be a wildcard or regexp, not %s", right.Op)
	}

	return nil
}

func validateIn(e *Expression) (err error) {
	if e == nil {
		return nil
	}

	if e.Left == nil {
		return errors.New("IN validation: column must not be nil")
	}

	if !isLiteralExpr(e.Left) {
		return fmt.Errorf("IN validation: value must be a literal, not %s", reflect.TypeOf(e.Left))
	}

	if e.Right == nil {
		return errors.New("IN validation: must have two values")
	}

	right, ok := e.Right.(*Expression)
	if !ok {
		return fmt.Errorf("IN validation: right side must be an expression, not %s", reflect.TypeOf(e.Right))
	}

	if right.Op != List {
		return fmt.Errorf("IN validation: right side must be a list, not %s", right.Op)
	}

	return nil
}

func validateList(e *Expression) (err error) {
	if e == nil {
		return nil
	}

	if e.Left == nil {
		return errors.New("LIST validation: value must not be nil")
	}

	if e.Right != nil {
		return errors.New("LIST validation: must not have two values")
	}

	if !isListOfLiteralExprs(e.Left) {
		return fmt.Errorf("LIST validation: value must be a list of literals, not %s", reflect.TypeOf(e.Left))
	}

	return nil
}

func isListOfLiteralExprs(in any) bool {
	e, isList := in.([]*Expression)
	if !isList {
		return false
	}
	for _, v := range e {
		if !isLiteralExpr(v) {
			return false
		}
	}
	return true
}

func isLiteralExpr(in any) bool {
	e, isExpr := in.(*Expression)
	return isExpr && (e.Op == Literal || e.Op == Wild || e.Op == Regexp) && isLiteral(e.Left)
}

func isLiteral(in any) bool {
	return isString(in) || isNum(in) || isBool(in) || isColumn(in)
}

func isColumn(in any) bool {
	_, is := in.(Column)
	return is
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
