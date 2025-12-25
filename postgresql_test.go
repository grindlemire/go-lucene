package lucene

import (
	"strings"
	"testing"
)

func TestPostgresSQLEndToEnd(t *testing.T) {
	type tc struct {
		input        string
		want         string
		defaultField string
		err          string
	}

	tcs := map[string]tc{
		// "single_literal": {
		// 	input: "a",
		// 	want:  `a`,
		// },
		"basic_equal": {
			input: "a:b",
			want:  `"a" = 'b'`,
		},
		"basic_equal_with_number": {
			input: "a:5",
			want:  `"a" = 5`,
		},
		"basic_greater_with_number": {
			input: "a:>22",
			want:  `"a" > 22`,
		},
		"basic_greater_eq_with_number": {
			input: "a:>=22",
			want:  `"a" >= 22`,
		},
		"basic_less_with_number": {
			input: "a:<22",
			want:  `"a" < 22`,
		},
		"basic_less_eq_with_number": {
			input: "a:<=22",
			want:  `"a" <= 22`,
		},
		"basic_greater_less_with_number": {
			input: "a:<22 AND b:>33",
			want:  `("a" < 22) AND ("b" > 33)`,
		},
		"basic_greater_less_eq_with_number": {
			input: "a:<=22 AND b:>=33",
			want:  `("a" <= 22) AND ("b" >= 33)`,
		},
		"basic_wild_equal_with_*": {
			input: "a:b*",
			want:  `"a" SIMILAR TO 'b%'`,
		},
		"basic_wild_equal_with_?": {
			input: "a:b?z",
			want:  `"a" SIMILAR TO 'b_z'`,
		},
		"basic_inclusive_range": {
			input: "a:[* TO 5]",
			want:  `"a" <= 5`,
		},
		"basic_exclusive_range": {
			input: "a:{* TO 5}",
			want:  `"a" < 5`,
		},
		"range_over_strings": {
			input: "a:{foo TO bar}",
			want:  `"a" BETWEEN 'foo' AND 'bar'`,
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
			want:  `"a" ~ '/b [c]/'`,
		},
		"regexp_with_keywords": {
			input: `a:/b "[c]/`,
			want:  `"a" ~ '/b "[c]/'`,
		},
		"regexp_with_escaped_chars": {
			input: `url:/example.com\/foo\/bar\/.*/`,
			want:  `"url" ~ '/example.com\/foo\/bar\/.*/'`,
		},
		"basic_default_AND": {
			input: "a b",
			want:  `'a' AND 'b'`,
		},
		"default_to_AND_with_subexpressions": {
			input: "a:b c:d",
			want:  `("a" = 'b') AND ("c" = 'd')`,
		},
		"basic_and": {
			input: "a AND b",
			want:  `'a' AND 'b'`,
		},
		"and_with_nesting": {
			input: "a:foo AND b:bar",
			want:  `("a" = 'foo') AND ("b" = 'bar')`,
		},
		"basic_or": {
			input: "a OR b",
			want:  `'a' OR 'b'`,
		},
		"or_with_nesting": {
			input: "a:foo OR b:bar",
			want:  `("a" = 'foo') OR ("b" = 'bar')`,
		},
		"range_operator_inclusive": {
			input: "a:[1 TO 5]",
			want:  `"a" >= 1 AND "a" <= 5`,
		},
		"range_operator_inclusive_unbound": {
			input: `a:[* TO 200]`,
			want:  `"a" <= 200`,
		},
		"range_operator_exclusive": {
			input: `a:{"ab" TO "az"}`,
			want:  `"a" BETWEEN 'ab' AND 'az'`,
		},
		"range_operator_exclusive_unbound": {
			input: `a:{2 TO *}`,
			want:  `"a" > 2`,
		},
		"basic_not": {
			input: "NOT b",
			want:  `NOT('b')`,
		},
		"nested_not": {
			input: "a:foo OR NOT b:bar",
			want:  `("a" = 'foo') OR (NOT("b" = 'bar'))`,
		},
		"term_grouping": {
			input: "(a:foo OR b:bar) AND c:baz",
			want:  `(("a" = 'foo') OR ("b" = 'bar')) AND ("c" = 'baz')`,
		},
		"value_grouping": {
			input: "a:(foo OR baz OR bar)",
			want:  `"a" IN ('foo', 'baz', 'bar')`,
		},
		"basic_must": {
			input: "+a:b",
			want:  `"a" = 'b'`,
		},
		"basic_must_not": {
			input: "-a:b",
			want:  `NOT("a" = 'b')`,
		},
		"basic_nested_must_not": {
			input: "d:e AND (-a:b AND +f:e)",
			want:  `("d" = 'e') AND ((NOT("a" = 'b')) AND ("f" = 'e'))`,
		},
		"basic_escaping": {
			input: `a:\(1\+1\)\:2`,
			want:  `"a" = '(1+1):2'`,
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
			want:  `((("title" = 'foo') OR ("title" = 'bar')) AND (("body" = 'foo') OR ("body" = 'bar'))) OR ("k" = 'v')`,
		},
		"fuzzy_key_value": {
			input: "a:b~2 AND foo",
			err:   "unable to render operator [FUZZY]",
		},
		"precedence_works": {
			input: "a:b AND c:d OR e:f OR h:i AND j:k",
			want:  `((("a" = 'b') AND ("c" = 'd')) OR ("e" = 'f')) OR (("h" = 'i') AND ("j" = 'k'))`,
		},
		"test_precedence_weaving": {
			input: "a OR b AND c OR d",
			want:  `('a' OR ('b' AND 'c')) OR 'd'`,
		},
		"test_precedence_weaving_with_not": {
			input: "NOT a OR b AND NOT c OR d",
			want:  `((NOT('a')) OR ('b' AND (NOT('c')))) OR 'd'`,
		},
		"test_equals_in_precedence": {
			input: "a:az OR b:bz AND NOT c:z OR d",
			want:  `(("a" = 'az') OR (("b" = 'bz') AND (NOT("c" = 'z')))) OR 'd'`,
		},
		"test_parens_in_precedence": {
			input: "a AND (c OR d)",
			want:  `'a' AND ('c' OR 'd')`,
		},
		"test_range_precedence_simple": {
			input: "c:[* to -1] OR d",
			want:  `("c" <= -1) OR 'd'`,
		},
		"test_range_precedence": {
			input: "a OR b AND c:[* to -1] OR d",
			want:  `('a' OR ('b' AND ("c" <= -1))) OR 'd'`,
		},
		"test_full_precedence": {
			input: "a OR b AND c:[* to -1] OR d AND NOT +e:f",
			want:  `('a' OR ('b' AND ("c" <= -1))) OR ('d' AND (NOT("e" = 'f')))`,
		},
		"test_elastic_greater_than_precedence": {
			input: "a:>10 AND -b:<=-20",
			want:  `("a" > 10) AND (NOT("b" <= -20))`,
		},
		"escape_quotes": {
			input: "a:'b'",
			want:  `"a" = '''b'''`,
		},
		"name_starts_with_number": {
			input: "1a:b",
			want:  `"1a" = 'b'`,
		},
		"default_field_and": {
			input:        `title:"The Right Way" AND go`,
			want:         `("title" = 'The Right Way') AND ("default" = 'go')`,
			defaultField: "default",
		},
		"default_field_or": {
			input:        `title:"The Right Way" OR go`,
			want:         `("title" = 'The Right Way') OR ("default" = 'go')`,
			defaultField: "default",
		},
		"default_field_not": {
			input:        `title:"The Right Way" AND NOT(go)`,
			want:         `("title" = 'The Right Way') AND (NOT("default" = 'go'))`,
			defaultField: "default",
		},
		"asterisk_in_literal_are_regular_expression": {
			input: `foo:*`,
			want:  `"foo" SIMILAR TO '%'`,
		},
		"implicit_and_with_subexpressions": {
			input: "a:b c:d",
			want:  `("a" = 'b') AND ("c" = 'd')`,
		},
		"implicit_and_with_negated_subexpressions": {
			input: "-a:b -c:d",
			want:  `(NOT("a" = 'b')) AND (NOT("c" = 'd'))`,
		},
		"implicit_and_with_explicit_negation": {
			input: "a:b NOT c:d",
			want:  `("a" = 'b') AND (NOT("c" = 'd'))`,
		},
		"implicit_and_with_subexpressions_and_default_field": {
			input:        `title:"Foo" a b`,
			want:         `(("title" = 'Foo') AND ("default" = 'a')) AND ("default" = 'b')`,
			defaultField: "default",
		},
		"implicit_and_with_negated_subexpressions_and_default_field": {
			input:        `title:"Foo" -a:c b`,
			want:         `(("title" = 'Foo') AND (NOT("a" = 'c'))) AND ("default" = 'b')`,
			defaultField: "default",
		},
		"implicit_and_with_negated_subexpressions_and_default_field_reversed": {
			input:        `title:"Foo" a:c -b`,
			want:         `(("title" = 'Foo') AND ("a" = 'c')) AND (NOT("default" = 'b'))`,
			defaultField: "default",
		},
		"implicit_and_with_explicit_subexpression_and_default_field": {
			input:        `title:"Foo" a:b NOT c`,
			want:         `(("title" = 'Foo') AND ("a" = 'b')) AND (NOT("default" = 'c'))`,
			defaultField: "default",
		},
		"implicit_and_with_explicit_subexpression_and_keyword_field": {
			input: `title:"Foo" a:b NOT k:c`,
			want:  `(("title" = 'Foo') AND ("a" = 'b')) AND (NOT("k" = 'c'))`,
		},
		"implicit_and_with_quotes": {
			input:        `"jakarta apache" -"Apache Lucene"`,
			want:         `("default" = 'jakarta apache') AND (NOT("default" = 'Apache Lucene'))`,
			defaultField: "default",
		},
		"implicit_and_with_exclamation_mark_as_alternative_to_not": {
			input:        `"jakarta apache" !"Apache Lucene"`,
			want:         `("default" = 'jakarta apache') AND (NOT("default" = 'Apache Lucene'))`,
			defaultField: "default",
		},
		"implicit_and_with_exclamation_mark_as_alternative_to_not_and_default_field": {
			input:        `"jakarta apache" !"Apache Lucene"`,
			want:         `("default" = 'jakarta apache') AND (NOT("default" = 'Apache Lucene'))`,
			defaultField: "default",
		},
		"exclamation_mark_inside_quotes_is_literal": {
			input:        `"text with ! inside"`,
			want:         `"default" = 'text with ! inside'`,
			defaultField: "default",
		},
		"exclamation_mark_inside_regexp_is_literal": {
			input: "field:/pattern with ! inside/",
			want:  `"field" ~ '/pattern with ! inside/'`,
		},
	}

	for name, tc := range tcs {
		t.Run(name, func(t *testing.T) {
			got, err := ToPostgres(tc.input, WithDefaultField(tc.defaultField))
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
				expr, err := Parse(tc.input)
				if err != nil {
					t.Fatalf("unable to parse expression: %v", err)
				}
				t.Fatalf("\nwant %s\ngot  %s\nparsed expression: %#v\n", tc.want, got, expr)
			}
		})
	}
}

func TestPostgresParameterizedSQLEndToEnd(t *testing.T) {
	type tc struct {
		input        string
		wantStr      string
		wantParams   []any
		defaultField string
		err          string
	}

	tcs := map[string]tc{
		// "single_literal": {
		// 	input: "a",
		// 	want:  `a`,
		// },
		"basic_equal": {
			input:      "a:b",
			wantStr:    `"a" = $1`,
			wantParams: []any{"b"},
		},
		"basic_equal_with_number": {
			input:      "a:5",
			wantStr:    `"a" = $1`,
			wantParams: []any{5},
		},
		"basic_greater_with_number": {
			input:      "a:>22",
			wantStr:    `"a" > $1`,
			wantParams: []any{22},
		},
		"basic_greater_eq_with_number": {
			input:      "a:>=22",
			wantStr:    `"a" >= $1`,
			wantParams: []any{22},
		},
		"basic_less_with_number": {
			input:      "a:<22",
			wantStr:    `"a" < $1`,
			wantParams: []any{22},
		},
		"basic_less_eq_with_number": {
			input:      "a:<=22",
			wantStr:    `"a" <= $1`,
			wantParams: []any{22},
		},
		"basic_greater_less_with_number": {
			input:      "a:<22 AND b:>33",
			wantStr:    `("a" < $1) AND ("b" > $2)`,
			wantParams: []any{22, 33},
		},
		"basic_greater_less_eq_with_number": {
			input:      "a:<=22 AND b:>=33",
			wantStr:    `("a" <= $1) AND ("b" >= $2)`,
			wantParams: []any{22, 33},
		},
		"basic_wild_equal_with_*": {
			input:      "a:b*",
			wantStr:    `"a" SIMILAR TO $1`,
			wantParams: []any{"b%"},
		},
		"basic_wild_equal_with_?": {
			input:      "a:b?z",
			wantStr:    `"a" SIMILAR TO $1`,
			wantParams: []any{"b_z"},
		},
		"basic_inclusive_range": {
			input:      "a:[* TO 5]",
			wantStr:    `"a" <= $1`,
			wantParams: []any{5},
		},
		"basic_exclusive_range": {
			input:      "a:{* TO 5}",
			wantStr:    `"a" < $1`,
			wantParams: []any{5},
		},
		"range_over_strings": {
			input:      "a:{foo TO bar}",
			wantStr:    `"a" BETWEEN $1 AND $2`,
			wantParams: []any{"foo", "bar"},
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
			input:      "a:/b [c]/",
			wantStr:    `"a" ~ $1`,
			wantParams: []any{"/b [c]/"},
		},
		"regexp_with_keywords": {
			input:      `a:/b "[c]/`,
			wantStr:    `"a" ~ $1`,
			wantParams: []any{`/b "[c]/`},
		},
		"regexp_with_escaped_chars": {
			input:      `url:/example.com\/foo\/bar\/.*/`,
			wantStr:    `"url" ~ $1`,
			wantParams: []any{`/example.com\/foo\/bar\/.*/`},
		},
		"basic_default_AND": {
			input:      "a b",
			wantStr:    `$1 AND $2`,
			wantParams: []any{"a", "b"},
		},
		"default_to_AND_with_subexpressions": {
			input:      "a:b c:d",
			wantStr:    `("a" = $1) AND ("c" = $2)`,
			wantParams: []any{"b", "d"},
		},
		"basic_and": {
			input:      "a AND b",
			wantStr:    `$1 AND $2`,
			wantParams: []any{"a", "b"},
		},
		"and_with_nesting": {
			input:      "a:foo AND b:bar",
			wantStr:    `("a" = $1) AND ("b" = $2)`,
			wantParams: []any{"foo", "bar"},
		},
		"basic_or": {
			input:      "a OR b",
			wantStr:    `$1 OR $2`,
			wantParams: []any{"a", "b"},
		},
		"or_with_nesting": {
			input:      "a:foo OR b:bar",
			wantStr:    `("a" = $1) OR ("b" = $2)`,
			wantParams: []any{"foo", "bar"},
		},
		"range_operator_inclusive": {
			input:      "a:[1 TO 5]",
			wantStr:    `"a" >= $1 AND "a" <= $2`,
			wantParams: []any{1, 5},
		},
		"range_operator_inclusive_unbound": {
			input:      `a:[* TO 200]`,
			wantStr:    `"a" <= $1`,
			wantParams: []any{200},
		},
		"range_operator_exclusive": {
			input:      `a:{"ab" TO "az"}`,
			wantStr:    `"a" BETWEEN $1 AND $2`,
			wantParams: []any{"ab", "az"},
		},
		"range_operator_exclusive_unbound": {
			input:      `a:{2 TO *}`,
			wantStr:    `"a" > $1`,
			wantParams: []any{2},
		},
		"basic_not": {
			input:      "NOT b",
			wantStr:    `NOT($1)`,
			wantParams: []any{"b"},
		},
		"nested_not": {
			input:      "a:foo OR NOT b:bar",
			wantStr:    `("a" = $1) OR (NOT("b" = $2))`,
			wantParams: []any{"foo", "bar"},
		},
		"term_grouping": {
			input:      "(a:foo OR b:bar) AND c:baz",
			wantStr:    `(("a" = $1) OR ("b" = $2)) AND ("c" = $3)`,
			wantParams: []any{"foo", "bar", "baz"},
		},
		"value_grouping": {
			input:      "a:(foo OR baz OR bar)",
			wantStr:    `"a" IN ($1, $2, $3)`,
			wantParams: []any{"foo", "baz", "bar"},
		},
		"basic_must": {
			input:      "+a:b",
			wantStr:    `"a" = $1`,
			wantParams: []any{"b"},
		},
		"basic_must_not": {
			input:      "-a:b",
			wantStr:    `NOT("a" = $1)`,
			wantParams: []any{"b"},
		},
		"basic_nested_must_not": {
			input:      "d:e AND (-a:b AND +f:e)",
			wantStr:    `("d" = $1) AND ((NOT("a" = $2)) AND ("f" = $3))`,
			wantParams: []any{"e", "b", "e"},
		},
		"basic_escaping": {
			input:      `a:\(1\+1\)\:2`,
			wantStr:    `"a" = $1`,
			wantParams: []any{"(1+1):2"},
		},
		"escaped_column_name": {
			input:      `foo\ bar:b`,
			wantStr:    `"foo bar" = $1`,
			wantParams: []any{"b"},
		},
		"boost_key_value": {
			input: "a:b^2 AND foo",
			err:   "unable to render operator [BOOST]",
		},
		"nested_sub_expressions": {
			input:      "((title:foo OR title:bar) AND (body:foo OR body:bar)) OR k:v",
			wantStr:    `((("title" = $1) OR ("title" = $2)) AND (("body" = $3) OR ("body" = $4))) OR ("k" = $5)`,
			wantParams: []any{"foo", "bar", "foo", "bar", "v"},
		},
		"fuzzy_key_value": {
			input: "a:b~2 AND foo",
			err:   "unable to render operator [FUZZY]",
		},
		"precedence_works": {
			input:      "a:b AND c:d OR e:f OR h:i AND j:k",
			wantStr:    `((("a" = $1) AND ("c" = $2)) OR ("e" = $3)) OR (("h" = $4) AND ("j" = $5))`,
			wantParams: []any{"b", "d", "f", "i", "k"},
		},
		"test_precedence_weaving": {
			input:      "a OR b AND c OR d",
			wantStr:    `($1 OR ($2 AND $3)) OR $4`,
			wantParams: []any{"a", "b", "c", "d"},
		},
		"test_precedence_weaving_with_not": {
			input:      "NOT a OR b AND NOT c OR d",
			wantStr:    `((NOT($1)) OR ($2 AND (NOT($3)))) OR $4`,
			wantParams: []any{"a", "b", "c", "d"},
		},
		"test_equals_in_precedence": {
			input:      "a:az OR b:bz AND NOT c:z OR d",
			wantStr:    `(("a" = $1) OR (("b" = $2) AND (NOT("c" = $3)))) OR $4`,
			wantParams: []any{"az", "bz", "z", "d"},
		},
		"test_parens_in_precedence": {
			input:      "a AND (c OR d)",
			wantStr:    `$1 AND ($2 OR $3)`,
			wantParams: []any{"a", "c", "d"},
		},
		"test_range_precedence_simple": {
			input:      "c:[* to -1] OR d",
			wantStr:    `("c" <= $1) OR $2`,
			wantParams: []any{-1, "d"},
		},
		"test_range_precedence": {
			input:      "a OR b AND c:[* to -1] OR d",
			wantStr:    `($1 OR ($2 AND ("c" <= $3))) OR $4`,
			wantParams: []any{"a", "b", -1, "d"},
		},
		"test_full_precedence": {
			input:      "a OR b AND c:[* to -1] OR d AND NOT +e:f",
			wantStr:    `($1 OR ($2 AND ("c" <= $3))) OR ($4 AND (NOT("e" = $5)))`,
			wantParams: []any{"a", "b", -1, "d", "f"},
		},
		"test_elastic_greater_than_precedence": {
			input:      "a:>10 AND -b:<=-20",
			wantStr:    `("a" > $1) AND (NOT("b" <= $2))`,
			wantParams: []any{10, -20},
		},
		"escape_quotes": {
			input:      "a:'b'",
			wantStr:    `"a" = $1`,
			wantParams: []any{"'b'"},
		},
		"name_starts_with_number": {
			input:      "1a:b",
			wantStr:    `"1a" = $1`,
			wantParams: []any{"b"},
		},
		"default_field_and": {
			input:        `title:"The Right Way" AND go`,
			wantStr:      `("title" = $1) AND ("default" = $2)`,
			wantParams:   []any{"The Right Way", "go"},
			defaultField: "default",
		},
		"default_field_or": {
			input:        `title:"The Right Way" OR go`,
			wantStr:      `("title" = $1) OR ("default" = $2)`,
			wantParams:   []any{"The Right Way", "go"},
			defaultField: "default",
		},
		"default_field_not": {
			input:        `title:"The Right Way" AND NOT(go)`,
			wantStr:      `("title" = $1) AND (NOT("default" = $2))`,
			wantParams:   []any{"The Right Way", "go"},
			defaultField: "default",
		},
		"default_bare_field": {
			input:        `this is an example`,
			wantStr:      `((("default" = $1) AND ("default" = $2)) AND ("default" = $3)) AND ("default" = $4)`,
			wantParams:   []any{"this", "is", "an", "example"},
			defaultField: "default",
		},
		"default_single_literal": {
			input:        `a`,
			wantStr:      `"default" = $1`,
			wantParams:   []any{"a"},
			defaultField: "default",
		},
		"question_marks_in_literal_are_regular_expression": {
			input:      `foo:abc?`,
			wantStr:    `"foo" SIMILAR TO $1`,
			wantParams: []any{"abc_"},
		},
		"start asterisk_in_literal_are_regular_expression": {
			input:      `foo:*`,
			wantStr:    `"foo" SIMILAR TO $1`,
			wantParams: []any{"%"},
		},
		"implicit_and_with_subexpressions_and_default_field": {
			input:        `title:"Foo" a b`,
			wantStr:      `(("title" = $1) AND ("default" = $2)) AND ("default" = $3)`,
			wantParams:   []any{"Foo", "a", "b"},
			defaultField: "default",
		},
		"implicit_and_with_negated_subexpressions_and_default_field": {
			input:        `title:"Foo" -a:c b`,
			wantStr:      `(("title" = $1) AND (NOT("a" = $2))) AND ("default" = $3)`,
			wantParams:   []any{"Foo", "c", "b"},
			defaultField: "default",
		},
		"implicit_and_with_negated_subexpressions_and_default_field_reversed": {
			input:        `title:"Foo" a:c -b`,
			wantStr:      `(("title" = $1) AND ("a" = $2)) AND (NOT("default" = $3))`,
			wantParams:   []any{"Foo", "c", "b"},
			defaultField: "default",
		},
	}

	for name, tc := range tcs {
		t.Run(name, func(t *testing.T) {
			gotStr, gotParams, err := ToParameterizedPostgres(tc.input, WithDefaultField(tc.defaultField))
			if err != nil {
				// if we got an expect error then we are fine
				if tc.err != "" && strings.Contains(err.Error(), tc.err) {
					return
				}
				t.Fatalf("unexpected error rendering expression: %v", err)
			}

			if tc.err != "" {
				t.Fatalf("\nexpected error [%s]\ngot: %s", tc.err, gotStr)
			}

			if gotStr != tc.wantStr {
				expr, err := Parse(tc.input)
				if err != nil {
					t.Fatalf("unable to parse expression: %v", err)
				}
				t.Fatalf("\nwant %s\ngot  %s\nparsed expression: %#v\n", tc.wantStr, gotStr, expr)
			}

			if len(gotParams) != len(tc.wantParams) {
				t.Fatalf("expected %d params(%v), got %d (%v)", len(tc.wantParams), tc.wantParams, len(gotParams), gotParams)
			}

			for i := range gotParams {
				if gotParams[i] != tc.wantParams[i] {
					t.Fatalf("expected param %d to be %v, got %v", i, tc.wantParams[i], gotParams[i])
				}
			}
		})
	}
}
