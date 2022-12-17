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
