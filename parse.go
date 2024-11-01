package lucene

import (
	"fmt"
	"reflect"
	"strconv"
	"strings"

	"github.com/grindlemire/go-lucene/internal/lex"
	"github.com/grindlemire/go-lucene/pkg/lucene/expr"
	"github.com/grindlemire/go-lucene/pkg/lucene/reduce"
)

type opt func(*parser)

// WithDefaultField sets the default field to equate literals to.
// For example a:b AND "c" will be parsed as a:b AND myfield:"c"
func WithDefaultField(field string) opt {
	return func(p *parser) {
		p.defaultField = field
	}
}

// Parse will parse a lucene expression string using a buffer and the shift reduce algorithm. The returned expression
// is an AST that can be rendered to a variety of different formats.
func Parse(input string, opts ...opt) (e *expr.Expression, err error) {
	p := &parser{
		lex:          lex.Lex(input),
		stack:        []any{},
		nonTerminals: []lex.Token{{Typ: lex.TStart}},
	}

	for _, opt := range opts {
		opt(p)
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
	lex          *lex.Lexer
	stack        []any
	nonTerminals []lex.Token

	defaultField string
}

func (p *parser) parse() (e *expr.Expression, err error) {
	for {
		next := p.lex.Peek()
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

			// edge case for a single literal in the expression and a default field specified
			if final.Op == expr.Literal && p.defaultField != "" {
				final = expr.Expr(p.defaultField, expr.Equals, final.Left)
			}

			return final, nil
		}

		if p.shouldShift(next) {
			tok := p.shift()
			if lex.IsTerminal(tok) {
				// if we have a terminal parse it and put it on the stack
				lit, err := parseLiteral(tok)
				if err != nil {
					return e, err
				}

				// we should always check if the current top of the stack is another token
				// if it isn't then we have an implicit AND we need to inject.
				if len(p.stack) > 0 {
					_, isTopToken := p.stack[len(p.stack)-1].(lex.Token)
					if !isTopToken {
						implAnd := lex.Token{Typ: lex.TAnd, Val: "AND"}
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

				p.stack = append(p.stack, lit)
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

	// if we are ever attempting to move past a subexpr we need to parse it before moving on
	if anyClosingBracket(curr) {
		return false
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

func (p *parser) shouldAccept(next lex.Token) bool {
	return len(p.stack) == 1 &&
		next.Typ == lex.TEOF
}

func (p *parser) reduce() (err error) {
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
		top, p.nonTerminals, reduced = reduce.Reduce(top, p.nonTerminals, p.defaultField)

		// if we consumed some non terminals during the reduce it means we successfully reduced
		if reduced {
			// if we successfully reduced re-add it to the top of the stack and return
			p.stack = append(p.stack, top...)
			return nil
		}
	}
}

func parseLiteral(token lex.Token) (e any, err error) {
	// if it is a quote then remove escape
	if token.Typ == lex.TQuoted {
		return expr.Lit(strings.ReplaceAll(token.Val, "\"", "")), nil
	}

	// if it is a regexp then parse it
	if token.Typ == lex.TRegexp {
		return expr.REGEXP(token.Val), nil
	}

	// attempt to parse it as an integer
	ival, err := strconv.Atoi(token.Val)
	if err == nil {
		return expr.Lit(ival), nil
	}

	// attempt to parse it as a float
	fval, err := strconv.ParseFloat(token.Val, 64)
	if err == nil {
		return expr.Lit(fval), nil
	}

	// if it contains unescaped wildcards then it is a wildcard string
	if strings.ContainsAny(token.Val, "*?") {
		return expr.WILD(token.Val), nil
	}

	// if it contains an escape string then strip it out now
	if strings.Contains(token.Val, `\`) {
		return expr.Lit(strings.ReplaceAll(token.Val, `\`, "")), nil
	}

	return expr.Lit(token.Val), nil
}
