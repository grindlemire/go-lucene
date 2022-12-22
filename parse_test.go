package lucene

import (
	"fmt"
	"reflect"
	"testing"

	"github.com/grindlemire/go-lucene/pkg/lucene/expr"
)

const errTemplate = "%s:\n    wanted %#v\n    got    %#v"

func TestParseLucene(t *testing.T) {
	type tc struct {
		input    string
		expected *expr.Expression
	}

	tcs := map[string]tc{
		"single_literal": {
			input:    "a",
			expected: expr.Lit("a"),
		},
		"basic_equal": {
			input:    "a:b",
			expected: expr.Eq(expr.Lit("a"), expr.Lit("b")),
		},
		"basic_equal_with_number": {
			input:    "a:5",
			expected: expr.Eq(expr.Lit("a"), expr.Lit(5)),
		},
		"basic_wild_equal_with_*": {
			input:    "a:b*",
			expected: expr.Eq(expr.Lit("a"), expr.WILD("b*")),
		},
		"basic_wild_equal_with_?": {
			input:    "a:b?z",
			expected: expr.Eq(expr.Lit("a"), expr.WILD("b?z")),
		},
		"regexp": {
			input:    "a:/b [c]/",
			expected: expr.Eq(expr.Lit("a"), expr.REGEXP("/b [c]/")),
		},
		"regexp_with_keywords": {
			input:    `a:/b "[c]/`,
			expected: expr.Eq(expr.Lit("a"), expr.REGEXP(`/b "[c]/`)),
		},
		"default_to_AND_with_literals": {
			input: "a b",
			expected: expr.AND(
				expr.Lit("a"),
				expr.Lit("b"),
			),
		},
		"basic_default_AND": {
			input:    "a b",
			expected: expr.AND(expr.Lit("a"), expr.Lit("b")),
		},
		"default_to_AND_with_subexpressions": {
			input: "a:b c:d",
			expected: expr.AND(
				expr.Eq(expr.Lit("a"), expr.Lit("b")),
				expr.Eq(expr.Lit("c"), expr.Lit("d")),
			),
		},
		"basic_and": {
			input: "a AND b",
			expected: expr.AND(
				expr.Lit("a"),
				expr.Lit("b"),
			),
		},
		"and_with_nesting": {
			input: "a:foo AND b:bar",
			expected: expr.AND(
				expr.Eq(expr.Lit("a"), expr.Lit("foo")),
				expr.Eq(expr.Lit("b"), expr.Lit("bar")),
			),
		},
		"basic_or": {
			input: "a OR b",
			expected: expr.OR(
				expr.Lit("a"),
				expr.Lit("b"),
			),
		},
		"range_operator_inclusive": {
			input: "a:[1 TO 5]",
			expected: expr.Eq(
				expr.Lit("a"),
				expr.Rang(expr.Lit(1), expr.Lit(5), true),
			),
		},
		"range_operator_inclusive_unbound": {
			input: `a:[* TO 200]`,
			expected: expr.Eq(
				expr.Lit("a"),
				expr.Rang(expr.WILD("*"), expr.Lit(200), true),
			),
		},
		"range_operator_exclusive": {
			input:    `a:{"ab" TO "az"}`,
			expected: expr.Eq(expr.Lit("a"), expr.Rang(expr.Lit("ab"), expr.Lit("az"), false)),
		},
		"range_operator_exclusive_unbound": {
			input: `a:{2 TO *}`,
			expected: expr.Eq(
				expr.Lit("a"),
				expr.Rang(expr.Lit(2), expr.WILD("*"), false),
			),
		},
		"or_with_nesting": {
			input: "a:foo OR b:bar",
			expected: expr.OR(
				expr.Eq(expr.Lit("a"), expr.Lit("foo")),
				expr.Eq(expr.Lit("b"), expr.Lit("bar")),
			),
		},
		"basic_not": {
			input:    "NOT b",
			expected: expr.NOT(expr.Lit("b")),
		},
		"nested_not": {
			input: "a:foo OR NOT b:bar",
			expected: expr.OR(
				expr.Eq(expr.Lit("a"), expr.Lit("foo")),
				expr.NOT(expr.Eq(expr.Lit("b"), expr.Lit("bar"))),
			),
		},
		"term_grouping": {
			input: "(a:foo OR b:bar) AND c:baz",
			expected: expr.AND(
				expr.OR(
					expr.Eq(expr.Lit("a"), expr.Lit("foo")),
					expr.Eq(expr.Lit("b"), expr.Lit("bar")),
				),
				expr.Eq(expr.Lit("c"), expr.Lit("baz")),
			),
		},
		"value_grouping": {
			input: "a:(foo OR baz OR bar)",
			expected: expr.Eq(
				expr.Lit("a"),
				expr.OR(
					expr.OR(
						expr.Lit("foo"),
						expr.Lit("baz"),
					),
					expr.Lit("bar"),
				),
			),
		},
		"basic_must": {
			input: "+a:b",
			expected: expr.MUST(
				expr.Eq(expr.Lit("a"), expr.Lit("b")),
			),
		},
		"basic_must_not": {
			input: "-a:b",
			expected: expr.MUSTNOT(
				expr.Eq(expr.Lit("a"), expr.Lit("b")),
			),
		},
		"basic_nested_must_not": {
			input: "d:e AND (-a:b AND +f:e)",
			expected: expr.AND(
				expr.Eq(expr.Lit("d"), expr.Lit("e")),
				expr.AND(
					expr.MUSTNOT(expr.Eq(expr.Lit("a"), expr.Lit("b"))),
					expr.MUST(expr.Eq(expr.Lit("f"), expr.Lit("e"))),
				),
			),
		},
		"basic_escaping": {
			input:    `a:\(1\+1\)\:2`,
			expected: expr.Eq(expr.Lit("a"), expr.Lit(`\(1\+1\)\:2`)),
		},
		"boost_key_value": {
			input: "a:b^2 AND foo",
			expected: expr.AND(
				expr.BOOST(expr.Eq(expr.Lit("a"), expr.Lit("b")), 2),
				expr.Lit("foo"),
			),
		},
		"boost_literal": {
			input:    "foo^4",
			expected: expr.BOOST(expr.Lit("foo"), 4),
		},
		"boost_literal_in_compound": {
			input: "a:b AND foo^4",
			expected: expr.AND(
				expr.Eq(expr.Lit("a"), expr.Lit("b")),
				expr.BOOST(expr.Lit("foo"), 4),
			),
		},
		"boost_literal_leading": {
			input: "foo^4 AND a:b",
			expected: expr.AND(
				expr.BOOST(expr.Lit("foo"), 4),
				expr.Eq(expr.Lit("a"), expr.Lit("b")),
			),
		},
		"boost_quoted_literal": {
			input: `"foo bar"^4 AND a:b`,
			expected: expr.AND(
				expr.BOOST(expr.Lit("foo bar"), 4),
				expr.Eq(expr.Lit("a"), expr.Lit("b")),
			),
		},
		"boost_sub_expression": {
			input: "(title:foo OR title:bar)^1.5 AND (body:foo OR body:bar)",
			expected: expr.AND(
				expr.BOOST(
					expr.OR(
						expr.Eq(expr.Lit("title"), expr.Lit("foo")),
						expr.Eq(expr.Lit("title"), expr.Lit("bar")),
					),
					1.5),
				expr.OR(
					expr.Eq(expr.Lit("body"), expr.Lit("foo")),
					expr.Eq(expr.Lit("body"), expr.Lit("bar")),
				),
			),
		},
		"nested_sub_expressions_with_boost": {
			input: "((title:foo)^1.2 OR title:bar) AND (body:foo OR body:bar)",
			expected: expr.AND(
				expr.OR(
					expr.BOOST(expr.Eq(expr.Lit("title"), expr.Lit("foo")), 1.2),
					expr.Eq(expr.Lit("title"), expr.Lit("bar")),
				),
				expr.OR(
					expr.Eq(expr.Lit("body"), expr.Lit("foo")),
					expr.Eq(expr.Lit("body"), expr.Lit("bar")),
				),
			),
		},
		"nested_sub_expressions": {
			input: "((title:foo OR title:bar) AND (body:foo OR body:bar)) OR k:v",
			expected: expr.OR(
				expr.AND(
					expr.OR(
						expr.Eq(expr.Lit("title"), expr.Lit("foo")),
						expr.Eq(expr.Lit("title"), expr.Lit("bar")),
					),

					expr.OR(
						expr.Eq(expr.Lit("body"), expr.Lit("foo")),
						expr.Eq(expr.Lit("body"), expr.Lit("bar")),
					),
				),
				expr.Eq(expr.Lit("k"), expr.Lit("v")),
			),
		},
		"fuzzy_key_value": {
			input: "a:b~2 AND foo",
			expected: expr.AND(
				expr.FUZZY(expr.Eq(expr.Lit("a"), expr.Lit("b")), 2),
				expr.Lit("foo"),
			),
		},
		"fuzzy_key_value_default": {
			input: "a:b~ AND foo",
			expected: expr.AND(
				expr.FUZZY(expr.Eq(expr.Lit("a"), expr.Lit("b")), 1),
				expr.Lit("foo"),
			),
		},
		"fuzzy_literal": {
			input:    "foo~4",
			expected: expr.FUZZY(expr.Lit("foo"), 4),
		},
		"fuzzy_literal_default": {
			input:    "foo~",
			expected: expr.FUZZY(expr.Lit("foo"), 1),
		},
		"fuzzy_literal_in_compound": {
			input: "a:b AND foo~4",
			expected: expr.AND(
				expr.Eq(expr.Lit("a"), expr.Lit("b")),
				expr.FUZZY(expr.Lit("foo"), 4),
			),
		},
		"fuzzy_literal_in_implicit_compound": {
			input: "a:b foo~4",
			expected: expr.AND(
				expr.Eq(expr.Lit("a"), expr.Lit("b")),
				expr.FUZZY(expr.Lit("foo"), 4),
			),
		},
		"fuzzy_literal_leading": {
			input: "foo~4 AND a:b",
			expected: expr.AND(
				expr.FUZZY(expr.Lit("foo"), 4),
				expr.Eq(expr.Lit("a"), expr.Lit("b")),
			),
		},
		"fuzzy_literal_leading_in_implicit_compound": {
			input: "foo~4 AND a:b",
			expected: expr.AND(
				expr.FUZZY(expr.Lit("foo"), 4),
				expr.Eq(expr.Lit("a"), expr.Lit("b")),
			),
		},
		"fuzzy_quoted_literal": {
			input: `"foo bar"~4 AND a:b`,
			expected: expr.AND(
				expr.FUZZY(expr.Lit("foo bar"), 4),
				expr.Eq(expr.Lit("a"), expr.Lit("b")),
			),
		},
		"fuzzy_sub_expression": {
			input: "(title:foo OR title:bar)~2 AND (body:foo OR body:bar)",
			expected: expr.AND(
				expr.FUZZY(
					expr.OR(
						expr.Eq(expr.Lit("title"), expr.Lit("foo")),
						expr.Eq(expr.Lit("title"), expr.Lit("bar")),
					),
					2),
				expr.OR(
					expr.Eq(expr.Lit("body"), expr.Lit("foo")),
					expr.Eq(expr.Lit("body"), expr.Lit("bar")),
				),
			),
		},
		"nested_sub_expressions_with_fuzzy": {
			input: "((title:foo)~ OR title:bar) AND (body:foo OR body:bar)",
			expected: expr.AND(
				expr.OR(
					expr.FUZZY(expr.Eq(expr.Lit("title"), expr.Lit("foo")), 1),
					expr.Eq(expr.Lit("title"), expr.Lit("bar")),
				),

				expr.OR(
					expr.Eq(expr.Lit("body"), expr.Lit("foo")),
					expr.Eq(expr.Lit("body"), expr.Lit("bar")),
				),
			),
		},
		"precedence_works": {
			input: "a:b AND c:d OR e:f OR h:i AND j:k",
			expected: expr.OR(
				expr.OR(
					expr.AND(
						expr.Eq(expr.Lit("a"), expr.Lit("b")),
						expr.Eq(expr.Lit("c"), expr.Lit("d")),
					),
					expr.Eq(expr.Lit("e"), expr.Lit("f"))),
				expr.AND(
					expr.Eq(expr.Lit("h"), expr.Lit("i")),
					expr.Eq(expr.Lit("j"), expr.Lit("k")),
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
				t.Fatalf(errTemplate, "parsed expression doesn't match expected", tc.expected, parsed)
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
		"range_with_invalid_min": {
			input: "[(a OR b) TO *]",
		},
		"range_with_invalid_max": {
			input: "[* TO (a OR b)]",
		},
		"nested_validation_works": {
			input: "(A=B AND C=(D OR E)) OR (expr.NOT(+a:[* TO]))",
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
		"+foo OR (expr.NOT(B))",
		"A = bar",
		"NOT(b = c)",
		"z:[* TO 10]",
		"x:[10 TO *] AND expr.NOT(y:[1 TO 5]",
		"(+a:b -c:d) OR (z:[1 TO *] expr.NOT(foo))",
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

func TestBufParse(t *testing.T) {
	type tc struct {
		input string
		want  *expr.Expression
	}

	tcs := map[string]tc{
		"basic_inclusive_range": {
			input: "a:[* TO 5]",
			want:  expr.Eq(expr.Lit("a"), expr.Rang(expr.WILD("*"), expr.Lit(5), true)),
		},
		"basic_exclusive_range": {
			input: "a:{* TO 5}",
			want:  expr.Eq(expr.Lit("a"), expr.Rang(expr.WILD("*"), expr.Lit(5), false)),
		},
		"range_over_strings": {
			input: "a:{foo TO bar}",
			want:  expr.Eq(expr.Lit("a"), expr.Rang(expr.Lit("foo"), expr.Lit("bar"), false)),
		},
		"basic_fuzzy": {
			input: "b AND a~",
			want:  expr.AND(expr.Lit("b"), expr.FUZZY(expr.Lit("a"), 1)),
		},
		"fuzzy_power": {
			input: "b AND a~10",
			want:  expr.AND(expr.Lit("b"), expr.FUZZY(expr.Lit("a"), 10)),
		},
		"basic_boost": {
			input: "b AND a^",
			want:  expr.AND(expr.Lit("b"), expr.BOOST(expr.Lit("a"), 1.0)),
		},
		"boost_power": {
			input: "b AND a^10",
			want:  expr.AND(expr.Lit("b"), expr.BOOST(expr.Lit("a"), 10.0)),
		},
		"most_basic": {
			input: "a AND b",
			want: expr.AND(
				expr.Lit("a"),
				expr.Lit("b"),
			),
		},
		"test_expr": {
			input: "a OR b AND c OR d",
			want: expr.OR(
				expr.OR(
					expr.Lit("a"),
					expr.AND(expr.Lit("b"), expr.Lit("c")),
				),
				expr.Lit("d"),
			),
		},
		"test_not": {
			input: "NOT a OR b AND NOT c OR d",
			want: expr.OR(
				expr.OR(
					expr.NOT(expr.Lit("a")),
					expr.AND(expr.Lit("b"), expr.NOT(expr.Lit("c"))),
				),
				expr.Lit("d"),
			),
		},
		"test_equals": {
			input: "a:az OR b:bz AND NOT c:z OR d",
			want: expr.OR(
				expr.OR(
					expr.Eq(expr.Lit("a"), expr.Lit("az")),
					expr.AND(
						expr.Eq(expr.Lit("b"), expr.Lit("bz")),
						expr.NOT(
							expr.Eq(expr.Lit("c"), expr.Lit("z")),
						),
					),
				),
				expr.Lit("d"),
			),
		},
		"test_parens": {
			input: "a AND (c OR d)",
			want: expr.AND(
				expr.Lit("a"),
				expr.OR(
					expr.Lit("c"),
					expr.Lit("d"),
				),
			),
		},
		"test_full_precedance": {
			input: "a OR b AND c:[* to -1] OR d AND NOT +e:f",
			want: expr.OR(
				expr.OR(
					expr.Lit("a"),
					expr.AND(
						expr.Lit("b"),
						expr.Eq(
							expr.Lit("c"),
							expr.Rang(expr.WILD("*"), expr.Lit(-1), true),
						),
					),
				),
				expr.AND(
					expr.Lit("d"),
					expr.NOT(
						expr.MUST(expr.Eq(expr.Lit("e"), expr.Lit("f"))),
					),
				),
			),
		},
		"test_full_precedance_with_suffixes": {
			input: "a OR b AND c OR d~ AND NOT +(e:f)^10",
			want: expr.OR(
				expr.OR(
					expr.Lit("a"),
					expr.AND(expr.Lit("b"), expr.Lit("c")),
				),
				expr.AND(
					expr.FUZZY(expr.Lit("d"), 1),
					expr.NOT(
						expr.BOOST(
							expr.MUST(
								expr.Eq(expr.Lit("e"), expr.Lit("f")),
							),
							10.0,
						),
					),
				),
			),
		},
		"test_not_expr": {
			input: "(NOT a OR b) AND NOT(c OR d)",
			want: expr.AND(
				expr.OR(expr.NOT(expr.Lit("a")), expr.Lit("b")),
				expr.NOT(expr.OR(expr.Lit("c"), expr.Lit("d"))),
			),
		},
		"single_literal": {
			input: "a",
			want:  expr.Lit("a"),
		},
		"basic_expr.equal": {
			input: "a:b",
			want: expr.Eq(
				expr.Lit("a"),
				expr.Lit("b"),
			),
		},
		"basic_expr.equal_with_number": {
			input: "a:5",
			want: expr.Eq(
				expr.Lit("a"),
				expr.Lit(5),
			),
		},
		"basic_wild_expr.equal_with_*": {
			input: "a:b*",
			want: expr.Eq(
				expr.Lit("a"),
				expr.WILD("b*"),
			),
		},
		"basic_wild_expr.equal_with_?": {
			input: "a:b?z",
			want: expr.Eq(
				expr.Lit("a"),
				expr.WILD("b?z"),
			),
		},
		"regexp": {
			input: "a:/b [c]/",
			want: expr.Eq(
				expr.Lit("a"), expr.REGEXP("/b [c]/"),
			),
		},
		"regexp_with_keywords": {
			input: `a:/b "[c]/`,
			want: expr.Eq(
				expr.Lit("a"), expr.REGEXP(`/b "[c]/`),
			),
		},
		"basic_not": {
			input: "NOT b",
			want:  expr.NOT(expr.Lit("b")),
		},
		"nested_not": {
			input: "a:foo OR NOT b:bar AND NOT c:baz",
			want: expr.OR(
				expr.Eq(expr.Lit("a"), expr.Lit("foo")),
				expr.AND(
					expr.NOT(expr.Eq(expr.Lit("b"), expr.Lit("bar"))),
					expr.NOT(expr.Eq(expr.Lit("c"), expr.Lit("baz"))),
				),
			),
		},
	}

	for name, tc := range tcs {
		t.Run(name, func(t *testing.T) {
			got, err := Parse(tc.input)
			if err != nil {
				t.Fatalf("wanted no error, got: %v", err)
			}
			if !reflect.DeepEqual(tc.want, got) {
				fmt.Printf("\n%+v\n", got)
				t.Fatalf(errTemplate, "Parsed expression doesn't match", tc.want, got)
			}
		})
	}
}
