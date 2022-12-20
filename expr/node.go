package expr

import (
	"errors"
	"fmt"
	"reflect"
)

// And ...
type And struct {
	Left  Expression
	Right Expression
}

func (a And) String() string {
	leftStr := fmt.Sprintf("%v", a.Left)
	if !shouldWrap(a.Left) {
		leftStr = fmt.Sprintf("(%s)", leftStr)
	}

	rightStr := fmt.Sprintf("%v", a.Right)
	if !shouldWrap(a.Right) {
		rightStr = fmt.Sprintf("(%s)", rightStr)
	}
	return fmt.Sprintf("%v AND %v", leftStr, rightStr)
}

// Insert ...
func (a *And) Insert(e Expression) (Expression, error) {
	if a.Left == nil {
		a.Left = e
		return a, nil
	}

	if a.Right == nil {
		a.Right = e
		return a, nil
	}

	// if we are inserting a term into a full and then we are doing an implicit compound operation
	return &And{Left: a, Right: e}, nil
}

// Or ...
type Or struct {
	Left  Expression
	Right Expression
}

func (o Or) String() string {
	leftStr := fmt.Sprintf("%v", o.Left)
	if !shouldWrap(o.Left) {
		leftStr = fmt.Sprintf("(%s)", leftStr)
	}

	rightStr := fmt.Sprintf("%v", o.Right)
	if !shouldWrap(o.Right) {
		rightStr = fmt.Sprintf("(%s)", rightStr)
	}
	return fmt.Sprintf("%s OR %s", leftStr, rightStr)
}

// Insert ...
func (o *Or) Insert(e Expression) (Expression, error) {
	if o.Left == nil {
		o.Left = e
		return o, nil
	}

	if o.Right == nil {
		o.Right = e
		return o, nil
	}

	// if we are inserting a term into a full and then we are doing an implicit compound operation
	if o.Left != nil && o.Right != nil {
		return &And{Left: o, Right: e}, nil
	}

	return nil, errors.New("attempting to insert an expression into a full OR clause")
}

// Not ...
type Not struct {
	Sub Expression
}

func (n Not) String() string {
	return fmt.Sprintf("NOT %v", n.Sub)
}

// Insert ...
func (n *Not) Insert(e Expression) (Expression, error) {
	n.Sub = e
	return n, nil
}

// Range ...
type Range struct {
	Min       *Literal
	Max       *Literal
	Inclusive bool
}

func (r Range) String() string {
	if r.Inclusive {
		return fmt.Sprintf("[%s TO %s]", r.Min, r.Max)
	}
	return fmt.Sprintf("{%s TO %s}", r.Min, r.Max)

}

// Insert ...
func (r *Range) Insert(e Expression) (Expression, error) {
	if r.Min == nil {
		switch exp := e.(type) {
		case *Literal:
			r.Min = exp
			return r, nil
		case *WildLiteral:
			if exp.Value != "*" {
				return nil, fmt.Errorf("May only uses * as a wildcard in a range value, not [%s]", exp.Value)
			}
			r.Min = &Literal{exp.Value}
			return r, nil
		default:
			return nil, fmt.Errorf("unable to insert [%v] expression as max in a range", reflect.TypeOf(exp))
		}
	}

	// if we are inserting an expression into a full range query we must be trying to do a compound operation
	if r.Min != nil && r.Max != nil {
		return &And{Left: r, Right: e}, nil
	}

	switch exp := e.(type) {
	case *Literal:
		r.Max = exp
		return r, nil
	case *WildLiteral:
		if exp.Value != "*" {
			return nil, fmt.Errorf("May only uses * as a wildcard in a range value, not [%s]", exp.Value)
		}
		r.Max = &Literal{exp.Value}
		return r, nil
	default:
		return nil, fmt.Errorf("unable to insert [%v] expression as max in a range", reflect.TypeOf(exp))
	}
}

// Must ...
type Must struct {
	Sub Expression
}

func (m Must) String() string {
	return fmt.Sprintf("+%v", m.Sub)
}

// Insert ...
func (m *Must) Insert(e Expression) (Expression, error) {
	m.Sub = e
	return m, nil
}

// MustNot ...
type MustNot struct {
	Sub Expression
}

func (m MustNot) String() string {
	return fmt.Sprintf("-%v", m.Sub)
}

// Insert ...
func (m *MustNot) Insert(e Expression) (Expression, error) {
	m.Sub = e
	return m, nil
}

// Boost ...
type Boost struct {
	Sub   Expression
	Power float32
}

func (b Boost) String() string {
	return fmt.Sprintf("Boost(%s^%v)", b.Sub, b.Power)
}

// Insert ...
func (b *Boost) Insert(e Expression) (Expression, error) {
	// if we are inserting a value into a boost then we must be doing a compound operation
	return &And{Left: b, Right: e}, nil
}

// Fuzzy ...
type Fuzzy struct {
	Sub      Expression
	Distance int
}

func (b Fuzzy) String() string {
	if b.Distance == 1 {
		return fmt.Sprintf("Fuzzy(%s~)", b.Sub)
	}
	return fmt.Sprintf("Fuzzy(%s~%v)", b.Sub, b.Distance)
}

// Insert ...
func (b *Fuzzy) Insert(e Expression) (Expression, error) {
	// if we are inserting a value into a fuzzy then we must be doing a compound operation
	return &And{Left: b, Right: e}, nil
}
