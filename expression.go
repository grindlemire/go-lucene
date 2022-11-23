package lucene

import (
	"fmt"
	"strconv"

	"github.com/pkg/errors"
)

// Expr is a parsed lucene expression
type Expr interface{}

// Literal is a literal value for an expression
type Literal[T any] struct {
	Value T
}

func (l Literal[T]) String() string {
	return fmt.Sprintf("%v", l.Value)
}

// Equals is an equals expression
type Equals struct {
	Left  Expr
	Right Expr
}

func (e Equals) String() string {
	return fmt.Sprintf("%s = %s", e.Left, e.Right)
}

// And is a boolean and expression
type And struct {
	Left  Expr
	Right Expr
}

// Parse will parse the string into lucene expressions
func Parse(input string) (e Expr, err error) {
	// setup the lexer
	lex := lex(input)

	// while we have stuff to process
	//     process the next token
	//     convert the token to an expression, potentially parsing more
	tok := lex.nextItem()
	for tok.typ != itemEOF {
		if tok.typ == itemErr {
			return e, errors.Errorf("error parsing: %s", tok.val)
		}

		e, err = updateExpr(e, tok)
		if err != nil {
			return e, errors.Errorf("error crafting expression: %s", err)
		}
		tok = lex.nextItem()
	}
	return e, nil
}

func updateExpr(in Expr, tok item) (out Expr, err error) {
	if in == nil {
		return newExpr(tok)
	}
	return addToExpr(in, tok)
}

func newExpr(tok item) (e Expr, err error) {
	switch tok.typ {
	case itemStr:
		return &Literal[string]{
			Value: tok.val,
		}, nil
	case itemInt:
		i, err := strconv.Atoi(tok.val)
		if err != nil {
			return e, err
		}
		return &Literal[int]{
			Value: i,
		}, nil
	default:
		return e, errors.Errorf("unexpected expression: %s", tok)
	}
}

func addToExpr(in Expr, tok item) (out Expr, err error) {
	if in == nil {
		return out, errors.Errorf("cannot add to nil expression")
	}

	if isLiteral(in) {
		if tok.typ != itemEqual {
			return out, errors.Errorf("existing expression is literal [%s] and incoming token [%s] is not a connector", in, tok)
		}

		return &Equals{
			Left:  in,
			Right: nil,
		}, nil
	}

	if e, ok := in.(*Equals); ok && isLiteralTok(tok.typ) {
		r, err := newExpr(tok)
		if err != nil {
			return out, err
		}
		e.Right = r
		return e, nil
	}

	return out, errors.Errorf("unable to add [%s] to existing expression [%v]", tok, in)
}

func isLiteral(e Expr) bool {
	switch e.(type) {
	case *Literal[string], *Literal[int]:
		return true
	default:
		return false
	}
}

func isLiteralTok(typ itemType) bool {
	switch typ {
	case itemStr, itemInt:
		return true
	default:
		return false
	}
}

// rules:
// 1. if you have a literal you must have a connector next (=, in, >, etc.)
// 2. if you have a connector you must have a compound next
// 3. compounds may have compounds next
// general layout for how this will be opened up to the user in the end
// Term {
// 	left: Term {
// 		Left: Literal
// 		Operator
// 		Right: Literal
// 	}
// 	Operator
// 	right: Term {
// 		Left: Literal
// 		Operator
// 		Right: Literal
// 	}
// }
