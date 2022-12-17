package lucene

import (
	"fmt"
	"reflect"
	"strings"

	"github.com/grindlemire/go-lucene/expr"
)

type stringer interface {
	String() string
}

// BufParse will parse using a buffer and the shift reduce algo
func BufParse(input string) (e expr.Expression, err error) {
	p := &bufParser{
		lex:   lex(input),
		stack: []stringer{},
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
	lex   *lexer
	stack []stringer
}

func (p *bufParser) parse() (e expr.Expression, err error) {

	for {
		// shift out of the lexer
		tok := p.shift()
		// if no more input then return
		if tok.typ == tEOF || tok.typ == tRPAREN {
			if len(p.stack) != 1 {
				return e, fmt.Errorf("unable to parse expression. Stack: %v", p.stack)
			}
			e, ok := p.stack[0].(expr.Expression)
			if !ok {
				return e, fmt.Errorf("root node not an expression, got [%s]", reflect.TypeOf(p.stack[1]))
			}
			return e, nil
		}

		// if we have a left paren then we are diving into a subexpression so recurse
		if tok.typ == tLPAREN {
			e, err := (&bufParser{p.lex, []stringer{}}).parse()
			if err != nil {
				return e, err
			}
			p.stack = push(p.stack, e)
		} else {
			// otherwise match the token
			p.stack = push(p.stack, tok)
		}

		fmt.Printf("--------\nSTACK IS NOW: %+v\n", p.stack)

		// run reduce functions
		err = p.reduce(tok)
		if err != nil {
			return e, err
		}
	}
}

func (p *bufParser) shift() (tok token) {
	return p.lex.nextToken()
}

type reducer func(p *bufParser) (matched bool, err error)

var reducers = []reducer{
	and,
	or,
	literal,
	equal,
	not,
}

func equal(p *bufParser) (matched bool, err error) {
	if len(p.stack) != 3 {
		// fmt.Printf("NOT EQUAL - len not correct\n")
		return false, nil
	}

	// ensure middle token is an equals
	tok, ok := p.stack[1].(token)
	if !ok || (tok.typ != tEQUAL && tok.typ != tCOLON) {
		// fmt.Printf("NOT EQUAL - not tEQUAL or tCOLON\n")
		return false, nil
	}

	// make sure the left is a literal and right is an expression
	left, ok := p.stack[0].(*expr.Literal)
	if !ok {
		// fmt.Printf("NOT EQUAL - left not literal\n")
		return false, nil
	}
	right, ok := p.stack[2].(expr.Expression)
	if !ok {
		// fmt.Printf("NOT EQUAL - right not expression\n")
		return false, nil
	}

	p.stack = []stringer{
		EQ(
			left,
			right,
		),
	}
	fmt.Printf("IS EQUAL\n")
	return true, nil
}

func literal(p *bufParser) (matched bool, err error) {
	// we have to have at least one item in the stack
	if len(p.stack) < 1 {
		// fmt.Printf("NOT LITERAL - empty\n")
		return false, nil
	}

	// don't process unless its a raw token
	tok, ok := p.stack[len(p.stack)-1].(token)
	if !ok {
		// fmt.Printf("NOT LITERAL - not token\n")
		return false, nil
	}

	switch tok.typ {
	case tLITERAL, tQUOTED:
		_, p.stack = pop(p.stack)

		e, err := parseLiteral(tok)
		if err != nil {
			return false, err
		}

		p.stack = push(p.stack, e)
		fmt.Printf("IS LITERAL\n")
		// TODO, do we need to parse floats here?
		return true, nil
	case tREGEXP:
		// strip the quotes off because we don't need them
		val := strings.ReplaceAll(tok.val, "/", "")
		_, p.stack = pop(p.stack)
		p.stack = push(p.stack, REGEXP(val))
		fmt.Printf("IS REGEXP LITERAL\n")
		return true, nil
	default:
		// fmt.Printf("NOT LITERAL - not tLITERAL, tQUOTED, tREGEXP\n")
		return false, nil
	}
}

func and(p *bufParser) (matched bool, err error) {
	// if we don't have 3 items in the buffer it's not an AND clause
	if len(p.stack) != 3 {
		// fmt.Printf("NOT AND - len not correct\n")
		return false, nil
	}

	// if the middle token is not an AND token do nothing
	operatorToken, ok := p.stack[1].(token)
	if !ok || operatorToken.typ != tAND {
		// fmt.Printf("NOT AND - operator wrong\n")
		return false, nil
	}

	// make sure the left and right clauses are expressions
	left, ok := p.stack[0].(expr.Expression)
	if !ok {
		// fmt.Printf("NOT AND - left not expr\n")
		return false, nil
	}
	right, ok := p.stack[2].(expr.Expression)
	if !ok {
		// fmt.Printf("NOT AND - right not expr\n")
		return false, nil
	}

	// we have a valid AND clause. Replace it in the stack
	p.stack = []stringer{
		AND(
			left,
			right,
		),
	}
	fmt.Printf("IS AND\n")
	return true, nil
}

func or(p *bufParser) (matched bool, err error) {
	// if we don't have 3 items in the buffer it's not an OR clause
	if len(p.stack) != 3 {
		// fmt.Printf("NOT OR - len not correct\n")
		return false, nil
	}

	// if the middle token is not an OR token do nothing
	operatorToken, ok := p.stack[1].(token)
	if !ok || operatorToken.typ != tOR {
		// fmt.Printf("NOT OR - operator wrong\n")
		return false, nil
	}

	// make sure the left and right clauses are expressions
	left, ok := p.stack[0].(expr.Expression)
	if !ok {
		// fmt.Printf("NOT OR - left not expr\n")
		return false, nil
	}
	right, ok := p.stack[2].(expr.Expression)
	if !ok {
		// fmt.Printf("NOT OR - right not expr\n")
		return false, nil
	}

	// we have a valid OR clause. Replace it in the stack
	p.stack = []stringer{
		OR(
			left,
			right,
		),
	}
	fmt.Printf("IS OR\n")
	return true, nil
}

func not(p *bufParser) (matched bool, err error) {
	if len(p.stack) < 2 {
		return false, nil
	}

	// if the second to last token is not the NOT operator do nothing
	operatorToken, ok := p.stack[len(p.stack)-2].(token)
	if !ok || operatorToken.typ != tNOT {
		return false, nil
	}

	// make sure the thing to be negated is already a parsed
	negated, ok := p.stack[len(p.stack)-1].(expr.Expression)
	if !ok {
		return false, nil
	}

	p.stack = p.stack[:len(p.stack)-2]
	p.stack = push(p.stack, NOT(negated))
	fmt.Printf("IS NOT\n")
	return true, nil
}

func (p *bufParser) reduce(tok token) (err error) {
	for i := 0; i < len(reducers); i++ {
		matched, err := reducers[i](p)
		if err != nil {
			return err
		}
		// if we matched we need to recheck all our reducers to see
		// if we can further reduce the expression. This is a clever
		// short cut so we don't have to go back to the outer loop.
		if matched {
			fmt.Printf("--- rerunning with %v\n", p.stack)
			i = -1
		}
	}
	return nil
}

func push(stack []stringer, s stringer) []stringer {
	return append(stack, s)
}

func pop(stack []stringer) (stringer, []stringer) {
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
