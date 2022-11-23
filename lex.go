package lucene

import (
	"fmt"
	"unicode"
	"unicode/utf8"
)

// item is an item that the lexer parsed from the source
type item struct {
	typ itemType // the type of the item
	pos int      // the position of the item in the string
	val string   // the value of the item
}

// String is a string representation of a lex item
func (i item) String() string {
	switch {
	case i.typ == itemErr:
		return i.val
	case len(i.val) > 10:
		return fmt.Sprintf("%.10q...", i.val)
	}
	return fmt.Sprintf("%q", i.val)
}

type itemType int

const (
	itemErr itemType = iota
	itemInt
	itemStr
	itemQuote
	itemEqual
	itemEOF
)

type stateFn func(*lexer) stateFn

type lexer struct {
	input string // the input to parse

	pos      int  // the position of the cursor
	start    int  // the start of the current token
	inVal    bool // whether we are currently in a value or not
	currItem item // the current item being worked on
	atEOF    bool // whether we have finished parsing the string or not

}

func lex(input string) lexer {
	return lexer{
		input: input,
		pos:   0,
		start: 0,
	}
}

func (l *lexer) nextItem() item {
	// default to returning EOF
	l.currItem = item{
		typ: itemEOF,
		pos: l.pos,
		val: "EOF",
	}

	// default to consuming spaces unless we are actively
	// parsing a value
	state := lexSpace
	if l.inVal {
		state = lexVal
	}

	// run the state machine until we have a token
	for {
		state = state(l)
		if state == nil {
			// fmt.Printf("Finished parsing next token: %s\n", l.currItem)
			return l.currItem
		}
	}
}

func lexSpace(l *lexer) stateFn {
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
			l.inVal = true
			return lexVal
		}
	}
}

func lexVal(l *lexer) stateFn {
	// fmt.Printf("Lexing val\n")
	l.start = l.pos
	switch r := l.next(); {
	case r == '=':
		l.inVal = false
		return l.emit(itemEqual)
	case isAlphaNumeric(r):
		l.backup()
		return lexWord
	default:
		l.errorf("error parsing token [%s]", string(r))
	}
	return nil
}

func lexWord(l *lexer) stateFn {
	// fmt.Printf("Lexing word\n")
	for {
		switch r := l.next(); {
		case isAlphaNumeric(r):
			// do nothing
		default:
			l.backup()
			l.inVal = false
			return l.emit(itemStr)
		}
	}
}

// assembleItem returns the item at the current input point with the specified type
// and advances the input.
func (l *lexer) assembleItem(t itemType) item {
	i := item{
		typ: t,
		pos: l.start,
		val: l.input[l.start:l.pos],
	}
	// update the lexer's start for the next token to be the current position
	l.start = l.pos
	return i
}

// emit passes the trailing text as an item back to the parser.
func (l *lexer) emit(t itemType) stateFn {
	l.currItem = l.assembleItem(t)
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
	// fmt.Printf("Decoding [%s] | left: [%s]\n", string(r), l.input[l.pos:])
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

// isSpace reports whether r is a space character.
func isSpace(r rune) bool {
	return r == ' ' || r == '\t' || r == '\r' || r == '\n'
}

// errorf returns an error token and terminates the scan by passing
// back a nil pointer that will be the next state, terminating l.nextItem.
func (l *lexer) errorf(format string, args ...any) stateFn {
	l.currItem = item{
		typ: itemErr,
		pos: l.start,
		val: fmt.Sprintf(format, args...),
	}
	l.start = 0
	l.pos = 0
	l.input = l.input[:0]
	return nil
}
