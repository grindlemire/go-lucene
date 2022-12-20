package lucene

import (
	"fmt"
	"reflect"
	"strconv"

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

// BufParse will parse using a buffer and the shift reduce algo
func BufParse(input string) (e expr.Expression, err error) {
	p := &bufParser{
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

type bufParser struct {
	lex          *lexer
	stack        []stringer
	nonTerminals []token
}

func (p *bufParser) parse() (e expr.Expression, err error) {

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

func (p *bufParser) shift() (tok token) {
	return p.lex.nextToken()
}

func (p *bufParser) shouldShift(next token) bool {
	if next.typ == tEOF {
		return false
	}

	if next.typ == tERR {
		return false
	}

	if isTerminal(next) {
		return true
	}

	curr := p.nonTerminals[len(p.nonTerminals)-1]

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
	if curr.typ == tRPAREN {
		return false
	}

	fmt.Printf("CURR NON TERMINAL [%s] VAL: %d | NEXT [%s] VAL: %d | shouldshift? %v\n", curr, int(curr.typ), next, int(next.typ), hasLessPrecedance(curr, next))
	return hasLessPrecedance(curr, next)
}

func (p *bufParser) shouldAccept(next token) bool {
	return len(p.stack) == 1 &&
		next.typ == tEOF
}

func (p *bufParser) reduce() (err error) {
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
		top, reduced = tryReduce(top)
		if reduced {
			// if we successfully reduced re-add it to the top of the stack and return
			p.stack = append(p.stack, top...)
			_, p.nonTerminals = pop(p.nonTerminals)
			fmt.Printf("REDUCED SO NOW STACK IS: %s\n", p.stack)
			return nil
		}
	}
}

func tryReduce(elems []stringer) ([]stringer, bool) {
	for _, reducer := range reducers {
		elems, matched := reducer(elems)
		if matched {
			return elems, matched
		}
	}
	return elems, false
}

type reducer func(elems []stringer) ([]stringer, bool)

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

func equal(elems []stringer) ([]stringer, bool) {
	if len(elems) != 3 {
		// fmt.Printf("NOT EQUAL - len not correct\n")
		return elems, false
	}

	// ensure middle token is an equals
	tok, ok := elems[1].(token)
	if !ok || (tok.typ != tEQUAL && tok.typ != tCOLON) {
		// fmt.Printf("NOT EQUAL - not tEQUAL or tCOLON\n")
		return elems, false
	}

	// make sure the left is a literal and right is an expression
	left, ok := elems[0].(*expr.Literal)
	if !ok {
		// fmt.Printf("NOT EQUAL - left not literal\n")
		return elems, false
	}
	right, ok := elems[2].(expr.Expression)
	if !ok {
		// fmt.Printf("NOT EQUAL - right not expression\n")
		return elems, false
	}

	elems = []stringer{
		EQ(
			left,
			right,
		),
	}
	fmt.Printf("IS EQUAL\n")
	return elems, true
}

func and(elems []stringer) ([]stringer, bool) {
	// if we don't have 3 items in the buffer it's not an AND clause
	if len(elems) != 3 {
		// fmt.Printf("NOT AND - len not correct\n")
		return elems, false
	}

	// if the middle token is not an AND token do nothing
	operatorToken, ok := elems[1].(token)
	if !ok || operatorToken.typ != tAND {
		// fmt.Printf("NOT AND - operator wrong\n")
		return elems, false
	}

	// make sure the left and right clauses are expressions
	left, ok := elems[0].(expr.Expression)
	if !ok {
		// fmt.Printf("NOT AND - left not expr\n")
		return elems, false
	}
	right, ok := elems[2].(expr.Expression)
	if !ok {
		// fmt.Printf("NOT AND - right not expr\n")
		return elems, false
	}

	// we have a valid AND clause. Replace it in the stack
	elems = []stringer{
		AND(
			left,
			right,
		),
	}
	fmt.Printf("IS AND\n")
	return elems, true
}

func or(elems []stringer) ([]stringer, bool) {
	// if we don't have 3 items in the buffer it's not an OR clause
	if len(elems) != 3 {
		// fmt.Printf("NOT OR - len not correct\n")
		return elems, false
	}

	// if the middle token is not an OR token do nothing
	operatorToken, ok := elems[1].(token)
	if !ok || operatorToken.typ != tOR {
		// fmt.Printf("NOT OR - operator wrong\n")
		return elems, false
	}

	// make sure the left and right clauses are expressions
	left, ok := elems[0].(expr.Expression)
	if !ok {
		// fmt.Printf("NOT OR - left not expr\n")
		return elems, false
	}
	right, ok := elems[2].(expr.Expression)
	if !ok {
		// fmt.Printf("NOT OR - right not expr\n")
		return elems, false
	}

	// we have a valid OR clause. Replace it in the stack
	elems = []stringer{
		OR(
			left,
			right,
		),
	}
	fmt.Printf("IS OR\n")
	return elems, true
}

func not(elems []stringer) ([]stringer, bool) {
	if len(elems) < 2 {
		return elems, false
	}

	// if the second to last token is not the NOT operator do nothing
	operatorToken, ok := elems[len(elems)-2].(token)
	if !ok || operatorToken.typ != tNOT {
		return elems, false
	}

	// make sure the thing to be negated is already a parsed
	negated, ok := elems[len(elems)-1].(expr.Expression)
	if !ok {
		return elems, false
	}

	elems = elems[:len(elems)-2]
	elems = push(elems, NOT(negated))
	fmt.Printf("IS NOT\n")
	return elems, true
}

func sub(elems []stringer) ([]stringer, bool) {
	// all the internal terms should have reduced by the time we hit this reducer
	if len(elems) != 3 {
		return elems, false
	}

	open, ok := elems[0].(token)
	if !ok || open.typ != tLPAREN {
		return elems, false
	}

	closed, ok := elems[len(elems)-1].(token)
	if !ok || closed.typ != tRPAREN {
		return elems, false
	}

	fmt.Printf("IS SUB\n")
	return []stringer{elems[1]}, true
}

func must(elems []stringer) ([]stringer, bool) {
	if len(elems) != 2 {
		return elems, false
	}

	must, ok := elems[0].(token)
	if !ok || must.typ != tPLUS {
		return elems, false
	}

	rest, ok := elems[1].(expr.Expression)
	if !ok {
		return elems, false
	}

	return []stringer{MUST(rest)}, true
}

func mustNot(elems []stringer) ([]stringer, bool) {
	if len(elems) != 2 {
		return elems, false
	}

	must, ok := elems[0].(token)
	if !ok || must.typ != tMINUS {
		return elems, false
	}

	rest, ok := elems[1].(expr.Expression)
	if !ok {
		return elems, false
	}

	return []stringer{MUSTNOT(rest)}, true
}

func fuzzy(elems []stringer) ([]stringer, bool) {
	// we are in the case with an implicit 1 fuzzy distance
	if len(elems) == 2 {
		must, ok := elems[1].(token)
		if !ok || must.typ != tTILDE {
			return elems, false
		}

		rest, ok := elems[0].(expr.Expression)
		if !ok {
			return elems, false
		}

		return []stringer{FUZZY(rest, 1)}, true
	}

	if len(elems) != 3 {
		return elems, false
	}

	must, ok := elems[1].(token)
	if !ok || must.typ != tTILDE {
		return elems, false
	}

	rest, ok := elems[0].(expr.Expression)
	if !ok {
		return elems, false
	}

	power, ok := elems[2].(*expr.Literal)
	if !ok {
		return elems, false
	}

	ipower, err := strconv.Atoi(power.String())
	if err != nil {
		return elems, false
	}

	return []stringer{FUZZY(rest, ipower)}, true
}

func boost(elems []stringer) ([]stringer, bool) {
	// we are in the case with an implicit 1 fuzzy distance
	if len(elems) == 2 {
		must, ok := elems[1].(token)
		if !ok || must.typ != tCARROT {
			return elems, false
		}

		rest, ok := elems[0].(expr.Expression)
		if !ok {
			return elems, false
		}

		return []stringer{BOOST(rest, 1.0)}, true
	}

	if len(elems) != 3 {
		return elems, false
	}

	must, ok := elems[1].(token)
	if !ok || must.typ != tCARROT {
		return elems, false
	}

	rest, ok := elems[0].(expr.Expression)
	if !ok {
		return elems, false
	}

	power, ok := elems[2].(*expr.Literal)
	if !ok {
		return elems, false
	}

	fpower, err := toPositiveFloat(power.String())
	if err != nil {
		return elems, false
	}

	return []stringer{BOOST(rest, fpower)}, true
}

func rangeop(elems []stringer) ([]stringer, bool) {
	// we need a [, begin, TO, end, ] to have a range operator which is 5 elems
	if len(elems) != 5 {
		return elems, false
	}

	fmt.Printf("ELEMS IN RANGE: %v\n", elems)

	open, ok := elems[0].(token)
	if !ok || (open.typ != tLSQUARE && open.typ != tLCURLY) {
		fmt.Printf("OPEN NOT RIGHT\n")
		return elems, false
	}

	closed, ok := elems[4].(token)
	if !ok || (closed.typ != tRSQUARE && closed.typ != tRCURLY) {
		fmt.Printf("CLOSED NOT RIGHT\n")
		return elems, false
	}

	to, ok := elems[2].(token)
	if !ok || to.typ != tTO {
		fmt.Printf("NOT TO: 00%s00\n", elems[2])
		return elems, false
	}

	start, ok := elems[1].(expr.Expression)
	if !ok {
		fmt.Printf("NOT START\n")
		return elems, false
	}

	end, ok := elems[3].(expr.Expression)
	if !ok {
		fmt.Printf("NOT END\n")
		return elems, false
	}

	return []stringer{Rang(
		start, end, (open.typ == tLSQUARE && closed.typ == tRSQUARE),
	)}, true

}

func push(stack []stringer, s stringer) []stringer {
	return append(stack, s)
}

func pop[T any](stack []T) (T, []T) {
	return stack[len(stack)-1], stack[:len(stack)-1]
}

func EQ(a expr.Expression, b expr.Expression) expr.Expression {
	return &expr.Equals{
		Term:  a.(*expr.Literal).Value.(string),
		Value: b,
	}
}

func AND(a, b expr.Expression) expr.Expression {
	return &expr.And{
		Left:  a,
		Right: b,
	}
}

func OR(a, b expr.Expression) expr.Expression {
	return &expr.Or{
		Left:  a,
		Right: b,
	}
}

func Lit(val any) expr.Expression {
	return &expr.Literal{
		Value: val,
	}
}

func Wild(val any) expr.Expression {
	return &expr.WildLiteral{
		Literal: expr.Literal{
			Value: val,
		},
	}
}

func Rang(min, max expr.Expression, inclusive bool) expr.Expression {
	lmin, ok := min.(*expr.Literal)
	if !ok {
		wmin, ok := min.(*expr.WildLiteral)
		if !ok {
			panic("must only pass a *expr.Literal or *WildLiteral to the Rang function")
		}
		lmin = &expr.Literal{Value: wmin.Value}
	}

	lmax, ok := max.(*expr.Literal)
	if !ok {
		wmax, ok := max.(*expr.WildLiteral)
		if !ok {
			panic("must only pass a *expr.Literal or *WildLiteral to the Rang function")
		}
		lmax = &expr.Literal{Value: wmax.Value}
	}
	return &expr.Range{
		Inclusive: inclusive,
		Min:       lmin,
		Max:       lmax,
	}
}

func NOT(e expr.Expression) expr.Expression {
	return &expr.Not{
		Sub: e,
	}
}

func MUST(e expr.Expression) expr.Expression {
	return &expr.Must{
		Sub: e,
	}
}

func MUSTNOT(e expr.Expression) expr.Expression {
	return &expr.MustNot{
		Sub: e,
	}
}

func BOOST(e expr.Expression, power float32) expr.Expression {
	return &expr.Boost{
		Sub:   e,
		Power: power,
	}
}

func FUZZY(e expr.Expression, distance int) expr.Expression {
	return &expr.Fuzzy{
		Sub:      e,
		Distance: distance,
	}
}

func REGEXP(val any) expr.Expression {
	return &expr.RegexpLiteral{
		Literal: expr.Literal{Value: val},
	}
}
