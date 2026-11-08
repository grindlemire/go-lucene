package expr

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
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
	// A minus right before a digit lexes as part of a negative number, so
	// -5 would parse as the number -5 rather than MUST_NOT(5).
	if r, _ := utf8.DecodeRuneInString(s); unicode.IsDigit(r) {
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

	if e.boostPower > 1 {
		return fmt.Sprintf("%s^%.1f", wrap(e.Left, postfixOps), e.boostPower)
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

	if e.fuzzyDistance > 1 {
		return fmt.Sprintf("%s~%d", wrap(e.Left, postfixOps), e.fuzzyDistance)
	}

	return fmt.Sprintf("%s~", wrap(e.Left, postfixOps))
}

func renderRange(e *Expression, verbose bool) string {
	boundary := e.Right.(*RangeBoundary)
	if verbose {
		if boundary.Inclusive {
			return fmt.Sprintf("%#v:[%#v TO %#v]", e.Left, boundary.Min, boundary.Max)
		}

		return fmt.Sprintf("%#v:{%#v TO %#v}", e.Left, boundary.Min, boundary.Max)
	}
	if boundary.Inclusive {
		return fmt.Sprintf("%s:[%s TO %s]", e.Left, boundary.Min, boundary.Max)
	}

	return fmt.Sprintf("%s:{%s TO %s}", e.Left, boundary.Min, boundary.Max)
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

	switch v := e.Left.(type) {
	case string:
		// The parser keeps a wildcard or regex as the text it read, with its
		// escapes and slashes, so that text lexes back to the same term.
		if e.Op == Wild || e.Op == Regexp {
			return v
		}
		if !isPlainTerm(v) || readsAsValue(v) {
			return quotePhrase(v)
		}
		return v
	case Column:
		// A field that looks like a number or null is still read as a
		// column, so only the lexer decides whether it needs quotes.
		if !isPlainTerm(string(v)) || isKeyword(string(v)) {
			return quotePhrase(string(v))
		}
		return string(v)
	case float64:
		return formatFloat(v)
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

// isPlainTerm reports whether s lexes as one word with no escapes: letters,
// digits, _ % @ # and $, and also . and - after the first character. This
// follows lexWord in internal/lex. * and ? are left out because they would
// make the word a wildcard.
func isPlainTerm(s string) bool {
	for i, r := range s {
		switch {
		case r == '_' || unicode.IsLetter(r) || unicode.IsDigit(r) || strings.ContainsRune("%@#$", r):
		case i > 0 && (r == '.' || r == '-'):
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
