package reduce

import (
	"math"

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

	// we consumed one terminal, the =
	return []any{FieldGroup(term, value)}, drop(nonTerminals, 1), true
}

// FieldGroup returns field:value the way the parser reads it. A plain OR of
// terms, as in a:(x OR y), becomes a:x OR a:y written as an IN list. Any
// other value gets field on every term that has no field of its own, so
// a:(x AND -y) is a:x AND -a:y and a:(b:c OR d) is b:c OR a:d. A single
// term is a plain field:term.
//
// The SQL drivers call it too, so an EQUALS on a group that was built in
// code or read from JSON renders the same as the parsed query.
func FieldGroup(field, value *expr.Expression) *expr.Expression {
	if literals, ok := isChainedOrLiterals(value); ok && len(literals) > 1 {
		return expr.IN(field, expr.LIST(literals))
	}
	return applyField(field, value)
}

func isChainedOrLiterals(in *expr.Expression) (out []*expr.Expression, ok bool) {
	if in == nil {
		return out, false
	}

	if in.Op == expr.Literal || in.Op == expr.Null {
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
			WrapDefaultField(left, defaultField),
			WrapDefaultField(right, defaultField),
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
			WrapDefaultField(left, defaultField),
			WrapDefaultField(right, defaultField),
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
			WrapDefaultField(negated, defaultField),
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
	return []any{expr.MUST(WrapDefaultField(rest, defaultField))}, drop(nonTerminals, 1), true
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
	return []any{expr.MUSTNOT(WrapDefaultField(rest, defaultField))}, drop(nonTerminals, 1), true
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
		if idistance, ok := wholeNumber(distance); ok {
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
		if fpower, ok := positiveNumber(power); ok {
			// Valid power - reduce all 3 elements
			return []any{expr.BOOST(rest, fpower)}, drop(nonTerminals, 1), true
		}
	}

	// elems[2] is NOT a valid power - reduce just [expr, ^] with implicit power
	// Return the remaining elements to stay on the stack for further processing
	result := append([]any{expr.BOOST(rest, 1.0)}, elems[2:]...)
	return result, drop(nonTerminals, 1), true
}

// rangeop reduces [begin TO end] to a range with no field yet. The equal
// reducer gives it one, so a:[1 TO 5] and a:(x OR [1 TO 5]) both get field
// a. A range that never gets a field fails validation.
func rangeop(elems []any, nonTerminals []lex.Token, defaultField string) ([]any, []lex.Token, bool) {
	// we need a [, begin, TO, end, ] to have a range operator which is 5 elems
	if len(elems) != 5 {
		return elems, nonTerminals, false
	}

	open, ok := elems[0].(lex.Token)
	if !ok || (open.Typ != lex.TLSquare && open.Typ != lex.TLCurly) {
		return elems, nonTerminals, false
	}

	closed, ok := elems[4].(lex.Token)
	if !ok || (closed.Typ != lex.TRSquare && closed.Typ != lex.TRCurly) {
		return elems, nonTerminals, false
	}

	to, ok := elems[2].(lex.Token)
	if !ok || to.Typ != lex.TTO {
		return elems, nonTerminals, false
	}

	start, ok := elems[1].(*expr.Expression)
	if !ok {
		return elems, nonTerminals, false
	}

	end, ok := elems[3].(*expr.Expression)
	if !ok {
		return elems, nonTerminals, false
	}

	// we consumed three terminals, the [, TO, and ]
	return []any{expr.Rang(
		nil, start, end, (open.Typ == lex.TLSquare && closed.Typ == lex.TRSquare),
	)}, drop(nonTerminals, 3), true
}

func drop[T any](stack []T, i int) []T {
	return stack[:len(stack)-i]
}

// positiveNumber returns the value of a number literal greater than zero. The
// parser has already decided which terms are numbers, so a term it kept as
// text, such as 0x1p3 or inf, is not a boost power.
func positiveNumber(e *expr.Expression) (float64, bool) {
	if e.Op != expr.Literal {
		return 0, false
	}
	switch n := e.Left.(type) {
	case int:
		return float64(n), n > 0
	case float64:
		return n, n > 0
	default:
		return 0, false
	}
}

// wholeNumber returns the value of a number literal with no fractional part,
// such as 2 or 2.0. A quoted or escaped term like "2" is text, not a number.
func wholeNumber(e *expr.Expression) (int, bool) {
	if e.Op != expr.Literal {
		return 0, false
	}
	switch n := e.Left.(type) {
	case int:
		return n, true
	case float64:
		if n != math.Trunc(n) || math.Abs(n) > math.MaxInt32 {
			return 0, false
		}
		return int(n), true
	default:
		return 0, false
	}
}

// WrapDefaultField gives a term with no field the default field, so that
// a:b AND "c" compares "c" against the default field. A term is a word,
// phrase, number, wildcard, regex, null or range, and the term under a boost
// or fuzzy, whose reducers don't wrap it. AND, OR, NOT, + and - wrap their
// own operands when they are reduced, so they come back as they are, and so
// does a term that has a field. An empty field or a nil e changes nothing.
//
// It is exported for the parser in the lucene package, which calls it on the
// whole query once parsing is done. It is not meant to apply a default field
// to an expression built some other way: an OR of terms, for example, comes
// back unchanged.
func WrapDefaultField(e *expr.Expression, field string) *expr.Expression {
	if field == "" || e == nil {
		return e
	}
	switch e.Op {
	case expr.Literal, expr.Wild, expr.Regexp, expr.Null, expr.Range, expr.Boost, expr.Fuzzy:
		return applyField(expr.Lit(expr.Column(field)), e)
	default:
		return e
	}
}

// applyField applies field to every term in e that has no field of its own,
// the way Lucene reads a field group: a:(x AND -y) is a:x AND -a:y. A term
// that has its own field keeps it, so a:(b:c OR d) is b:c OR a:d.
func applyField(field, e *expr.Expression) *expr.Expression {
	switch e.Op {
	case expr.Literal, expr.Wild, expr.Regexp, expr.Null:
		f := *field
		return expr.Eq(&f, e)
	case expr.Range:
		boundary, ok := e.Right.(*expr.RangeBoundary)
		if e.Left != nil || !ok {
			return e
		}
		f := *field
		return expr.Rang(&f, boundary.Min, boundary.Max, boundary.Inclusive)
	case expr.And, expr.Or:
		left, lok := e.Left.(*expr.Expression)
		right, rok := e.Right.(*expr.Expression)
		if !lok || !rok {
			return e
		}
		return expr.Expr(applyField(field, left), e.Op, applyField(field, right))
	case expr.Not, expr.Must, expr.MustNot, expr.Boost, expr.Fuzzy:
		inner, ok := e.Left.(*expr.Expression)
		if !ok {
			return e
		}
		// copy e so a boost power or fuzzy distance carries over
		wrapped := *e
		wrapped.Left = applyField(field, inner)
		return &wrapped
	default:
		// EQUALS, LIKE, IN and comparisons already have a field.
		return e
	}
}
