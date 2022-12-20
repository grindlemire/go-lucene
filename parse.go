package lucene

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/grindlemire/go-lucene/expr"
)

// Parse will parse the lucene grammar out of a string
func Parse(input string) (e expr.Expression, err error) {
	p := parser{
		lex:    lex(input),
		tokIdx: -1,
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
	// keep an internal representation of tokens in case we need to backtrack
	tokIdx int
	tokens []token
	lex    *lexer

	hasMust    bool
	hasMustNot bool

	// this tracks how many open subexpressions we are in. It must be 0 at the end of the parse.
	subExpressionCount int
}

func (p *parser) next() (t token) {
	if p.tokIdx < len(p.tokens)-1 {
		p.tokIdx++
		t = p.tokens[p.tokIdx]

		return t
	}

	// if we have parsed all existing tokens get another
	t = p.lex.nextToken()
	p.tokens = append(p.tokens, t)
	p.tokIdx++
	return t

}

func (p *parser) backup() {
	if p.tokIdx < 0 {
		return
	}
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

func (p *parser) parse() (e expr.Expression, err error) {
	for {
		token := p.next()
		if token.typ == tEOF {
			return e, p.checkExpressionStack()
		}

		if !canAcceptNextToken(e, token) {
			p.backup()
			sub, err := p.parse()
			if err != nil {
				return e, err
			}

			return e.Insert(sub)
		}

		switch token.typ {
		case tERR:
			return e, errors.New(token.val)

		// literal value:
		// 		- we parse the literal to a real type rather than a string representation
		// 		  and then transition the expression state based on seeing a literal.
		case tLITERAL:
			parsed, err := parseLiteral(token)
			if err != nil {
				return e, fmt.Errorf("unable to parse literal %w", err)
			}
			if e == nil {
				e = parsed
				continue // break out of switch and parse next token
			}

			e, err = e.Insert(parsed)
			if err != nil {
				return e, fmt.Errorf("unable to insert literal into expression: %w", err)
			}

		// quoted value:
		// 		- we make this quoted value a literal string and ignore keywords and whitespace
		case tQUOTED:
			// strip the quotes off because we don't need them
			val := strings.ReplaceAll(token.val, "\"", "")
			literal := &expr.Literal{
				Value: val,
			}

			if e == nil {
				e = literal
				continue // breaks out of the switch and parse next token
			}

			e, err = e.Insert(literal)
			if err != nil {
				return e, fmt.Errorf("unable to insert quoted string into expression: %w", err)
			}

		// regexp value:
		// 	- we make this regexp value a literal string and ignore everything in it, much like a quote
		case tREGEXP:
			// strip the quotes off because we don't need them
			val := strings.ReplaceAll(token.val, "/", "")
			literal := &expr.RegexpLiteral{
				Literal: expr.Literal{val},
			}

			if e == nil {
				e = literal
				continue // breaks out of the switch and parse next token
			}

			e, err = e.Insert(literal)
			if err != nil {
				return e, fmt.Errorf("unable to insert quoted string into expression: %w", err)
			}

		// equal operator:
		//		- if we see an equal we enforce that we have literals and transition the
		// 		  the expression state to handle the equal.
		case tEQUAL, tCOLON:
			if e == nil {
				return e, errors.New("invalid syntax: can't start expression with '= or :'")
			}

			// this is a hack but idk how to do it otherwise. The must and must nots must only
			// apply to the equals directly following them
			e, err = e.Insert(&expr.Equals{IsMust: p.hasMust, IsMustNot: p.hasMustNot})
			if err != nil {
				return e, fmt.Errorf("error updating expression with equals token: %w", err)
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

			not := &expr.Not{
				Sub: sub,
			}

			if e == nil {
				e = not
				break
			}
			e.Insert(not)
		// boolean operators:
		//		- these just wrap the existing terms
		case tAND:
			and := &expr.And{
				Left: e,
			}
			right, err := p.parse()
			if err != nil {
				return e, fmt.Errorf("unable to build AND clause: %w", err)
			}
			and.Right = right
			return and, nil
		case tOR:
			or := &expr.Or{
				Left: e,
			}
			right, err := p.parse()
			if err != nil {
				return e, fmt.Errorf("unable to build AND clause: %w", err)
			}
			or.Right = right
			return or, nil

		// subexpressions
		// 		- if you see a left paren then recursively parse the expression.
		// 		- if you see a right paren we must be done with the current recursion
		case tLPAREN:
			p.updateExpressionStack(token.val)
			sub, err := p.parse()
			if err != nil {
				return e, fmt.Errorf("unable to parse sub-expression: %w", err)
			}
			if e != nil {
				e, err = e.Insert(sub)
				if err != nil {
					return e, err
				}
				break
			}

			e = sub
		case tRPAREN:
			p.updateExpressionStack(token.val)
			if p.subExpressionCount < 0 {
				return e, errors.New("unbalanced closing paren")
			}
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
				return e, fmt.Errorf("unable to parse inclusive range: %w", err)
			}
			// we are inclusive so update that here
			r, ok := sub.(*expr.Range)
			if !ok {
				return e, errors.New("brackets must surround a range query (hint: use the TO operator in the brackets)")
			}
			r.Inclusive = true
			e, err = e.Insert(r)
			if err != nil {
				return e, err
			}
		case tLCURLY:
			if e == nil {
				return e, errors.New("unable to insert range into empty expression")
			}
			sub, err := p.parse()
			if err != nil {
				return e, fmt.Errorf("unable to parse inclusive range: %w", err)
			}
			// we are inclusive so update that here
			r, ok := sub.(*expr.Range)
			if !ok {
				return e, errors.New("brackets must surround a range query (hint: use the TO operator in the brackets)")
			}
			r.Inclusive = false
			e, err = e.Insert(r)
			if err != nil {
				return e, err
			}
		case tTO:
			e, err = (&expr.Range{}).Insert(e)
			if err != nil {
				return nil, err
			}
		case tRSQUARE, tRCURLY:
			return e, nil

		// must and must not operators
		// 		- if we see a plus or minus then we need to apply it to the next term only
		case tPLUS:
			p.hasMust = true
		case tMINUS:
			p.hasMustNot = true

		// boost operator
		//     - if we see a carrot we get the boost term and wrap left term in the boost
		case tCARROT:
			next := p.next()

			if next.typ != tLITERAL {
				return e, errors.New("term boost must be follow by positive number")
			}

			f, err := toPositiveFloat(next.val)
			if err != nil {
				return e, fmt.Errorf("not able to parse boost number: %w", err)
			}

			e, err = wrapInBoost(e, f)
			if err != nil {
				return e, fmt.Errorf("unable to wrap expression in boost: %w", err)
			}

		// fuzzy search operator
		//     - if we see a tilde try to fuzzy try to wrap the left term in a fuzzy search with an optional edit distance
		//     - according to https://lucene.apache.org/core/7_3_1/core/org/apache/lucene/search/FuzzyQuery.html#defaultMinSimilarity
		//       the minSimilarity rating is deprecated so this can just be an edit distance.
		case tTILDE:
			next := p.next()

			if next.typ != tLITERAL {
				p.backup()
				e, err = wrapInFuzzy(e, 1)
				if err != nil {
					return e, fmt.Errorf("not able to wrap expression in fuzzy search: %w", err)
				}
				continue
			}

			i, err := toPositiveInt(next.val)
			if err != nil {
				return e, fmt.Errorf("not able to parse fuzzy distance: %w", err)
			}

			e, err = wrapInFuzzy(e, i)
			if err != nil {
				return e, fmt.Errorf("unable to wrap expression in boost: %w", err)
			}
		}

	}
}

func (p *parser) updateExpressionStack(s string) {
	if s == "(" {
		p.subExpressionCount++
		return
	}

	p.subExpressionCount--
	return
}

func (p *parser) checkExpressionStack() error {
	if p.subExpressionCount != 0 {
		return fmt.Errorf("unterminated paren")
	}

	return nil
}

func canAcceptNextToken(curr expr.Expression, token token) bool {
	if curr == nil {
		return true
	}
	switch cast := curr.(type) {
	case *expr.Literal, *expr.WildLiteral, *expr.Range, *expr.RegexpLiteral:
		return true
	case *expr.Equals:
		if cast.Value == nil {
			return token.typ == tLITERAL ||
				token.typ == tQUOTED ||
				token.typ == tREGEXP ||
				token.typ == tLCURLY ||
				token.typ == tLSQUARE ||
				token.typ == tLPAREN
		}
		return token.typ == tAND ||
			token.typ == tOR ||
			token.typ == tCARROT ||
			token.typ == tTILDE ||
			token.typ == tRPAREN
	default:
		return token.typ == tAND ||
			token.typ == tOR ||
			token.typ == tRPAREN ||
			token.typ == tCARROT ||
			token.typ == tTILDE
	}
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

func parseLiteral(token token) (e expr.Expression, err error) {
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

func wrapInBoost(e expr.Expression, power float32) (expr.Expression, error) {
	if e == nil {
		return e, errors.New("carrot must follow another expression")
	}

	e = &expr.Boost{
		Sub:   e,
		Power: power,
	}
	return e, nil
}

func wrapInFuzzy(e expr.Expression, distance int) (expr.Expression, error) {
	if e == nil {
		return e, errors.New("carrot must follow another expression")
	}

	e = &expr.Fuzzy{
		Sub:      e,
		Distance: distance,
	}
	return e, nil
}
