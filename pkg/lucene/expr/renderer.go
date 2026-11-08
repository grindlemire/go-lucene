package expr

import (
	"fmt"
	"math"
	"slices"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/grindlemire/go-lucene/internal/wildcard"
)

type renderer func(e *Expression, verbose bool) string

var renderers = map[Operator]renderer{
	Equals:    renderField,
	And:       renderBasic,
	Or:        renderBasic,
	Not:       renderWrapper,
	Range:     renderRange,
	Must:      renderMust,
	MustNot:   renderMustNot,
	Boost:     renderBoost,
	Fuzzy:     renderFuzzy,
	Literal:   renderLiteral,
	Wild:      renderLiteral,
	Regexp:    renderLiteral,
	Greater:   renderField,
	Less:      renderField,
	GreaterEq: renderField,
	LessEq:    renderField,
	Like:      renderField,
	In:        renderField,
	List:      renderList,
	Null:      renderNullLiteral,
}

// compareSymbols are written between the colon and the value, as in a:>5.
var compareSymbols = map[Operator]string{
	Greater:   ">",
	Less:      "<",
	GreaterEq: ">=",
	LessEq:    "<=",
}

// renderField prints the operators that the parser builds from field:value.
// LIKE is a:fo* or a:/re/, IN is a:(b OR c) and a comparison is a:>5.
func renderField(e *Expression, verbose bool) string {
	if verbose {
		if e.Op == Equals {
			return fmt.Sprintf("%#v:%#v", e.Left, e.Right)
		}
		return renderBasic(e, true)
	}
	// The drivers read a string literal on the right of a LIKE as a pattern,
	// so it prints as a wildcard term. As a phrase it would parse back as an
	// exact match.
	if r, ok := e.Right.(*Expression); ok && e.Op == Like && r.Op == Literal {
		if s, ok := r.Left.(string); ok {
			return fmt.Sprintf("%s:%s", e.Left, wildcardTerm(s))
		}
	}
	return fmt.Sprintf("%s:%s%s", e.Left, compareSymbols[e.Op], fieldValue(e.Right))
}

// fieldValue prints the value after "field:". A single term or an IN list
// prints as is. Anything else was a group in the query and keeps its
// parentheses: "a:b:c" does not parse and "a:b AND c" is (a:b) AND c.
func fieldValue(in any) string {
	if e, ok := in.(*Expression); ok {
		switch e.Op {
		case Literal, Wild, Regexp, Null, List:
		default:
			return fmt.Sprintf("(%s)", e)
		}
	}
	return fmt.Sprintf("%s", in)
}

func renderBasic(e *Expression, verbose bool) string {
	if verbose {
		return fmt.Sprintf("(%#v) %s (%#v)", e.Left, toString[e.Op], e.Right)
	}
	return fmt.Sprintf("%s %s %s", binaryOperand(e.Left, e.Op, false), toString[e.Op], binaryOperand(e.Right, e.Op, true))
}

// binds ranks how tightly an operator holds its operands. The parser binds AND
// tighter than OR and groups a chain from the left, so "a OR b AND c" is
// a OR (b AND c) and "a AND b AND c" is (a AND b) AND c.
func binds(op Operator) int {
	switch op {
	case Or:
		return 1
	case And:
		return 2
	default:
		return 3
	}
}

// binaryOperand wraps an AND or OR operand in parentheses only where the
// parser would group it differently without them: on the left when it binds
// looser than its parent, on the right when it binds looser or the same.
func binaryOperand(in any, parent Operator, right bool) string {
	if e, ok := in.(*Expression); ok && (e.Op == And || e.Op == Or) {
		child, outer := binds(e.Op), binds(parent)
		if child < outer || (right && child == outer) {
			return fmt.Sprintf("(%s)", e)
		}
	}
	return fmt.Sprintf("%s", in)
}

var (
	// prefixOps need parentheses under + and -. "--a:b" does not parse and
	// "-a:b^2" is BOOST(MUST_NOT(a:b)).
	prefixOps = []Operator{And, Or, Not, Must, MustNot, Boost, Fuzzy}
	// postfixOps need parentheses under ^ and ~. A prefixed term needs none:
	// "-a:b^2" already parses as BOOST(MUST_NOT(a:b)).
	postfixOps = []Operator{And, Or, Not, Boost, Fuzzy}
)

func wrap(in any, ops []Operator) string {
	if e, ok := in.(*Expression); ok {
		for _, op := range ops {
			if e.Op == op {
				return fmt.Sprintf("(%s)", e)
			}
		}
	}
	return fmt.Sprintf("%s", in)
}

func renderWrapper(e *Expression, verbose bool) string {
	if verbose {
		return fmt.Sprintf("%s(%#v)", toString[e.Op], e.Left)
	}
	return fmt.Sprintf("%s(%s)", toString[e.Op], e.Left)
}

func renderMustNot(e *Expression, verbose bool) string {
	if verbose {
		return fmt.Sprintf("%s(%#v)", toString[e.Op], e.Left)
	}
	s := wrap(e.Left, prefixOps)
	// A minus right before a digit, or before a dot and a digit, lexes as
	// part of a negative number, so -5 and -.5 would parse as numbers rather
	// than MUST_NOT(5) and MUST_NOT(.5).
	if r, _ := utf8.DecodeRuneInString(strings.TrimPrefix(s, ".")); unicode.IsDigit(r) {
		s = "(" + s + ")"
	}
	return "-" + s
}

func renderMust(e *Expression, verbose bool) string {
	if verbose {
		return fmt.Sprintf("%s(%#v)", toString[e.Op], e.Left)
	}
	return fmt.Sprintf("+%s", wrap(e.Left, prefixOps))
}

func renderBoost(e *Expression, verbose bool) string {
	if verbose {
		if e.boostPower > 1 {
			return fmt.Sprintf("%s(%#v^%.1f)", toString[e.Op], e.Left, e.boostPower)
		}

		return fmt.Sprintf("%s(%#v)", toString[e.Op], e.Left)
	}

	// A bare ^ parses as a power of 1, so any other power is written out.
	if e.boostPower != 1 {
		return fmt.Sprintf("%s^%s", wrap(e.Left, postfixOps), formatFloat(e.boostPower))
	}

	return fmt.Sprintf("%s^", wrap(e.Left, postfixOps))
}

func renderFuzzy(e *Expression, verbose bool) string {
	if verbose {
		if e.fuzzyDistance > 1 {
			return fmt.Sprintf("%s(%#v~%d)", toString[e.Op], e.Left, e.fuzzyDistance)
		}

		return fmt.Sprintf("%s(%#v)", toString[e.Op], e.Left)
	}

	// A bare ~ parses as a distance of 1, so any other distance is written out.
	if e.fuzzyDistance != 1 {
		return fmt.Sprintf("%s~%d", wrap(e.Left, postfixOps), e.fuzzyDistance)
	}

	return fmt.Sprintf("%s~", wrap(e.Left, postfixOps))
}

func renderRange(e *Expression, verbose bool) string {
	boundary := e.Right.(*RangeBoundary)

	// The parser builds a range before it knows the field, so a parse error
	// can print one without a field.
	field := ""
	if verbose {
		if e.Left != nil {
			field = fmt.Sprintf("%#v:", e.Left)
		}
		if boundary.Inclusive {
			return fmt.Sprintf("%s[%#v TO %#v]", field, boundary.Min, boundary.Max)
		}

		return fmt.Sprintf("%s{%#v TO %#v}", field, boundary.Min, boundary.Max)
	}

	if e.Left != nil {
		field = fmt.Sprintf("%s:", e.Left)
	}
	if boundary.Inclusive {
		return fmt.Sprintf("%s[%s TO %s]", field, boundary.Min, boundary.Max)
	}

	return fmt.Sprintf("%s{%s TO %s}", field, boundary.Min, boundary.Max)
}

func renderList(e *Expression, verbose bool) string {
	vals := e.Left.([]*Expression)
	strs := []string{}
	for _, v := range vals {
		if verbose {
			strs = append(strs, fmt.Sprintf("%#v", v.Left))
			continue
		}
		strs = append(strs, fmt.Sprint(v))
	}

	if verbose {
		return fmt.Sprintf("LIST(%s)", strings.Join(strs, ", "))
	}

	// The parser builds an IN list from a group of terms joined by OR.
	return fmt.Sprintf("(%s)", strings.Join(strs, " OR "))
}

func renderNullLiteral(e *Expression, verbose bool) string {
	if verbose {
		return "NULL"
	}
	return "null"
}

func renderLiteral(e *Expression, verbose bool) string {
	if verbose {
		return fmt.Sprintf("%s(%#v)", toString[e.Op], e.Left)
	}

	// A number keeps the text it was written as. The lexer read that text as
	// one token, so it parses back to the same number and text.
	if e.numberText != "" {
		return e.numberText
	}

	switch v := e.Left.(type) {
	case string:
		if e.Op == Wild {
			return wildcardTerm(v)
		}
		if e.Op == Regexp {
			return regexpTerm(v)
		}
		return literalTerm(v)
	case Column:
		// A field that looks like a number or null is still read as a
		// column, so mostly the lexer decides whether it needs quotes. The
		// exception is null in another case: the parser reads a bare NULL
		// field as the column null, so NULL keeps its quotes.
		s := string(v)
		if !isPlainTerm(s) || isKeyword(s) || (strings.EqualFold(s, "null") && s != "null") {
			return quotePhrase(s)
		}
		return s
	case float64:
		// The parser keeps no text for a float written as its plain digits,
		// so print those digits, with .0 if they would read back as an int.
		d := strconv.FormatFloat(v, 'f', -1, 64)
		if _, err := strconv.Atoi(d); err == nil {
			d += ".0"
		}
		return d
	}

	return fmt.Sprintf("%v", e.Left)
}

// formatFloat prints f so the parser reads it back as the same float: always
// with a decimal point or an exponent, so 5.0 does not come back as the int 5,
// and with no + in the exponent, which the lexer would split off.
func formatFloat(f float64) string {
	if abs := math.Abs(f); abs != 0 && (abs < 1e-4 || abs >= 1e21) {
		return strings.Replace(strconv.FormatFloat(f, 'e', -1, 64), "e+", "e", 1)
	}
	s := strconv.FormatFloat(f, 'f', -1, 64)
	if !strings.Contains(s, ".") {
		s += ".0"
	}
	return s
}

// literalTerm prints the string s as a term: bare when it lexes as one word
// that the parser reads as s, and as a phrase otherwise.
func literalTerm(s string) string {
	if !isPlainTerm(s) || readsAsValue(s) {
		return quotePhrase(s)
	}
	return s
}

// wildcardTerm prints a wildcard's value as a term that lexes back as one
// word. The parser keeps a wildcard's text as written, with a backslash
// before every character the lexer would split off or read differently, so
// that text prints as it is. A wildcard built in code holds only its value,
// so String() adds those backslashes. It keeps the escapes already there and
// leaves the * and ? that are wildcards.
//
// A value with no wildcard left, such as fo\*, cannot be a wildcard term:
// the parser reads it as a plain term. It prints as the text it matches.
func wildcardTerm(s string) string {
	chars := wildcard.Split(s)
	if !slices.ContainsFunc(chars, wildcard.Char.IsWildcard) {
		return literalTerm(wildcard.Unescape(s))
	}
	var b strings.Builder
	for i, c := range chars {
		if c.Escaped || (!c.IsWildcard() && !bareInWildcard(chars, i)) {
			b.WriteByte('\\')
		}
		b.WriteString(c.Text)
	}
	return b.String()
}

// bareInWildcard reports whether the unescaped character chars[i] lexes as
// part of a word, as lexVal and lexWord in internal/lex read it: letters,
// digits, _ % @ # $ and the dot anywhere, a - after the first character, and
// a - first only when a number follows, as in -5*. Anything else, such as a
// space, a quote or a + (which the parser escapes), needs a backslash.
func bareInWildcard(chars []wildcard.Char, i int) bool {
	r, _ := utf8.DecodeRuneInString(chars[i].Text)
	switch {
	case r == '_' || r == '.' || unicode.IsLetter(r) || unicode.IsDigit(r) || strings.ContainsRune("%@#$", r):
		return true
	case r == '-' && i > 0:
		return true
	case r == '-':
		// The lexer reads a leading - as part of the word only when a digit,
		// or a dot and a digit, follows.
		next := chars[i+1:]
		if len(next) > 0 && next[0].Text == "." && !next[0].Escaped {
			next = next[1:]
		}
		if len(next) == 0 || next[0].Escaped {
			return false
		}
		d, _ := utf8.DecodeRuneInString(next[0].Text)
		return unicode.IsDigit(d)
	}
	return false
}

// regexpTerm prints a regex's value as a /.../ term. The parser keeps a regex
// with its slashes and with every slash inside it escaped. A regex built in
// code may have neither, so String() adds the slashes and escapes a slash
// inside, along with a backslash at the end, which would escape the closing
// slash.
func regexpTerm(s string) string {
	inner := s
	if len(s) >= 2 && s[0] == '/' && s[len(s)-1] == '/' {
		inner = s[1 : len(s)-1]
	}
	var b strings.Builder
	b.WriteByte('/')
	for i := 0; i < len(inner); i++ {
		switch {
		case inner[i] == '\\' && i+1 < len(inner):
			b.WriteString(inner[i : i+2])
			i++
		case inner[i] == '\\' || inner[i] == '/':
			b.WriteByte('\\')
			b.WriteByte(inner[i])
		default:
			b.WriteByte(inner[i])
		}
	}
	b.WriteByte('/')
	return b.String()
}

// isPlainTerm reports whether s lexes as one word with no escapes: letters,
// digits, _ % @ # and $, and also ., - and + after the first character. This
// follows lexWord in internal/lex. * and ? are left out because they would
// make the word a wildcard.
func isPlainTerm(s string) bool {
	for i, r := range s {
		switch {
		case r == '_' || unicode.IsLetter(r) || unicode.IsDigit(r) || strings.ContainsRune("%@#$", r):
		case i > 0 && (r == '.' || r == '-' || r == '+'):
		default:
			return false
		}
	}
	return s != ""
}

// isKeyword reports whether s lexes as an operator rather than a word.
func isKeyword(s string) bool {
	switch strings.ToUpper(s) {
	case "AND", "OR", "NOT", "TO":
		return true
	}
	return false
}

// readsAsValue reports whether the parser would read the bare word s as
// something other than the string s: a keyword, null or a number. Anything
// strconv reads as a number counts, even where the parser keeps it a word
// (inf, 0x10), so quoting stays correct if the parser's number rules change.
func readsAsValue(s string) bool {
	if isKeyword(s) || strings.EqualFold(s, "null") {
		return true
	}
	if _, err := strconv.ParseInt(s, 0, 64); err == nil {
		return true
	}
	_, err := strconv.ParseFloat(s, 64)
	return err == nil
}

var phraseEscaper = strings.NewReplacer(`\`, `\\`, `"`, `\"`)

// quotePhrase writes s as a double-quoted phrase. The parser unescapes only
// \" and \\ inside a phrase, so escaping those two is enough.
func quotePhrase(s string) string {
	return `"` + phraseEscaper.Replace(s) + `"`
}
