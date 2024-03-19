package lex

import (
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"
)

const eof = -1

// Token is a parsed token from the input buffer sent to the lexer
type Token struct {
	Typ TokType // the type of the item
	pos int     // the position of the item in the string
	Val string  // the value of the item
}

// String is a string representation of a lex item
func (i Token) String() string {
	switch {
	case i.Typ == TErr:
		return i.Val
	case len(i.Val) > 10:
		return fmt.Sprintf("%.10q...", i.Val)
	}
	return fmt.Sprintf("%q", i.Val)
}

// precedence : > ) > + > - > ~ > ^ > NOT > AND > OR > (

// TokType is an enum of token types that can be parsed by the lexer. Order matters here for non terminals
// with a lower number meaning a higher precedence.
type TokType int

// types of tokens that can be parsed
const (
	// terminal characters
	TErr TokType = iota
	TLiteral
	TQuoted
	TRegexp

	// precedence of operators. Order matters here. This might need to be abstracted
	// to a grammar specific precedence but for now it is fine here.
	TEqual
	TGreater
	TLess
	TColon
	TPlus
	TMinus
	TTilde
	TCarrot
	TNot
	TAnd
	TOr
	TRParen
	TLParen

	// operators that do not have a set precedence because we specifically handle them
	// due to ambiguities in the grammar
	TLCurly
	TRCurly
	TTO
	TLSquare
	TRSquare

	// start and end operators
	TEOF
	TStart
)

var symbols = map[rune]TokType{
	'(': TLParen,
	')': TRParen,
	'[': TLSquare,
	']': TRSquare,
	'{': TLCurly,
	'}': TRCurly,
	':': TColon,
	'+': TPlus,
	'=': TEqual,
	'>': TGreater,
	'~': TTilde,
	'^': TCarrot,
	'<': TLess,
	// minus is not included because we have to special case it for negative numbers
	// '-': tMINUS,
}

var tokStrings = map[TokType]string{
	TErr:     "tERR",
	TLiteral: "tLITERAL",
	TQuoted:  "tQUOTED",
	TRegexp:  "tREGEXP",
	TEqual:   "tEQUAL",
	TLParen:  "tLPAREN",
	TRParen:  "tRPAREN",
	TAnd:     "tAND",
	TOr:      "tOR",
	TNot:     "tNOT",
	TLSquare: "tLSQUARE",
	TRSquare: "tRSQUARE",
	TLCurly:  "tLCURLY",
	TRCurly:  "tRCURLY",
	TTO:      "tTO",
	TColon:   "tCOLON",
	TPlus:    "tPLUS",
	TMinus:   "tMINUS",
	TGreater: "tGREATER",
	TLess:    "tLESS",
	TTilde:   "tTILDE",
	TCarrot:  "tCARROT",
	TEOF:     "tEOF",
	TStart:   "tSTART",
}

func (tt TokType) String() string {
	return tokStrings[tt]
}

// terminalTokens contains a map of terminal tokens.
// Uses empty struct value to conserve memory.
var terminalTokens = map[TokType]struct{}{
	TErr:     {},
	TLiteral: {},
	TQuoted:  {},
	TRegexp:  {},
	TEOF:     {},
}

// IsTerminal checks wether a specific token is a terminal token meaning
// it can't be matched in the grammar.
func IsTerminal(tok Token) bool {
	_, terminal := terminalTokens[tok.Typ]

	return terminal
}

// HasLessPrecedence checks if a current token has lower precedence than the next.
// There is a specific ordering in the iota (lower numbers = higher precedence) indicating
// whether the operator has more precedence or not.
func HasLessPrecedence(current Token, next Token) bool {
	// left associative. If we see another of the same type don't add onto the pile.
	// right associative would return true here.
	if current.Typ == next.Typ {
		return false
	}

	// lower numbers mean higher precedence
	return current.Typ > next.Typ
}

type tokenStateFn func(*Lexer) tokenStateFn

// Lexer is a lexer that will parse an input string into tokens for consumption by a
// grammar parser.
type Lexer struct {
	input string // the input to parse

	pos      int   // the position of the cursor
	start    int   // the start of the current token
	currItem Token // the current item being worked on
	atEOF    bool  // whether we have finished parsing the string or not
}

// Lex creates a lexer for an input string
func Lex(input string) *Lexer {
	return &Lexer{
		input: input,
		pos:   0,
		start: 0,
	}
}

// Next parses and returns just the next token in the input.
func (l *Lexer) Next() Token {
	// default to returning EOF
	l.currItem = Token{
		Typ: TEOF,
		pos: l.pos,
		Val: "EOF",
	}

	// run the state machine until we have a token
	for state := lexSpace; state != nil; {
		state = state(l)
	}

	return l.currItem
}

// Peek looks at the the next token but does not impact the lexer state
// note this is intentionally not a pointer because we don't want any changes to take affect here.
func (l Lexer) Peek() Token {
	if l.currItem.Typ == TEOF {
		return l.currItem
	}

	return l.Next()
}

// lexSpace is the first state that we always start with
func lexSpace(l *Lexer) tokenStateFn {
	for {
		switch l.next() {
		case eof:
			return nil
		case ' ', '\t', '\r', '\n':
			continue
		default:
			// transition to being in a value
			l.backup()
			return lexVal
		}
	}
}

func lexVal(l *Lexer) tokenStateFn {
	l.start = l.pos
	switch r := l.next(); {
	case isAlphaNumeric(r) || isWildcard(r) || isEscape(r):
		l.backup()
		return lexWord
	case isSymbol(r):
		return l.emit(symbols[r])
	// special case minus sign since it can be a negative number or a minus
	case r == '-':
		if !unicode.IsDigit(l.peek()) {
			return l.emit(TMinus)
		}
		l.backup()
		return lexWord

	case r == '"' || r == '\'':
		l.backup()
		return lexPhrase
	case r == '/':
		l.backup()
		return lexRegexp
	default:
		l.errorf("error parsing token [%s]", string(r))
	}
	return nil
}

func lexPhrase(l *Lexer) tokenStateFn {
	open := l.next()

	for {
		switch r := l.next(); {
		case isAlphaNumeric(r) || isWildcard(r) || isEscape(r):
			// do nothing
		case r == ' ' || r == '\t' || r == '\r' || r == '\n':
			// do nothing
		case r == open:
			return l.emit(TQuoted)
		case r == eof:
			return l.errorf("unterminated quote")
		}
	}
}

func lexRegexp(l *Lexer) tokenStateFn {
	// theoretically allow us to use anything to specify a regexp
	open := l.next()

	for {
		switch r := l.next(); {
		case isAlphaNumeric(r) || isWildcard(r):
			// do nothing
		case isEscape(r):
			l.next() // just ignore the next character
		case r == ' ' || r == '\t' || r == '\r' || r == '\n':
			// do nothing
		case r == open:
			return l.emit(TRegexp)
		case r == eof:
			return l.errorf("unterminated regexp")
		}
	}
}

func lexWord(l *Lexer) tokenStateFn {
loop:
	for {
		switch r := l.next(); {
		case isAlphaNumeric(r) || isWildcard(r) || r == '.' || r == '-':
			// do nothing
		case isEscape(r):
			l.next() // just ignore the next character
		default:
			l.backup()
			break loop
		}
	}

	switch strings.ToUpper(l.currWord()) {
	case "AND":
		return l.emit(TAnd)
	case "OR":
		return l.emit(TOr)
	case "NOT":
		return l.emit(TNot)
	case "TO":
		return l.emit(TTO)
	}
	return l.emit(TLiteral)
}

func (l *Lexer) currWord() string {
	return l.input[l.start:l.pos]
}

// toTok returns the item at the current input point with the specified type
// and advances the input.
func (l *Lexer) toTok(t TokType) Token {
	i := Token{
		Typ: t,
		pos: l.start,
		Val: l.input[l.start:l.pos],
	}
	// update the lexer's start for the next token to be the current position
	l.start = l.pos
	return i
}

// emit passes the trailing text as an item back to the parser.
func (l *Lexer) emit(t TokType) tokenStateFn {
	l.currItem = l.toTok(t)
	return nil
}

// next moves one rune forward in the input string and returns the consumed rune
func (l *Lexer) next() rune {
	if int(l.pos) >= len(l.input) {
		l.atEOF = true
		return eof
	}
	r, width := utf8.DecodeRuneInString(l.input[l.pos:])
	l.pos += width
	return r
}

// peek returns but does not consume the next rune in the input.
func (l *Lexer) peek() rune {
	r := l.next()
	l.backup()
	return r
}

// backup steps back one rune.
func (l *Lexer) backup() {
	if !l.atEOF && l.pos > 0 {
		_, width := utf8.DecodeLastRuneInString(l.input[:l.pos])
		l.pos -= width
	}
}

// errorf returns an error token and terminates the scan by passing
// back a nil pointer that will be the next state, terminating l.nextToken.
func (l *Lexer) errorf(format string, args ...any) tokenStateFn {
	l.currItem = Token{
		Typ: TErr,
		pos: l.start,
		Val: fmt.Sprintf(format, args...),
	}
	l.start = 0
	l.pos = 0
	l.input = l.input[:0]
	return nil
}

// isAlphaNumeric reports whether r is an alphabetic, digit, or underscore.
func isAlphaNumeric(r rune) bool {
	return r == '_' || unicode.IsLetter(r) || unicode.IsDigit(r)
}

// isWildcard checks whether the string contains any wildcard characters.
func isWildcard(r rune) bool {
	return r == '*' || r == '?'
}

// isSpace reports whether r is a space character.
func isSpace(r rune) bool {
	return r == ' ' || r == '\t' || r == '\r' || r == '\n'
}

// isEscape checks whether the character is an escape character
func isEscape(r rune) bool {
	return r == '\\'
}

// isSymbol checks whether the run is one of the reserved symbols
func isSymbol(r rune) bool {
	_, found := symbols[r]
	return found
}
