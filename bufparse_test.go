package lucene

import (
	"reflect"
	"testing"

	"github.com/grindlemire/go-lucene/expr"
)

func TestBufParse(t *testing.T) {
	type tc struct {
		input string
		want  expr.Expression
	}

	tcs := map[string]tc{
		"basic_inclusive_range": {
			input: "a:[* TO 5]",
			want:  EQ(Lit("a"), Rang(Lit("*"), Lit(5), true)),
		},
		"basic_exclusive_range": {
			input: "a:{* TO 5}",
			want:  EQ(Lit("a"), Rang(Lit("*"), Lit(5), false)),
		},
		"range_over_strings": {
			input: "a:{foo TO bar}",
			want:  EQ(Lit("a"), Rang(Lit("foo"), Lit("bar"), false)),
		},
		"basic_fuzzy": {
			input: "b AND a~",
			want:  AND(Lit("b"), FUZZY(Lit("a"), 1)),
		},
		"fuzzy_power": {
			input: "b AND a~10",
			want:  AND(Lit("b"), FUZZY(Lit("a"), 10)),
		},
		"basic_boost": {
			input: "b AND a^",
			want:  AND(Lit("b"), BOOST(Lit("a"), 1.0)),
		},
		"boost_power": {
			input: "b AND a^10",
			want:  AND(Lit("b"), BOOST(Lit("a"), 10.0)),
		},
		"most_basic": {
			input: "a AND b",
			want: AND(
				Lit("a"),
				Lit("b"),
			),
		},
		"test_expr": {
			input: "a OR b AND c OR d",
			want: OR(
				OR(
					Lit("a"),
					AND(Lit("b"), Lit("c")),
				),
				Lit("d"),
			),
		},
		"test_not": {
			input: "NOT a OR b AND NOT c OR d",
			want: OR(
				OR(
					NOT(Lit("a")),
					AND(Lit("b"), NOT(Lit("c"))),
				),
				Lit("d"),
			),
		},
		"test_equals": {
			input: "a:az OR b:bz AND NOT c:z OR d",
			want: OR(
				OR(
					EQ(Lit("a"), Lit("az")),
					AND(
						EQ(Lit("b"), Lit("bz")),
						NOT(
							EQ(Lit("c"), Lit("z")),
						),
					),
				),
				Lit("d"),
			),
		},
		"test_parens": {
			input: "a AND (c OR d)",
			want: AND(
				Lit("a"),
				OR(
					Lit("c"),
					Lit("d"),
				),
			),
		},
		"test_full_precedance": {
			input: "a OR b AND c:[* to -1] OR d AND NOT +e:f",
			want: OR(
				OR(
					Lit("a"),
					AND(Lit("b"), EQ(Lit("c"), Rang(Lit("*"), Lit(-1), true))),
				),
				AND(
					Lit("d"),
					NOT(MUST(EQ(Lit("e"), Lit("f")))),
				),
			),
		},
		"test_full_precedance_with_suffixes": {
			input: "a OR b AND c OR d~ AND NOT +(e:f)^10",
			want: OR(
				OR(
					Lit("a"),
					AND(Lit("b"), Lit("c")),
				),
				AND(
					FUZZY(Lit("d"), 1),
					NOT(
						MUST(
							BOOST(
								EQ(Lit("e"), Lit("f")),
								10.0,
							),
						),
					),
				),
			),
		},
		"test_not_expr": {
			input: "(NOT a OR b) AND NOT(c OR d)",
			want: AND(
				OR(NOT(Lit("a")), Lit("b")),
				NOT(OR(Lit("c"), Lit("d"))),
			),
		},
		"single_literal": {
			input: "a",
			want:  Lit("a"),
		},
		"basic_equal": {
			input: "a:b",
			want: EQ(
				Lit("a"),
				Lit("b"),
			),
		},
		"basic_equal_with_number": {
			input: "a:5",
			want: EQ(
				Lit("a"),
				Lit(5),
			),
		},
		"basic_wild_equal_with_*": {
			input: "a:b*",
			want: EQ(
				Lit("a"),
				Wild("b*"),
			),
		},
		"basic_wild_equal_with_?": {
			input: "a:b?z",
			want: EQ(
				Lit("a"),
				Wild("b?z"),
			),
		},
		"regexp": {
			input: "a:/b [c]/",
			want: EQ(
				Lit("a"), REGEXP("b [c]"),
			),
		},
		"regexp_with_keywords": {
			input: `a:/b "[c]/`,
			want: EQ(
				Lit("a"), REGEXP(`b "[c]`),
			),
		},
		"basic_not": {
			input: "NOT b",
			want:  NOT(Lit("b")),
		},
		"nested_not": {
			input: "a:foo OR NOT b:bar AND NOT c:baz",
			want: OR(
				EQ(Lit("a"), Lit("foo")),
				AND(
					NOT(EQ(Lit("b"), Lit("bar"))),
					NOT(EQ(Lit("c"), Lit("baz"))),
				),
			),
		},
	}

	for name, tc := range tcs {
		t.Run(name, func(t *testing.T) {
			got, err := BufParse(tc.input)
			if err != nil {
				t.Fatalf("wanted no error, got: %v", err)
			}
			if !reflect.DeepEqual(tc.want, got) {
				t.Fatalf(errTemplate, "error parsing", tc.want, got)
			}
		})
	}
}
