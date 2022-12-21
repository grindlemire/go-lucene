package lucene

import (
	"strconv"

	"github.com/grindlemire/go-lucene/expr"
)

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
		return elems, nonTerminals, false
	}

	// ensure middle token is an equals
	tok, ok := elems[1].(token)
	if !ok || (tok.typ != tEQUAL && tok.typ != tCOLON) {
		return elems, nonTerminals, false
	}

	// make sure the left is a literal and right is an expression
	left, ok := elems[0].(*expr.Literal)
	if !ok {
		return elems, nonTerminals, false
	}
	right, ok := elems[2].(expr.Expression)
	if !ok {
		return elems, nonTerminals, false
	}

	elems = []stringer{
		expr.Eq(
			left,
			right,
		),
	}
	// we consumed one terminal, the =
	return elems, drop(nonTerminals, 1), true
}

func and(elems []stringer, nonTerminals []token) ([]stringer, []token, bool) {
	// if we don't have 3 items in the buffer it's not an AND clause
	if len(elems) != 3 {
		return elems, nonTerminals, false
	}

	// if the middle token is not an AND token do nothing
	operatorToken, ok := elems[1].(token)
	if !ok || operatorToken.typ != tAND {
		return elems, nonTerminals, false
	}

	// make sure the left and right clauses are expressions
	left, ok := elems[0].(expr.Expression)
	if !ok {
		return elems, nonTerminals, false
	}
	right, ok := elems[2].(expr.Expression)
	if !ok {
		return elems, nonTerminals, false
	}

	// we have a valid AND clause. Replace it in the stack
	elems = []stringer{
		expr.AND(
			left,
			right,
		),
	}
	// we consumed one terminal, the AND
	return elems, drop(nonTerminals, 1), true
}

func or(elems []stringer, nonTerminals []token) ([]stringer, []token, bool) {
	// if we don't have 3 items in the buffer it's not an OR clause
	if len(elems) != 3 {
		return elems, nonTerminals, false
	}

	// if the middle token is not an OR token do nothing
	operatorToken, ok := elems[1].(token)
	if !ok || operatorToken.typ != tOR {
		return elems, nonTerminals, false
	}

	// make sure the left and right clauses are expressions
	left, ok := elems[0].(expr.Expression)
	if !ok {
		return elems, nonTerminals, false
	}
	right, ok := elems[2].(expr.Expression)
	if !ok {
		return elems, nonTerminals, false
	}

	// we have a valid OR clause. Replace it in the stack
	elems = []stringer{
		expr.OR(
			left,
			right,
		),
	}
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
	elems = append(elems, expr.NOT(negated))
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
		return elems, nonTerminals, false
	}

	closed, ok := elems[4].(token)
	if !ok || (closed.typ != tRSQUARE && closed.typ != tRCURLY) {
		return elems, nonTerminals, false
	}

	to, ok := elems[2].(token)
	if !ok || to.typ != tTO {
		return elems, nonTerminals, false
	}

	start, ok := elems[1].(expr.Expression)
	if !ok {
		return elems, nonTerminals, false
	}

	end, ok := elems[3].(expr.Expression)
	if !ok {
		return elems, nonTerminals, false
	}

	// we consumed three terminals, the [, TO, and ]
	return []stringer{expr.Rang(
		start, end, (open.typ == tLSQUARE && closed.typ == tRSQUARE),
	)}, drop(nonTerminals, 3), true

}
