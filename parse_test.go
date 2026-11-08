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
		"wildcard_value": {input: `0:1*`, want: expr.LIKE("0", expr.WILD("1*"))},
		"null_value":     {input: `0:null`, want: expr.Eq("0", expr.NULL())},
		"negated":        {input: `NOT 0:1`, want: expr.NOT(expr.Eq("0", 1))},
		// Numbers that are not followed by a colon stay numbers.
		"value_stays_number": {input: `a:0`, want: expr.Eq("a", 0)},
		"bare_number":        {input: `1`, want: expr.Lit(1)},
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
		"field_group_and":      {`a:(b AND c)`, `a:(b AND c)`},
		"field_group_must_not": {`a:(-b)`, `a:(-b)`},
		"field_group_boost":    {`a:(b^2)`, `a:(b^2.0)`},
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
