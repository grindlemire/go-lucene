package lucene

import (
	"encoding/json"
	"reflect"
	"strings"
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
		"bare_null": {
			input: "field:null",
			want:  expr.Eq("field", expr.NULL()),
		},
		"bare_null_uppercase": {
			input: "field:NULL",
			want:  expr.Eq("field", expr.NULL()),
		},
		"bare_null_mixed_case": {
			input: "field:Null",
			want:  expr.Eq("field", expr.NULL()),
		},
		"quoted_null_stays_string": {
			input: `field:"null"`,
			want:  expr.Eq("field", "null"),
		},
		"null_as_field_name_stays_string": {
			input: "null:foo",
			want:  expr.Eq("null", "foo"),
		},
		"null_wildcard_stays_wildcard": {
			input: "field:nul*",
			want:  expr.LIKE("field", "nul*"),
		},
		"or_with_null_folds_to_in_list": {
			input: "field:(a OR null)",
			want:  expr.IN(expr.Lit("field"), expr.LIST(expr.Lit("a"), expr.NULL())),
		},
		"or_chain_with_null": {
			input: "field:(a OR b OR null)",
			want:  expr.IN(expr.Lit("field"), expr.LIST(expr.Lit("a"), expr.Lit("b"), expr.NULL())),
		},
		"or_with_only_nulls_folds": {
			input: "field:(null OR null)",
			want:  expr.IN(expr.Lit("field"), expr.LIST(expr.NULL(), expr.NULL())),
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
		"basic_greater_less_with_number": {
			input: "a:<22 AND b:>33",
			want:  expr.AND(expr.LESS("a", 22), expr.GREATER("b", 33)),
		},
		"basic_greater_less_eq_with_number": {
			input: "a:<=22 AND b:>=33",
			want:  expr.AND(expr.LESSEQ("a", 22), expr.GREATEREQ("b", 33)),
		},
		"basic_wild_equal_with_*": {
			input: "a:b*",
			want:  expr.LIKE("a", "b*"),
		},
		"basic_wild_equal_with_?": {
			input: "a:b?z",
			want:  expr.LIKE("a", expr.WILD("b?z")),
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
		"regexp_with_escaped_chars": {
			input: `url:/example.com\/foo\/bar\/.*/`,
			want:  expr.Eq("url", expr.REGEXP(`/example.com\/foo\/bar\/.*/`)),
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
			want: expr.IN(
				"a",
				expr.LIST(
					expr.Lit("foo"),
					expr.Lit("baz"),
					expr.Lit("bar"),
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
			want:  expr.Eq("a", expr.Lit(`(1+1):2`)),
		},
		"escaped_column_name": {
			input: `foo\ bar:b`,
			want:  expr.Eq(`foo bar`, "b"),
		},
		"phrase_with_escaped_double_quote": {
			input: `name:"foo\"bar"`,
			want:  expr.Eq("name", `foo"bar`),
		},
		"single_quoted_phrase_plain": {
			input: `name:'foo bar'`,
			want:  expr.Eq("name", `foo bar`),
		},
		"single_quoted_phrase_with_escaped_quote": {
			input: `name:'foo\'bar'`,
			want:  expr.Eq("name", `foo'bar`),
		},
		"single_quoted_phrase_with_escaped_backslash": {
			input: `name:'foo\\bar'`,
			want:  expr.Eq("name", `foo\bar`),
		},
		"single_quoted_phrase_preserves_unescaped_backslash": {
			input: `name:'C:\temp'`,
			want:  expr.Eq("name", `C:\temp`),
		},
		"phrase_with_escaped_backslash": {
			input: `name:"foo\\bar"`,
			want:  expr.Eq("name", `foo\bar`),
		},
		"phrase_preserves_unescaped_backslash": {
			input: `name:"C:\temp"`,
			want:  expr.Eq("name", `C:\temp`),
		},
		"phrase_escaped_quote_prevents_injection": {
			input: `name:"acme\" OR admin:true OR name:\"x"`,
			want:  expr.Eq("name", `acme" OR admin:true OR name:"x`),
		},
		"phrase_escaped_backslash_at_boundary": {
			// the escaped-backslash pair sits immediately before the real
			// closing quote, pinning the off-by-one at the end of the scan.
			input: `name:"foo\\"`,
			want:  expr.Eq("name", `foo\`),
		},
		"phrase_with_multibyte_utf8_near_escape": {
			input: `name:"café\"日本語"`,
			want:  expr.Eq("name", `café"日本語`),
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
		"boost_key_implicit_power_before_term": {
			input: "color:red^ k1:v1",
			want: expr.AND(
				expr.BOOST(expr.Eq("color", "red"), 1),
				expr.Eq("k1", "v1"),
			),
		},
		"boost_key_power_before_term": {
			input: "color:red^2 k1:v1",
			want: expr.AND(
				expr.BOOST(expr.Eq("color", "red"), 2),
				expr.Eq("k1", "v1"),
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
		"fuzzy_key_implicit_distance_before_term": {
			input: "color:red~ k1:v1",
			want: expr.AND(
				expr.FUZZY(expr.Eq("color", "red"), 1),
				expr.Eq("k1", "v1"),
			),
		},
		"fuzzy_key_distance_before_term": {
			input: "color:red~2 k1:v1",
			want: expr.AND(
				expr.FUZZY(expr.Eq("color", "red"), 2),
				expr.Eq("k1", "v1"),
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
		"test_range_precedence_simple": {
			input: "c:[* to -1] OR d",
			want: expr.OR(
				expr.Rang("c", expr.WILD("*"), -1, true),
				"d",
			),
		},
		"test_range_precedence": {
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
		"test_full_precedence": {
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
		"test_full_precedence_with_suffixes": {
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
		"test_elastic_greater_than_precedence": {
			input: "a:>10 AND -b:<=-20",
			want: expr.AND(
				expr.GREATER("a", 10),
				expr.MUSTNOT(
					expr.LESSEQ("b", -20),
				),
			),
		},
		"implicit_and_with_subexpressions": {
			input: "a:b c:d",
			want: expr.AND(
				expr.Eq("a", "b"),
				expr.Eq("c", "d"),
			),
		},
		"implicit_and_with_negated_subexpressions": {
			input: "-a:b -c:d",
			want: expr.AND(
				expr.MUSTNOT(expr.Eq("a", "b")),
				expr.MUSTNOT(expr.Eq("c", "d")),
			),
		},
		"implicit_and_with_explicit_negation": {
			input: "a:b NOT c:d",
			want: expr.AND(
				expr.Eq("a", "b"),
				expr.NOT(expr.Eq("c", "d")),
			),
		},
		"implicit_and_with_boosted_subexpressions": {
			input: "a:b^ c:d^10",
			want: expr.AND(
				expr.BOOST(expr.Eq("a", "b"), 1),
				expr.BOOST(expr.Eq("c", "d"), 10),
			),
		},
		"implicit_and_with_boosted_subexpressions_reversed": {
			input: "a:b^10 c:d^",
			want: expr.AND(
				expr.BOOST(expr.Eq("a", "b"), 10),
				expr.BOOST(expr.Eq("c", "d"), 1),
			),
		},
		"implicit_and_with_fuzzy_subexpressions": {
			input: "a:b~ c:d~10",
			want: expr.AND(
				expr.FUZZY(expr.Eq("a", "b"), 1),
				expr.FUZZY(expr.Eq("c", "d"), 10),
			),
		},
		"implicit_and_with_fuzzy_subexpressions_reversed": {
			input: "a:b~10 c:d~",
			want: expr.AND(
				expr.FUZZY(expr.Eq("a", "b"), 10),
				expr.FUZZY(expr.Eq("c", "d"), 1),
			),
		},
		"implicit_and_with_precedence_operator": {
			input: `-k2:v2 k1:v1`,
			want: expr.AND(
				expr.MUSTNOT(expr.Eq("k2", "v2")),
				expr.Eq("k1", "v1"),
			),
		},
		"implicit_and_with_multiple_clauses": {
			input: `-(k1:v1) k2:v2 -k3:v3`,
			want: expr.AND(
				expr.AND(
					expr.MUSTNOT(expr.Eq("k1", "v1")),
					expr.Eq("k2", "v2"),
				),
				expr.MUSTNOT(expr.Eq("k3", "v3")),
			),
		},
		"implicit_and_with_multiple_clauses_inverted": {
			input: `k2:v2 -k1:v1 k3:v3`,
			want: expr.AND(
				expr.AND(
					expr.Eq("k2", "v2"),
					expr.MUSTNOT(expr.Eq("k1", "v1")),
				),
				expr.Eq("k3", "v3"),
			),
		},
		"implicit_and_before_group": {
			input: `k1:v1 (k2:v2 OR k3:v3)`,
			want: expr.AND(
				expr.Eq("k1", "v1"),
				expr.OR(
					expr.Eq("k2", "v2"),
					expr.Eq("k3", "v3"),
				),
			),
		},
		"implicit_and_between_groups": {
			input: `(k1:v1) (k2:v2)`,
			want: expr.AND(
				expr.Eq("k1", "v1"),
				expr.Eq("k2", "v2"),
			),
		},
		"implicit_and_between_or_groups": {
			input: `(k1:v1 OR k2:v2) (k3:v3 OR k4:v4)`,
			want: expr.AND(
				expr.OR(
					expr.Eq("k1", "v1"),
					expr.Eq("k2", "v2"),
				),
				expr.OR(
					expr.Eq("k3", "v3"),
					expr.Eq("k4", "v4"),
				),
			),
		},
		"implicit_and_before_field_group": {
			input: `k1:v1 k2:(v2 OR v3)`,
			want: expr.AND(
				expr.Eq("k1", "v1"),
				expr.IN(expr.Lit("k2"), expr.LIST(expr.Lit("v2"), expr.Lit("v3"))),
			),
		},
		"implicit_and_after_boost_before_group": {
			input: `k1:v1^2 (k2:v2)`,
			want: expr.AND(
				expr.BOOST(expr.Eq("k1", "v1"), 2),
				expr.Eq("k2", "v2"),
			),
		},
		"parenthesized_precedence": {
			input: `k1:v1 -(k2:v2 OR k3:v3)`,
			want: expr.AND(
				expr.Eq("k1", "v1"),
				expr.MUSTNOT(
					expr.OR(
						expr.Eq("k2", "v2"),
						expr.Eq("k3", "v3"),
					),
				),
			),
		},
		// percent sign tests — % is not a reserved Lucene character and must
		// be treated as a regular literal character in bare terms.
		"percent_sign_bare_literal": {
			input: "100%",
			want:  expr.Lit("100%"),
		},
		"percent_sign_implicit_and": {
			input: "DMSO 100%",
			want:  expr.AND(expr.Lit("DMSO"), expr.Lit("100%")),
		},
		"percent_sign_explicit_and": {
			input: "DMSO AND 100%",
			want:  expr.AND(expr.Lit("DMSO"), expr.Lit("100%")),
		},
		"percent_sign_field_value": {
			input: "concentration:100%",
			want:  expr.Eq("concentration", "100%"),
		},
		"percent_sign_mid_word": {
			input: "50%done",
			want:  expr.Lit("50%done"),
		},
		// comma and semicolon are not Lucene operators and should be treated
		// as whitespace separators (issue #48).
		"comma_as_separator": {
			input: "a, b",
			want:  expr.AND(expr.Lit("a"), expr.Lit("b")),
		},
		"comma_between_multiple_terms": {
			input: "a, b, c",
			want:  expr.AND(expr.AND(expr.Lit("a"), expr.Lit("b")), expr.Lit("c")),
		},
		"semicolon_as_separator": {
			input: "a; b",
			want:  expr.AND(expr.Lit("a"), expr.Lit("b")),
		},
		// @, #, $ are valid literal characters.
		"at_sign_in_literal": {
			input: "user@example.com",
			want:  expr.Lit("user@example.com"),
		},
		"hash_in_literal": {
			input: "#channel",
			want:  expr.Lit("#channel"),
		},
		"dollar_in_literal": {
			input: "$var",
			want:  expr.Lit("$var"),
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
				// occasionally this test fails and the error message makes the test look like
				// the want and got are equivalent. This is almost always an unexported var is different
				// Using testify/require will show the error if it shows up
				// require.Equal(t, tc.want, gotSerialized)
				t.Fatalf(errTemplate, "roundtrip serialization is not stable", tc.want, gotSerialized)
			}
		})
	}
}

func TestParseLuceneWithDefaultField(t *testing.T) {
	type tc struct {
		input        string
		defaultField string
		want         *expr.Expression
	}

	tcs := map[string]tc{
		"single_literal": {
			input:        "a",
			defaultField: "foo",
			want:         expr.Eq("foo", "a"),
		},
		"quoted_literal": {
			input:        `"a"`,
			defaultField: "foo",
			want:         expr.Eq("foo", "a"),
		},
		"number_literal": {
			input:        `7`,
			defaultField: "foo",
			want:         expr.Eq("foo", 7),
		},
		"multiple_literals": {
			input:        "a b",
			defaultField: "foo",
			want:         expr.AND(expr.Eq("foo", "a"), expr.Eq("foo", "b")),
		},
		"basic_and": {
			input:        "a AND b",
			defaultField: "foo",
			want:         expr.AND(expr.Eq("foo", "a"), expr.Eq("foo", "b")),
		},
		"standalone_null": {
			input:        "null",
			defaultField: "foo",
			want:         expr.Eq("foo", expr.NULL()),
		},
		"empty_quoted_string": {
			input:        `""`,
			defaultField: "foo",
			want:         expr.Eq("foo", expr.Lit("")),
		},
	}

	for name, tc := range tcs {
		t.Run(name, func(t *testing.T) {
			got, err := Parse(tc.input, WithDefaultField(tc.defaultField))
			if err != nil {
				t.Fatalf("wanted no error, got: %v", err)
			}
			if !reflect.DeepEqual(tc.want, got) {
				t.Fatalf(errTemplate, "parsed expression doesn't match", tc.want, got)
			}
		})
	}
}

// TestParseDefaultFieldReachesEveryTerm checks that the default field reaches
// every term with no field, not only words, phrases and numbers. A wildcard,
// regex, null, range, or a term under a boost or fuzzy only got it, if at
// all, when it was the whole query. x OR null failed to render and
// x OR foo* rendered the wildcard as a standalone string.
func TestParseDefaultFieldReachesEveryTerm(t *testing.T) {
	tcs := map[string]struct {
		input string
		want  *expr.Expression
	}{
		"or_null":     {`x OR null`, expr.OR(expr.Eq("default", "x"), expr.Eq("default", expr.NULL()))},
		"and_null":    {`null AND x`, expr.AND(expr.Eq("default", expr.NULL()), expr.Eq("default", "x"))},
		"not_null":    {`NOT null`, expr.NOT(expr.Eq("default", expr.NULL()))},
		"or_wildcard": {`x OR foo*`, expr.OR(expr.Eq("default", "x"), expr.LIKE("default", expr.WILD("foo*")))},
		"wildcard":    {`foo*`, expr.LIKE("default", expr.WILD("foo*"))},
		"must_not":    {`x -foo*`, expr.AND(expr.Eq("default", "x"), expr.MUSTNOT(expr.LIKE("default", expr.WILD("foo*"))))},
		"must":        {`+/re/`, expr.MUST(expr.LIKE("default", expr.REGEXP("/re/")))},
		"regexp":      {`/re/`, expr.LIKE("default", expr.REGEXP("/re/"))},
		"or_regexp":   {`x OR /re/`, expr.OR(expr.Eq("default", "x"), expr.LIKE("default", expr.REGEXP("/re/")))},
		"range":       {`[1 TO 5]`, expr.Rang("default", 1, 5, true)},
		"or_range":    {`x OR {1 TO 5}`, expr.OR(expr.Eq("default", "x"), expr.Rang("default", 1, 5, false))},
		"and_range":   {`x [1 TO *]`, expr.AND(expr.Eq("default", "x"), expr.Rang("default", 1, "*", true))},
		"boost":       {`x^2`, expr.BOOST(expr.Eq("default", "x"), 2.0)},
		"or_boost":    {`x^2 OR y`, expr.OR(expr.BOOST(expr.Eq("default", "x"), 2.0), expr.Eq("default", "y"))},
		"fuzzy":       {`foo~`, expr.FUZZY(expr.Eq("default", "foo"), 1)},
		"and_fuzzy":   {`x AND foo~2`, expr.AND(expr.Eq("default", "x"), expr.FUZZY(expr.Eq("default", "foo"), 2))},
		"group":       {`(foo*)`, expr.LIKE("default", expr.WILD("foo*"))},
		"next_to_field": {
			`a:b OR foo*`,
			expr.OR(expr.Eq("a", "b"), expr.LIKE("default", expr.WILD("foo*"))),
		},
		// A quoted term is a phrase, even when it is the whole query and
		// holds * or /. It used to be read again as a wildcard or regex.
		"quoted_star":   {`"fo*"`, expr.Eq("default", expr.Lit("fo*"))},
		"quoted_slash":  {`"/re/"`, expr.Eq("default", expr.Lit("/re/"))},
		"quoted_or":     {`"fo*" OR x`, expr.OR(expr.Eq("default", expr.Lit("fo*")), expr.Eq("default", "x"))},
		"quoted_number": {`"5"`, expr.Eq("default", expr.Lit("5"))},
		// Unchanged.
		"word":      {`x`, expr.Eq("default", "x")},
		"or_words":  {`x OR y`, expr.OR(expr.Eq("default", "x"), expr.Eq("default", "y"))},
		"null":      {`null`, expr.Eq("default", expr.NULL())},
		"field_ok":  {`a:foo*`, expr.LIKE("a", expr.WILD("foo*"))},
		"field_grp": {`a:(x OR foo*)`, expr.OR(expr.Eq("a", "x"), expr.LIKE("a", expr.WILD("foo*")))},
	}

	for name, tc := range tcs {
		t.Run(name, func(t *testing.T) {
			got, err := Parse(tc.input, WithDefaultField("default"))
			if err != nil {
				t.Fatalf("unexpected parse error: %v", err)
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf(errTemplate, "parsed expression does not match", tc.want, got)
			}
		})
	}

	// Without a default field the terms stay bare, as before.
	noDefault := map[string]*expr.Expression{
		`x OR foo*`: expr.OR(expr.Lit("x"), expr.WILD("foo*")),
		`x OR null`: expr.OR(expr.Lit("x"), expr.NULL()),
		`"fo*"`:     expr.Lit("fo*"),
		`foo~`:      expr.FUZZY(expr.Lit("foo"), 1),
	}
	for input, want := range noDefault {
		t.Run("no_default/"+input, func(t *testing.T) {
			got, err := Parse(input)
			if err != nil {
				t.Fatalf("unexpected parse error: %v", err)
			}
			if !reflect.DeepEqual(got, want) {
				t.Fatalf(errTemplate, "parsed expression does not match", want, got)
			}
		})
	}
}

// TestParseDefaultFieldInsideFieldGroup checks that a field group like
// a:(x OR y) keeps its terms on field a when a default field is set. The
// default field used to be applied to them, so a:(x OR null) parsed as
// a:(default:x OR null). Terms outside any field group still get it.
func TestParseDefaultFieldInsideFieldGroup(t *testing.T) {
	tcs := map[string]struct {
		input string
		want  *expr.Expression
	}{
		"null_in_group": {
			input: `a:(x OR null)`,
			want:  expr.IN(expr.Lit("a"), expr.LIST(expr.Lit("x"), expr.NULL())),
		},
		"terms_in_group": {
			input: `a:(x OR y)`,
			want:  expr.IN(expr.Lit("a"), expr.LIST(expr.Lit("x"), expr.Lit("y"))),
		},
		"phrase_in_group": {
			input: `a:("foo bar" OR baz)`,
			want:  expr.IN(expr.Lit("a"), expr.LIST(expr.Lit("foo bar"), expr.Lit("baz"))),
		},
		"equals_sign_group": {
			input: `a=(x OR null)`,
			want:  expr.IN(expr.Lit("a"), expr.LIST(expr.Lit("x"), expr.NULL())),
		},
		"default_applies_after_group": {
			input: `a:(x OR y) AND z`,
			want: expr.AND(
				expr.IN(expr.Lit("a"), expr.LIST(expr.Lit("x"), expr.Lit("y"))),
				expr.Eq("default", "z"),
			),
		},
		"default_applies_before_group": {
			input: `z a:(x OR y)`,
			want: expr.AND(
				expr.Eq("default", "z"),
				expr.IN(expr.Lit("a"), expr.LIST(expr.Lit("x"), expr.Lit("y"))),
			),
		},
		"bare_group_gets_default": {
			input: `(x OR y)`,
			want:  expr.OR(expr.Eq("default", "x"), expr.Eq("default", "y")),
		},
		"default_applies_inside_bare_group_after_field_term": {
			input: `(a:x OR y)`,
			want:  expr.OR(expr.Eq("a", "x"), expr.Eq("default", "y")),
		},
	}

	for name, tc := range tcs {
		t.Run(name, func(t *testing.T) {
			got, err := Parse(tc.input, WithDefaultField("default"))
			if err != nil {
				t.Fatalf("wanted no error, got: %v", err)
			}
			if !reflect.DeepEqual(tc.want, got) {
				t.Fatalf(errTemplate, "parsed expression doesn't match", tc.want, got)
			}
		})
	}
}

// TestParseFieldGroupIgnoresDefaultField checks queries where every term sits
// inside a field group: each must parse the same with and without a default
// field, whatever operators or modifiers are in or around the group.
func TestParseFieldGroupIgnoresDefaultField(t *testing.T) {
	inputs := map[string]string{
		"nested_group":          `a:(x OR (y AND z))`,
		"double_parens":         `a:((x OR null))`,
		"implicit_and":          `a:(x y)`,
		"must_not_in_group":     `a:(x -y)`,
		"must_in_group":         `a:(+x OR y)`,
		"not_in_group":          `a:(x OR NOT y)`,
		"wildcard_in_group":     `a:(x OR foo*)`,
		"regexp_in_group":       `a:(x OR /re/)`,
		"field_term_in_group":   `a:(b:c OR d)`,
		"range_in_group":        `a:(x OR b:[1 TO 5])`,
		"boost_in_group":        `a:(x^2 OR y)`,
		"fuzzy_in_group":        `a:(x~ OR y)`,
		"bare_range_in_group":   `a:(x OR [1 TO 5])`,
		"boosted_group":         `a:(x OR y)^2`,
		"fuzzy_group":           `a:(x OR y)~`,
		"must_group":            `+a:(x OR null)`,
		"must_not_group":        `-a:(x OR null)`,
		"not_group":             `NOT a:(x OR null)`,
		"two_groups":            `a:(x OR y) AND b:(z OR null)`,
		"group_inside_bare_one": `(a:(x OR y) OR b:z)`,
	}

	for name, input := range inputs {
		t.Run(name, func(t *testing.T) {
			want, err := Parse(input)
			if err != nil {
				t.Fatalf("wanted no error without a default field, got: %v", err)
			}
			got, err := Parse(input, WithDefaultField("default"))
			if err != nil {
				t.Fatalf("wanted no error with a default field, got: %v", err)
			}
			if !reflect.DeepEqual(want, got) {
				t.Fatalf(errTemplate, "default field changed the parse", want, got)
			}
		})
	}
}

// TestParseFieldGroupAppliesFieldToEachTerm checks that a field group applies
// its field to every term inside it, as Lucene does. Only a plain OR of terms
// used to get the field: a:(x AND y) parsed as a:((x AND y)), comparing the
// column to a boolean expression, and a:(x OR [1 TO 5]) failed to parse. A
// term with its own field keeps it, so a:(b:c) is b:c.
func TestParseFieldGroupAppliesFieldToEachTerm(t *testing.T) {
	tcs := map[string]struct {
		input string
		want  *expr.Expression
	}{
		"and":          {`a:(x AND y)`, expr.AND(expr.Eq("a", "x"), expr.Eq("a", "y"))},
		"implicit_and": {`a:(x y)`, expr.AND(expr.Eq("a", "x"), expr.Eq("a", "y"))},
		"must_not":     {`a:(x -y)`, expr.AND(expr.Eq("a", "x"), expr.MUSTNOT(expr.Eq("a", "y")))},
		"must":         {`a:(+x OR y)`, expr.OR(expr.MUST(expr.Eq("a", "x")), expr.Eq("a", "y"))},
		"not":          {`a:(x OR NOT y)`, expr.OR(expr.Eq("a", "x"), expr.NOT(expr.Eq("a", "y")))},
		"only_not":     {`a:(NOT x)`, expr.NOT(expr.Eq("a", "x"))},
		"wildcard":     {`a:(x OR foo*)`, expr.OR(expr.Eq("a", "x"), expr.LIKE("a", expr.WILD("foo*")))},
		"regexp":       {`a:(x OR /re/)`, expr.OR(expr.Eq("a", "x"), expr.LIKE("a", expr.REGEXP("/re/")))},
		"null_and":     {`a:(x AND null)`, expr.AND(expr.Eq("a", "x"), expr.Eq("a", expr.NULL()))},
		"phrase":       {`a:("x y" AND z)`, expr.AND(expr.Eq("a", "x y"), expr.Eq("a", "z"))},
		"range":        {`a:(x OR [1 TO 5])`, expr.OR(expr.Eq("a", "x"), expr.Rang("a", 1, 5, true))},
		"exclusive":    {`a:(x OR {1 TO 5})`, expr.OR(expr.Eq("a", "x"), expr.Rang("a", 1, 5, false))},
		"only_range":   {`a:([1 TO *])`, expr.Rang("a", 1, "*", true)},
		"nested_or": {
			`a:(x OR (y AND z))`,
			expr.OR(expr.Eq("a", "x"), expr.AND(expr.Eq("a", "y"), expr.Eq("a", "z"))),
		},
		"nested_and": {
			`a:((x OR y) AND z)`,
			expr.AND(expr.OR(expr.Eq("a", "x"), expr.Eq("a", "y")), expr.Eq("a", "z")),
		},
		"nested_group": {
			`a:(x AND (y OR (z -w)))`,
			expr.AND(
				expr.Eq("a", "x"),
				expr.OR(expr.Eq("a", "y"), expr.AND(expr.Eq("a", "z"), expr.MUSTNOT(expr.Eq("a", "w")))),
			),
		},
		"boost":        {`a:(x^2 OR y)`, expr.OR(expr.BOOST(expr.Eq("a", "x"), 2.0), expr.Eq("a", "y"))},
		"fuzzy":        {`a:(x~2 AND y)`, expr.AND(expr.FUZZY(expr.Eq("a", "x"), 2), expr.Eq("a", "y"))},
		"own_field":    {`a:(b:c OR d)`, expr.OR(expr.Eq("b", "c"), expr.Eq("a", "d"))},
		"only_field":   {`a:(b:c)`, expr.Eq("b", "c")},
		"own_range":    {`a:(x OR b:[1 TO 5])`, expr.OR(expr.Eq("a", "x"), expr.Rang("b", 1, 5, true))},
		"own_group":    {`a:(x AND b:(y OR z))`, expr.AND(expr.Eq("a", "x"), expr.IN("b", expr.LIST(expr.Lit("y"), expr.Lit("z"))))},
		"equals_sign":  {`a=(x AND y)`, expr.AND(expr.Eq("a", "x"), expr.Eq("a", "y"))},
		"equals_range": {`a=[1 TO 5]`, expr.Rang("a", 1, 5, true)},
		"two_ranges": {
			`a:([1 TO 5] OR [7 TO 9])`,
			expr.OR(expr.Rang("a", 1, 5, true), expr.Rang("a", 7, 9, true)),
		},
		"implicit_and_range": {
			`a:(x [1 TO 5])`,
			expr.AND(expr.Eq("a", "x"), expr.Rang("a", 1, 5, true)),
		},
		"implicit_and_ranges": {
			`a:([1 TO 5] {7 TO 9})`,
			expr.AND(expr.Rang("a", 1, 5, true), expr.Rang("a", 7, 9, false)),
		},
		"numeric": {`0:(x AND y)`, expr.AND(expr.Eq("0", "x"), expr.Eq("0", "y"))},
		"must_not_group": {
			`-a:(x AND y)`,
			expr.MUSTNOT(expr.AND(expr.Eq("a", "x"), expr.Eq("a", "y"))),
		},
		"boosted_group": {
			`a:(x AND y)^2`,
			expr.BOOST(expr.AND(expr.Eq("a", "x"), expr.Eq("a", "y")), 2.0),
		},
		"next_to_term": {
			`a:(x AND y) OR b:z`,
			expr.OR(expr.AND(expr.Eq("a", "x"), expr.Eq("a", "y")), expr.Eq("b", "z")),
		},
		// Unchanged: a single term, and a plain OR of terms, which is an IN.
		"single_term":   {`a:(x)`, expr.Eq("a", "x")},
		"single_wild":   {`a:(foo*)`, expr.LIKE("a", expr.WILD("foo*"))},
		"or_of_terms":   {`a:(x OR y)`, expr.IN("a", expr.LIST(expr.Lit("x"), expr.Lit("y")))},
		"double_parens": {`a:((x OR y))`, expr.IN("a", expr.LIST(expr.Lit("x"), expr.Lit("y")))},
		"plain_range":   {`a:[1 TO 5]`, expr.Rang("a", 1, 5, true)},
		"mixed_range":   {`a:{1 TO 5]`, expr.Rang("a", 1, 5, false)},
	}

	for name, tc := range tcs {
		for variant, opts := range map[string][]Opt{"no_default": nil, "default_field": {WithDefaultField("default")}} {
			t.Run(name+"/"+variant, func(t *testing.T) {
				got, err := Parse(tc.input, opts...)
				if err != nil {
					t.Fatalf("unexpected parse error: %v", err)
				}
				if !reflect.DeepEqual(got, tc.want) {
					t.Fatalf(errTemplate, "parsed expression does not match", tc.want, got)
				}
			})
		}
	}
}

// TestParseCompareGroupIsError checks that a comparison takes a single term.
// a:>(x OR y) used to parse as a comparison against a boolean expression.
// Lucene has no comparison syntax, and Elasticsearch's query_string only reads
// a:>5 as one term with a > in front, so neither gives a meaning to a
// comparison on a group or a range.
func TestParseCompareGroupIsError(t *testing.T) {
	inputs := map[string]string{
		"or":         `a:>(x OR y)`,
		"and":        `a:>=(x AND y)`,
		"must_not":   `a:<(x -y)`,
		"less_eq":    `a:<=(x OR y)`,
		"field":      `a:>(b:c)`,
		"range":      `a:>[1 TO 5]`,
		"group_rang": `a:>([1 TO 5])`,
	}
	for name, input := range inputs {
		for variant, opts := range map[string][]Opt{"no_default": nil, "default_field": {WithDefaultField("default")}} {
			t.Run(name+"/"+variant, func(t *testing.T) {
				got, err := Parse(input, opts...)
				if err == nil {
					t.Fatalf("expected an error, got %#v", got)
				}
				if !strings.Contains(err.Error(), "must be a single term") {
					t.Fatalf("expected a single-term error, got: %v", err)
				}
			})
		}
	}

	// A single term in parentheses is still a comparison.
	got, err := Parse(`a:>(5)`)
	if err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}
	if want := expr.GREATER("a", 5); !reflect.DeepEqual(got, want) {
		t.Fatalf(errTemplate, "parsed expression does not match", want, got)
	}
}

// TestParseRangeNeedsField checks that a range outside a field group, with no
// default field, is still an error, now with a message that says why.
func TestParseRangeNeedsField(t *testing.T) {
	for _, input := range []string{`[1 TO 5]`, `x OR [1 TO 5]`, `a:x AND {1 TO 5}`, `a:x [1 TO 5]`} {
		t.Run(input, func(t *testing.T) {
			got, err := Parse(input)
			if err == nil {
				t.Fatalf("expected an error, got %#v", got)
			}
			if !strings.Contains(err.Error(), "range needs a field") {
				t.Fatalf("expected a range field error, got: %v", err)
			}
		})
	}
}

// TestParseNumberSpellings checks numbers written with a + in the exponent or
// a leading dot. a:1E+3 used to parse as a:1E AND +3, and a:.5 and a:-.5
// failed to parse.
func TestParseNumberSpellings(t *testing.T) {
	tcs := map[string]struct {
		input string
		want  *expr.Expression
	}{
		"exponent_plus":       {`a:1E+3`, expr.Eq("a", expr.NumberLit(1000.0, "1E+3"))},
		"exponent_plus_lower": {`a:1e+3`, expr.Eq("a", expr.NumberLit(1000.0, "1e+3"))},
		"negative_exponent":   {`a:-2.5e+2`, expr.Eq("a", expr.NumberLit(-250.0, "-2.5e+2"))},
		"exponent_in_range":   {`a:[1e+2 TO 1E+3]`, expr.Rang("a", expr.NumberLit(100.0, "1e+2"), expr.NumberLit(1000.0, "1E+3"), true)},
		"exponent_compare":    {`a:>=1e+3`, expr.GREATEREQ("a", expr.NumberLit(1000.0, "1e+3"))},
		"exponent_in_list":    {`a:(1e+3 OR 2)`, expr.IN("a", expr.LIST(expr.NumberLit(1000.0, "1e+3"), expr.Lit(2)))},
		"exponent_boost":      {`a:b^1e+1`, expr.BOOST(expr.Eq("a", "b"), 10.0)},
		"leading_dot":         {`a:.5`, expr.Eq("a", expr.NumberLit(0.5, ".5"))},
		"negative_dot":        {`a:-.5`, expr.Eq("a", expr.NumberLit(-0.5, "-.5"))},
		"dot_range":           {`a:[-.5 TO .5]`, expr.Rang("a", expr.NumberLit(-0.5, "-.5"), expr.NumberLit(0.5, ".5"), true)},
		"dot_exponent":        {`a:.5e+1`, expr.Eq("a", expr.NumberLit(5.0, ".5e+1"))},
		"dot_word":            {`a:.foo`, expr.Eq("a", ".foo")},
		"dot_field":           {`.5:x`, expr.Eq(".5", "x")},
		"minus_dot_word":      {`-.foo`, expr.MUSTNOT(expr.Lit(".foo"))},
		"exponent_minus":      {`a:1e-3`, expr.Eq("a", expr.NumberLit(0.001, "1e-3"))},

		// A + inside a term is part of it, as in Lucene and like -. These
		// used to split at the +, so a:be+3 parsed as a:be AND +3.
		"plus_after_word":   {`a:be+3`, expr.Eq("a", "be+3")},
		"plus_after_number": {`a:1+2`, expr.Eq("a", "1+2")},
		"plus_at_end":       {`a:c++`, expr.Eq("a", "c++")},
		"plus_no_exponent":  {`a:1e+`, expr.Eq("a", "1e+")},
		"plus_in_field":     {`a+b:c`, expr.Eq("a+b", "c")},
		"plus_in_range":     {`a:[a+b TO c+d]`, expr.Rang("a", "a+b", "c+d", true)},
		"plus_in_group":     {`a:(c++ OR b)`, expr.IN("a", expr.LIST(expr.Lit("c++"), expr.Lit("b")))},
		// A + that starts a term still means MUST.
		"plus_after_space": {`a:b +c:d`, expr.AND(expr.Eq("a", "b"), expr.MUST(expr.Eq("c", "d")))},
		"plus_after_paren": {`(a:b)+c:d`, expr.AND(expr.Eq("a", "b"), expr.MUST(expr.Eq("c", "d")))},
	}

	for name, tc := range tcs {
		t.Run(name, func(t *testing.T) {
			got, err := Parse(tc.input)
			if err != nil {
				t.Fatalf("unexpected parse error: %v", err)
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf(errTemplate, "parsed expression does not match", tc.want, got)
			}
		})
	}

	// A term can't start with +, so a:+5 stays an error, as in Lucene and
	// like a:-x. Escape it to search for the text: a:\+5.
	for _, input := range []string{`a:+5`, `a:+.5`, `a: +b`} {
		t.Run("error/"+input, func(t *testing.T) {
			if got, err := Parse(input); err == nil {
				t.Fatalf("expected an error, got %#v", got)
			}
		})
	}
}

// TestParseReportsLexerError checks that a query the lexer rejects fails
// with the lexer's message. The parser used to drop it and report "no items
// left to reduce" instead.
func TestParseReportsLexerError(t *testing.T) {
	tcs := map[string]struct {
		input string
		want  string
	}{
		"unterminated_quote":  {`a:"abc`, "unterminated quote"},
		"unterminated_regexp": {`a:/re`, "unterminated regexp"},
		"unknown_character":   {`a:b & c`, "error parsing token [&]"},
		"after_a_term":        {`a:b OR "c`, "unterminated quote"},
		// A trailing backslash used to stay in the term: a*\ was the
		// wildcard a*\ and a:\ compared a to the empty string.
		"trailing_backslash":       {`a*\`, "term can not end with an escape character"},
		"trailing_backslash_value": {`a:\`, "term can not end with an escape character"},
		"lone_backslash":           {`\`, "term can not end with an escape character"},
		"backslash_in_group":       {`a:(b OR c\`, "term can not end with an escape character"},
	}
	for name, tc := range tcs {
		t.Run(name, func(t *testing.T) {
			got, err := Parse(tc.input)
			if err == nil {
				t.Fatalf("expected an error, got %#v", got)
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("expected an error containing %q, got: %v", tc.want, err)
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
		"boost_before_colon_nonterminal": {
			input: "color:red^:k1",
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
		"fuzzy_before_colon_nonterminal": {
			input: "color:red~:k1",
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
		_, _ = Parse(in)
	})
}

// TestParseSeparatorCharacters validates that commas, semicolons, and other
// non-Lucene characters do not cause parse errors (issue #48).
func TestParseSeparatorCharacters(t *testing.T) {
	inputs := map[string]string{
		"comma_from_issue_48": `\[*\] Actual go routines \[*\], allocated objects in bytes \[*\], allocated objects \[*\]`,
		"semicolons":          `a; b; c`,
		"mixed_separators":    `a, b; c`,
		"email_address":       `user@example.com`,
		"hash_tag":            `#channel`,
		"dollar_variable":     `$var`,
		"field_with_at":       `recipient:user@example.com`,
		"field_with_hash":     `tag:#important`,
	}

	for name, input := range inputs {
		t.Run(name, func(t *testing.T) {
			_, err := Parse(input)
			if err != nil {
				t.Fatalf("expected no error, got: %v", err)
			}
		})
	}
}

// TestQuotedPhraseRoundTrip verifies that a phrase containing an escaped
// quote (double- or single-delimited) survives Parse -> String() -> Parse
// unchanged (issue #59).
func TestQuotedPhraseRoundTrip(t *testing.T) {
	tcs := map[string]string{
		"escaped_quote":                   `name:"foo\"bar"`,
		"escaped_quote_with_space":        `name:"foo\"bar baz"`,
		"escaped_single_quote_no_space":   `name:'it\'s'`,
		"escaped_single_quote_with_space": `name:'it\'s here'`,
		"escaped_backslash_with_space":    `name:"foo bar\\"`,
	}

	for name, input := range tcs {
		t.Run(name, func(t *testing.T) {
			first, err := Parse(input)
			if err != nil {
				t.Fatalf("expected no error parsing %q, got: %v", input, err)
			}

			rendered := first.String()
			second, err := Parse(rendered)
			if err != nil {
				t.Fatalf("expected no error re-parsing rendered form %q, got: %v", rendered, err)
			}

			if !reflect.DeepEqual(first, second) {
				t.Fatalf(errTemplate, "round trip through String() changed the expression", first, second)
			}
		})
	}
}

// TestParseRangeBoundMustBeSingleTerm validates that each side of a range
// is a single term (a word, number, phrase, or *). Anything else used to
// parse and then render only part of the bound into SQL (issue #68).
func TestParseRangeBoundMustBeSingleTerm(t *testing.T) {
	invalid := map[string]string{
		"issue_repro":            `0:[0 0 TO 0]`,
		"implicit_and_min":       `a:[1 2 TO 3]`,
		"implicit_and_exclusive": `a:{x y TO z}`,
		"implicit_and_phrases":   `a:["x" "y" TO z]`,
		"explicit_and_min":       `a:[1 AND 2 TO 3]`,
		"explicit_or_min":        `a:[1 OR 2 TO 3]`,
		"grouped_or_min":         `a:[(1 OR 2) TO 3]`,
		"grouped_or_max":         `a:[1 TO (3 OR 4)]`,
		"grouped_and_max":        `a:[1 TO (3 4)]`,
		"grouped_field_max":      `a:[1 TO (b:c)]`,
		"nested_range_max":       `a:[1 TO (b:[1 TO 2])]`,
		"not_min":                `a:[NOT 1 TO 3]`,
		"must_min":               `a:[+1 TO 3]`,
		"must_not_min":           `a:[-x TO 3]`,
		"field_min":              `a:[b:c TO 3]`,
		"fuzzy_min":              `a:[1~ TO 3]`,
		"fuzzy_max":              `a:[1 TO 3~]`,
		"boost_min":              `a:[1^2 TO 3]`,
		"sql_breakout_min":       `a:["\" AND 1); DROP TABLE test; --" z TO q]`,
	}

	for name, input := range invalid {
		for variant, opts := range map[string][]Opt{"no_default": nil, "default_field": {WithDefaultField("default")}} {
			t.Run(name+"/"+variant, func(t *testing.T) {
				e, err := Parse(input, opts...)
				if err == nil {
					sql, _ := ToPostgres(input, opts...)
					t.Fatalf("expected parse error for %q, got %#v which renders to postgres as %q", input, e, sql)
				}
				if !strings.Contains(err.Error(), "must be a single term") {
					t.Fatalf("expected a single-term range bound error for %q, got: %v", input, err)
				}
			})
		}
	}

	valid := map[string]string{
		"inclusive":         `a:[1 TO 5]`,
		"exclusive":         `a:{1 TO 5}`,
		"mixed_brackets":    `a:[1 TO 5}`,
		"unbounded_min":     `a:[* TO 5]`,
		"unbounded_max":     `a:{2 TO *}`,
		"fully_unbounded":   `a:[* TO *]`,
		"quoted_bounds":     `a:{"ab" TO "az"}`,
		"quoted_with_space": `a:["x y" TO z]`,
		"single_quoted":     `a:['x y' TO z]`,
		"negative_number":   `a:[-1 TO 3]`,
		"float_bounds":      `a:[1.5 TO 3.25]`,
		"lowercase_to":      `a:[1 to 3]`,
		"followed_by_term":  `a:[1 TO 3] b`,
		"wildcard_bound":    `a:[ab* TO c]`,
		"regexp_bounds":     `a:[/x/ TO /y/]`,
		"null_bound":        `a:[null TO 3]`, // parses; rejected at render time
		"grouped_ranges":    `(a:[1 TO 2] OR b:{x TO y}) AND c:d`,
	}

	for name, input := range valid {
		for variant, opts := range map[string][]Opt{"no_default": nil, "default_field": {WithDefaultField("default")}} {
			t.Run(name+"/"+variant, func(t *testing.T) {
				if _, err := Parse(input, opts...); err != nil {
					t.Fatalf("expected %q to parse, got: %v", input, err)
				}
			})
		}
	}
}

// TestParseNumericFieldIsColumn checks that a field name that looks like a
// number is still a column, with its text kept as written. It used to parse
// as a number, so 0:1 rendered as `0 = 1` and the parameterized renderers
// bound the field name as a value.
func TestParseNumericFieldIsColumn(t *testing.T) {
	tcs := map[string]struct {
		input        string
		defaultField string
		want         *expr.Expression
	}{
		"int_field":      {input: `0:1`, want: expr.Eq("0", 1)},
		"leading_zeros":  {input: `007:x`, want: expr.Eq("007", "x")},
		"float_field":    {input: `1.50:x`, want: expr.Eq("1.50", "x")},
		"exponent_field": {input: `1e3:x`, want: expr.Eq("1e3", "x")},
		"range":          {input: `0:[1 TO 2]`, want: expr.Rang("0", 1, 2, true)},
		"greater":        {input: `0:>1`, want: expr.GREATER("0", 1)},
		"less_eq":        {input: `0:<=1`, want: expr.LESSEQ("0", 1)},
		"in_group":       {input: `0:(1 OR 2)`, want: expr.IN("0", expr.LIST(expr.Lit(1), expr.Lit(2)))},
		// = works like :, and a field in parentheses is still a field. Both
		// used to leave the number a number, so 1=1 rendered 1 = 1.
		"equals_sign":         {input: `1=1`, want: expr.Eq("1", 1)},
		"equals_spaced":       {input: `1 = 1`, want: expr.Eq("1", 1)},
		"equals_leading_zero": {input: `007=x`, want: expr.Eq("007", "x")},
		"equals_float":        {input: `1.50=x`, want: expr.Eq("1.50", "x")},
		"equals_negative":     {input: `-1=x`, want: expr.Eq("-1", "x")},
		"equals_group":        {input: `1=(x OR y)`, want: expr.IN("1", expr.LIST(expr.Lit("x"), expr.Lit("y")))},
		"equals_range":        {input: `1=[1 TO 2]`, want: expr.Rang("1", 1, 2, true)},
		"paren_wildcard":      {input: `(0):*`, want: expr.LIKE("0", expr.WILD("*"))},
		"paren_value":         {input: `(0):1`, want: expr.Eq("0", 1)},
		"paren_leading_zero":  {input: `(007):x`, want: expr.Eq("007", "x")},
		"paren_exponent":      {input: `(1e3):x`, want: expr.Eq("1e3", "x")},
		"double_paren":        {input: `((0)):1`, want: expr.Eq("0", 1)},
		"paren_range":         {input: `(0):[1 TO 2]`, want: expr.Rang("0", 1, 2, true)},
		"paren_greater":       {input: `(0):>1`, want: expr.GREATER("0", 1)},
		"paren_equals":        {input: `(1)=1`, want: expr.Eq("1", 1)},
		"in_and": {
			input: `a:x AND 1=1`,
			want:  expr.AND(expr.Eq("a", "x"), expr.Eq("1", 1)),
		},
		"wildcard_value": {input: `0:1*`, want: expr.LIKE("0", expr.WILD("1*"))},
		"null_value":     {input: `0:null`, want: expr.Eq("0", expr.NULL())},
		"negated":        {input: `NOT 0:1`, want: expr.NOT(expr.Eq("0", 1))},
		// Numbers that are not used as a field stay numbers.
		"value_stays_number": {input: `a:0`, want: expr.Eq("a", 0)},
		"equals_value":       {input: `a=0`, want: expr.Eq("a", 0)},
		"paren_number_value": {input: `a:(0)`, want: expr.Eq("a", 0)},
		"compare_value":      {input: `a:>=0`, want: expr.GREATEREQ("a", 0)},
		"bare_number":        {input: `1`, want: expr.Lit(1)},
		"paren_number":       {input: `(1)`, want: expr.Lit(1)},
		"bare_number_and_field": {
			input: `1 AND 0:1`,
			want:  expr.AND(expr.Lit(1), expr.Eq("0", 1)),
		},
		"default_field": {
			input:        `0:1 2`,
			defaultField: "df",
			want:         expr.AND(expr.Eq("0", 1), expr.Eq("df", 2)),
		},
		"default_field_bare_number": {
			input:        `2`,
			defaultField: "df",
			want:         expr.Eq("df", 2),
		},
	}

	for name, tc := range tcs {
		t.Run(name, func(t *testing.T) {
			got, err := Parse(tc.input, WithDefaultField(tc.defaultField))
			if err != nil {
				t.Fatalf("unexpected parse error: %v", err)
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf(errTemplate, "parsed expression does not match", tc.want, got)
			}
		})
	}
}

// TestParseWildcardFieldIsColumn checks that a field name with a * or ? in it
// is a column named by its text with the escapes removed, as a field with
// only escaped wildcards is. It used to keep the escapes, so fo\*o*:x was the
// column fo\*o* and 1e+5*:x was 1e\+5*.
func TestParseWildcardFieldIsColumn(t *testing.T) {
	tcs := map[string]struct {
		input string
		want  *expr.Expression
	}{
		"plain":            {input: `fo*:x`, want: expr.Eq("fo*", "x")},
		"escaped_star":     {input: `fo\*o*:x`, want: expr.Eq("fo*o*", "x")},
		"plus":             {input: `1e+5*:x`, want: expr.Eq("1e+5*", "x")},
		"escaped_slash":    {input: `a\\b*:x`, want: expr.Eq(`a\b*`, "x")},
		"question":         {input: `fo?:x`, want: expr.Eq("fo?", "x")},
		"equals_sign":      {input: `a+b*=x`, want: expr.Eq("a+b*", "x")},
		"paren":            {input: `(a+b*):x`, want: expr.Eq("a+b*", "x")},
		"range":            {input: `a+b*:[1 TO 2]`, want: expr.Rang("a+b*", 1, 2, true)},
		"wildcard_value":   {input: `a+b*:y*`, want: expr.LIKE("a+b*", expr.WILD("y*"))},
		"value_stays_wild": {input: `a:1e+5*`, want: expr.LIKE("a", expr.WILD(`1e\+5*`))},
	}

	for name, tc := range tcs {
		t.Run(name, func(t *testing.T) {
			got, err := Parse(tc.input)
			if err != nil {
				t.Fatalf("unexpected parse error: %v", err)
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf(errTemplate, "parsed expression does not match", tc.want, got)
			}
		})
	}
}

// TestParseNonFiniteNumberIsTerm checks that words like inf and NaN parse as
// plain terms. strconv.ParseFloat accepts them, and the resulting +Inf/NaN
// float used to render as a bare `+Inf` or `NaN`, which SQL reads as a column
// reference rather than a value.
func TestParseNonFiniteNumberIsTerm(t *testing.T) {
	tcs := map[string]struct {
		input string
		want  *expr.Expression
	}{
		"inf":          {`a:inf`, expr.Eq("a", "inf")},
		"infinity":     {`a:Infinity`, expr.Eq("a", "Infinity")},
		"nan":          {`a:NaN`, expr.Eq("a", "NaN")},
		"inf_field":    {`inf:1`, expr.Eq("inf", 1)},
		"inf_in_range": {`a:[1 TO inf]`, expr.Rang("a", 1, "inf", true)},
		"inf_in_list":  {`a:(1 OR nan)`, expr.IN("a", expr.LIST(expr.Lit(1), expr.Lit("nan")))},
		"finite_float": {`a:1.5`, expr.Eq("a", 1.5)},
	}

	for name, tc := range tcs {
		t.Run(name, func(t *testing.T) {
			got, err := Parse(tc.input)
			if err != nil {
				t.Fatalf("unexpected parse error: %v", err)
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf(errTemplate, "parsed expression does not match", tc.want, got)
			}
		})
	}
}

// TestParseNonDecimalNumberIsTerm checks that a term is a number only when it
// is a plain decimal number (issue #69). strconv.ParseFloat also accepts hex
// floats and underscore digit separators, so 0x1p3 used to become 8 and 1_000
// became 1000. They now stay terms with their text unchanged, in every spot
// that turns a term into a number.
func TestParseNonDecimalNumberIsTerm(t *testing.T) {
	tcs := map[string]struct {
		input string
		want  *expr.Expression
	}{
		"hex_float":          {`a:0x1p3`, expr.Eq("a", "0x1p3")},
		"hex_float_upper":    {`a:0X1P-2`, expr.Eq("a", "0X1P-2")},
		"negative_hex_float": {`a:-0x1p3`, expr.Eq("a", "-0x1p3")},
		"digit_separator":    {`a:1_000`, expr.Eq("a", "1_000")},
		"negative_separator": {`a:-1_0`, expr.Eq("a", "-1_0")},
		"separator_in_float": {`a:1_0.5`, expr.Eq("a", "1_0.5")},
		"separator_in_range": {`a:[1 TO 1_000]`, expr.Rang("a", 1, "1_000", true)},
		"hex_float_in_range": {`a:{0x1p3 TO 9}`, expr.Rang("a", "0x1p3", 9, false)},
		"comparison":         {`a:>0x1p3`, expr.GREATER("a", "0x1p3")},
		"in_list":            {`a:(1_000 OR 0x1p3)`, expr.IN("a", expr.LIST(expr.Lit("1_000"), expr.Lit("0x1p3")))},
		"boost_hex_float":    {`a:b^0x1p3`, expr.AND(expr.BOOST(expr.Eq("a", "b"), 1.0), "0x1p3")},
		"boost_separator":    {`a:b^1_0`, expr.AND(expr.BOOST(expr.Eq("a", "b"), 1.0), "1_0")},
		"boost_inf":          {`a:b^inf`, expr.AND(expr.BOOST(expr.Eq("a", "b"), 1.0), "inf")},
		"boost_quoted":       {`a:b^"5"`, expr.AND(expr.BOOST(expr.Eq("a", "b"), 1.0), "5")},
		"fuzzy_separator":    {`a:b~1_0`, expr.AND(expr.FUZZY(expr.Eq("a", "b"), 1), "1_0")},
		"fuzzy_quoted":       {`a:b~"2"`, expr.AND(expr.FUZZY(expr.Eq("a", "b"), 1), "2")},
		"fuzzy_escaped":      {`a:b~\2`, expr.AND(expr.FUZZY(expr.Eq("a", "b"), 1), "2")},
	}

	for name, tc := range tcs {
		t.Run(name, func(t *testing.T) {
			got, err := Parse(tc.input)
			if err != nil {
				t.Fatalf("unexpected parse error: %v", err)
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf(errTemplate, "parsed expression does not match", tc.want, got)
			}
		})
	}

	defaultFieldTcs := map[string]struct {
		input string
		want  *expr.Expression
	}{
		"single_term": {`0x1p3`, expr.Eq("f", "0x1p3")},
		"in_and":      {`x AND 1_000`, expr.AND(expr.Eq("f", "x"), expr.Eq("f", "1_000"))},
	}

	for name, tc := range defaultFieldTcs {
		t.Run("default_field_"+name, func(t *testing.T) {
			got, err := Parse(tc.input, WithDefaultField("f"))
			if err != nil {
				t.Fatalf("unexpected parse error: %v", err)
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf(errTemplate, "parsed expression does not match", tc.want, got)
			}
		})
	}
}

// TestParseDecimalNumberForms pins which terms parse as numbers. Integers that
// fit in an int are ints, other decimal numbers are float64s, and escaping or
// quoting a number keeps its text. Spellings that print differently from how
// they were written, like 007 or 1e3, are pinned in TestDecimalNumberFormsRender.
func TestParseDecimalNumberForms(t *testing.T) {
	tcs := map[string]struct {
		input string
		want  *expr.Expression
	}{
		"int":                 {`a:5`, expr.Eq("a", 5)},
		"negative_int":        {`a:-5`, expr.Eq("a", -5)},
		"float":               {`a:1.5`, expr.Eq("a", 1.5)},
		"max_int":             {`a:9223372036854775807`, expr.Eq("a", 9223372036854775807)},
		"float_overflow":      {`a:1e400`, expr.Eq("a", "1e400")},
		"float_underflow":     {`a:1e-400`, expr.Eq("a", "1e-400")},
		"negative_underflow":  {`a:-1.5e-400`, expr.Eq("a", "-1.5e-400")},
		"underflow_bound":     {`a:[1e-400 TO 1]`, expr.Rang("a", "1e-400", 1, true)},
		"zero_tiny_exponent":  {`a:0.0e-400`, expr.Eq("a", expr.NumberLit(0.0, "0.0e-400"))},
		"smallest_float":      {`a:5e-324`, expr.Eq("a", expr.NumberLit(5e-324, "5e-324"))},
		"hex_int":             {`a:0x10`, expr.Eq("a", "0x10")},
		"escaped_digits":      {`a:\007`, expr.Eq("a", "007")},
		"quoted_digits":       {`a:"007"`, expr.Eq("a", "007")},
		"escaped_plus":        {`a:\+5`, expr.Eq("a", "+5")},
		"non_ascii_digits":    {`a:١٢٣`, expr.Eq("a", "١٢٣")},
		"leading_zeros_field": {`007:5`, expr.Eq("007", 5)},
		"hex_float_field":     {`0x1p3:5`, expr.Eq("0x1p3", 5)},
		"boost_exponent":      {`a:b^1e1`, expr.BOOST(expr.Eq("a", "b"), 10.0)},
		"boost_fraction":      {`a:b^0.5`, expr.BOOST(expr.Eq("a", "b"), 0.5)},
		"fuzzy_leading_zero":  {`a:b~02`, expr.FUZZY(expr.Eq("a", "b"), 2)},
		"fuzzy_whole_float":   {`a:b~2.0`, expr.FUZZY(expr.Eq("a", "b"), 2)},
	}

	for name, tc := range tcs {
		t.Run(name, func(t *testing.T) {
			got, err := Parse(tc.input)
			if err != nil {
				t.Fatalf("unexpected parse error: %v", err)
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf(errTemplate, "parsed expression does not match", tc.want, got)
			}
		})
	}
}

// TestGroupingRoundTrip verifies that String() writes the parentheses that
// grouping needs, and only those, so Parse -> String() -> Parse gives the same
// expression and flat chains print without extra nesting (issue #64).
func TestGroupingRoundTrip(t *testing.T) {
	tcs := map[string]struct {
		input string
		want  string
	}{
		"or_inside_and":        {`a:1 AND (b:2 OR c:3)`, `a:1 AND (b:2 OR c:3)`},
		"or_inside_and_left":   {`(a:1 OR b:2) AND c:3`, `(a:1 OR b:2) AND c:3`},
		"and_inside_or":        {`a:1 OR (b:2 AND c:3)`, `a:1 OR b:2 AND c:3`},
		"and_inside_and_right": {`a:1 AND (b:2 AND c:3)`, `a:1 AND (b:2 AND c:3)`},
		"implicit_precedence":  {`a:1 AND b:2 OR c:3`, `a:1 AND b:2 OR c:3`},
		"or_inside_or_right":   {`a:1 OR (b:2 OR c:3)`, `a:1 OR (b:2 OR c:3)`},
		"flat_and_chain":       {`a:1 AND b:2 AND c:3`, `a:1 AND b:2 AND c:3`},
		"flat_or_chain":        {`a:1 OR b:2 OR c:3`, `a:1 OR b:2 OR c:3`},
		"field_group_and":      {`a:(b AND c)`, `a:b AND a:c`},
		"field_group_must_not": {`a:(-b)`, `-a:b`},
		"field_group_boost":    {`a:(b^2)`, `a:b^2.0`},
		"boost_of_must_not":    {`-a:1^2`, `-a:1^2.0`},
		"must_not_of_boost":    {`-(a:1^2)`, `-(a:1^2.0)`},
		"must_not_group":       {`-(a:1 OR b:2)`, `-(a:1 OR b:2)`},
		"must_group":           {`+(a:1 OR b:2)`, `+(a:1 OR b:2)`},
		"boost_group":          {`(a:1 OR b:2)^2`, `(a:1 OR b:2)^2.0`},
		"boost_group_default":  {`(a:1 OR b:2)^`, `(a:1 OR b:2)^`},
		"fuzzy_group":          {`(a:1 OR b:2)~2`, `(a:1 OR b:2)~2`},
		"not_group":            {`NOT (a:1 OR b:2)`, `NOT(a:1 OR b:2)`},
		"nested":               {`a:1 AND (b:2 OR (c:3 AND -(d:4 OR e:5)))`, `a:1 AND (b:2 OR c:3 AND -(d:4 OR e:5))`},
		"flat_and":             {`a:1 AND b:2`, `a:1 AND b:2`},
		"must_not_must_not":    {`-(-a:1)`, `-(-a:1)`},
		"must_must_not":        {`+(-a:1)`, `+(-a:1)`},
		"must_must":            {`+(+a:1)`, `+(+a:1)`},
		"must_not_not":         {`-(NOT a:1)`, `-(NOT(a:1))`},
		"fuzzy_of_boost":       {`(a:1^2)~2`, `(a:1^2.0)~2`},
		"and_of_must_not":      {`a:1 AND -b:2`, `a:1 AND -b:2`},
		"must_term":            {`-a:1`, `-a:1`},
	}

	for name, tc := range tcs {
		t.Run(name, func(t *testing.T) {
			first, err := Parse(tc.input)
			if err != nil {
				t.Fatalf("expected no error parsing %q, got: %v", tc.input, err)
			}

			rendered := first.String()
			if rendered != tc.want {
				t.Errorf("String():\n got: %s\nwant: %s", rendered, tc.want)
			}

			second, err := Parse(rendered)
			if err != nil {
				t.Fatalf("expected no error re-parsing rendered form %q, got: %v", rendered, err)
			}

			if !reflect.DeepEqual(first, second) {
				t.Fatalf(errTemplate, "round trip through String() changed the expression", first, second)
			}
		})
	}
}

// TestParseEscapedWildcardChars checks that an escaped * or ? is a literal
// character, as in Lucene. A term whose only * and ? are escaped is a plain
// term. A wildcard keeps its text as written, escapes included, so the
// drivers can tell a literal * from a wildcard.
func TestParseEscapedWildcardChars(t *testing.T) {
	tcs := map[string]*expr.Expression{
		`a:fo\*`:          expr.Eq("a", expr.Lit("fo*")),
		`a:fo\?`:          expr.Eq("a", expr.Lit("fo?")),
		`a:\*`:            expr.Eq("a", expr.Lit("*")),
		`a:\*x\?`:         expr.Eq("a", expr.Lit("*x?")),
		`a:\*x*`:          expr.LIKE("a", expr.WILD(`\*x*`)),
		`a:fo\*bar*`:      expr.LIKE("a", expr.WILD(`fo\*bar*`)),
		`a:fo\\*`:         expr.LIKE("a", expr.WILD(`fo\\*`)),
		`a:fo\ b?r`:       expr.LIKE("a", expr.WILD(`fo\ b?r`)),
		`a:(fo\* OR b)`:   expr.IN("a", expr.LIST(expr.Lit("fo*"), expr.Lit("b"))),
		`fo\*:x`:          expr.Eq("fo*", "x"),
		`a:[fo\* TO z]`:   expr.Rang("a", expr.Lit("fo*"), "z", true),
		`a:fo\* AND b:c*`: expr.AND(expr.Eq("a", expr.Lit("fo*")), expr.LIKE("b", expr.WILD("c*"))),
		// An escaped backslash is one literal backslash. Both used to be
		// dropped, so foo\\bar read as foobar.
		`a:foo\\bar`: expr.Eq("a", expr.Lit(`foo\bar`)),
		`a:\\`:       expr.Eq("a", expr.Lit(`\`)),
		`a:\\\*`:     expr.Eq("a", expr.Lit(`\*`)),
		// The lexer keeps the + of an exponent in a word. In a wildcard it
		// is a literal +, so it is escaped: SIMILAR TO reads a bare + as
		// "one or more".
		`a:1e+5*`: expr.LIKE("a", expr.WILD(`1e\+5*`)),
	}
	for in, want := range tcs {
		t.Run(in, func(t *testing.T) {
			got, err := Parse(in)
			if err != nil {
				t.Fatalf("unexpected parse error: %v", err)
			}
			if !reflect.DeepEqual(got, want) {
				t.Fatalf(errTemplate, "parsed expression does not match", want, got)
			}
		})
	}
}
