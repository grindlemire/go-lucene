package lucene

// import (
// 	"fmt"

// 	"github.com/pkg/errors"
// )

// // Expr is a parsed lucene expression
// type Expr interface {
// 	String() string
// }

// // Literal is a literal value for an expression
// type Literal[T any] struct {
// 	Value T
// }

// func (l Literal[T]) String() string {
// 	return fmt.Sprintf("%v", l.Value)
// }

// // // Equals is an equals expression
// // type Equals struct {
// // 	Left  Expr
// // 	Right Expr
// // }

// // func (e Equals) String() string {
// // 	return fmt.Sprintf("%s = %s", e.Left, e.Right)
// // }

// // // And is a boolean and expression
// // type And struct {
// // 	Left  Expr
// // 	Right Expr
// // }

// // Parse will parse the string into lucene expressions
// func Parse(input string) (Expr, error) {
// 	// setup the lexer
// 	lex := lex(input)
// 	e := &Expression{}

// 	// while we have stuff to process
// 	//     process the next token
// 	//     convert the token to an expression, potentially parsing more
// 	tok := lex.nextItem()
// 	for tok.typ != tEOF {
// 		if tok.typ == tERR {
// 			return e, errors.Errorf("error parsing: %s", tok.val)
// 		}

// 		err := e.update(tok)
// 		if err != nil {
// 			return e, errors.Errorf("error crafting expression: %s", err)
// 		}
// 		tok = lex.nextItem()
// 	}
// 	return e, nil
// }

// // Operator is an operator for the expression
// type Operator int

// func (o Operator) String() string {
// 	return "="
// }

// // Operators
// const (
// 	EQUALS Operator = iota
// )

// // Expression is a search expression
// type Expression struct {
// 	left     Expr
// 	operator Operator
// 	right    Expr
// }

// func (e Expression) String() string {
// 	return fmt.Sprintf("%s %s %s", e.left, e.operator, e.right)
// }

// func isPrefix(tok token) bool {
// 	switch tok.typ {
// 	case tQUOTED:
// 		return true
// 	}

// 	return false
// }

// func (e *Expression) update(tok token) (err error) {
// 	if e == nil {
// 		e = &Expression{}
// 	}

// 	// if prefix(tok) {
// 	// 	return insertExpr(lookForward())
// 	// }

// 	// if expr(tok) {
// 	// 	return insertExpr()
// 	// }

// 	// if operator() {
// 	// 	return attachOperator()
// 	// }

// 	return nil

// 	// a way to do it
// 	// parsedExpr, err := parseExpr(tok)
// 	// if err != nil && err != errOperatorNotExpression {
// 	// 	return err
// 	// }

// 	// if err == errOperatorNotExpression {
// 	// 	parsedOperator, err := parseOperator(tok)
// 	// 	if err != nil {
// 	// 		return err
// 	// 	}

// 	// 	return e.attachOperator(parsedOperator)
// 	// }

// 	// // the the current expression is missing a left clause, then
// 	// // assume the token is for the left
// 	// if e.left == nil {
// 	// 	e.left = parsedExpr
// 	// 	return
// 	// }

// 	// if e.right == nil {
// 	// 	e.right = parsedExpr
// 	// }

// 	// return e.expand(parsedExpr)
// }

// func (e *Expression) attachOperator(o Operator) (err error) {
// 	if e.left == nil {
// 		return errors.New("cannot attach operator to expression without left clause")
// 	}

// 	e.operator = o
// 	return nil
// }

// func (e *Expression) expand(newTerm Expr) (err error) {
// 	e = &Expression{
// 		left: e,
// 	}
// 	return nil
// }

// var errOperatorNotExpression = errors.New("operator is not expression")

// func parseOperator(tok token) (o Operator, err error) {
// 	switch tok.typ {
// 	case tEQUAL:
// 		return EQUALS, nil
// 	default:
// 		return o, errors.Errorf("attempting to parse unsupported operator: [%v]", tok.typ)
// 	}
// }

// func parseExpr(tok token) (e Expr, err error) {
// 	switch tok.typ {
// 	case tLITERAL:
// 		return &Literal[string]{
// 			Value: tok.val,
// 		}, nil
// 	// case itemInt:
// 	// 	i, err := strconv.Atoi(tok.val)
// 	// 	if err != nil {
// 	// 		return e, err
// 	// 	}
// 	// 	return &Literal[int]{
// 	// 		Value: i,
// 	// 	}, nil
// 	case tEQUAL:
// 		return nil, errOperatorNotExpression
// 	default:
// 		return nil, errors.Errorf("trying to parse unsupported expression type: %s", tok)
// 	}
// }

// // func addTok(in Expr, tok item) (out Expr, err error) {
// // 	if in == nil {
// // 		return out, errors.Errorf("cannot add to nil expression")
// // 	}

// // 	if isLiteral(in) {
// // 		if tok.typ != itemEqual {
// // 			return out, errors.Errorf("existing expression is literal [%s] and incoming token [%s] is not a connector", in, tok)
// // 		}

// // 		return &Equals{
// // 			Left:  in,
// // 			Right: nil,
// // 		}, nil
// // 	}

// // 	if e, ok := in.(*Equals); ok && isLiteralTok(tok.typ) {
// // 		r, err := newExpr(tok)
// // 		if err != nil {
// // 			return out, err
// // 		}
// // 		e.Right = r
// // 		return e, nil
// // 	}

// // 	return out, errors.Errorf("unable to add [%s] to existing expression [%v]", tok, in)
// // }

// // func isLiteral(e Expr) bool {
// // 	switch e.(type) {
// // 	case *Literal[string], *Literal[int]:
// // 		return true
// // 	default:
// // 		return false
// // 	}
// // }

// // func isLiteralTok(typ itemType) bool {
// // 	switch typ {
// // 	case itemStr, itemInt:
// // 		return true
// // 	default:
// // 		return false
// // 	}
// // }

// // rules:
// // 1. if you have a literal you must have a connector next (=, in, >, etc.)
// // 2. if you have a connector you must have a compound next
// // 3. compounds may have compounds next
// // general layout for how this will be opened up to the user in the end
// // Term {
// // 	left: Term {
// // 		Left: Literal
// // 		Operator
// // 		Right: Literal
// // 	}
// // 	Operator
// // 	right: Term {
// // 		Left: Literal
// // 		Operator
// // 		Right: Literal
// // 	}
// // }

// // type termStateFn func(item) error

// // // Parser ...
// // type Parser struct {
// // 	tokens []item
// // }

// // func (p *Parser) term() termStateFn {
// // 	return func(tok item) error {
// // 		return nil
// // 	}
// // }

// // (a:v AND b:x) OR c:t
// // Two states:
// // 1. No Expression -> Create one
