package lucene

import (
	"strings"
	"testing"

	"github.com/grindlemire/go-lucene/pkg/driver"
)

func TestPostgresSQLEndToEnd(t *testing.T) {
	type tc struct {
		input string
		want  string
		err   string
	}

	tcs := map[string]tc{
		// "single_literal": {
		// 	input: "a",
		// 	want:  "a",
		// },
		"basic_equal": {
			input: "a:b",
			want:  "a = 'b'",
		},
		"basic_equal_with_number": {
			input: "a:5",
			want:  "a = 5",
		},
		"basic_greater_with_number": {
			input: "a:>22",
			want:  "a > 22",
		},
		"basic_greater_eq_with_number": {
			input: "a:>=22",
			want:  "a >= 22",
		},
		"basic_less_with_number": {
			input: "a:<22",
			want:  "a < 22",
		},
		"basic_less_eq_with_number": {
			input: "a:<=22",
			want:  "a <= 22",
		},
		"basic_greater_less_with_number": {
			input: "a:<22 AND b:>33",
			want:  "(a < 22) AND (b > 33)",
		},
		"basic_greater_less_eq_with_number": {
			input: "a:<=22 AND b:>=33",
			want:  "(a <= 22) AND (b >= 33)",
		},
		"basic_wild_equal_with_*": {
			input: "a:b*",
			want:  "a SIMILAR TO 'b%'",
		},
		"basic_wild_equal_with_?": {
			input: "a:b?z",
			want:  "a SIMILAR TO 'b_z'",
		},
		"basic_inclusive_range": {
			input: "a:[* TO 5]",
			want:  "a <= 5",
		},
		"basic_exclusive_range": {
			input: "a:{* TO 5}",
			want:  "a < 5",
		},
		"range_over_strings": {
			input: "a:{foo TO bar}",
			want:  "a BETWEEN 'foo' AND 'bar'",
		},
		"basic_fuzzy": {
			input: "b AND a~",
			err:   "unable to render operator [FUZZY]",
		},
		"fuzzy_power": {
			input: "b AND a~10",
			err:   "unable to render operator [FUZZY]",
		},
		"basic_boost": {
			input: "b AND a^",
			err:   "unable to render operator [BOOST]",
		},
		"boost_power": {
			input: "b AND a^10",
			err:   "unable to render operator [BOOST]",
		},
		"regexp": {
			input: "a:/b [c]/",
			want:  "a ~ '/b [c]/'",
		},
		"regexp_with_keywords": {
			input: `a:/b "[c]/`,
			want:  `a ~ '/b "[c]/'`,
		},
		"basic_default_AND": {
			input: "a b",
			want:  "('a') AND ('b')",
		},
		"default_to_AND_with_subexpressions": {
			input: "a:b c:d",
			want:  "(a = 'b') AND (c = 'd')",
		},
		"basic_and": {
			input: "a AND b",
			want:  "('a') AND ('b')",
		},
		"and_with_nesting": {
			input: "a:foo AND b:bar",
			want:  "(a = 'foo') AND (b = 'bar')",
		},
		"basic_or": {
			input: "a OR b",
			want:  "('a') OR ('b')",
		},
		"or_with_nesting": {
			input: "a:foo OR b:bar",
			want:  "(a = 'foo') OR (b = 'bar')",
		},
		"range_operator_inclusive": {
			input: "a:[1 TO 5]",
			want:  "a >= 1 AND a <= 5",
		},
		"range_operator_inclusive_unbound": {
			input: `a:[* TO 200]`,
			want:  "a <= 200",
		},
		"range_operator_exclusive": {
			input: `a:{"ab" TO "az"}`,
			want:  "a BETWEEN 'ab' AND 'az'",
		},
		"range_operator_exclusive_unbound": {
			input: `a:{2 TO *}`,
			want:  "a > 2",
		},
		"basic_not": {
			input: "NOT b",
			want:  "NOT('b')",
		},
		"nested_not": {
			input: "a:foo OR NOT b:bar",
			want:  "(a = 'foo') OR (NOT(b = 'bar'))",
		},
		"term_grouping": {
			input: "(a:foo OR b:bar) AND c:baz",
			want:  "((a = 'foo') OR (b = 'bar')) AND (c = 'baz')",
		},
		"value_grouping": {
			input: "a:(foo OR baz OR bar)",
			want:  "a IN ('foo', 'baz', 'bar')",
		},
		"basic_must": {
			input: "+a:b",
			want:  "a = 'b'",
		},
		"basic_must_not": {
			input: "-a:b",
			want:  "NOT(a = 'b')",
		},
		"basic_nested_must_not": {
			input: "d:e AND (-a:b AND +f:e)",
			want:  "(d = 'e') AND ((NOT(a = 'b')) AND (f = 'e'))",
		},
		"basic_escaping": {
			input: `a:\(1\+1\)\:2`,
			want:  `a = '(1+1):2'`,
		},
		"escaped_column_name": {
			input: `foo\ bar:b`,
			want:  `"foo bar" = 'b'`,
		},
		"boost_key_value": {
			input: "a:b^2 AND foo",
			err:   "unable to render operator [BOOST]",
		},
		"nested_sub_expressions": {
			input: "((title:foo OR title:bar) AND (body:foo OR body:bar)) OR k:v",
			want:  "(((title = 'foo') OR (title = 'bar')) AND ((body = 'foo') OR (body = 'bar'))) OR (k = 'v')",
		},
		"fuzzy_key_value": {
			input: "a:b~2 AND foo",
			err:   "unable to render operator [FUZZY]",
		},
		"precedence_works": {
			input: "a:b AND c:d OR e:f OR h:i AND j:k",
			want:  "(((a = 'b') AND (c = 'd')) OR (e = 'f')) OR ((h = 'i') AND (j = 'k'))",
		},
		"test_precedence_weaving": {
			input: "a OR b AND c OR d",
			want:  "(('a') OR (('b') AND ('c'))) OR ('d')",
		},
		"test_precedence_weaving_with_not": {
			input: "NOT a OR b AND NOT c OR d",
			want:  "((NOT('a')) OR (('b') AND (NOT('c')))) OR ('d')",
		},
		"test_equals_in_precedence": {
			input: "a:az OR b:bz AND NOT c:z OR d",
			want:  "((a = 'az') OR ((b = 'bz') AND (NOT(c = 'z')))) OR ('d')",
		},
		"test_parens_in_precedence": {
			input: "a AND (c OR d)",
			want:  "('a') AND (('c') OR ('d'))",
		},
		"test_range_precedance_simple": {
			input: "c:[* to -1] OR d",
			want:  "(c <= -1) OR ('d')",
		},
		"test_range_precedance": {
			input: "a OR b AND c:[* to -1] OR d",
			want:  "(('a') OR (('b') AND (c <= -1))) OR ('d')",
		},
		"test_full_precedance": {
			input: "a OR b AND c:[* to -1] OR d AND NOT +e:f",
			want:  "(('a') OR (('b') AND (c <= -1))) OR (('d') AND (NOT(e = 'f')))",
		},
		"test_elastic_greater_than_precedance": {
			input: "a:>10 AND -b:<=-20",
			want:  "(a > 10) AND (NOT(b <= -20))",
		},
	}

	for name, tc := range tcs {
		t.Run(name, func(t *testing.T) {

			expr, err := Parse(tc.input)
			if err != nil {
				t.Fatal(err)
			}

			got, err := driver.NewPostgresDriver().Render(expr)
			if err != nil {
				// if we got an expect error then we are fine
				if tc.err != "" && strings.Contains(err.Error(), tc.err) {
					return
				}
				t.Fatalf("unexpected error rendering expression: %v", err)
			}

			if tc.err != "" {
				t.Fatalf("\nexpected error [%s]\ngot: %s", tc.err, got)
			}

			if got != tc.want {
				t.Fatalf("\nwant %s\ngot  %s\nparsed expression: %#v\n", tc.want, got, expr)
			}
		})
	}
}
