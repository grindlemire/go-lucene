package expr

import (
	"errors"
	"fmt"
	"reflect"
	"slices"
	"strings"
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
	Null:      validateNull,
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

	if isNil(e.Right) {
		return errors.New("EQUALS validation: right value must not be nil")
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

	if isNil(e.Right) {
		return errors.New("COMPARE validation: right value must not be nil")
	}

	// A comparison takes one term, as in a:>5. Neither Lucene nor
	// Elasticsearch gives a meaning to a:>(x OR y) or a:>[1 TO 5].
	if right, ok := e.Right.(*Expression); ok && !isLiteralExpr(right) {
		return fmt.Errorf("COMPARE validation: the value in a %s comparison must be a single term, not %s %s expression", e.Op, article(right.Op), right.Op)
	}

	return nil
}

func validateAnd(e *Expression) (err error) {
	if e == nil {
		return nil
	}

	if isNil(e.Left) {
		return errors.New("AND validation: left value must not be nil")
	}

	if isNil(e.Right) {
		return errors.New("AND validation: right value must not be nil")
	}

	return nil
}

func validateOr(e *Expression) (err error) {
	if e == nil {
		return nil
	}

	if isNil(e.Left) {
		return errors.New("OR validation: left value must not be nil")
	}

	if isNil(e.Right) {
		return errors.New("OR validation: right value must not be nil")
	}

	return nil
}

func validateNot(e *Expression) (err error) {
	if e == nil {
		return nil
	}

	if isNil(e.Left) {
		return errors.New("NOT validation: sub expression must not be nil")
	}

	if !isNil(e.Right) {
		return errors.New("NOT validation: must not have two sub expressions")
	}

	return nil
}

func validateRange(e *Expression) (err error) {
	if e == nil {
		return nil
	}

	if isNil(e.Left) {
		return errors.New("RANGE validation: a range needs a field, as in field:[a TO b]")
	}

	if isNil(e.Right) {
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

	if isNil(boundary.Min) {
		return errors.New("RANGE validation: range boundary must have a minimum")
	}

	if isNil(boundary.Max) {
		return errors.New("RANGE validation: range boundary must have a maximum")
	}

	if err := validateRangeBound("minimum", boundary.Min); err != nil {
		return err
	}

	return validateRangeBound("maximum", boundary.Max)
}

// validateRangeBound checks that a range bound is a single term, using the
// same rules the driver applies when it renders the bound. A null expression
// is accepted so the renderer can return its dedicated error for it. A
// literal, wildcard, or regexp expression is checked by the value it holds (a
// bare * wildcard is an open bound, any other wildcard is literal text), and
// any other expression is rejected. The value itself may
// be any Go value that IsASTOrJSONContainer does not flag, so named numeric
// types and time.Time bounds are accepted for the parameterized renderers.
func validateRangeBound(side string, bound any) error {
	val := bound
	if e, ok := bound.(*Expression); ok && e != nil {
		switch e.Op {
		case Null:
			return nil
		case Literal, Wild, Regexp:
			val = e.Left
		default:
			// An unquoted space or colon inside a bound is what produces an
			// AND, OR or EQUALS here, so only those get the quoting hint.
			hint := ""
			if e.Op == And || e.Op == Or || e.Op == Equals {
				hint = "; quote a bound that contains spaces or ':'"
			}
			return fmt.Errorf("RANGE validation: range %s must be a single term, not %s %s expression%s", side, article(e.Op), e.Op, hint)
		}
	}

	if isNil(val) {
		return fmt.Errorf("RANGE validation: range boundary must have a %s", side)
	}
	if IsASTOrJSONContainer(val) {
		return fmt.Errorf("RANGE validation: range %s must be a single term, not %T", side, val)
	}
	return nil
}

// article returns "an" for an operator name that starts with a vowel and
// "a" otherwise.
func article(op Operator) string {
	if name := op.String(); name != "" && strings.ContainsRune("AEIOU", rune(name[0])) {
		return "an"
	}
	return "a"
}

func validateMust(e *Expression) (err error) {
	if e == nil {
		return nil
	}

	if isNil(e.Left) {
		return errors.New("MUST validation: sub expression must not be nil")
	}

	if !isNil(e.Right) {
		return errors.New("MUST validation: must not have two sub expressions")
	}

	return nil
}

func validateMustNot(e *Expression) (err error) {
	if e == nil {
		return nil
	}

	if isNil(e.Left) {
		return errors.New("MUST_NOT validation: sub expression must not be nil")
	}

	if !isNil(e.Right) {
		return errors.New("MUST_NOT validation: must not have two sub expressions")
	}

	return nil
}

func validateBoost(e *Expression) (err error) {
	if e == nil {
		return nil
	}

	if isNil(e.Left) {
		return errors.New("BOOST validation: sub expression must not be nil")
	}

	if !isNil(e.Right) {
		return errors.New("BOOST validation: must not have two sub expressions")
	}

	return nil
}

func validateFuzzy(e *Expression) (err error) {
	if e == nil {
		return nil
	}

	if isNil(e.Left) {
		return errors.New("FUZZY validation: sub expression must not be nil")
	}

	if !isNil(e.Right) {
		return errors.New("FUZZY validation: must not have two sub expressions")
	}

	return nil
}

func validateLiteral(e *Expression) (err error) {
	if e == nil {
		return nil
	}

	if isNil(e.Left) {
		return errors.New("LITERAL validation: value must not be nil")
	}

	if !isNil(e.Right) {
		return errors.New("LITERAL validation: must not have two values")
	}

	if !isLiteral(e.Left) {
		return fmt.Errorf("LITERAL validation: value must be a literal, not %s", reflect.TypeOf(e.Left))
	}

	if p, ok := e.Left.(JSONPath); ok {
		return p.Validate()
	}

	return nil
}

func validateWild(e *Expression) (err error) {
	if e == nil {
		return nil
	}

	if isNil(e.Left) {
		return errors.New("WILDCARD validation: value must not be nil")
	}

	if !isNil(e.Right) {
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

	if isNil(e.Left) {
		return errors.New("REGEXP validation: value must not be nil")
	}

	if !isNil(e.Right) {
		return errors.New("REGEXP validation: must not have two values")
	}

	if !isLiteral(e.Left) {
		return fmt.Errorf("REGEXP validation: value must be a literal, not %s", reflect.TypeOf(e.Left))
	}

	return nil
}

func validateNull(e *Expression) (err error) {
	if e == nil {
		return nil
	}
	if e.Left != nil {
		return errors.New("NULL validation: must not have a left value")
	}
	if !isNil(e.Right) {
		return errors.New("NULL validation: must not have a right value")
	}
	return nil
}

func validateLike(e *Expression) (err error) {
	if e == nil {
		return nil
	}

	if isNil(e.Left) {
		return errors.New("LIKE validation: column must not be nil")
	}

	if !isLiteralExpr(e.Left) {
		return fmt.Errorf("LIKE validation: value must be a literal, not %s", reflect.TypeOf(e.Left))
	}

	if isNil(e.Right) {
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

	if isNil(e.Left) {
		return errors.New("IN validation: column must not be nil")
	}

	if !isLiteralExpr(e.Left) {
		return fmt.Errorf("IN validation: value must be a literal, not %s", reflect.TypeOf(e.Left))
	}

	if isNil(e.Right) {
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

	if isNil(e.Left) {
		return errors.New("LIST validation: value must not be nil")
	}

	if !isNil(e.Right) {
		return errors.New("LIST validation: must not have two values")
	}

	if items, ok := e.Left.([]*Expression); ok && len(items) == 0 {
		return errors.New("LIST validation: must have at least one value")
	}

	if items, ok := e.Left.([]*Expression); ok && slices.Contains(items, nil) {
		return errors.New("LIST validation: item must not be nil")
	}

	if items, ok := e.Left.([]*Expression); ok {
		for _, item := range items {
			if !isLiteralExpr(item) {
				return fmt.Errorf("LIST validation: item must be a literal, not %#v", item)
			}
		}
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

// isNil reports whether in is nil or a nil *Expression or *RangeBoundary.
func isNil(in any) bool {
	switch v := in.(type) {
	case nil:
		return true
	case *Expression:
		return v == nil
	case *RangeBoundary:
		return v == nil
	default:
		return false
	}
}

func isLiteralExpr(in any) bool {
	e, isExpr := in.(*Expression)
	if !isExpr || e == nil {
		return false
	}
	if e.Op == Null {
		return true
	}
	return (e.Op == Literal || e.Op == Wild || e.Op == Regexp) && isLiteral(e.Left)
}

func isLiteral(in any) bool {
	return isString(in) || isNum(in) || isBool(in) || isColumn(in)
}

func isColumn(in any) bool {
	switch in.(type) {
	case Column, JSONPath:
		return true
	default:
		return false
	}
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
