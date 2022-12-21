package expr

import (
	"fmt"
)

// And represents an AND expression that connects two sub expressions.
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

// Or represents an OR expression that connects two sub expressions.
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

// Not represents a NOT expression that negates its sub expression.
type Not struct {
	Sub Expression
}

func (n Not) String() string {
	return fmt.Sprintf("NOT %v", n.Sub)
}

// Range represents a range operator in lucene that spans a minimum to a maximum value and an
// inclusivity.
type Range struct {
	Min       Expression
	Max       Expression
	Inclusive bool
}

func (r Range) String() string {
	if r.Inclusive {
		return fmt.Sprintf("[%s TO %s]", r.Min, r.Max)
	}
	return fmt.Sprintf("{%s TO %s}", r.Min, r.Max)
}

// Must represents a lucene MUST operator that indicates an expression must be present.
type Must struct {
	Sub Expression
}

func (m Must) String() string {
	return fmt.Sprintf("+%v", m.Sub)
}

// MustNot represents a lucene MUST_NOT operator that indicates an expression must not be present.
type MustNot struct {
	Sub Expression
}

func (m MustNot) String() string {
	return fmt.Sprintf("-%v", m.Sub)
}

// Boost represents a boost operator that gives more weight to the sub expression within.
type Boost struct {
	Sub   Expression
	Power float32
}

func (b Boost) String() string {
	return fmt.Sprintf("Boost(%s^%v)", b.Sub, b.Power)
}

// Fuzzy represents a fuzzy operator that matches terms similar to the sub expression.
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
