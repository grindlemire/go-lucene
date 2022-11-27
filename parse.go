package lucene

import (
	"reflect"
	"strconv"
	"strings"

	"github.com/pkg/errors"
)

// k1:[1 TO 5] AND (k2:fo? OR k3:"foo bar") -k5:(boo ba bi*)
// OR {
// 	AND {
// 		RANGE_INCLUSIVE {
// 			k1,
// 			1, 5
// 		},
// 		OR {
// 			WILDCARD {
// 				k2
// 				"fo?"
// 			}
// 			EQUAL {
// 				k3
// 				"foo bar"
// 			}
// 		}
// 	}
// 	NOT {
// 		WILDCARD {
// 			k5
// 			[ba*]
// 		}
// 	}
// }

// Expression ...
type Expression interface {
	// String() string
	// Render() (string, error)
	insert(e Expression) (Expression, error)
}

// Equals ...
type Equals struct {
	term  string
	value Expression

	isMust    bool
	isMustNot bool
}

func (eq *Equals) insert(e Expression) (Expression, error) {
	literal, isLiteral := e.(*Literal)
	if eq.term == "" && !isLiteral {
		return nil, errors.New("an equals expression must have a string as a term")
	}

	if eq.term == "" && isLiteral {
		str, ok := literal.val.(string)
		if !ok {
			return nil, errors.Errorf("unable to insert non string [%v] into equals term", reflect.TypeOf(literal.val))
		}

		eq.term = str
		return eq, nil
	}

	eq.value = e
	// this is a hack but idk how to do it otherwise. The must and must nots must only
	// apply to the equals directly following them
	if eq.isMust {
		eq.isMust = false
		return &Must{expr: eq}, nil
	}

	if eq.isMustNot {
		eq.isMustNot = false
		return &MustNot{expr: eq}, nil
	}
	return eq, nil
}

// RangeInclusive ...
type RangeInclusive struct {
	term  string
	start any
	end   any
}

// And ...
type And struct {
	left  Expression
	right Expression
}

func (a *And) insert(e Expression) (Expression, error) {
	if a.left == nil {
		a.left = e
		return a, nil
	}

	if a.right == nil {
		a.right = e
		return a, nil
	}

	return nil, errors.New("attempting to insert an expression into a full AND clause")
}

// Or ...
type Or struct {
	left  Expression
	right Expression
}

func (o *Or) insert(e Expression) (Expression, error) {
	if o.left == nil {
		o.left = e
		return o, nil
	}

	if o.right == nil {
		o.right = e
		return o, nil
	}

	return nil, errors.New("attempting to insert an expression into a full OR clause")
}

// Not ...
type Not struct {
	expr Expression
}

func (n *Not) insert(e Expression) (Expression, error) {
	n.expr = e
	return n, nil
}

// Literal ...
type Literal struct {
	val any
}

func (l *Literal) insert(e Expression) (Expression, error) {
	switch exp := e.(type) {
	case *Equals:
		return exp.insert(l)
	default:
		return nil, errors.Errorf("unable to update expression with literal and [%v]", reflect.TypeOf(e))
	}
}

// WildLiteral indicates the literal has regex values in it and should be matched as a regex
type WildLiteral struct{ Literal }

// Range ...
type Range struct {
	Min       Expression
	Max       Expression
	Inclusive bool
}

func (r *Range) insert(e Expression) (Expression, error) {
	if r.Min == nil {
		return nil, errors.New("should not be able to have a TO expression without a minimum")
	}

	switch exp := e.(type) {
	case *Literal, *WildLiteral:
		r.Max = exp
		return r, nil
	default:
		return nil, errors.Errorf("unable to insert [%v] expression as max in a range", reflect.TypeOf(exp))
	}
}

// Must ...
type Must struct {
	expr Expression
}

func (m *Must) insert(e Expression) (Expression, error) {
	m.expr = e
	return m, nil
}

// MustNot ...
type MustNot struct {
	expr Expression
}

func (m *MustNot) insert(e Expression) (Expression, error) {
	m.expr = e
	return m, nil
}

type parser struct {
	// keep an internal representation of tokens in case we need to backtrack
	tokIdx int
	tokens []token
	lex    *lexer

	hasMust    bool
	hasMustNot bool
}

func (p *parser) next() (t token) {
	if p.tokIdx < len(p.tokens) {
		t = p.tokens[p.tokIdx]
		p.tokIdx++
		return t
	}

	// if we have parsed all existing tokens get another
	t = p.lex.nextToken()
	p.tokens = append(p.tokens, t)
	p.tokIdx++
	return t

}

func (p *parser) backup() {
	p.tokIdx--
}

func (p *parser) peek() (t token) {
	// if we have parsed all existing tokens get another but don't increment the pointer
	if p.tokIdx == len(p.tokens)-1 {
		t = p.lex.nextToken()
		p.tokens = append(p.tokens, t)
		return t
	}

	// just return what is at the current pointer
	return p.tokens[p.tokIdx]
}

func (p *parser) parse() (e Expression, err error) {
	for {
		token := p.next()
		if token.typ == tEOF {
			return e, err
		}

		switch token.typ {
		case tERR:
			return e, errors.Errorf(token.val)
		case tEOF:
			return e, nil

		// literal value:
		// 		- we parse the literal to a real type rather than a string representation
		// 		  and then transition the expression state based on seeing a literal.
		case tLITERAL:
			p.backup()
			literal, err := p.parseLiteral()
			if err != nil {
				return e, errors.Wrap(err, "unable to parse literal")
			}
			if e == nil {
				e = literal
				break // breaks out of the switch, not the for loop
			}
			e, err = e.insert(literal)
			if err != nil {
				return e, errors.Wrap(err, "unable to insert literal into expression")
			}

		// quoted value:
		// 		- we make this quoted value a literal string and ignore keywords and whitespace
		case tQUOTED:
			literal := &Literal{
				val: token.val,
			}

			if e == nil {
				e = literal
				break // breaks out of the switch, not the for loop
			}

			e, err = e.insert(literal)
			if err != nil {
				return e, errors.Wrap(err, "unable to insert quoted string into expression")
			}

		// equal operator:
		//		- if we see an equal we enforce that we have literals and transition the
		// 		  the expression state to handle the equal.
		case tEQUAL, tCOLON:
			if e == nil {
				return e, errors.New("invalid syntax: can't start expression with '='")
			}

			// this is a hack but idk how to do it otherwise. The must and must nots must only
			// apply to the equals directly following them
			e, err = e.insert(&Equals{isMust: p.hasMust, isMustNot: p.hasMustNot})
			if err != nil {
				return e, errors.Wrap(err, "error updating expression with equals token")
			}
			p.hasMust = false
			p.hasMustNot = false

		// not operator
		// 		- if we see a not then parse the following expression and wrap it with not
		case tNOT:
			sub, err := p.parse()
			if err != nil {
				return e, err
			}
			not := &Not{
				expr: sub,
			}

			if e == nil {
				e = not
				break
			}

			e.insert(not)
		// boolean operators:
		//		- these just wrap the existing terms
		case tAND:
			and := &And{
				left: e,
			}
			right, err := p.parse()
			if err != nil {
				return e, errors.Wrap(err, "unable to build AND clause")
			}
			and.right = right
			e = and
		case tOR:
			or := &Or{
				left: e,
			}
			right, err := p.parse()
			if err != nil {
				return e, errors.Wrap(err, "unable to build AND clause")
			}
			or.right = right
			e = or

		// subexpressions
		// 		- if you see a left paren then recursively parse the expression.
		// 		- if you see a right paren we must be done with the current recursion
		case tLPAREN:
			sub, err := p.parse()
			if err != nil {
				return e, errors.Wrap(err, "unable to parse sub-expression")
			}

			if e != nil {
				e, err = e.insert(sub)
				if err != nil {
					return e, err
				}
				break
			}

			e = sub
		case tRPAREN:
			return e, nil

		// range operators
		//		- if you see a left square/curly bracket then parse the sub expression that has to be a range
		// 		- then insert it into the existing expression (should only be for the equals expression)
		case tLSQUARE:
			if e == nil {
				return e, errors.New("unable to insert range into empty expression")
			}
			sub, err := p.parse()
			if err != nil {
				return e, errors.Wrap(err, "unable to parse inclusive range")
			}
			// we are inclusive so update that here
			r, ok := sub.(*Range)
			if !ok {
				return e, errors.New("brackets must surround a range query (hint: use the TO operator in the brackets)")
			}
			r.Inclusive = true
			e, err = e.insert(r)
			if err != nil {
				return e, err
			}
		case tLCURLY:
			if e == nil {
				return e, errors.New("unable to insert range into empty expression")
			}
			sub, err := p.parse()
			if err != nil {
				return e, errors.Wrap(err, "unable to parse inclusive range")
			}
			// we are inclusive so update that here
			r, ok := sub.(*Range)
			if !ok {
				return e, errors.New("brackets must surround a range query (hint: use the TO operator in the brackets)")
			}
			r.Inclusive = false
			e, err = e.insert(r)
			if err != nil {
				return e, err
			}
		case tTO:
			switch e.(type) {
			case *Literal, *WildLiteral:
				// do nothing
			default:
				return nil, errors.New("the TO keyword must follow a literal expression")
			}

			r := &Range{
				Min: e,
			}
			e = r
		case tRSQUARE, tRCURLY:
			return e, nil

		// must and must not operators
		// 		- if we see a plus or minus then we need to apply it to the next term only
		case tPLUS:
			p.hasMust = true
		case tMINUS:
			p.hasMustNot = true

			// TODO:
			// potentially handle >, < for niceness
			// term boosting
			// fuzzy matching
		}

	}
}

func (p *parser) parseBoolean(e Expression) (Expression, error) {
	// assume e is expression that will be put into an and clause
	and := &And{
		left: e,
	}

	for {
		token := p.next()
		switch token.typ {
		case tERR:
			return nil, errors.Errorf(token.val)
		case tEOF:
			return nil, errors.New("unterminitated boolean expression")

		case tLITERAL:
			and.right = &Literal{token.val}
			return and, nil

		default:
			return nil, errors.New("unable to insert a sub expression in a boolean")
		}
	}
}

func (p *parser) parseLiteral() (e Expression, err error) {
	val := p.next().val
	ival, err := strconv.Atoi(val)
	if err == nil {
		return &Literal{val: ival}, nil
	}

	if strings.ContainsAny(val, "*?") {
		return &WildLiteral{Literal{val: val}}, nil
	}

	return &Literal{val: val}, nil

}

// Parse will parse the lucene grammar out of a string
func Parse(input string) (e Expression, err error) {
	p := parser{
		lex: lex(input),
	}
	return p.parse()
}
