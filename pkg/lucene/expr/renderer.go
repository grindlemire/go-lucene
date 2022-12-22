package expr

import (
	"fmt"
	"strings"
)

type renderer func(e *Expression, verbose bool) string

var renderers = map[Operation]renderer{
	Equals:  renderEquals,
	And:     renderBasic,
	Or:      renderBasic,
	Not:     renderWrapper,
	Range:   renderRange,
	Must:    renderMust,
	MustNot: renderMustNot,
	Boost:   renderBoost,
	Fuzzy:   renderFuzzy,
	Literal: renderLiteral,
	Wild:    renderLiteral,
	Regexp:  renderLiteral,
}

func renderEquals(e *Expression, verbose bool) string {
	if verbose {
		return fmt.Sprintf("%#v:%#v", e.Left, e.Right)
	}
	return fmt.Sprintf("%s:%s", e.Left, e.Right)
}

func renderBasic(e *Expression, verbose bool) string {
	if verbose {
		return fmt.Sprintf("(%#v) %s (%#v)", e.Left, toJSON[e.Op], e.Right)
	}
	return fmt.Sprintf("%s %s %s", e.Left, toJSON[e.Op], e.Right)
}

func renderWrapper(e *Expression, verbose bool) string {
	if verbose {
		return fmt.Sprintf("%s(%#v)", toJSON[e.Op], e.Left)
	}
	return fmt.Sprintf("%s(%s)", toJSON[e.Op], e.Left)
}

func renderMustNot(e *Expression, verbose bool) string {
	if verbose {
		return fmt.Sprintf("%s(%#v)", toJSON[e.Op], e.Left)
	}
	return fmt.Sprintf("-%s", e.Left)
}

func renderMust(e *Expression, verbose bool) string {
	if verbose {
		return fmt.Sprintf("%s(%#v)", toJSON[e.Op], e.Left)
	}
	return fmt.Sprintf("+%s", e.Left)
}

func renderBoost(e *Expression, verbose bool) string {
	if verbose {
		if e.boostPower > 1 {
			return fmt.Sprintf("%s(%#v^%.1f)", toJSON[e.Op], e.Left, e.boostPower)
		}

		return fmt.Sprintf("%s(%#v)", toJSON[e.Op], e.Left)
	}

	if e.boostPower > 1 {
		return fmt.Sprintf("%s^%.1f", e.Left, e.boostPower)
	}

	return fmt.Sprintf("%s^", e.Left)
}

func renderFuzzy(e *Expression, verbose bool) string {
	if verbose {
		if e.fuzzyDistance > 1 {
			return fmt.Sprintf("%s(%#v~%d)", toJSON[e.Op], e.Left, e.fuzzyDistance)
		}

		return fmt.Sprintf("%s(%#v)", toJSON[e.Op], e.Left)
	}

	if e.fuzzyDistance > 1 {
		return fmt.Sprintf("%s~%d", e.Left, e.fuzzyDistance)
	}

	return fmt.Sprintf("%s~", e.Left)
}

func renderRange(e *Expression, verbose bool) string {
	if verbose {
		if e.rangeInclusive {
			return fmt.Sprintf("[%#v TO %#v]", e.Left, e.Right)
		}

		return fmt.Sprintf("{%#v TO %#v}", e.Left, e.Right)
	}
	if e.rangeInclusive {
		return fmt.Sprintf("[%s TO %s]", e.Left, e.Right)
	}

	return fmt.Sprintf("{%s TO %s}", e.Left, e.Right)
}

func renderLiteral(e *Expression, verbose bool) string {
	if verbose {
		return fmt.Sprintf("%s(%#v)", toJSON[e.Op], e.Left)
	}

	s, isStr := e.Left.(string)
	if isStr && strings.ContainsAny(s, " ") {
		return fmt.Sprintf(`"%s"`, s)
	}

	return fmt.Sprintf("%v", e.Left)
}
