package lucene

import (
	"reflect"
	"testing"

	"github.com/grindlemire/go-lucene/expr"
)

const errTemplate = "%s:\n    wanted %v\n    got    %v"

func TestParseLucene(t *testing.T) {
	type tc struct {
		input    string
		expected expr.Expression
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
		"regexp": {
			input: "a:/b [c]/",
			expected: EQ(
				Lit("a"), REGEXP("b [c]"),
			),
		},
		"regexp_with_keywords": {
			input: `a:/b "[c]/`,
			expected: EQ(
				Lit("a"), REGEXP(`b "[c]`),
			),
		},
		"default_to_AND_with_literals": {
			input: "a b",
			expected: AND(
				Lit("a"),
				Lit("b"),
			),
		},
		"default_to_AND_with_subexpressions": {
			input: "a:b c:d",
			expected: AND(
				EQ(Lit("a"), Lit("b")),
				EQ(Lit("c"), Lit("d")),
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
				Rang(Lit("ab"), Lit("az"), false),
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
		"boost_key_value": {
			input: "a:b^2 AND foo",
			expected: AND(
				BOOST(EQ(Lit("a"), Lit("b")), 2),
				Lit("foo"),
			),
		},
		"boost_literal": {
			input:    "foo^4",
			expected: BOOST(Lit("foo"), 4),
		},
		"boost_literal_in_compound": {
			input: "a:b AND foo^4",
			expected: AND(
				EQ(Lit("a"), Lit("b")),
				BOOST(Lit("foo"), 4),
			),
		},
		"boost_literal_leading": {
			input: "foo^4 AND a:b",
			expected: AND(
				BOOST(Lit("foo"), 4),
				EQ(Lit("a"), Lit("b")),
			),
		},
		"boost_quoted_literal": {
			input: `"foo bar"^4 AND a:b`,
			expected: AND(
				BOOST(Lit("foo bar"), 4),
				EQ(Lit("a"), Lit("b")),
			),
		},
		"boost_sub_expression": {
			input: "(title:foo OR title:bar)^1.5 AND (body:foo OR body:bar)",
			expected: AND(
				BOOST(
					OR(
						EQ(Lit("title"), Lit("foo")),
						EQ(Lit("title"), Lit("bar")),
					),
					1.5),
				OR(
					EQ(Lit("body"), Lit("foo")),
					EQ(Lit("body"), Lit("bar")),
				),
			),
		},
		"nested_sub_expressions_with_boost": {
			input: "((title:foo)^1.2 OR title:bar) AND (body:foo OR body:bar)",
			expected: AND(
				OR(
					BOOST(EQ(Lit("title"), Lit("foo")), 1.2),
					EQ(Lit("title"), Lit("bar")),
				),

				OR(
					EQ(Lit("body"), Lit("foo")),
					EQ(Lit("body"), Lit("bar")),
				),
			),
		},
		"nested_sub_expressions": {
			input: "((title:foo OR title:bar) AND (body:foo OR body:bar)) OR k:v",
			expected: OR(
				AND(
					OR(
						EQ(Lit("title"), Lit("foo")),
						EQ(Lit("title"), Lit("bar")),
					),

					OR(
						EQ(Lit("body"), Lit("foo")),
						EQ(Lit("body"), Lit("bar")),
					),
				),
				EQ(Lit("k"), Lit("v")),
			),
		},
		"fuzzy_key_value": {
			input: "a:b~2 AND foo",
			expected: AND(
				FUZZY(EQ(Lit("a"), Lit("b")), 2),
				Lit("foo"),
			),
		},
		"fuzzy_key_value_default": {
			input: "a:b~ AND foo",
			expected: AND(
				FUZZY(EQ(Lit("a"), Lit("b")), 1),
				Lit("foo"),
			),
		},
		"fuzzy_literal": {
			input:    "foo~4",
			expected: FUZZY(Lit("foo"), 4),
		},
		"fuzzy_literal_default": {
			input:    "foo~",
			expected: FUZZY(Lit("foo"), 1),
		},
		"fuzzy_literal_in_compound": {
			input: "a:b AND foo~4",
			expected: AND(
				EQ(Lit("a"), Lit("b")),
				FUZZY(Lit("foo"), 4),
			),
		},
		"fuzzy_literal_in_implicit_compound": {
			input: "a:b foo~4",
			expected: AND(
				EQ(Lit("a"), Lit("b")),
				FUZZY(Lit("foo"), 4),
			),
		},
		"fuzzy_literal_leading": {
			input: "foo~4 AND a:b",
			expected: AND(
				FUZZY(Lit("foo"), 4),
				EQ(Lit("a"), Lit("b")),
			),
		},
		"fuzzy_literal_leading_in_implicit_compound": {
			input: "foo~4 AND a:b",
			expected: AND(
				FUZZY(Lit("foo"), 4),
				EQ(Lit("a"), Lit("b")),
			),
		},
		"fuzzy_quoted_literal": {
			input: `"foo bar"~4 AND a:b`,
			expected: AND(
				FUZZY(Lit("foo bar"), 4),
				EQ(Lit("a"), Lit("b")),
			),
		},
		"fuzzy_sub_expression": {
			input: "(title:foo OR title:bar)~2 AND (body:foo OR body:bar)",
			expected: AND(
				FUZZY(
					OR(
						EQ(Lit("title"), Lit("foo")),
						EQ(Lit("title"), Lit("bar")),
					),
					2),
				OR(
					EQ(Lit("body"), Lit("foo")),
					EQ(Lit("body"), Lit("bar")),
				),
			),
		},
		"nested_sub_expressions_with_fuzzy": {
			input: "((title:foo)~ OR title:bar) AND (body:foo OR body:bar)",
			expected: AND(
				OR(
					FUZZY(EQ(Lit("title"), Lit("foo")), 1),
					EQ(Lit("title"), Lit("bar")),
				),

				OR(
					EQ(Lit("body"), Lit("foo")),
					EQ(Lit("body"), Lit("bar")),
				),
			),
		},
		"precedence_works": {
			input: "a:b AND c:d OR e:f OR h:i AND j:k",
			expected: OR(
				AND(
					EQ(Lit("a"), Lit("b")),
					EQ(Lit("c"), Lit("d")),
				),
				OR(
					EQ(Lit("e"), Lit("f")),
					AND(
						EQ(Lit("h"), Lit("i")),
						EQ(Lit("j"), Lit("k")),
					),
				),
			),
		},
	}

	for name, tc := range tcs {
		t.Run(name, func(t *testing.T) {
			parsed, err := Parse(tc.input)
			if err != nil {
				t.Fatalf("wanted no error, got: %v", err)
			}
			if !reflect.DeepEqual(tc.expected, parsed) {
				t.Fatalf(errTemplate, "error parsing", tc.expected, parsed)
			}
		})
	}
}

func TestParseFailure(t *testing.T) {
	type tc struct {
		input string
	}

	tcs := map[string]tc{
		"unpaired_paren": {
			input: "(a AND b",
		},
		"unbalanced_paren": {
			input: "(a AND b))",
		},
		"unbalanced_nested_paren": {
			input: "(a AND (b AND c)",
		},
		"equal_without_rhs": {
			input: "a = ",
		},
		"equal_without_lhs": {
			input: "= b",
		},
		"empty_parens_nil": {
			input: "() = ()",
		},
		"and_without_rhs": {
			input: "a AND",
		},
		"or_without_rhs": {
			input: "a OR",
		},
		"not_without_subexpression_1": {
			input: "NOT",
		},
		"not_without_subexpression_2": {
			input: "NOT()",
		},
		"must_without_subexpression_1": {
			input: "+",
		},
		"must_without_subexpression_2": {
			input: "+()",
		},
		"mustnot_without_subexpression_1": {
			input: "-",
		},
		"mustnot_without_subexpression_2": {
			input: "-()",
		},
		"boost_without_subexpression_1": {
			input: "^2",
		},
		"boost_without_subexpression_2": {
			input: "()^2",
		},
		"fuzzy_without_subexpression_1": {
			input: "~2",
		},
		"fuzzy_without_subexpression_2": {
			input: "()~2",
		},
		"fuzzy_without_subexpression_3": {
			input: "~",
		},
		"fuzzy_without_subexpression_4": {
			input: "()~",
		},
		"range_without_min": {
			input: "[ TO 5]",
		},
		"range_without_max": {
			input: "[* TO ]",
		},
		"nested_validation_works": {
			input: "(A=B AND C=(D OR E)) OR (NOT(+a:[* TO]))",
		},
	}

	for name, tc := range tcs {
		t.Run(name, func(t *testing.T) {
			_, err := Parse(tc.input)
			if err == nil {
				t.Fatalf("expected error but did not get one")
			}
		})
	}
}

func FuzzParse(f *testing.F) {
	tcs := []string{
		"A:B AND C:D",
		"+foo OR (NOT(B))",
		"A = bar",
		"NOT(b = c)",
		"z:[* TO 10]",
		"x:[10 TO *] AND NOT(y:[1 TO 5]",
		"(+a:b -c:d) OR (z:[1 TO *] NOT(foo))",
		`+bbq:"woo"`,
		`-bbq:"woo"`,
	}
	for _, tc := range tcs {
		f.Add(tc)
	}
	f.Fuzz(func(t *testing.T, in string) {
		Parse(in)
	})

}
