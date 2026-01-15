package driver

import (
	"testing"

	"github.com/grindlemire/go-lucene/pkg/lucene/expr"
)

const errTemplate = "%s:\n    wanted %s\n    got    %s"

func TestSQLDriver(t *testing.T) {
	type tc struct {
		input *expr.Expression
		want  string
	}

	tcs := map[string]tc{
		"simple_equals": {
			input: expr.Eq("a", 5),
			want:  `"a" = 5`,
		},
		"simple_and": {
			input: expr.AND(expr.Eq("a", 5), expr.Eq("b", "foo")),
			want:  `("a" = 5) AND ("b" = 'foo')`,
		},
		"simple_or": {
			input: expr.OR(expr.Eq("a", 5), expr.Eq("b", "foo")),
			want:  `("a" = 5) OR ("b" = 'foo')`,
		},
		"simple_not": {
			input: expr.NOT(expr.Eq("a", 1)),
			want:  `NOT("a" = 1)`,
		},
		"simple_like": {
			input: expr.LIKE("a", "%(b|d)%"),
			want:  `"a" SIMILAR TO '%(b|d)%'`,
		},
		"string_range": {
			input: expr.Rang("a", "foo", "bar", true),
			want:  `"a" BETWEEN 'foo' AND 'bar'`,
		},
		"mixed_number_range": {
			input: expr.Rang("a", 1.1, 10, true),
			want:  `"a" >= 1.10 AND "a" <= 10.00`,
		},
		"mixed_number_range_exlusive": {
			input: expr.Rang("a", 1, 10.1, false),
			want:  `"a" > 1.00 AND "a" < 10.10`,
		},
		"int_range": {
			input: expr.Rang("a", 1, 10, true),
			want:  `"a" >= 1 AND "a" <= 10`,
		},
		"int_range_exlusive": {
			input: expr.Rang("a", 1, 10, false),
			want:  `"a" > 1 AND "a" < 10`,
		},
		"float_range": {
			input: expr.Rang("a", 1.0, 10.0, true),
			want:  `"a" >= 1 AND "a" <= 10`,
		},
		"float_range_exlusive": {
			input: expr.Rang("a", 1.0, 10.0, false),
			want:  `"a" > 1 AND "a" < 10`,
		},
		"lt_range": {
			input: expr.Rang("a", "*", 10, false),
			want:  `"a" < 10`,
		},
		"lte_range": {
			input: expr.Rang("a", "*", 10, true),
			want:  `"a" <= 10`,
		},
		"gt_range": {
			input: expr.Rang("a", 1, "*", false),
			want:  `"a" > 1`,
		},
		"gte_range": {
			input: expr.Rang("a", 1, "*", true),
			want:  `"a" >= 1`,
		},
		"lt": {
			input: expr.LESS("a", 10),
			want:  `"a" < 10`,
		},
		"lte": {
			input: expr.LESSEQ("a", 10),
			want:  `"a" <= 10`,
		},
		"gt": {
			input: expr.GREATER("a", 10),
			want:  `"a" > 10`,
		},
		"gte": {
			input: expr.GREATEREQ("a", 10),
			want:  `"a" >= 10`,
		},
		"must_ignored": {
			input: expr.MUST(expr.Eq("a", 1)),
			want:  `"a" = 1`,
		},
		"nested_filter": {
			input: expr.Expr(
				expr.Expr(
					expr.Expr(
						"a",
						expr.Equals,
						"foo",
					),
					expr.Or,
					expr.Expr(
						"b",
						expr.Equals,
						expr.REGEXP("/b*ar/"),
					),
				),
				expr.And,
				expr.Expr(
					expr.Rang("c", "aaa", "*", false),
					expr.Not,
				),
			),
			want: `(("a" = 'foo') OR ("b" ~ 'b*ar')) AND (NOT("c" BETWEEN 'aaa' AND '*'))`,
		},
		"space_in_fieldname": {
			input: expr.Eq("a b", 1),
			want:  `"a b" = 1`,
		},
		"equals_in_equals": {
			input: expr.Eq("a", expr.Eq("b", 1)),
			want:  `"a" = ("b" = 1)`,
		},
		"regexp": {
			input: expr.REGEXP("/b*ar/"),
			want:  `'b*ar'`,
		},
	}

	for name, tc := range tcs {
		t.Run(name, func(t *testing.T) {
			got, err := NewPostgresDriver().Render(tc.input)
			if err != nil {
				t.Fatalf("got an unexpected error when rendering: %v", err)
			}

			if tc.want != got {
				t.Fatalf(errTemplate, "generated sql does not match", tc.want, got)
			}
		})
	}
}
