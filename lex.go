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

const (
	tERR tokType = iota
	tLITERAL
	tQUOTED
	tEQUAL
	tAND
	tOR
	tNOT
	tLPAREN
	tRPAREN
	tLSQUARE
	tRSQUARE
	tLCURLY
	tRCURLY
	tCOLON
	tPLUS
	tMINUS
	tGREATER
	tLESS
	tEOF
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
	'<': tLESS,
}

// STILL NEED TO SUPPORT
//- ~ (fuzzy searches)
//- ~10 (proximity searches)
//- f:foo^2 (boost a term)

func (tt tokType) String() string {
	return map[tokType]string{
		tERR:     "tERR",
		tLITERAL: "tLITERAL",
		tQUOTED:  "tQUOTED",
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
		tCOLON:   "tCOLON",
		tPLUS:    "tPLUS",
		tMINUS:   "tMINUS",
		tGREATER: "tGREATER",
		tLESS:    "tLESS",
		tEOF:     "tEOF",
	}[tt]
}

type tokenStateFn func(*lexer) tokenStateFn

type lexer struct {
	input string // the input to parse

	pos      int   // the position of the cursor
	start    int   // the start of the current token
	currItem token // the current item being worked on
	atEOF    bool  // whether we have finished parsing the string or not

}

func lex(input string) lexer {
	return lexer{
		input: input,
		pos:   0,
		start: 0,
	}
}

func (l *lexer) nextItem() token {
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

	// fmt.Printf("Finished parsing next token: %s\n", l.currItem)
	return l.currItem
}

func lexSpace(l *lexer) tokenStateFn {
	// fmt.Printf("Lexing space\n")
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
	// fmt.Printf("Lexing val\n")
	l.start = l.pos
	switch r := l.next(); {
	case isAlphaNumeric(r) || isWildcard(r) || isEscape(r):
		l.backup()
		return lexWord
	case isSymbol(r):
		return l.emit(symbols[r])
	case r == '"' || r == '\'':
		l.backup()
		return lexQuote
	default:
		l.errorf("error parsing token [%s]", string(r))
	}
	return nil
}

func lexQuote(l *lexer) tokenStateFn {
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

func lexWord(l *lexer) tokenStateFn {
	// fmt.Printf("Lexing word\n")
loop:
	for {
		switch r := l.next(); {
		case isAlphaNumeric(r) || isWildcard(r):
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
	}
	fmt.Printf("WORD: %s\n", l.currWord())
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
// back a nil pointer that will be the next state, terminating l.nextItem.
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
