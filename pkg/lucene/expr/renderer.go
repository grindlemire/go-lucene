package expr

import (
	"fmt"
	"strings"
)

type renderer func(e *Expression, verbose bool) string

var renderers = map[Operator]renderer{
	Equals:    renderEquals,
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
	Greater:   renderBasic,
	Less:      renderBasic,
	GreaterEq: renderBasic,
	LessEq:    renderBasic,
	Like:      renderBasic,
	In:        renderBasic,
	List:      renderList,
}

func renderEquals(e *Expression, verbose bool) string {
	if verbose {
		return fmt.Sprintf("%#v:%#v", e.Left, e.Right)
	}
	return fmt.Sprintf("%s:%s", e.Left, e.Right)
}

func renderBasic(e *Expression, verbose bool) string {
	if verbose {
		return fmt.Sprintf("(%#v) %s (%#v)", e.Left, toString[e.Op], e.Right)
	}
	return fmt.Sprintf("%s %s %s", e.Left, toString[e.Op], e.Right)
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
	return fmt.Sprintf("-%s", e.Left)
}

func renderMust(e *Expression, verbose bool) string {
	if verbose {
		return fmt.Sprintf("%s(%#v)", toString[e.Op], e.Left)
	}
	return fmt.Sprintf("+%s", e.Left)
}

func renderBoost(e *Expression, verbose bool) string {
	if verbose {
		if e.boostPower > 1 {
			return fmt.Sprintf("%s(%#v^%.1f)", toString[e.Op], e.Left, e.boostPower)
		}

		return fmt.Sprintf("%s(%#v)", toString[e.Op], e.Left)
	}

	if e.boostPower > 1 {
		return fmt.Sprintf("%s^%.1f", e.Left, e.boostPower)
	}

	return fmt.Sprintf("%s^", e.Left)
}

func renderFuzzy(e *Expression, verbose bool) string {
	if verbose {
		if e.fuzzyDistance > 1 {
			return fmt.Sprintf("%s(%#v~%d)", toString[e.Op], e.Left, e.fuzzyDistance)
		}

		return fmt.Sprintf("%s(%#v)", toString[e.Op], e.Left)
	}

	if e.fuzzyDistance > 1 {
		return fmt.Sprintf("%s~%d", e.Left, e.fuzzyDistance)
	}

	return fmt.Sprintf("%s~", e.Left)
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
		strs = append(strs, fmt.Sprintf("%s", v.Left))
	}

	if verbose {
		return fmt.Sprintf("LIST(%s)", strings.Join(strs, ", "))
	}

	return fmt.Sprintf("(%s)", strings.Join(strs, ", "))
}

func renderLiteral(e *Expression, verbose bool) string {
	if verbose {
		return fmt.Sprintf("%s(%#v)", toString[e.Op], e.Left)
	}

	s, isStr := e.Left.(string)
	if isStr && strings.ContainsAny(s, " ") {
		return fmt.Sprintf(`"%s"`, s)
	}

	return fmt.Sprintf("%v", e.Left)
}
