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
		// if should_shift
		//     do_it
		// else reduce
		next := p.lex.peekNextToken()
		fmt.Printf("NEXT TOKEN: %s\n", next)
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

				if len(p.stack) > 0 {
					_, isTopLiteral := p.stack[len(p.stack)-1].(*expr.Literal)
					if isTopLiteral {
						// if we have a literal as the previous parsed thing then
						// we must be in an implicit AND and should reduce
						p.stack = push(p.stack, token{typ: tAND})
					}
				}

				fmt.Printf("PUSHING EXPR [%s] onto stack\n", e)
				p.stack = push(p.stack, e)
				continue
			}
			// otherwise just push the token on the stack
			fmt.Printf("PUSHING TOKEN [%s] onto stack\n", tok)
			p.stack = push(p.stack, tok)
			p.nonTerminals = append(p.nonTerminals, tok)
			continue
		}
		fmt.Printf("NOT SHIFTING FOR %s\n", next)
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

	fmt.Printf("CURR NON TERMINAL [%s] VAL: %d | NEXT [%s] VAL: %d | shouldshift? %v\n", curr, int(curr.typ), next, int(next.typ), hasLessPrecedance(curr, next))
	return hasLessPrecedance(curr, next)
}

func (p *parser) shouldAccept(next token) bool {
	return len(p.stack) == 1 &&
		next.typ == tEOF
}

func (p *parser) reduce() (err error) {
	// until_reduced
	//    peek on top of stack
	// 	  if can reduce
	//       do it
	//       return
	fmt.Printf("REDUCING: %v\n", p.stack)
	top := []stringer{}
	for {
		if len(p.stack) == 0 {
			return fmt.Errorf("error parsing, no items left to reduce, current state: %v", top)
		}
		// pull the top off the stack
		var s stringer
		s, p.stack = pop(p.stack)
		top = append([]stringer{s}, top...)

		// try to reduce with all our reducers
		var reduced bool
		top, p.nonTerminals, reduced = tryReduce(top, p.nonTerminals)

		// if we consumed some non terminals during the reduce it means we successfully reduced
		if reduced {
			// if we successfully reduced re-add it to the top of the stack and return
			p.stack = append(p.stack, top...)
			fmt.Printf("REDUCED SO NOW STACK IS: %s\n", p.stack)
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

type reducer func(elems []stringer, nonTerminals []token) ([]stringer, []token, bool)

var reducers = []reducer{
	and,
	or,
	equal,
	not,
	sub,
	must,
	mustNot,
	fuzzy,
	boost,
	rangeop,
}

func equal(elems []stringer, nonTerminals []token) ([]stringer, []token, bool) {
	if len(elems) != 3 {
		// fmt.Printf("NOT EQUAL - len not correct\n")
		return elems, nonTerminals, false
	}

	// ensure middle token is an equals
	tok, ok := elems[1].(token)
	if !ok || (tok.typ != tEQUAL && tok.typ != tCOLON) {
		// fmt.Printf("NOT EQUAL - not tEQUAL or tCOLON\n")
		return elems, nonTerminals, false
	}

	// make sure the left is a literal and right is an expression
	left, ok := elems[0].(*expr.Literal)
	if !ok {
		// fmt.Printf("NOT EQUAL - left not literal\n")
		return elems, nonTerminals, false
	}
	right, ok := elems[2].(expr.Expression)
	if !ok {
		// fmt.Printf("NOT EQUAL - right not expression\n")
		return elems, nonTerminals, false
	}

	elems = []stringer{
		expr.EQ(
			left,
			right,
		),
	}
	fmt.Printf("IS EQUAL\n")
	// we consumed one terminal, the =
	return elems, drop(nonTerminals, 1), true
}

func and(elems []stringer, nonTerminals []token) ([]stringer, []token, bool) {
	// // special case for when we have a default implicit AND
	// if len(elems) == 2 {
	// 	// make sure the left and right clauses are expressions
	// 	left, ok := elems[0].(expr.Expression)
	// 	if !ok {
	// 		// fmt.Printf("NOT AND - left not expr\n")
	// 		return elems, nonTerminals, false
	// 	}
	// 	right, ok := elems[1].(expr.Expression)
	// 	if !ok {
	// 		// fmt.Printf("NOT AND - right not expr\n")
	// 		return elems, nonTerminals, false
	// 	}

	// 	// we have a valid implicit AND clause. Replace it in the stack
	// 	elems = []stringer{
	// 		expr.AND(
	// 			left,
	// 			right,
	// 		),
	// 	}
	// 	fmt.Printf("IS AND\n")
	// 	// we add in the implicit AND terminal
	// 	return elems, append(nonTerminals, token{typ: tIMPLICIT_AND}), true
	// }

	// if we don't have 3 items in the buffer it's not an AND clause
	if len(elems) != 3 {
		// fmt.Printf("NOT AND - len not correct\n")
		return elems, nonTerminals, false
	}

	// if the middle token is not an AND token do nothing
	operatorToken, ok := elems[1].(token)
	if !ok || operatorToken.typ != tAND {
		// fmt.Printf("NOT AND - operator wrong\n")
		return elems, nonTerminals, false
	}

	// make sure the left and right clauses are expressions
	left, ok := elems[0].(expr.Expression)
	if !ok {
		// fmt.Printf("NOT AND - left not expr\n")
		return elems, nonTerminals, false
	}
	right, ok := elems[2].(expr.Expression)
	if !ok {
		// fmt.Printf("NOT AND - right not expr\n")
		return elems, nonTerminals, false
	}

	// we have a valid AND clause. Replace it in the stack
	elems = []stringer{
		expr.AND(
			left,
			right,
		),
	}
	fmt.Printf("IS AND\n")
	// we consumed one terminal, the AND
	return elems, drop(nonTerminals, 1), true
}

func or(elems []stringer, nonTerminals []token) ([]stringer, []token, bool) {
	// if we don't have 3 items in the buffer it's not an OR clause
	if len(elems) != 3 {
		// fmt.Printf("NOT OR - len not correct\n")
		return elems, nonTerminals, false
	}

	// if the middle token is not an OR token do nothing
	operatorToken, ok := elems[1].(token)
	if !ok || operatorToken.typ != tOR {
		// fmt.Printf("NOT OR - operator wrong\n")
		return elems, nonTerminals, false
	}

	// make sure the left and right clauses are expressions
	left, ok := elems[0].(expr.Expression)
	if !ok {
		// fmt.Printf("NOT OR - left not expr\n")
		return elems, nonTerminals, false
	}
	right, ok := elems[2].(expr.Expression)
	if !ok {
		// fmt.Printf("NOT OR - right not expr\n")
		return elems, nonTerminals, false
	}

	// we have a valid OR clause. Replace it in the stack
	elems = []stringer{
		expr.OR(
			left,
			right,
		),
	}
	fmt.Printf("IS OR\n")
	// we consumed one terminal, the OR
	return elems, drop(nonTerminals, 1), true
}

func not(elems []stringer, nonTerminals []token) ([]stringer, []token, bool) {
	if len(elems) < 2 {
		return elems, nonTerminals, false
	}

	// if the second to last token is not the NOT operator do nothing
	operatorToken, ok := elems[len(elems)-2].(token)
	if !ok || operatorToken.typ != tNOT {
		return elems, nonTerminals, false
	}

	// make sure the thing to be negated is already a parsed
	negated, ok := elems[len(elems)-1].(expr.Expression)
	if !ok {
		return elems, nonTerminals, false
	}

	elems = elems[:len(elems)-2]
	elems = push(elems, expr.NOT(negated))
	fmt.Printf("IS NOT\n")
	// we consumed one terminal, the NOT
	return elems, drop(nonTerminals, 1), true
}

func sub(elems []stringer, nonTerminals []token) ([]stringer, []token, bool) {
	// all the internal terms should have reduced by the time we hit this reducer
	if len(elems) != 3 {
		return elems, nonTerminals, false
	}

	open, ok := elems[0].(token)
	if !ok || open.typ != tLPAREN {
		return elems, nonTerminals, false
	}

	closed, ok := elems[len(elems)-1].(token)
	if !ok || closed.typ != tRPAREN {
		return elems, nonTerminals, false
	}

	fmt.Printf("IS SUB\n")
	// we consumed two terminals, the ( and )
	return []stringer{elems[1]}, drop(nonTerminals, 2), true
}

func must(elems []stringer, nonTerminals []token) ([]stringer, []token, bool) {
	if len(elems) != 2 {
		return elems, nonTerminals, false
	}

	must, ok := elems[0].(token)
	if !ok || must.typ != tPLUS {
		return elems, nonTerminals, false
	}

	rest, ok := elems[1].(expr.Expression)
	if !ok {
		return elems, nonTerminals, false
	}

	// we consumed 1 terminal, the +
	return []stringer{expr.MUST(rest)}, drop(nonTerminals, 1), true
}

func mustNot(elems []stringer, nonTerminals []token) ([]stringer, []token, bool) {
	if len(elems) != 2 {
		return elems, nonTerminals, false
	}

	must, ok := elems[0].(token)
	if !ok || must.typ != tMINUS {
		return elems, nonTerminals, false
	}

	rest, ok := elems[1].(expr.Expression)
	if !ok {
		return elems, nonTerminals, false
	}
	// we consumed one terminal, the -
	return []stringer{expr.MUSTNOT(rest)}, drop(nonTerminals, 1), true
}

func fuzzy(elems []stringer, nonTerminals []token) ([]stringer, []token, bool) {
	// we are in the case with an implicit 1 fuzzy distance
	if len(elems) == 2 {
		must, ok := elems[1].(token)
		if !ok || must.typ != tTILDE {
			return elems, nonTerminals, false
		}

		rest, ok := elems[0].(expr.Expression)
		if !ok {
			return elems, nonTerminals, false
		}

		// we consumed one terminal, the ~
		return []stringer{expr.FUZZY(rest, 1)}, drop(nonTerminals, 1), true
	}

	if len(elems) != 3 {
		return elems, nonTerminals, false
	}

	must, ok := elems[1].(token)
	if !ok || must.typ != tTILDE {
		return elems, nonTerminals, false
	}

	rest, ok := elems[0].(expr.Expression)
	if !ok {
		return elems, nonTerminals, false
	}

	power, ok := elems[2].(*expr.Literal)
	if !ok {
		return elems, nonTerminals, false
	}

	ipower, err := strconv.Atoi(power.String())
	if err != nil {
		return elems, nonTerminals, false
	}

	// we consumed one terminal, the ~
	return []stringer{expr.FUZZY(rest, ipower)}, drop(nonTerminals, 1), true
}

func boost(elems []stringer, nonTerminals []token) ([]stringer, []token, bool) {
	// we are in the case with an implicit 1 fuzzy distance
	if len(elems) == 2 {
		must, ok := elems[1].(token)
		if !ok || must.typ != tCARROT {
			return elems, nonTerminals, false
		}

		rest, ok := elems[0].(expr.Expression)
		if !ok {
			return elems, nonTerminals, false
		}

		// we consumed one terminal, the ^
		return []stringer{expr.BOOST(rest, 1.0)}, drop(nonTerminals, 1), true
	}

	if len(elems) != 3 {
		return elems, nonTerminals, false
	}

	must, ok := elems[1].(token)
	if !ok || must.typ != tCARROT {
		return elems, nonTerminals, false
	}

	rest, ok := elems[0].(expr.Expression)
	if !ok {
		return elems, nonTerminals, false
	}

	power, ok := elems[2].(*expr.Literal)
	if !ok {
		return elems, nonTerminals, false
	}

	fpower, err := toPositiveFloat(power.String())
	if err != nil {
		return elems, nonTerminals, false
	}

	// we consumed one terminal, the ^
	return []stringer{expr.BOOST(rest, fpower)}, drop(nonTerminals, 1), true
}

func rangeop(elems []stringer, nonTerminals []token) ([]stringer, []token, bool) {
	// we need a [, begin, TO, end, ] to have a range operator which is 5 elems
	if len(elems) != 5 {
		return elems, nonTerminals, false
	}

	open, ok := elems[0].(token)
	if !ok || (open.typ != tLSQUARE && open.typ != tLCURLY) {
		fmt.Printf("OPEN NOT RIGHT\n")
		return elems, nonTerminals, false
	}

	closed, ok := elems[4].(token)
	if !ok || (closed.typ != tRSQUARE && closed.typ != tRCURLY) {
		fmt.Printf("CLOSED NOT RIGHT\n")
		return elems, nonTerminals, false
	}
	fmt.Printf("ELEMS IN RANGE: %v\n", elems)

	to, ok := elems[2].(token)
	if !ok || to.typ != tTO {
		fmt.Printf("NOT TO: 00%s00\n", elems[2])
		return elems, nonTerminals, false
	}

	start, ok := elems[1].(expr.Expression)
	if !ok {
		fmt.Printf("NOT START\n")
		return elems, nonTerminals, false
	}

	end, ok := elems[3].(expr.Expression)
	if !ok {
		fmt.Printf("NOT END\n")
		return elems, nonTerminals, false
	}

	// we consumed three terminals, the [, TO, and ]
	return []stringer{expr.Rang(
		start, end, (open.typ == tLSQUARE && closed.typ == tRSQUARE),
	)}, drop(nonTerminals, 3), true

}

func push(stack []stringer, s stringer) []stringer {
	return append(stack, s)
}

func drop[T any](stack []T, i int) []T {
	return stack[:len(stack)-i]
}

func pop[T any](stack []T) (T, []T) {
	return stack[len(stack)-1], stack[:len(stack)-1]
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

func toPositiveInt(in string) (i int, err error) {
	i, err = strconv.Atoi(in)
	if err == nil && i > 0 {
		return i, nil
	}

	return i, fmt.Errorf("[%v] is not a positive number", in)
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
