package lucene

import (
	"fmt"
	"unicode"
	"unicode/utf8"
)

// token is an token that the lexer parsed from the source
type token struct {
	typ tokType // the type of the item
	pos int     // the position of the item in the string
	val string  // the value of the item
}

// String is a string representation of a lex item
func (i token) String() string {
	switch {
	case i.typ == tERR:
		return i.val
	case len(i.val) > 10:
		return fmt.Sprintf("%.10q...", i.val)
	}
	return fmt.Sprintf("%q", i.val)
}

type tokType int

// precedance : > ) > + > - > ~ > ^ > NOT > AND > OR > (

const (
	// terminal characters
	tERR tokType = iota
	tLITERAL
	tQUOTED
	tREGEXP

	// precedance of operators. Order matters here
	tEQUAL
	tCOLON
	tPLUS
	tMINUS
	tTILDE
	tCARROT
	tNOT
	tAND
	tOR
	tRPAREN
	tLPAREN

	// TODO figure out how these fit in here
	tLCURLY
	tRCURLY
	tTO
	tLSQUARE
	tRSQUARE
	tGREATER
	tLESS

	// start and end operators
	tEOF
	tSTART
)

var symbols = map[rune]tokType{
	'(': tLPAREN,
	')': tRPAREN,
	'[': tLSQUARE,
	']': tRSQUARE,
	'{': tLCURLY,
	'}': tRCURLY,
	':': tCOLON,
	'+': tPLUS,
	'-': tMINUS,
	'=': tEQUAL,
	'>': tGREATER,
	'~': tTILDE,
	'^': tCARROT,
	'<': tLESS,
}

func (tt tokType) String() string {
	return map[tokType]string{
		tERR:     "tERR",
		tLITERAL: "tLITERAL",
		tQUOTED:  "tQUOTED",
		tREGEXP:  "tREGEXP",
		tEQUAL:   "tEQUAL",
		tLPAREN:  "tLPAREN",
		tRPAREN:  "tRPAREN",
		tAND:     "tAND",
		tOR:      "tOR",
		tNOT:     "tNOT",
		tLSQUARE: "tLSQUARE",
		tRSQUARE: "tRSQUARE",
		tLCURLY:  "tLCURLY",
		tRCURLY:  "tRCURLY",
		tTO:      "tTO",
		tCOLON:   "tCOLON",
		tPLUS:    "tPLUS",
		tMINUS:   "tMINUS",
		tGREATER: "tGREATER",
		tLESS:    "tLESS",
		tTILDE:   "tTILDE",
		tCARROT:  "tCARROT",
		tEOF:     "tEOF",
		tSTART:   "tSTART",
	}[tt]
}

func isTerminal(tok token) bool {
	return map[tokType]bool{
		tERR:     true,
		tLITERAL: true,
		tQUOTED:  true,
		tREGEXP:  true,
		tEQUAL:   false,
		tLPAREN:  false,
		tRPAREN:  false,
		tAND:     false,
		tOR:      false,
		tNOT:     false,
		tLSQUARE: false,
		tRSQUARE: false,
		tLCURLY:  false,
		tRCURLY:  false,
		tTO:      false,
		tCOLON:   false,
		tPLUS:    false,
		tMINUS:   false,
		tGREATER: false,
		tLESS:    false,
		tTILDE:   false,
		tCARROT:  false,
		tEOF:     true,
		tSTART:   false,
	}[tok.typ]
}

func hasLessPrecedance(current token, next token) bool {
	// left associative. If we see another one just do our current one now
	if current.typ == next.typ {
		return false
	}

	// fmt.Printf("%s > %s | %d > %d\n", current.typ, next.typ, int(current.typ), int(next.typ))
	// lower numbers mean higher precedance
	return current.typ > next.typ
}

type tokenStateFn func(*lexer) tokenStateFn

type lexer struct {
	input string // the input to parse

	pos      int   // the position of the cursor
	start    int   // the start of the current token
	currItem token // the current item being worked on
	atEOF    bool  // whether we have finished parsing the string or not

}

func lex(input string) *lexer {
	return &lexer{
		input: input,
		pos:   0,
		start: 0,
	}
}

func (l *lexer) nextToken() token {
	// default to returning EOF
	l.currItem = token{
		typ: tEOF,
		pos: l.pos,
		val: "EOF",
	}

	// run the state machine until we have a token
	for state := lexSpace; state != nil; {
		state = state(l)
	}

	return l.currItem
}

// note this is intentionally not a pointer because we don't want any changes to
// take affect here.
func (l lexer) peekNextToken() token {
	if l.currItem.typ == tEOF {
		return l.currItem
	}

	return l.nextToken()
}

func lexSpace(l *lexer) tokenStateFn {
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

func lexVal(l *lexer) tokenStateFn {
	l.start = l.pos
	switch r := l.next(); {
	case isAlphaNumeric(r) || isWildcard(r) || isEscape(r):
		l.backup()
		return lexWord
	case isSymbol(r):
		return l.emit(symbols[r])
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

func lexPhrase(l *lexer) tokenStateFn {
	open := l.next()

	for {
		switch r := l.next(); {
		case isAlphaNumeric(r) || isWildcard(r) || isEscape(r):
			// do nothing
		case r == ' ' || r == '\t' || r == '\r' || r == '\n':
			// do nothing
		case r == open:
			return l.emit(tQUOTED)
		case r == eof:
			return l.errorf("unterminated quote")
		}
	}
}

func lexRegexp(l *lexer) tokenStateFn {
	// theoretically allow us to use anything to specify a regexp
	open := l.next()

	for {
		switch r := l.next(); {
		case isAlphaNumeric(r) || isWildcard(r) || isEscape(r):
			// do nothing
		case r == ' ' || r == '\t' || r == '\r' || r == '\n':
			// do nothing
		case r == open:
			return l.emit(tREGEXP)
		case r == eof:
			return l.errorf("unterminated regexp")
		}
	}
}

func lexWord(l *lexer) tokenStateFn {
loop:
	for {
		switch r := l.next(); {
		case isAlphaNumeric(r) || isWildcard(r) || r == '.':
			// do nothing
		case isEscape(r):
			l.next() // just ignore the next character
		default:
			l.backup()
			break loop
		}
	}

	switch l.currWord() {
	case "AND":
		return l.emit(tAND)
	case "OR":
		return l.emit(tOR)
	case "NOT":
		return l.emit(tNOT)
	case "TO":
		return l.emit(tTO)
	}
	return l.emit(tLITERAL)
}

func (l *lexer) currWord() string {
	return l.input[l.start:l.pos]
}

// toTok returns the item at the current input point with the specified type
// and advances the input.
func (l *lexer) toTok(t tokType) token {
	i := token{
		typ: t,
		pos: l.start,
		val: l.input[l.start:l.pos],
	}
	// update the lexer's start for the next token to be the current position
	l.start = l.pos
	return i
}

// emit passes the trailing text as an item back to the parser.
func (l *lexer) emit(t tokType) tokenStateFn {
	l.currItem = l.toTok(t)
	return nil
}

const eof = -1

// next moves one rune forward in the input string and returns the consumed rune
func (l *lexer) next() rune {
	if int(l.pos) >= len(l.input) {
		l.atEOF = true
		return eof
	}
	r, width := utf8.DecodeRuneInString(l.input[l.pos:])
	l.pos += width
	return r
}

// peek returns but does not consume the next rune in the input.
func (l *lexer) peek() rune {
	r := l.next()
	l.backup()
	return r
}

// backup steps back one rune.
func (l *lexer) backup() {
	if !l.atEOF && l.pos > 0 {
		_, width := utf8.DecodeLastRuneInString(l.input[:l.pos])
		l.pos -= width
	}
}

// isAlphaNumeric reports whether r is an alphabetic, digit, or underscore.
func isAlphaNumeric(r rune) bool {
	return r == '_' || unicode.IsLetter(r) || unicode.IsDigit(r)
}

func isWildcard(r rune) bool {
	return r == '*' || r == '?'
}

// isSpace reports whether r is a space character.
func isSpace(r rune) bool {
	return r == ' ' || r == '\t' || r == '\r' || r == '\n'
}

func isEscape(r rune) bool {
	return r == '\\'
}

func isSymbol(r rune) bool {
	_, found := symbols[r]
	return found
}

// errorf returns an error token and terminates the scan by passing
// back a nil pointer that will be the next state, terminating l.nextToken.
func (l *lexer) errorf(format string, args ...any) tokenStateFn {
	l.currItem = token{
		typ: tERR,
		pos: l.start,
		val: fmt.Sprintf(format, args...),
	}
	l.start = 0
	l.pos = 0
	l.input = l.input[:0]
	return nil
}
