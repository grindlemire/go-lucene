package lucene

import (
	"fmt"
	"reflect"
	"regexp"
	"strconv"
	"strings"

	"github.com/grindlemire/go-lucene/internal/lex"
	"github.com/grindlemire/go-lucene/internal/wildcard"
	"github.com/grindlemire/go-lucene/pkg/lucene/expr"
	"github.com/grindlemire/go-lucene/pkg/lucene/reduce"
)

type Opt func(*parser)

// WithDefaultField sets the default field to equate literals to.
// For example a:b AND "c" will be parsed as a:b AND myfield:"c"
func WithDefaultField(field string) Opt {
	return func(p *parser) {
		p.defaultField = field
	}
}

// WithJSONFields marks columns that hold JSON. A dotted field on one of these
// columns, such as labels.category, parses to an expr.JSONPath and the SQL drivers
// render it with the dialect's JSON accessor. Dotted fields on other columns
// keep their current meaning: a column whose name contains a dot. A column
// name that is empty or contains a dot makes Parse return an error, because the
// first dot of a field always ends the column name.
func WithJSONFields(columns ...string) Opt {
	return func(p *parser) {
		if p.jsonFields == nil {
			p.jsonFields = map[string]bool{}
		}
		for _, c := range columns {
			if c == "" || strings.Contains(c, ".") {
				p.optErr = fmt.Errorf("WithJSONFields: column name %q must be non-empty and must not contain a dot", c)
				return
			}
			p.jsonFields[c] = true
		}
	}
}

// WithCaseInsensitiveFields makes string matches on the listed fields ignore
// case: equality, phrases, IN lists and wildcards. The SQL wraps the column
// and the value in LOWER(). Numbers, regexes, ranges, comparisons, field:*
// and null checks render as without the option. Field names match exactly.
// Calling it more than once adds to the list.
//
// The ToPostgres, ToSQLite, ToMySQL and ToMariaDB functions (and their
// parameterized forms) read this option. Parse ignores it: the expression it
// returns is the same. When rendering a parsed expression with a driver
// yourself, set driver.Base.CaseInsensitiveFields on the driver instead.
func WithCaseInsensitiveFields(fields ...string) Opt {
	return func(p *parser) {
		p.caseInsensitiveFields = append(p.caseInsensitiveFields, fields...)
	}
}

// Parse will parse a lucene expression string using a buffer and the shift reduce algorithm. The returned expression
// is an AST that can be rendered to a variety of different formats.
func Parse(input string, opts ...Opt) (e *expr.Expression, err error) {
	e, _, err = parseWithOptions(input, opts...)
	return e, err
}

// parseWithOptions parses input and also returns the parser, so the render
// functions can read options that apply at render time.
func parseWithOptions(input string, opts ...Opt) (e *expr.Expression, p *parser, err error) {
	p = &parser{
		lex:          lex.Lex(input),
		stack:        []any{},
		nonTerminals: []lex.Token{{Typ: lex.TStart}},
		numberText:   map[*expr.Expression]string{},
	}

	for _, opt := range opts {
		opt(p)
	}
	if p.optErr != nil {
		return e, p, p.optErr
	}

	ex, err := p.parse()
	if err != nil {
		return e, p, err
	}

	if len(p.jsonFields) > 0 {
		toJSONPaths(ex, p.jsonFields)
	}

	err = expr.Validate(ex)
	if err != nil {
		return e, p, err
	}

	return ex, p, nil
}

type parser struct {
	lex          *lex.Lexer
	stack        []any
	nonTerminals []lex.Token

	// numberText holds the text of each number literal as written, for
	// fieldToColumn.
	numberText map[*expr.Expression]string

	defaultField string
	jsonFields   map[string]bool
	optErr       error

	// fieldOps counts the ':' and '=' tokens on nonTerminals, so reduce can
	// tell whether it is inside a field's value without scanning the stack.
	fieldOps int

	// caseInsensitiveFields is read by the render functions, not the parser.
	caseInsensitiveFields []string
}

// toJSONPaths replaces each column named "<json column>.<path>" with an expr.JSONPath.
func toJSONPaths(e *expr.Expression, jsonFields map[string]bool) {
	if e == nil {
		return
	}
	if col, ok := e.Left.(expr.Column); ok {
		base, path, dotted := strings.Cut(string(col), ".")
		if dotted && jsonFields[base] {
			e.Left = expr.JSONPath{Column: expr.Column(base), Path: strings.Split(path, ".")}
		}
		return
	}
	for _, side := range []any{e.Left, e.Right} {
		switch v := side.(type) {
		case *expr.Expression:
			toJSONPaths(v, jsonFields)
		case []*expr.Expression:
			for _, child := range v {
				toJSONPaths(child, jsonFields)
			}
		}
	}
}

// checkJSONField rejects a field on a JSON column with an escaped dot, such
// as labels.a\.b. Every dot separates keys, so the key a.b cannot be
// addressed, and reading the path as a, b would match the wrong key.
func (p *parser) checkJSONField(tok lex.Token) error {
	if tok.Typ == lex.TQuoted || !strings.Contains(tok.Val, `\.`) {
		return nil
	}
	base, _, _ := strings.Cut(strings.ReplaceAll(tok.Val, `\`, ""), ".")
	if p.jsonFields[base] {
		return fmt.Errorf("json path key cannot contain a dot: %s", tok.Val)
	}
	return nil
}

func (p *parser) parse() (e *expr.Expression, err error) {
	for {
		next := p.lex.Peek()
		if next.Typ == lex.TErr {
			return e, fmt.Errorf("error parsing: %s", next.Val)
		}
		if p.shouldAccept(next) {
			if len(p.stack) != 1 {
				return e, fmt.Errorf("multiple expressions left after parsing: %v", p.stack)
			}
			final, ok := p.stack[0].(*expr.Expression)
			if !ok {
				return e, fmt.Errorf(
					"final parse didn't return an expression: %s [type: %s]",
					p.stack[0],
					reflect.TypeOf(final),
				)
			}

			// A query that is a single term, like x, foo* or [1 TO 5], has no
			// operator to give it the default field, so give it one here.
			return reduce.WrapDefaultField(final, p.defaultField), nil
		}

		if p.shouldShift(next) {
			tok := p.shift()
			if lex.IsTerminal(tok) {
				// if we have a terminal parse it and put it on the stack
				lit, err := parseLiteral(tok)
				if err != nil {
					return e, err
				}

				// Keep a number's text in case it turns out to be a field.
				if isNumberLiteral(lit) {
					p.numberText[lit.(*expr.Expression)] = tok.Val
				}
				if p.lex.Peek().Typ == lex.TColon {
					if err := p.checkJSONField(tok); err != nil {
						return e, err
					}
				}

				// we should always check if the current top of the stack is another token
				// if it isn't then we have an implicit AND we need to inject.
				if len(p.stack) > 0 {
					_, isTopToken := p.stack[len(p.stack)-1].(lex.Token)
					if !isTopToken {
						implAnd := lex.Token{Typ: lex.TAnd, Val: "AND"}
						// act as if we just saw an AND and check if we need to reduce the
						// current token stack first. Keep reducing until we can shift the AND.
						for !p.shouldShift(implAnd) {
							err = p.reduce()
							if err != nil {
								return e, err
							}
						}

						// if we have a literal as the previous parsed thing then
						// we must be in an implicit AND and should reduce
						p.stack = append(p.stack, implAnd)
						p.pushNonTerminal(implAnd)
					}
				}

				p.stack = append(p.stack, lit)
				continue
			}

			// Prefix operators (-, +, NOT), "(" and the start of a range start a new subexpression.
			// Inject an implicit AND when the top of the stack is an expression, so "-a:b -c:d",
			// "a:b (c:d)" and "a:(x [1 TO 5])" all get an AND.
			if len(p.stack) > 0 && (isPrefixOperator(tok) || tok.Typ == lex.TLParen || isRangeStart(tok)) {
				_, isTopToken := p.stack[len(p.stack)-1].(lex.Token)
				if !isTopToken {
					implAnd := lex.Token{Typ: lex.TAnd, Val: "AND"}
					for !p.shouldShift(implAnd) {
						err = p.reduce()
						if err != nil {
							return e, err
						}
					}
					p.stack = append(p.stack, implAnd)
					p.pushNonTerminal(implAnd)
				}
			}

			if tok.Typ == lex.TColon || tok.Typ == lex.TEqual {
				p.fieldToColumn()
			}

			// push the token on the stack
			p.stack = append(p.stack, tok)
			p.pushNonTerminal(tok)
			continue
		}

		err = p.reduce()
		if err != nil {
			return e, err
		}
	}
}

func (p *parser) shift() (tok lex.Token) {
	return p.lex.Next()
}

// shouldShift determines if the parser should shift or not. This might end up in the grammar specific
// packages and implemented for each grammar this parser supports but for now it can live at the top level.
func (p *parser) shouldShift(next lex.Token) bool {
	if next.Typ == lex.TEOF {
		return false
	}

	if next.Typ == lex.TErr {
		return false
	}

	curr := p.nonTerminals[len(p.nonTerminals)-1]

	// if we are ever attempting to move past a subexpr we need to parse it before moving on.
	// This MUST be checked before terminal handling, otherwise we'd shift a terminal
	// before reducing the parenthetical expression.
	if anyClosingBracket(curr) {
		return false
	}

	// if we have a terminal symbol then we always want to shift since it won't be
	// matched by any rule
	if lex.IsTerminal(next) {
		return true
	}

	// if we have an open grouping or the next one is we want to always shift
	if anyOpenBracket(curr, next) {
		return true
	}

	// we need the closing bracket to reduce the range subexpression so shift that on
	// if we see it
	if endingRangeSubExpr(next) {
		return true
	}

	// shift if our current token has less precedence than the next token
	return lex.HasLessPrecedence(curr, next)
}

func anyOpenBracket(curr, next lex.Token) bool {
	return curr.Typ == lex.TLSquare ||
		next.Typ == lex.TLSquare ||
		curr.Typ == lex.TLCurly ||
		next.Typ == lex.TLCurly ||
		curr.Typ == lex.TLParen ||
		next.Typ == lex.TLParen
}

func anyClosingBracket(curr lex.Token) bool {
	return curr.Typ == lex.TRParen ||
		curr.Typ == lex.TRSquare ||
		curr.Typ == lex.TRCurly
}

func endingRangeSubExpr(next lex.Token) bool {
	return next.Typ == lex.TRSquare || next.Typ == lex.TRCurly
}

// isPrefixOperator returns true for operators that start a new subexpression.
// These need implicit AND injection when they follow an expression.
func isPrefixOperator(tok lex.Token) bool {
	return tok.Typ == lex.TMinus || tok.Typ == lex.TPlus || tok.Typ == lex.TNot
}

// fieldToColumn is called before a ':' or '=' is shifted. The left side of
// field:value is always a column, even when it looks like a number or a
// wildcard. So a number on top of the stack becomes a column with its text as
// written, as in 0:1, 1=1 or (0):*, where 007:x is "007", not 7. A wildcard
// becomes a column with its escapes removed, since a column name is not a
// pattern: fo\*o*:x is "fo*o*", as fo\*o:x is "fo*o".
func (p *parser) fieldToColumn() {
	top := len(p.stack) - 1
	if top < 0 {
		return
	}
	lit, ok := p.stack[top].(*expr.Expression)
	if !ok {
		return
	}
	if text, ok := p.numberText[lit]; ok {
		p.stack[top] = expr.Lit(text)
		return
	}
	if s, ok := lit.Left.(string); ok && lit.Op == expr.Wild {
		p.stack[top] = expr.Lit(wildcard.Unescape(s))
	}
}

// isRangeStart returns true for the brackets that open a range.
func isRangeStart(tok lex.Token) bool {
	return tok.Typ == lex.TLSquare || tok.Typ == lex.TLCurly
}

func (p *parser) shouldAccept(next lex.Token) bool {
	return len(p.stack) == 1 &&
		next.Typ == lex.TEOF
}

func (p *parser) reduce() (err error) {
	// The terms in a field group like a:(x OR y) belong to field a, so they
	// must not get the default field. The ':' stays on the nonterminal stack
	// until the group is reduced into a:(...), so while it is there we are
	// reducing a field's value.
	defaultField := p.defaultField
	if p.fieldOps > 0 {
		defaultField = ""
	}

	top := []any{}
	for {
		if len(p.stack) == 0 {
			return fmt.Errorf("error parsing, no items left to reduce, current state: %v", top)
		}

		// pull the top off the stack
		s := p.stack[len(p.stack)-1]
		p.stack = p.stack[:len(p.stack)-1]

		// keep the original ordering when building up our subslice
		top = append([]any{s}, top...)

		// try to reduce with all our reducers
		var reduced bool
		before := p.nonTerminals
		top, p.nonTerminals, reduced = reduce.Reduce(top, p.nonTerminals, defaultField)

		// if we consumed some non terminals during the reduce it means we successfully reduced
		if reduced {
			// Reducers only pop from the end of nonTerminals, so the tokens they
			// consumed are the tail of the old stack.
			for _, tok := range before[len(p.nonTerminals):] {
				if isFieldOp(tok) {
					p.fieldOps--
				}
			}

			// If the reducer returned multiple elements and the first two are both expressions,
			// we need to inject an implicit AND between them (this happens when fuzzy/boost
			// does a partial reduction like [FUZZY(...), other-expr])
			if len(top) >= 2 {
				_, isFirstExpr := top[0].(*expr.Expression)
				_, isSecondExpr := top[1].(*expr.Expression)
				if isFirstExpr && isSecondExpr {
					// Insert AND between the two expressions: [expr1, expr2] -> [expr1, AND, expr2]
					implAnd := lex.Token{Typ: lex.TAnd, Val: "AND"}
					newTop := append([]any{top[0]}, implAnd)
					newTop = append(newTop, top[1:]...)
					top = newTop
					p.pushNonTerminal(implAnd)
				}
			}

			// if we successfully reduced re-add it to the top of the stack and return
			p.stack = append(p.stack, top...)
			return nil
		}
	}
}

func (p *parser) pushNonTerminal(tok lex.Token) {
	p.nonTerminals = append(p.nonTerminals, tok)
	if isFieldOp(tok) {
		p.fieldOps++
	}
}

// isFieldOp reports whether tok is a field operator: the ':' of field:value
// or the '=' of field=value. While one is on nonTerminals, its value is still
// being reduced.
func isFieldOp(tok lex.Token) bool {
	return tok.Typ == lex.TColon || tok.Typ == lex.TEqual
}

func parseLiteral(token lex.Token) (e any, err error) {
	// strip the delimiters (either " or ') and unescape \<delim> and \\.
	if token.Typ == lex.TQuoted {
		return expr.Lit(unescapePhrase(token.Val)), nil
	}

	// bare `null` keyword (case-insensitive) -> typed null literal.
	// Quoted "null" was already handled above; a wildcard like nul* is handled
	// below (we only match exact `null` modulo case here).
	if strings.EqualFold(token.Val, "null") {
		return expr.NULL(), nil
	}

	// if it is a regexp then parse it
	if token.Typ == lex.TRegexp {
		return expr.REGEXP(token.Val), nil
	}

	// attempt to parse it as an integer. Atoi only accepts decimal digits.
	ival, err := strconv.Atoi(token.Val)
	if err == nil {
		return expr.NumberLit(ival, token.Val), nil
	}

	// attempt to parse it as a float. ParseFloat also accepts inf, NaN, hex
	// floats like 0x1p3 and digit separators like 1_000; those stay terms
	// because only a plain decimal number is a number. A number too big for a
	// float64 fails to parse and stays a term, and so does one too small,
	// which ParseFloat rounds to 0.
	fval, err := strconv.ParseFloat(token.Val, 64)
	if err == nil && decimalNumber.MatchString(token.Val) && !underflows(token.Val, fval) {
		return expr.NumberLit(fval, token.Val), nil
	}

	// A term with an unescaped * or ? is a wildcard. It keeps its escapes,
	// which mark the characters it matches literally, as in fo\*bar*. A term
	// whose only * and ? are escaped, like fo\*, is a plain term.
	if wildcard.HasWildcard(token.Val) {
		return expr.WILD(escapePlus(token.Val)), nil
	}

	// Drop the escapes. An escaped backslash stays: foo\\bar is foo\bar.
	if strings.Contains(token.Val, `\`) {
		return expr.Lit(wildcard.Unescape(token.Val)), nil
	}

	return expr.Lit(token.Val), nil
}

// escapePlus escapes each unescaped + in a wildcard's text. The lexer keeps a
// + after the first character of a word, as in c++* or 1e+5*, and a + there
// is a literal character. In a pattern built in code, an unescaped + is a SIMILAR TO
// operator, so the parser's must be escaped to stay literal.
func escapePlus(text string) string {
	chars := wildcard.Split(text)
	for i, c := range chars {
		if c.Text == "+" {
			chars[i].Escaped = true
		}
	}
	return wildcard.Join(chars)
}

// decimalNumber matches a plain decimal number: an optional sign, digits with
// an optional fraction, and an optional exponent, such as 5, -1.5, 5., .5 or
// 1e-3. This is SQL's numeric literal syntax.
var decimalNumber = regexp.MustCompile(`^[+-]?([0-9]+\.?[0-9]*|\.[0-9]+)([eE][+-]?[0-9]+)?$`)

// underflows reports whether the decimal number text is not zero but parsed
// to f == 0, because it is too small for a float64, such as 1e-400.
func underflows(text string, f float64) bool {
	if f != 0 {
		return false
	}
	mantissa, _, _ := strings.Cut(strings.ToLower(text), "e")
	return strings.ContainsAny(mantissa, "123456789")
}

// isNumberLiteral reports whether parseLiteral turned a token into a number.
func isNumberLiteral(in any) bool {
	e, ok := in.(*expr.Expression)
	if !ok || e.Op != expr.Literal {
		return false
	}
	switch e.Left.(type) {
	case int, float64:
		return true
	default:
		return false
	}
}

// unescapePhrase strips the surrounding quote delimiters off a TQuoted token's
// raw value and unescapes only the delimiter itself and a literal backslash,
// e.g. \" -> " and \\ -> \. Any other backslash sequence is left untouched so
// that phrase content like a Windows path (C:\temp) round-trips unchanged.
func unescapePhrase(val string) string {
	delim := val[0]
	inner := val[1 : len(val)-1]

	var b strings.Builder
	b.Grow(len(inner))
	for i := 0; i < len(inner); i++ {
		if inner[i] == '\\' && i+1 < len(inner) && (inner[i+1] == delim || inner[i+1] == '\\') {
			i++
		}
		b.WriteByte(inner[i])
	}
	return b.String()
}
