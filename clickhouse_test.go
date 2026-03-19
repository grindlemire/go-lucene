package lucene

import (
	"strings"
	"testing"

	"github.com/AlxBystrov/go-lucene/pkg/driverclick"
)

func TestClickhouseSQLEndToEnd(t *testing.T) {
	type tc struct {
		input string
		want  string
		err   string
	}

	tcs := map[string]tc{
		// "single_literal": {
		// 	input: "a",
		// 	want:  `a`,
		// },
		"basic_equal": {
			input: "a:b",
			want:  `lowerUTF8(strings.value[indexOf(strings.name,'a')]) like lowerUTF8('%b%')`,
		},
		"basic_equal_with_number": {
			input: "a:5",
			want:  `numbers.value[indexOf(numbers.name,'a')] = 5`,
		},
		"basic_greater_with_number": {
			input: "a:>22",
			want:  `numbers.value[indexOf(numbers.name,'a')] > 22`,
		},
		"basic_greater_eq_with_number": {
			input: "a:>=22",
			want:  `numbers.value[indexOf(numbers.name,'a')] >= 22`,
		},
		"basic_less_with_number": {
			input: "a:<22",
			want:  `numbers.value[indexOf(numbers.name,'a')] < 22`,
		},
		"basic_less_eq_with_number": {
			input: "a:<=22",
			want:  `numbers.value[indexOf(numbers.name,'a')] <= 22`,
		},
		"basic_greater_less_with_number": {
			input: "a:<22 AND b:>33",
			want:  `(numbers.value[indexOf(numbers.name,'a')] < 22) AND (numbers.value[indexOf(numbers.name,'b')] > 33)`,
		},
		"basic_greater_less_eq_with_number": {
			input: "a:<=22 AND b:>=33",
			want:  `(numbers.value[indexOf(numbers.name,'a')] <= 22) AND (numbers.value[indexOf(numbers.name,'b')] >= 33)`,
		},
		"basic_wild_equal_with_*": {
			input: "a:b*",
			want:  `lowerUTF8(strings.value[indexOf(strings.name,'a')]) like lowerUTF8('b%')`,
		},
		"basic_wild_equal_with_?": {
			input: "a:b?z",
			want:  `lowerUTF8(strings.value[indexOf(strings.name,'a')]) like lowerUTF8('b_z')`,
		},
		"basic_inclusive_range": {
			input: "a:[* TO 5]",
			want:  `numbers.value[indexOf(numbers.name,'a')] <= 5`,
		},
		"basic_exclusive_range": {
			input: "a:{* TO 5}",
			want:  `numbers.value[indexOf(numbers.name,'a')] < 5`,
		},
		"range_over_strings": {
			input: "a:{foo TO bar}",
			want:  `strings.value[indexOf(strings.name,'a')] BETWEEN 'foo' AND 'bar'`,
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
			want:  `match(lowerUTF8(strings.value[indexOf(strings.name,'a')]),lowerUTF8('b [c]'))`,
		},
		"regexp_with_keywords": {
			input: `a:/b "[c]/`,
			want:  `match(lowerUTF8(strings.value[indexOf(strings.name,'a')]),lowerUTF8('b "[c]'))`,
		},
		"regexp_with_escaped_chars": {
			input: `url:/example.com\/foo\/bar\/.*/`,
			want:  `match(lowerUTF8(strings.value[indexOf(strings.name,'url')]),lowerUTF8('example.com\/foo\/bar\/.*'))`,
		},
		"basic_default_AND": {
			input: "a b",
			want:  `(lowerUTF8(strings.value[indexOf(strings.name,'msg')]) like lowerUTF8('%a%')) AND (lowerUTF8(strings.value[indexOf(strings.name,'msg')]) like lowerUTF8('%b%'))`,
		},
		"default_to_AND_with_subexpressions": {
			input: "a:b c:d",
			want:  `(lowerUTF8(strings.value[indexOf(strings.name,'a')]) like lowerUTF8('%b%')) AND (lowerUTF8(strings.value[indexOf(strings.name,'c')]) like lowerUTF8('%d%'))`,
		},
		"basic_and": {
			input: "a AND b",
			want:  `(lowerUTF8(strings.value[indexOf(strings.name,'msg')]) like lowerUTF8('%a%')) AND (lowerUTF8(strings.value[indexOf(strings.name,'msg')]) like lowerUTF8('%b%'))`,
		},
		"and_with_nesting": {
			input: "a:foo AND b:bar",
			want:  `(lowerUTF8(strings.value[indexOf(strings.name,'a')]) like lowerUTF8('%foo%')) AND (lowerUTF8(strings.value[indexOf(strings.name,'b')]) like lowerUTF8('%bar%'))`,
		},
		"basic_or": {
			input: "a OR b",
			want:  `(lowerUTF8(strings.value[indexOf(strings.name,'msg')]) like lowerUTF8('%a%')) OR (lowerUTF8(strings.value[indexOf(strings.name,'msg')]) like lowerUTF8('%b%'))`,
		},
		"or_with_nesting": {
			input: "a:foo OR b:bar",
			want:  `(lowerUTF8(strings.value[indexOf(strings.name,'a')]) like lowerUTF8('%foo%')) OR (lowerUTF8(strings.value[indexOf(strings.name,'b')]) like lowerUTF8('%bar%'))`,
		},
		"range_operator_inclusive": {
			input: "a:[1 TO 5]",
			want:  `numbers.value[indexOf(numbers.name,'a')] >= 1 AND numbers.value[indexOf(numbers.name,'a')] <= 5`,
		},
		"range_operator_inclusive_unbound": {
			input: `a:[* TO 200]`,
			want:  `numbers.value[indexOf(numbers.name,'a')] <= 200`,
		},
		"range_operator_exclusive": {
			input: `a:{"ab" TO "az"}`,
			want:  `strings.value[indexOf(strings.name,'a')] BETWEEN 'ab' AND 'az'`,
		},
		"range_operator_exclusive_unbound": {
			input: `a:{2 TO *}`,
			want:  `numbers.value[indexOf(numbers.name,'a')] > 2`,
		},
		"basic_not": {
			input: "NOT b",
			want:  `NOT(lowerUTF8(strings.value[indexOf(strings.name,'msg')]) like lowerUTF8('%b%'))`,
		},
		"nested_not": {
			input: "a:foo OR NOT b:bar",
			want:  `(lowerUTF8(strings.value[indexOf(strings.name,'a')]) like lowerUTF8('%foo%')) OR (NOT(lowerUTF8(strings.value[indexOf(strings.name,'b')]) like lowerUTF8('%bar%')))`,
		},
		"term_grouping": {
			input: "(a:foo OR b:bar) AND c:baz",
			want:  `((lowerUTF8(strings.value[indexOf(strings.name,'a')]) like lowerUTF8('%foo%')) OR (lowerUTF8(strings.value[indexOf(strings.name,'b')]) like lowerUTF8('%bar%'))) AND (lowerUTF8(strings.value[indexOf(strings.name,'c')]) like lowerUTF8('%baz%'))`,
		},
		"value_grouping": {
			input: "a:(foo OR baz OR bar)",
			want:  `lowerUTF8(strings.value[indexOf(strings.name,'a')]) like lowerUTF8((((lowerUTF8(strings.value[indexOf(strings.name,'msg')]) like lowerUTF8('%foo%')) OR (lowerUTF8(strings.value[indexOf(strings.name,'msg')]) like lowerUTF8('%baz%'))) OR (lowerUTF8(strings.value[indexOf(strings.name,'msg')]) like lowerUTF8('%bar%'))))`,
		},
		"basic_must": {
			input: "+a:b",
			want:  `lowerUTF8(strings.value[indexOf(strings.name,'a')]) like lowerUTF8('%b%')`,
		},
		"basic_must_not": {
			input: "-a:b",
			want:  `NOT(lowerUTF8(strings.value[indexOf(strings.name,'a')]) like lowerUTF8('%b%'))`,
		},
		"basic_nested_must_not": {
			input: "d:e AND (-a:b AND +f:e)",
			want:  `(lowerUTF8(strings.value[indexOf(strings.name,'d')]) like lowerUTF8('%e%')) AND ((NOT(lowerUTF8(strings.value[indexOf(strings.name,'a')]) like lowerUTF8('%b%'))) AND (lowerUTF8(strings.value[indexOf(strings.name,'f')]) like lowerUTF8('%e%')))`,
		},
		"basic_escaping": {
			input: `a:\(1\+1\)\:2`,
			want:  `lowerUTF8(strings.value[indexOf(strings.name,'a')]) like lowerUTF8('%(1+1):2%')`,
		},
		"escaped_column_name": {
			input: `foo\ bar:b`,
			want:  `lowerUTF8(strings.value[indexOf(strings.name,'foo bar')]) like lowerUTF8('%b%')`,
		},
		"boost_key_value": {
			input: "a:b^2 AND foo",
			err:   "unable to render operator [BOOST]",
		},
		"nested_sub_expressions": {
			input: "((title:foo OR title:bar) AND (body:foo OR body:bar)) OR k:v",
			want:  `(((lowerUTF8(strings.value[indexOf(strings.name,'title')]) like lowerUTF8('%foo%')) OR (lowerUTF8(strings.value[indexOf(strings.name,'title')]) like lowerUTF8('%bar%'))) AND ((lowerUTF8(strings.value[indexOf(strings.name,'body')]) like lowerUTF8('%foo%')) OR (lowerUTF8(strings.value[indexOf(strings.name,'body')]) like lowerUTF8('%bar%')))) OR (lowerUTF8(strings.value[indexOf(strings.name,'k')]) like lowerUTF8('%v%'))`,
		},
		"fuzzy_key_value": {
			input: "a:b~2 AND foo",
			err:   "unable to render operator [FUZZY]",
		},
		"precedence_works": {
			input: "a:b AND c:d OR e:f OR h:i AND j:k",
			want:  `(((lowerUTF8(strings.value[indexOf(strings.name,'a')]) like lowerUTF8('%b%')) AND (lowerUTF8(strings.value[indexOf(strings.name,'c')]) like lowerUTF8('%d%'))) OR (lowerUTF8(strings.value[indexOf(strings.name,'e')]) like lowerUTF8('%f%'))) OR ((lowerUTF8(strings.value[indexOf(strings.name,'h')]) like lowerUTF8('%i%')) AND (lowerUTF8(strings.value[indexOf(strings.name,'j')]) like lowerUTF8('%k%')))`,
		},
		"test_precedence_weaving": {
			input: "a OR b AND c OR d",
			want:  `((lowerUTF8(strings.value[indexOf(strings.name,'msg')]) like lowerUTF8('%a%')) OR ((lowerUTF8(strings.value[indexOf(strings.name,'msg')]) like lowerUTF8('%b%')) AND (lowerUTF8(strings.value[indexOf(strings.name,'msg')]) like lowerUTF8('%c%')))) OR (lowerUTF8(strings.value[indexOf(strings.name,'msg')]) like lowerUTF8('%d%'))`,
		},
		"test_precedence_weaving_with_not": {
			input: "NOT a OR b AND NOT c OR d",
			want:  `((NOT(lowerUTF8(strings.value[indexOf(strings.name,'msg')]) like lowerUTF8('%a%'))) OR ((lowerUTF8(strings.value[indexOf(strings.name,'msg')]) like lowerUTF8('%b%')) AND (NOT(lowerUTF8(strings.value[indexOf(strings.name,'msg')]) like lowerUTF8('%c%'))))) OR (lowerUTF8(strings.value[indexOf(strings.name,'msg')]) like lowerUTF8('%d%'))`,
		},
		"test_equals_in_precedence": {
			input: "a:az OR b:bz AND NOT c:z OR d",
			want:  `((lowerUTF8(strings.value[indexOf(strings.name,'a')]) like lowerUTF8('%az%')) OR ((lowerUTF8(strings.value[indexOf(strings.name,'b')]) like lowerUTF8('%bz%')) AND (NOT(lowerUTF8(strings.value[indexOf(strings.name,'c')]) like lowerUTF8('%z%'))))) OR (lowerUTF8(strings.value[indexOf(strings.name,'msg')]) like lowerUTF8('%d%'))`,
		},
		"test_parens_in_precedence": {
			input: "a AND (c OR d)",
			want:  `(lowerUTF8(strings.value[indexOf(strings.name,'msg')]) like lowerUTF8('%a%')) AND ((lowerUTF8(strings.value[indexOf(strings.name,'msg')]) like lowerUTF8('%c%')) OR (lowerUTF8(strings.value[indexOf(strings.name,'msg')]) like lowerUTF8('%d%')))`,
		},
		"test_range_precedence_simple": {
			input: "c:[* to -1] OR d",
			want:  `(numbers.value[indexOf(numbers.name,'c')] <= -1) OR (lowerUTF8(strings.value[indexOf(strings.name,'msg')]) like lowerUTF8('%d%'))`,
		},
		"test_range_precedence": {
			input: "a OR b AND c:[* to -1] OR d",
			want:  `((lowerUTF8(strings.value[indexOf(strings.name,'msg')]) like lowerUTF8('%a%')) OR ((lowerUTF8(strings.value[indexOf(strings.name,'msg')]) like lowerUTF8('%b%')) AND (numbers.value[indexOf(numbers.name,'c')] <= -1))) OR (lowerUTF8(strings.value[indexOf(strings.name,'msg')]) like lowerUTF8('%d%'))`,
		},
		"test_full_precedence": {
			input: "a OR b AND c:[* to -1] OR d AND NOT +e:f",
			want:  `((lowerUTF8(strings.value[indexOf(strings.name,'msg')]) like lowerUTF8('%a%')) OR ((lowerUTF8(strings.value[indexOf(strings.name,'msg')]) like lowerUTF8('%b%')) AND (numbers.value[indexOf(numbers.name,'c')] <= -1))) OR ((lowerUTF8(strings.value[indexOf(strings.name,'msg')]) like lowerUTF8('%d%')) AND (NOT(lowerUTF8(strings.value[indexOf(strings.name,'e')]) like lowerUTF8('%f%'))))`,
		},
		"test_elastic_greater_than_precedence": {
			input: "a:>10 AND -b:<=-20",
			want:  `(numbers.value[indexOf(numbers.name,'a')] > 10) AND (NOT(numbers.value[indexOf(numbers.name,'b')] <= -20))`,
		},
		// skip this test
		// "escape_quotes": {
		// 	input: `a:"i search for string"`,
		// 	want:  `lowerUTF8(strings.value[indexOf(strings.name,'a')]) like lowerUTF8('''b''')`,
		// },
		"name_starts_with_number": {
			input: "1a:b",
			want:  `lowerUTF8(strings.value[indexOf(strings.name,'1a')]) like lowerUTF8('%b%')`,
		},
	}

	for name, tc := range tcs {
		t.Run(name, func(t *testing.T) {
			expr, err := Parse(tc.input, WithDefaultField("msg"))
			if err != nil {
				t.Fatal(err)
			}

			got, err := driverclick.NewClickhouseDriver().Render(expr)
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

func TestClickhouseDriverFieldBindings(t *testing.T) {
	driver := driverclick.NewClickhouseDriver(
		driverclick.WithFieldBindings(map[string]driverclick.FieldBinding{
			"msg": {
				Type:    driverclick.StringField,
				Storage: driverclick.MaterializedColumn,
				Column:  "msg",
			},
			"latency_ms": {
				Type:    driverclick.NumberField,
				Storage: driverclick.MaterializedColumn,
				Column:  "latency_ms",
			},
			"message": {
				Type:    driverclick.StringField,
				Storage: driverclick.MaterializedColumn,
				Column:  "message_text",
			},
		}),
	)

	type tc struct {
		input string
		want  string
	}

	tcs := map[string]tc{
		"materialized_string_field": {
			input: "msg:445a2c13-ba5c-4c0d-a577-6eb879f5ebcf",
			want:  "lowerUTF8(msg) like lowerUTF8('%445a2c13-ba5c-4c0d-a577-6eb879f5ebcf%')",
		},
		"materialized_number_field": {
			input: "latency_ms:>100",
			want:  "latency_ms > 100",
		},
		"materialized_alias_column": {
			input: "message:/req-resp mode/",
			want:  "match(lowerUTF8(message_text),lowerUTF8('req-resp mode'))",
		},
		"fallback_to_array_storage": {
			input: "pod:my-pod",
			want:  "lowerUTF8(strings.value[indexOf(strings.name,'pod')]) like lowerUTF8('%my-pod%')",
		},
		"mixed_materialized_fields": {
			input: `msg:"search string" and pod:"my-pod"`,
			want:  "(lowerUTF8(msg) like lowerUTF8('%search string%')) AND (lowerUTF8(strings.value[indexOf(strings.name,'pod')]) like lowerUTF8('%my-pod%'))",
		},
	}

	for name, tc := range tcs {
		t.Run(name, func(t *testing.T) {
			expr, err := Parse(tc.input)
			if err != nil {
				t.Fatal(err)
			}

			got, err := driver.Render(expr)
			if err != nil {
				t.Fatalf("unexpected error rendering expression: %v", err)
			}

			if got != tc.want {
				t.Fatalf("\nwant %s\ngot  %s\nparsed expression: %#v\n", tc.want, got, expr)
			}
		})
	}
}
