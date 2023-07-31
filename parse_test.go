package lucene

import (
	"encoding/json"
	"fmt"
	"reflect"
	"testing"

	"github.com/grindlemire/go-lucene/pkg/lucene/expr"
)

const errTemplate = "%s:\n    wanted %#v\n    got    %#v"

func TestParseLucene(t *testing.T) {
	type tc struct {
		input string
		want  *expr.Expression
	}

	tcs := map[string]tc{
		"single_literal": {
			input: "a",
			want:  expr.Lit("a"),
		},
		"basic_equal": {
			input: "a:b",
			want:  expr.Eq("a", "b"),
		},
		"basic_equal_with_number": {
			input: "a:5",
			want:  expr.Eq("a", 5),
		},
		"basic_greater_with_number": {
			input: "a:>22",
			want:  expr.GREATER("a", 22),
		},
		"basic_greater_eq_with_number": {
			input: "a:>=22",
			want:  expr.GREATEREQ("a", 22),
		},
		"basic_less_with_number": {
			input: "a:<22",
			want:  expr.LESS("a", 22),
		},
		"basic_less_eq_with_number": {
			input: "a:<=22",
			want:  expr.LESSEQ("a", 22),
		},
		"basic_wild_equal_with_*": {
			input: "a:b*",
			want:  expr.Eq("a", "b*"),
		},
		"basic_wild_equal_with_?": {
			input: "a:b?z",
			want:  expr.Eq("a", expr.WILD("b?z")),
		},
		"basic_inclusive_range": {
			input: "a:[* TO 5]",
			want:  expr.Rang("a", expr.WILD("*"), 5, true),
		},
		"basic_exclusive_range": {
			input: "a:{* TO 5}",
			want:  expr.Rang("a", expr.WILD("*"), 5, false),
		},
		"range_over_strings": {
			input: "a:{foo TO bar}",
			want:  expr.Rang("a", "foo", "bar", false),
		},
		"basic_fuzzy": {
			input: "b AND a~",
			want:  expr.AND("b", expr.FUZZY("a", 1)),
		},
		"fuzzy_power": {
			input: "b AND a~10",
			want:  expr.AND("b", expr.FUZZY("a", 10)),
		},
		"basic_boost": {
			input: "b AND a^",
			want:  expr.AND("b", expr.BOOST("a", 1.0)),
		},
		"boost_power": {
			input: "b AND a^10",
			want:  expr.AND("b", expr.BOOST("a", 10.0)),
		},
		"regexp": {
			input: "a:/b [c]/",
			want:  expr.Eq("a", expr.REGEXP("/b [c]/")),
		},
		"regexp_with_keywords": {
			input: `a:/b "[c]/`,
			want:  expr.Eq("a", expr.REGEXP(`/b "[c]/`)),
		},
		"basic_default_AND": {
			input: "a b",
			want:  expr.AND("a", "b"),
		},
		"default_to_AND_with_subexpressions": {
			input: "a:b c:d",
			want: expr.AND(
				expr.Eq("a", "b"),
				expr.Eq("c", "d"),
			),
		},
		"basic_and": {
			input: "a AND b",
			want:  expr.AND("a", "b"),
		},
		"and_with_nesting": {
			input: "a:foo AND b:bar",
			want: expr.AND(
				expr.Eq("a", "foo"),
				expr.Eq("b", "bar"),
			),
		},
		"basic_or": {
			input: "a OR b",
			want: expr.OR(
				"a",
				"b",
			),
		},
		"range_operator_inclusive": {
			input: "a:[1 TO 5]",
			want:  expr.Rang("a", 1, 5, true),
		},
		"range_operator_inclusive_unbound": {
			input: `a:[* TO 200]`,
			want:  expr.Rang("a", expr.WILD("*"), expr.Lit(200), true),
		},
		"range_operator_exclusive": {
			input: `a:{"ab" TO "az"}`,
			want:  expr.Rang("a", expr.Lit("ab"), expr.Lit("az"), false),
		},
		"range_operator_exclusive_unbound": {
			input: `a:{2 TO *}`,
			want:  expr.Rang("a", expr.Lit(2), expr.WILD("*"), false),
		},
		"or_with_nesting": {
			input: "a:foo OR b:bar",
			want: expr.OR(
				expr.Eq("a", "foo"),
				expr.Eq("b", "bar"),
			),
		},
		"basic_not": {
			input: "NOT b",
			want:  expr.NOT("b"),
		},
		"nested_not": {
			input: "a:foo OR NOT b:bar",
			want: expr.OR(
				expr.Eq("a", "foo"),
				expr.NOT(expr.Eq("b", "bar")),
			),
		},
		"term_grouping": {
			input: "(a:foo OR b:bar) AND c:baz",
			want: expr.AND(
				expr.OR(
					expr.Eq("a", "foo"),
					expr.Eq("b", "bar"),
				),
				expr.Eq("c", "baz"),
			),
		},
		"value_grouping": {
			input: "a:(foo OR baz OR bar)",
			want: expr.Eq(
				"a",
				expr.OR(
					expr.OR(
						"foo",
						"baz",
					),
					"bar",
				),
			),
		},
		"basic_must": {
			input: "+a:b",
			want: expr.MUST(
				expr.Eq("a", "b"),
			),
		},
		"basic_must_not": {
			input: "-a:b",
			want: expr.MUSTNOT(
				expr.Eq("a", "b"),
			),
		},
		"basic_nested_must_not": {
			input: "d:e AND (-a:b AND +f:e)",
			want: expr.AND(
				expr.Eq("d", "e"),
				expr.AND(
					expr.MUSTNOT(expr.Eq("a", "b")),
					expr.MUST(expr.Eq("f", "e")),
				),
			),
		},
		"basic_escaping": {
			input: `a:\(1\+1\)\:2`,
			want:  expr.Eq("a", expr.Lit(`\(1\+1\)\:2`)),
		},
		"boost_key_value": {
			input: "a:b^2 AND foo",
			want: expr.AND(
				expr.BOOST(expr.Eq("a", "b"), 2),
				"foo",
			),
		},
		"boost_literal": {
			input: "foo^4",
			want:  expr.BOOST("foo", 4),
		},
		"boost_literal_in_compound": {
			input: "a:b AND foo^4",
			want: expr.AND(
				expr.Eq("a", "b"),
				expr.BOOST("foo", 4),
			),
		},
		"boost_literal_leading": {
			input: "foo^4 AND a:b",
			want: expr.AND(
				expr.BOOST("foo", 4),
				expr.Eq("a", "b"),
			),
		},
		"boost_quoted_literal": {
			input: `"foo bar"^4 AND a:b`,
			want: expr.AND(
				expr.BOOST(expr.Lit("foo bar"), 4),
				expr.Eq("a", "b"),
			),
		},
		"boost_sub_expression": {
			input: "(title:foo OR title:bar)^1.5 AND (body:foo OR body:bar)",
			want: expr.AND(
				expr.BOOST(
					expr.OR(
						expr.Eq("title", "foo"),
						expr.Eq("title", "bar"),
					),
					1.5),
				expr.OR(
					expr.Eq("body", "foo"),
					expr.Eq("body", "bar"),
				),
			),
		},
		"nested_sub_expressions_with_boost": {
			input: "((title:foo)^1.2 OR title:bar) AND (body:foo OR body:bar)",
			want: expr.AND(
				expr.OR(
					expr.BOOST(expr.Eq("title", "foo"), 1.2),
					expr.Eq("title", "bar"),
				),
				expr.OR(
					expr.Eq("body", "foo"),
					expr.Eq("body", "bar"),
				),
			),
		},
		"nested_sub_expressions": {
			input: "((title:foo OR title:bar) AND (body:foo OR body:bar)) OR k:v",
			want: expr.OR(
				expr.AND(
					expr.OR(
						expr.Eq("title", "foo"),
						expr.Eq("title", "bar"),
					),

					expr.OR(
						expr.Eq("body", "foo"),
						expr.Eq("body", "bar"),
					),
				),
				expr.Eq("k", "v"),
			),
		},
		"fuzzy_key_value": {
			input: "a:b~2 AND foo",
			want: expr.AND(
				expr.FUZZY(expr.Eq("a", "b"), 2),
				"foo",
			),
		},
		"fuzzy_key_value_default": {
			input: "a:b~ AND foo",
			want: expr.AND(
				expr.FUZZY(expr.Eq("a", "b"), 1),
				"foo",
			),
		},
		"fuzzy_literal": {
			input: "foo~4",
			want:  expr.FUZZY("foo", 4),
		},
		"fuzzy_literal_default": {
			input: "foo~",
			want:  expr.FUZZY("foo", 1),
		},
		"fuzzy_literal_in_compound": {
			input: "a:b AND foo~4",
			want: expr.AND(
				expr.Eq("a", "b"),
				expr.FUZZY("foo", 4),
			),
		},
		"fuzzy_literal_in_implicit_compound": {
			input: "a:b foo~4",
			want: expr.AND(
				expr.Eq("a", "b"),
				expr.FUZZY("foo", 4),
			),
		},
		"fuzzy_literal_leading": {
			input: "foo~4 AND a:b",
			want: expr.AND(
				expr.FUZZY("foo", 4),
				expr.Eq("a", "b"),
			),
		},
		"fuzzy_literal_leading_in_implicit_compound": {
			input: "foo~4 AND a:b",
			want: expr.AND(
				expr.FUZZY("foo", 4),
				expr.Eq("a", "b"),
			),
		},
		"fuzzy_quoted_literal": {
			input: `"foo bar"~4 AND a:b`,
			want: expr.AND(
				expr.FUZZY(expr.Lit("foo bar"), 4),
				expr.Eq("a", "b"),
			),
		},
		"fuzzy_sub_expression": {
			input: "(title:foo OR title:bar)~2 AND (body:foo OR body:bar)",
			want: expr.AND(
				expr.FUZZY(
					expr.OR(
						expr.Eq("title", "foo"),
						expr.Eq("title", "bar"),
					),
					2),
				expr.OR(
					expr.Eq("body", "foo"),
					expr.Eq("body", "bar"),
				),
			),
		},
		"nested_sub_expressions_with_fuzzy": {
			input: "((title:foo)~ OR title:bar) AND (body:foo OR body:bar)",
			want: expr.AND(
				expr.OR(
					expr.FUZZY(expr.Eq("title", "foo"), 1),
					expr.Eq("title", "bar"),
				),

				expr.OR(
					expr.Eq("body", "foo"),
					expr.Eq("body", "bar"),
				),
			),
		},
		"precedence_works": {
			input: "a:b AND c:d OR e:f OR h:i AND j:k",
			want: expr.OR(
				expr.OR(
					expr.AND(
						expr.Eq("a", "b"),
						expr.Eq("c", "d"),
					),
					expr.Eq("e", "f")),
				expr.AND(
					expr.Eq("h", "i"),
					expr.Eq("j", "k"),
				),
			),
		},
		"test_precedence_weaving": {
			input: "a OR b AND c OR d",
			want: expr.OR(
				expr.OR(
					"a",
					expr.AND("b", "c"),
				),
				"d",
			),
		},
		"test_precedence_weaving_with_not": {
			input: "NOT a OR b AND NOT c OR d",
			want: expr.OR(
				expr.OR(
					expr.NOT("a"),
					expr.AND("b", expr.NOT("c")),
				),
				"d",
			),
		},
		"test_equals_in_precedence": {
			input: "a:az OR b:bz AND NOT c:z OR d",
			want: expr.OR(
				expr.OR(
					expr.Eq("a", "az"),
					expr.AND(
						expr.Eq("b", "bz"),
						expr.NOT(
							expr.Eq("c", "z"),
						),
					),
				),
				"d",
			),
		},
		"test_parens_in_precedence": {
			input: "a AND (c OR d)",
			want: expr.AND(
				"a",
				expr.OR(
					"c",
					"d",
				),
			),
		},
		"test_range_precedance_simple": {
			input: "c:[* to -1] OR d",
			want: expr.OR(
				expr.Rang("c", expr.WILD("*"), -1, true),
				"d",
			),
		},
		"test_range_precedance": {
			input: "a OR b AND c:[* to -1] OR d",
			want: expr.OR(
				expr.OR(
					"a",
					expr.AND(
						"b",
						expr.Rang("c", expr.WILD("*"), -1, true),
					),
				),
				"d",
			),
		},
		"test_full_precedance": {
			input: "a OR b AND c:[* to -1] OR d AND NOT +e:f",
			want: expr.OR(
				expr.OR(
					"a",
					expr.AND(
						"b",
						expr.Rang("c", expr.WILD("*"), -1, true),
					),
				),
				expr.AND(
					"d",
					expr.NOT(
						expr.MUST(expr.Eq("e", "f")),
					),
				),
			),
		},
		"test_full_precedance_with_suffixes": {
			input: "a OR b AND c OR d~ AND NOT +(e:f)^10",
			want: expr.OR(
				expr.OR(
					"a",
					expr.AND("b", "c"),
				),
				expr.AND(
					expr.FUZZY("d", 1),
					expr.NOT(
						expr.BOOST(
							expr.MUST(
								expr.Eq("e", "f"),
							),
							10.0,
						),
					),
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
				t.Fatalf(errTemplate, "parsed expression doesn't match", tc.want, got)
			}

			raw, err := json.Marshal(got)
			if err != nil {
				t.Fatalf("wanted no error marshalling to json, got: %s", err)
			}

			var gotSerialized expr.Expression
			err = json.Unmarshal(raw, &gotSerialized)
			if err != nil {
				t.Fatalf("wanted no error unmarshalling from json, got: %s", err)
			}

			if !reflect.DeepEqual(got, &gotSerialized) {
				t.Fatalf(errTemplate, "roundtrip serialization is not stable", tc.want, got)
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
		"and_without_lhs": {
			input: "AND a",
		},
		"or_without_rhs": {
			input: "a OR",
		},
		"or_without_lhs": {
			input: "OR a",
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
		"nested_range_missing_max": {
			input: "(A:B AND C:(D OR E)) OR (NOT(+a:[* TO]))",
		},
		"invalid_implicit": {
			input: "a: b:c",
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
		"A:bar",
		"NOT(b:c)",
		"z:[* TO 10]",
		"x:[10 TO *] AND NOT(y:[1 TO 5]",
		"(+a:b -c:d) OR (z:[1 TO *] NOT(foo))",
		`+bbq:"woo yay"`,
		`-bbq:"woo"`,
		`(a:b)^10`,
		`a:foo~`,
	}
	for _, tc := range tcs {
		f.Add(tc)
	}
	f.Fuzz(func(t *testing.T, in string) {
		e, err := Parse(in)
		if err == nil {
			fmt.Printf("%s\n-----------\n", e)
		}
	})
}
