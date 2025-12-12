package reduce

import (
	"fmt"
	"strconv"

	"github.com/grindlemire/go-lucene/internal/lex"
	"github.com/grindlemire/go-lucene/pkg/lucene/expr"
)

// Reduce will reduce the elems and nonTerminals stacks using the available reducers and return
// those slices modified to contain the reduced expressions. The elems will contain the reduced
// expression the the nonTerminals will contain the modified stack of nonTerminals yet to be reduced.
func Reduce(elems []any, nonTerminals []lex.Token, defaultField string) ([]any, []lex.Token, bool) {
	for _, reducer := range reducers {
		elems, nonTerminals, reduced := reducer(elems, nonTerminals, defaultField)
		if reduced {
			return elems, nonTerminals, true
		}
	}
	return elems, nonTerminals, false
}

type reducer func(elems []any, nonTerminals []lex.Token, defaultField string) ([]any, []lex.Token, bool)

// reducers are the reducers that will be executed during the grammar parsing
var reducers = []reducer{
	and,
	or,
	fuzzy,
	boost,
	equal,
	compare,
	compareEq,
	not,
	sub,
	must,
	mustNot,
	rangeop,
}

func equal(elems []any, nonTerminals []lex.Token, defaultField string) ([]any, []lex.Token, bool) {
	if len(elems) != 3 {
		return elems, nonTerminals, false
	}

	// ensure the middle token is an equals
	tok, ok := elems[1].(lex.Token)
	if !ok || (tok.Typ != lex.TEqual && tok.Typ != lex.TColon) {
		return elems, nonTerminals, false
	}

	// make sure the left is a literal and right is an expression
	term, ok := elems[0].(*expr.Expression)
	if !ok {
		return elems, nonTerminals, false
	}
	value, ok := elems[2].(*expr.Expression)
	if !ok {
		return elems, nonTerminals, false
	}

	if literals, ok := isChainedOrLiterals(value); ok && len(literals) > 1 {
		elems = []any{
			expr.IN(
				term,
				expr.LIST(literals),
			),
		}
	} else {
		elems = []any{
			expr.Eq(
				term,
				value,
			),
		}
	}
	// we consumed one terminal, the =
	return elems, drop(nonTerminals, 1), true
}

func isChainedOrLiterals(in *expr.Expression) (out []*expr.Expression, ok bool) {
	if in == nil {
		return out, false
	}

	if in.Op == expr.Literal {
		return []*expr.Expression{in}, true
	}

	if in.Op == expr.Or {
		left, ok := in.Left.(*expr.Expression)
		if !ok {
			return out, false
		}
		right, ok := in.Right.(*expr.Expression)
		if !ok {
			return out, false
		}

		l, isLLiterals := isChainedOrLiterals(left)
		r, isRLiterals := isChainedOrLiterals(right)
		return append(l, r...), isLLiterals && isRLiterals
	}

	return out, false
}

func compare(elems []any, nonTerminals []lex.Token, defaultField string) ([]any, []lex.Token, bool) {
	if len(elems) != 4 {
		return elems, nonTerminals, false
	}

	// ensure our middle tokens start with a colon
	tok, ok := elems[1].(lex.Token)
	if !ok || (tok.Typ != lex.TColon) {
		return elems, nonTerminals, false
	}

	// ensure the colon is followed by a > or <
	tokCmp, ok := elems[2].(lex.Token)
	if !ok || (tokCmp.Typ != lex.TGreater && tokCmp.Typ != lex.TLess) {
		return elems, nonTerminals, false
	}

	// make sure the left is a literal and right is an expression
	term, ok := elems[0].(*expr.Expression)
	if !ok {
		return elems, nonTerminals, false
	}
	value, ok := elems[3].(*expr.Expression)
	if !ok {
		return elems, nonTerminals, false
	}

	if tokCmp.Typ == lex.TGreater {
		elems = []any{
			expr.GREATER(
				term,
				value,
			),
		}
	} else {
		elems = []any{
			expr.LESS(
				term,
				value,
			),
		}
	}

	return elems, drop(nonTerminals, 2), true
}

func compareEq(elems []any, nonTerminals []lex.Token, defaultField string) ([]any, []lex.Token, bool) {
	if len(elems) != 5 {
		return elems, nonTerminals, false
	}

	// ensure our middle tokens start with a colon
	tok, ok := elems[1].(lex.Token)
	if !ok || (tok.Typ != lex.TColon) {
		return elems, nonTerminals, false
	}

	// ensure the colon is followed by a > or <
	tokCmp, ok := elems[2].(lex.Token)
	if !ok || (tokCmp.Typ != lex.TGreater && tokCmp.Typ != lex.TLess) {
		return elems, nonTerminals, false
	}

	// ensure the middle tokens are followed by an =
	tokEp, ok := elems[3].(lex.Token)
	if !ok || (tokEp.Typ != lex.TEqual) {
		return elems, nonTerminals, false
	}

	// make sure the left is a literal and right is an expression
	term, ok := elems[0].(*expr.Expression)
	if !ok {
		return elems, nonTerminals, false
	}
	value, ok := elems[4].(*expr.Expression)
	if !ok {
		return elems, nonTerminals, false
	}

	if tokCmp.Typ == lex.TGreater {
		elems = []any{
			expr.GREATEREQ(
				term,
				value,
			),
		}
	} else {
		elems = []any{
			expr.LESSEQ(
				term,
				value,
			),
		}
	}

	return elems, drop(nonTerminals, 3), true

}

func and(elems []any, nonTerminals []lex.Token, defaultField string) ([]any, []lex.Token, bool) {
	// if we don't have 3 items in the buffer it's not an AND clause
	if len(elems) != 3 {
		return elems, nonTerminals, false
	}

	// if the middle token is not an AND token do nothing
	operatorToken, ok := elems[1].(lex.Token)
	if !ok || operatorToken.Typ != lex.TAnd {
		return elems, nonTerminals, false
	}

	// make sure the left and right clauses are expressions
	left, ok := elems[0].(*expr.Expression)
	if !ok {
		return elems, nonTerminals, false
	}
	right, ok := elems[2].(*expr.Expression)
	if !ok {
		return elems, nonTerminals, false
	}

	// we have a valid AND clause. Replace it in the stack
	elems = []any{
		expr.AND(
			wrapLiteral(left, defaultField),
			wrapLiteral(right, defaultField),
		),
	}
	// we consumed one terminal, the AND
	return elems, drop(nonTerminals, 1), true
}

func or(elems []any, nonTerminals []lex.Token, defaultField string) ([]any, []lex.Token, bool) {
	// if we don't have 3 items in the buffer it's not an OR clause
	if len(elems) != 3 {
		return elems, nonTerminals, false
	}

	// if the middle token is not an OR token do nothing
	operatorToken, ok := elems[1].(lex.Token)
	if !ok || operatorToken.Typ != lex.TOr {
		return elems, nonTerminals, false
	}

	// make sure the left and right clauses are expressions
	left, ok := elems[0].(*expr.Expression)
	if !ok {
		return elems, nonTerminals, false
	}
	right, ok := elems[2].(*expr.Expression)
	if !ok {
		return elems, nonTerminals, false
	}

	// we have a valid OR clause. Replace it in the stack
	elems = []any{
		expr.OR(
			wrapLiteral(left, defaultField),
			wrapLiteral(right, defaultField),
		),
	}
	// we consumed one terminal, the OR
	return elems, drop(nonTerminals, 1), true
}

func not(elems []any, nonTerminals []lex.Token, defaultField string) ([]any, []lex.Token, bool) {
	if len(elems) < 2 {
		return elems, nonTerminals, false
	}

	// if the second to last token is not the NOT operator do nothing
	operatorToken, ok := elems[len(elems)-2].(lex.Token)
	if !ok || operatorToken.Typ != lex.TNot {
		return elems, nonTerminals, false
	}

	// make sure the thing to be negated is already a parsed
	negated, ok := elems[len(elems)-1].(*expr.Expression)
	if !ok {
		return elems, nonTerminals, false
	}

	elems = elems[:len(elems)-2]
	elems = append(elems,
		expr.NOT(
			wrapLiteral(negated, defaultField),
		),
	)
	// we consumed one terminal, the NOT
	return elems, drop(nonTerminals, 1), true
}

func sub(elems []any, nonTerminals []lex.Token, defaultField string) ([]any, []lex.Token, bool) {
	// all the internal terms should have reduced by the time we hit this reducer
	if len(elems) != 3 {
		return elems, nonTerminals, false
	}

	open, ok := elems[0].(lex.Token)
	if !ok || open.Typ != lex.TLParen {
		return elems, nonTerminals, false
	}

	closed, ok := elems[len(elems)-1].(lex.Token)
	if !ok || closed.Typ != lex.TRParen {
		return elems, nonTerminals, false
	}

	// we consumed two terminals, the ( and )
	return []any{elems[1]}, drop(nonTerminals, 2), true
}

func must(elems []any, nonTerminals []lex.Token, defaultField string) ([]any, []lex.Token, bool) {
	if len(elems) != 2 {
		return elems, nonTerminals, false
	}

	must, ok := elems[0].(lex.Token)
	if !ok || must.Typ != lex.TPlus {
		return elems, nonTerminals, false
	}

	rest, ok := elems[1].(*expr.Expression)
	if !ok {
		return elems, nonTerminals, false
	}

	// we consumed 1 terminal, the +
	return []any{expr.MUST(rest)}, drop(nonTerminals, 1), true
}

func mustNot(elems []any, nonTerminals []lex.Token, defaultField string) ([]any, []lex.Token, bool) {
	if len(elems) != 2 {
		return elems, nonTerminals, false
	}

	must, ok := elems[0].(lex.Token)
	if !ok || must.Typ != lex.TMinus {
		return elems, nonTerminals, false
	}

	rest, ok := elems[1].(*expr.Expression)
	if !ok {
		return elems, nonTerminals, false
	}
	// we consumed one terminal, the -
	return []any{expr.MUSTNOT(rest)}, drop(nonTerminals, 1), true
}

func fuzzy(elems []any, nonTerminals []lex.Token, defaultField string) ([]any, []lex.Token, bool) {
	if len(elems) < 2 {
		return elems, nonTerminals, false
	}

	tilde, ok := elems[1].(lex.Token)
	if !ok || tilde.Typ != lex.TTilde {
		return elems, nonTerminals, false
	}

	rest, ok := elems[0].(*expr.Expression)
	if !ok {
		return elems, nonTerminals, false
	}

	// If we have exactly 2 elements, use implicit distance of 1
	if len(elems) == 2 {
		return []any{expr.FUZZY(rest, 1)}, drop(nonTerminals, 1), true
	}

	// We have 3+ elements. Check if elems[2] is a valid numeric distance
	if distance, ok := elems[2].(*expr.Expression); ok {
		if idistance, err := strconv.Atoi(distance.String()); err == nil {
			return []any{expr.FUZZY(rest, idistance)}, drop(nonTerminals, 1), true
		}
		// elems[2] is an Expression but not a valid numeric distance
		// This means we have [expr, ~, non-numeric-expr] which should be reduced
		// to [FUZZY(expr, 1), non-numeric-expr] so the parser can inject an implicit AND
		result := append([]any{expr.FUZZY(rest, 1)}, elems[2:]...)
		return result, drop(nonTerminals, 1), true
	}

	// elems[2] is NOT an Expression (might be a Token or something else)
	// This means we have [expr, ~, token/other] - reduce just [expr, ~] with implicit distance
	// The token/other will be handled in the next reduce cycle
	result := append([]any{expr.FUZZY(rest, 1)}, elems[2:]...)
	return result, drop(nonTerminals, 1), true
}

func boost(elems []any, nonTerminals []lex.Token, defaultField string) ([]any, []lex.Token, bool) {
	if len(elems) < 2 {
		return elems, nonTerminals, false
	}

	carrot, ok := elems[1].(lex.Token)
	if !ok || carrot.Typ != lex.TCarrot {
		return elems, nonTerminals, false
	}

	rest, ok := elems[0].(*expr.Expression)
	if !ok {
		return elems, nonTerminals, false
	}

	if len(elems) == 2 {
		return []any{expr.BOOST(rest, 1.0)}, drop(nonTerminals, 1), true
	}

	// We have 3+ elements. Check if elems[2] is a valid numeric power
	if power, ok := elems[2].(*expr.Expression); ok {
		if fpower, err := toPositiveFloat(power.String()); err == nil {
			// Valid power - reduce all 3 elements
			return []any{expr.BOOST(rest, fpower)}, drop(nonTerminals, 1), true
		}
	}

	// elems[2] is NOT a valid power - reduce just [expr, ^] with implicit power
	// Return the remaining elements to stay on the stack for further processing
	result := append([]any{expr.BOOST(rest, 1.0)}, elems[2:]...)
	return result, drop(nonTerminals, 1), true
}

func rangeop(elems []any, nonTerminals []lex.Token, defaultField string) ([]any, []lex.Token, bool) {
	// we need a term, :, [, begin, TO, end, ] to have a range operator which is 7 elems
	if len(elems) != 7 {
		return elems, nonTerminals, false
	}

	colon, ok := elems[1].(lex.Token)
	if !ok || colon.Typ != lex.TColon {
		return elems, nonTerminals, false
	}

	open, ok := elems[2].(lex.Token)
	if !ok || (open.Typ != lex.TLSquare && open.Typ != lex.TLCurly) {
		return elems, nonTerminals, false
	}

	closed, ok := elems[6].(lex.Token)
	if !ok || (closed.Typ != lex.TRSquare && closed.Typ != lex.TRCurly) {
		return elems, nonTerminals, false
	}

	to, ok := elems[4].(lex.Token)
	if !ok || to.Typ != lex.TTO {
		return elems, nonTerminals, false
	}

	term, ok := elems[0].(*expr.Expression)
	if !ok {
		return elems, nonTerminals, false
	}

	start, ok := elems[3].(*expr.Expression)
	if !ok {
		return elems, nonTerminals, false
	}

	end, ok := elems[5].(*expr.Expression)
	if !ok {
		return elems, nonTerminals, false
	}

	// we consumed four terminals, the :, [, TO, and ]
	return []any{expr.Rang(
		term, start, end, (open.Typ == lex.TLSquare && closed.Typ == lex.TRSquare),
	)}, drop(nonTerminals, 4), true
}

func drop[T any](stack []T, i int) []T {
	return stack[:len(stack)-i]
}

func toPositiveFloat(in string) (f float64, err error) {
	i, err := strconv.Atoi(in)
	if err == nil && i > 0 {
		return float64(i), nil
	}

	pf, err := strconv.ParseFloat(in, 64)
	if err == nil && pf > 0 {
		return float64(pf), nil
	}

	return f, fmt.Errorf("[%v] is not a positive float", in)
}

// wrapLiteral will wrap a literal expression in an equals expression for a defaultField.
// we need this because we want to support lucene expressions like a:b AND "c" which needs a default
// field to compare "c" against to be valid.
func wrapLiteral(lit *expr.Expression, field string) *expr.Expression {
	if lit.Op == expr.Literal && field != "" {
		return expr.Eq(expr.Column(field), lit)
	}
	return lit
}
