package expr

import (
	"fmt"
	"strings"
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
	return fmt.Sprintf("%s:%s%s", e.Left, compareSymbols[e.Op], wrap(e.Right, prefixOps))
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
	// prefixOps need parentheses under + and - and after "field:". "--a:b" and
	// "a:-b" do not parse, "-a:b^2" is BOOST(MUST_NOT(a:b)) and "a:b AND c" is
	// (a:b) AND c.
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
	return fmt.Sprintf("-%s", wrap(e.Left, prefixOps))
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

	// only a plain Literal can hold a raw quote character today (produced by
	// unescaping \" or \' inside a phrase), so only escape/quote-guard those.
	// Wild and Regexp also render through here but keep their prior,
	// space-only quoting - their raw text isn't phrase-delimiter-escaped and
	// wrapping it in quotes here would change what it re-parses as.
	s, isStr := e.Left.(string)
	if isStr && e.Op == Literal && strings.ContainsAny(s, ` "'`) {
		// the rendered phrase always uses " as its delimiter, so only " (and
		// the backslash that would otherwise escape it) need escaping here -
		// a bare ' just needs the value quoted so it isn't lexed as a word.
		escaped := strings.NewReplacer(`\`, `\\`, `"`, `\"`).Replace(s)
		return fmt.Sprintf(`"%s"`, escaped)
	}
	if isStr && strings.ContainsAny(s, " ") {
		return fmt.Sprintf(`"%s"`, s)
	}

	return fmt.Sprintf("%v", e.Left)
}
