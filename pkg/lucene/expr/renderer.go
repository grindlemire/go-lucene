package expr

import "fmt"

type renderer func(*Expression) string

var renderers = map[Expr]renderer{
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

func renderEquals(e *Expression) string {
	return fmt.Sprintf("%s:%s", e.Left, e.Right)
}

func renderBasic(e *Expression) string {
	return fmt.Sprintf("%s %s %s", e.Left, toJSON[e.Expr], e.Right)
}

func renderWrapper(e *Expression) string {
	return fmt.Sprintf("%s(%s)", toJSON[e.Expr], e.Left)
}

func renderMustNot(e *Expression) string {
	return fmt.Sprintf("-%s", e.Left)
}

func renderMust(e *Expression) string {
	return fmt.Sprintf("+%s", e.Left)
}

func renderBoost(e *Expression) string {
	if e.boostPower > 1 {
		return fmt.Sprintf("%s^%.1f", e.Left, e.boostPower)
	}

	return fmt.Sprintf("%s^", e.Left)
}

func renderFuzzy(e *Expression) string {
	if e.fuzzyDistance > 1 {
		return fmt.Sprintf("%s~%d", e.Left, e.fuzzyDistance)
	}

	return fmt.Sprintf("%s~", e.Left)
}

func renderRange(e *Expression) string {
	if e.rangeInclusive {
		return fmt.Sprintf("[%s TO %s]", e.Left, e.Right)
	}

	return fmt.Sprintf("{%s TO %s}", e.Left, e.Right)
}

func renderLiteral(e *Expression) string {
	return fmt.Sprintf("%v", e.Left)
}
