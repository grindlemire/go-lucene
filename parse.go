package lucene

import (
	"fmt"
	"reflect"
	"strconv"
	"strings"

	"github.com/grindlemire/go-lucene/expr"
)

// Grammar:
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

type stringer interface {
	String() string
}

// Parse will parse using a buffer and the shift reduce algo
func Parse(input string) (e expr.Expression, err error) {
	p := &parser{
		lex:          lex(input),
		stack:        []stringer{},
		nonTerminals: []token{{typ: tSTART}},
	}
	ex, err := p.parse()
	if err != nil {
		return e, err
	}

	err = expr.Validate(ex)
	if err != nil {
		return e, err
	}

	return ex, nil
}

type parser struct {
	lex          *lexer
	stack        []stringer
	nonTerminals []token
}

func (p *parser) parse() (e expr.Expression, err error) {
	for {
		next := p.lex.peekNextToken()
		if p.shouldAccept(next) {
			if len(p.stack) != 1 {
				return nil, fmt.Errorf("multiple expression left after parsing: %v", p.stack)
			}
			final, ok := p.stack[0].(expr.Expression)
			if !ok {
				return nil, fmt.Errorf("final parse didn't return an expression: %s [type: %s]", p.stack[0], reflect.TypeOf(final))
			}
			return final, nil
		}

		if p.shouldShift(next) {
			tok := p.shift()
			if isTerminal(tok) {
				// if we have a terminal parse it and put it on the stack
				e, err := parseLiteral(tok)
				if err != nil {
					return e, err
				}

				// we should always check if the current top of the stack is another token
				// if it isn't then we have an implicit AND we need to inject.
				if len(p.stack) > 0 {
					_, isTopToken := p.stack[len(p.stack)-1].(token)
					if !isTopToken {
						implAnd := token{typ: tAND, val: "AND"}
						// act as if we just saw an AND and check if we need to reduce the
						// current token stack first.
						if !p.shouldShift(implAnd) {
							err = p.reduce()
							if err != nil {
								return e, err
							}
						}

						// if we have a literal as the previous parsed thing then
						// we must be in an implicit AND and should reduce
						p.stack = append(p.stack, implAnd)
						p.nonTerminals = append(p.nonTerminals, implAnd)
					}
				}

				p.stack = append(p.stack, e)
				continue
			}
			// otherwise just push the token on the stack
			p.stack = append(p.stack, tok)
			p.nonTerminals = append(p.nonTerminals, tok)
			continue
		}

		err = p.reduce()
		if err != nil {
			return e, err
		}
	}
}

func (p *parser) shift() (tok token) {
	return p.lex.nextToken()
}

func (p *parser) shouldShift(next token) bool {
	if next.typ == tEOF {
		return false
	}

	if next.typ == tERR {
		return false
	}

	curr := p.nonTerminals[len(p.nonTerminals)-1]

	if isTerminal(next) {
		return true
	}

	// TODO see if we really need all this extra edge logic
	// if we have an open curly or the next one is we want to shift
	if curr.typ == tLSQUARE || next.typ == tLSQUARE || curr.typ == tLCURLY || next.typ == tLCURLY {
		return true
	}

	// if we are at the end of a range always shift
	if next.typ == tRSQUARE || next.typ == tRCURLY {
		return true
	}

	// if we have a parsed expression surrounded by parens we want to shift
	if curr.typ == tLPAREN && next.typ == tRPAREN {
		return true
	}

	// if the current or next is left paren we always want to shift
	if curr.typ == tLPAREN || next.typ == tLPAREN {
		return true
	}

	// if we are ever attempting to move past a subexpr we need to parse it.
	if curr.typ == tRPAREN || curr.typ == tRSQUARE || curr.typ == tRCURLY {
		return false
	}

	return hasLessPrecedance(curr, next)
}

func (p *parser) shouldAccept(next token) bool {
	return len(p.stack) == 1 &&
		next.typ == tEOF
}

func (p *parser) reduce() (err error) {
	top := []stringer{}
	for {
		if len(p.stack) == 0 {
			return fmt.Errorf("error parsing, no items left to reduce, current state: %v", top)
		}

		// pull the top off the stack
		s := p.stack[len(p.stack)-1]
		p.stack = p.stack[:len(p.stack)-1]

		// keep the original ordering when building up our subslice
		top = append([]stringer{s}, top...)

		// try to reduce with all our reducers
		var reduced bool
		top, p.nonTerminals, reduced = tryReduce(top, p.nonTerminals)

		// if we consumed some non terminals during the reduce it means we successfully reduced
		if reduced {
			// if we successfully reduced re-add it to the top of the stack and return
			p.stack = append(p.stack, top...)
			return nil
		}
	}
}

func tryReduce(elems []stringer, nonTerminals []token) ([]stringer, []token, bool) {
	for _, reducer := range reducers {
		elems, nonTerminals, reduced := reducer(elems, nonTerminals)
		if reduced {
			return elems, nonTerminals, true
		}
	}
	return elems, nonTerminals, false
}

func drop[T any](stack []T, i int) []T {
	return stack[:len(stack)-i]
}

func parseLiteral(token token) (e expr.Expression, err error) {
	if token.typ == tQUOTED {
		val := strings.ReplaceAll(token.val, "\"", "")
		return &expr.Literal{Value: val}, nil
	}

	if token.typ == tREGEXP {
		val := strings.ReplaceAll(token.val, "/", "")
		return &expr.RegexpLiteral{
			Literal: expr.Literal{Value: val},
		}, nil
	}

	val := token.val
	ival, err := strconv.Atoi(val)
	if err == nil {
		return &expr.Literal{Value: ival}, nil
	}

	if strings.ContainsAny(val, "*?") {
		return &expr.WildLiteral{Literal: expr.Literal{Value: val}}, nil
	}

	return &expr.Literal{Value: val}, nil

}

func toPositiveFloat(in string) (f float32, err error) {
	i, err := strconv.Atoi(in)
	if err == nil && i > 0 {
		return float32(i), nil
	}

	pf, err := strconv.ParseFloat(in, 64)
	if err == nil && pf > 0 {
		return float32(pf), nil
	}

	return f, fmt.Errorf("[%v] is not a positive number", in)
}
