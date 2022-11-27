package lucene

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseLucene(t *testing.T) {
	type tc struct {
		input    string
		expected Expression
	}

	tcs := map[string]tc{
		"single_literal": {
			input:    "a",
			expected: Lit("a"),
		},
		"basic_equal": {
			input: "a:b",
			expected: EQ(
				Lit("a"),
				Lit("b"),
			),
		},
		"basic_equal_with_number": {
			input: "a:5",
			expected: EQ(
				Lit("a"),
				Lit(5),
			),
		},
		"basic_wild_equal_with_*": {
			input: "a:b*",
			expected: EQ(
				Lit("a"),
				Wild("b*"),
			),
		},
		"basic_wild_equal_with_?": {
			input: "a:b?z",
			expected: EQ(
				Lit("a"),
				Wild("b?z"),
			),
		},
		"basic_and": {
			input: "a AND b",
			expected: AND(
				Lit("a"),
				Lit("b"),
			),
		},
		"and_with_nesting": {
			input: "a:foo AND b:bar",
			expected: AND(
				EQ(Lit("a"), Lit("foo")),
				EQ(Lit("b"), Lit("bar")),
			),
		},
		"basic_or": {
			input: "a OR b",
			expected: OR(
				Lit("a"),
				Lit("b"),
			),
		},
		"range_operator_inclusive": {
			input: "a:[1 TO 5]",
			expected: EQ(
				Lit("a"),
				Rang(Lit(1), Lit(5), true),
			),
		},
		"range_operator_inclusive_unbound": {
			input: `a:[* TO 200]`,
			expected: EQ(
				Lit("a"),
				Rang(Wild("*"), Lit(200), true),
			),
		},
		"range_operator_exclusive": {
			input: `a:{"ab" TO "az"}`,
			expected: EQ(
				Lit("a"),
				Rang(Lit(`"ab"`), Lit(`"az"`), false),
			),
		},
		"range_operator_exclusive_unbound": {
			input: `a:{2 TO *}`,
			expected: EQ(
				Lit("a"),
				Rang(Lit(2), Wild("*"), false),
			),
		},
		"or_with_nesting": {
			input: "a:foo OR b:bar",
			expected: OR(
				EQ(Lit("a"), Lit("foo")),
				EQ(Lit("b"), Lit("bar")),
			),
		},
		"basic_not": {
			input:    "NOT b",
			expected: NOT(Lit("b")),
		},
		"nested_not": {
			input: "a:foo OR NOT b:bar",
			expected: OR(
				EQ(Lit("a"), Lit("foo")),
				NOT(EQ(Lit("b"), Lit("bar"))),
			),
		},
		"term_grouping": {
			input: "(a:foo OR b:bar) AND c:baz",
			expected: AND(
				OR(
					EQ(Lit("a"), Lit("foo")),
					EQ(Lit("b"), Lit("bar")),
				),
				EQ(Lit("c"), Lit("baz")),
			),
		},
		"value_grouping": {
			input: "a:(foo OR baz OR bar)",
			expected: EQ(
				Lit("a"),
				OR(
					Lit("foo"),
					OR(
						Lit("baz"),
						Lit("bar"),
					),
				),
			),
		},
		"basic_must": {
			input: "+a:b",
			expected: MUST(
				EQ(Lit("a"), Lit("b")),
			),
		},
		"basic_must_not": {
			input: "-a:b",
			expected: MUSTNOT(
				EQ(Lit("a"), Lit("b")),
			),
		},
		"basic_nested_must_not": {
			input: "d:e AND (-a:b AND +f:e)",
			expected: AND(
				EQ(Lit("d"), Lit("e")),
				AND(
					MUSTNOT(EQ(Lit("a"), Lit("b"))),
					MUST(EQ(Lit("f"), Lit("e"))),
				),
			),
		},
		"basic_escaping": {
			input: `a:\(1\+1\)\:2`,
			expected: EQ(
				Lit("a"),
				Lit(`\(1\+1\)\:2`),
			),
		},
	}

	for name, tc := range tcs {
		t.Run(name, func(t *testing.T) {
			e, err := Parse(tc.input)
			require.NoError(t, err)
			assert.Equal(t, tc.expected, e)
		})
	}
}
func TestParseLoose(t *testing.T) {
	type tc struct {
		input    string
		expected Expression
	}

	tcs := map[string]tc{
		"single_literal": {
			input:    "a",
			expected: Lit("a"),
		},
		"basic_equal": {
			input: "a = b",
			expected: EQ(
				Lit("a"),
				Lit("b"),
			),
		},
		"basic_and": {
			input: "a AND b",
			expected: AND(
				Lit("a"),
				Lit("b"),
			),
		},
	}

	for name, tc := range tcs {
		t.Run(name, func(t *testing.T) {
			e, err := Parse(tc.input)
			require.NoError(t, err)
			assert.Equal(t, tc.expected, e)
		})
	}
}

func EQ(a *Literal, b Expression) *Equals {
	return &Equals{
		term:  a.val.(string),
		value: b,
	}
}

func AND(a, b Expression) *And {
	return &And{
		left:  a,
		right: b,
	}
}

func OR(a, b Expression) *Or {
	return &Or{
		left:  a,
		right: b,
	}
}

func Lit(val any) *Literal {
	return &Literal{
		val: val,
	}
}

func Wild(val any) *WildLiteral {
	return &WildLiteral{
		Literal{
			val: val,
		},
	}
}

func Rang(min, max Expression, inclusive bool) *Range {
	return &Range{
		Min:       min,
		Max:       max,
		Inclusive: inclusive,
	}
}

func NOT(e Expression) *Not {
	return &Not{
		expr: e,
	}
}

func MUST(e Expression) *Must {
	return &Must{
		expr: e,
	}
}

func MUSTNOT(e Expression) *MustNot {
	return &MustNot{
		expr: e,
	}
}
