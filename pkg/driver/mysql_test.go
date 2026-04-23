package driver

import (
	"testing"

	"github.com/grindlemire/go-lucene/pkg/lucene/expr"
)

// NOTE: MySQL's LIKE does not support alternation, character classes, or
// grouping. Lucene wildcard patterns containing |, (), [], {}, or + are
// routed through PrepareLikePattern to the REGEXP path with an anchored
// ^(...)$ translation so the match semantics line up with Postgres
// SIMILAR TO.

func TestMySQLDriver(t *testing.T) {
	type tc struct {
		input *expr.Expression
		want  string
	}

	tcs := map[string]tc{
		"simple_equals": {
			input: expr.Eq("a", 5),
			want:  "`a` = 5",
		},
		"simple_and": {
			input: expr.AND(expr.Eq("a", 5), expr.Eq("b", "foo")),
			want:  "(`a` = 5) AND (`b` = 'foo')",
		},
		"simple_or": {
			input: expr.OR(expr.Eq("a", 5), expr.Eq("b", "foo")),
			want:  "(`a` = 5) OR (`b` = 'foo')",
		},
		"simple_not": {
			input: expr.NOT(expr.Eq("a", 1)),
			want:  "NOT(`a` = 1)",
		},
		"simple_like_wildcard": {
			input: expr.LIKE("a", "b*"),
			want:  "`a` LIKE 'b%' ESCAPE '#'",
		},
		"simple_like_question": {
			input: expr.LIKE("a", "b?z"),
			want:  "`a` LIKE 'b_z' ESCAPE '#'",
		},
		"string_range_inclusive": {
			input: expr.Rang("a", "foo", "bar", true),
			want:  "`a` BETWEEN 'foo' AND 'bar'",
		},
		"string_range_exclusive": {
			input: expr.Rang("a", "foo", "bar", false),
			want:  "`a` > 'foo' AND `a` < 'bar'",
		},
		"mixed_number_range": {
			input: expr.Rang("a", 1.1, 10, true),
			want:  "`a` >= 1.1 AND `a` <= 10",
		},
		"mixed_number_range_exclusive": {
			input: expr.Rang("a", 1, 10.1, false),
			want:  "`a` > 1 AND `a` < 10.1",
		},
		"int_range": {
			input: expr.Rang("a", 1, 10, true),
			want:  "`a` >= 1 AND `a` <= 10",
		},
		"int_range_exclusive": {
			input: expr.Rang("a", 1, 10, false),
			want:  "`a` > 1 AND `a` < 10",
		},
		"float_range": {
			input: expr.Rang("a", 1.0, 10.0, true),
			want:  "`a` >= 1 AND `a` <= 10",
		},
		"float_range_exclusive": {
			input: expr.Rang("a", 1.0, 10.0, false),
			want:  "`a` > 1 AND `a` < 10",
		},
		"lt_range": {
			input: expr.Rang("a", "*", 10, false),
			want:  "`a` < 10",
		},
		"lte_range": {
			input: expr.Rang("a", "*", 10, true),
			want:  "`a` <= 10",
		},
		"gt_range": {
			input: expr.Rang("a", 1, "*", false),
			want:  "`a` > 1",
		},
		"gte_range": {
			input: expr.Rang("a", 1, "*", true),
			want:  "`a` >= 1",
		},
		"lt_range_float": {
			input: expr.Rang("a", "*", 10.5, false),
			want:  "`a` < 10.5",
		},
		"lte_range_float": {
			input: expr.Rang("a", "*", 10.5, true),
			want:  "`a` <= 10.5",
		},
		"gt_range_float": {
			input: expr.Rang("a", 1.5, "*", false),
			want:  "`a` > 1.5",
		},
		"gte_range_float": {
			input: expr.Rang("a", 1.5, "*", true),
			want:  "`a` >= 1.5",
		},
		"float_range_high_precision": {
			input: expr.Rang("a", 1.234, 5.678, true),
			want:  "`a` >= 1.234 AND `a` <= 5.678",
		},
		"float_range_high_precision_exclusive": {
			input: expr.Rang("a", 1.234, 5.678, false),
			want:  "`a` > 1.234 AND `a` < 5.678",
		},
		"lt": {
			input: expr.LESS("a", 10),
			want:  "`a` < 10",
		},
		"lte": {
			input: expr.LESSEQ("a", 10),
			want:  "`a` <= 10",
		},
		"gt": {
			input: expr.GREATER("a", 10),
			want:  "`a` > 10",
		},
		"gte": {
			input: expr.GREATEREQ("a", 10),
			want:  "`a` >= 10",
		},
		"must_ignored": {
			input: expr.MUST(expr.Eq("a", 1)),
			want:  "`a` = 1",
		},
		"nested_filter": {
			input: expr.Expr(
				expr.Expr(
					expr.Expr("a", expr.Equals, "foo"),
					expr.Or,
					expr.Expr("b", expr.Equals, expr.REGEXP("/b*ar/")),
				),
				expr.And,
				expr.Expr(
					expr.Rang("c", "aaa", "*", false),
					expr.Not,
				),
			),
			want: "((`a` = 'foo') OR (`b` REGEXP 'b*ar')) AND (NOT(`c` > 'aaa'))",
		},
		"space_in_fieldname": {
			input: expr.Eq("a b", 1),
			want:  "`a b` = 1",
		},
		"equals_in_equals": {
			input: expr.Eq("a", expr.Eq("b", 1)),
			want:  "`a` = (`b` = 1)",
		},
		"regexp": {
			input: expr.REGEXP("/b*ar/"),
			want:  `'b*ar'`,
		},
		"like_with_literal_percent": {
			input: expr.LIKE("field", "100%*"),
			want:  "`field` LIKE '100#%%' ESCAPE '#'",
		},
		"like_with_literal_underscore": {
			input: expr.LIKE("field", "foo_bar*"),
			want:  "`field` LIKE 'foo#_bar%' ESCAPE '#'",
		},
		"like_without_special_chars": {
			input: expr.LIKE("field", "clean*"),
			want:  "`field` LIKE 'clean%' ESCAPE '#'",
		},
		"like_percent_and_underscore_mixed": {
			input: expr.LIKE("field", "100%_test*"),
			want:  "`field` LIKE '100#%#_test%' ESCAPE '#'",
		},
		"like_with_alternation": {
			input: expr.LIKE("a", "*(b|d)*"),
			want:  "`a` REGEXP '^(.*(b|d).*)$'",
		},
		"like_with_char_class": {
			input: expr.LIKE("a", "foo[abc]*"),
			want:  "`a` REGEXP '^(foo[abc].*)$'",
		},
		"like_with_grouping": {
			input: expr.LIKE("a", "(foo)*"),
			want:  "`a` REGEXP '^((foo).*)$'",
		},
		"like_regex_preserves_literal_underscore": {
			input: expr.LIKE("a", "foo_bar|baz"),
			want:  "`a` REGEXP '^(foo_bar|baz)$'",
		},
		"string_with_backslash": {
			input: expr.Eq("path", `c:\foo\bar`),
			want:  "`path` = 'c:\\\\foo\\\\bar'",
		},
		"string_with_quote_and_backslash": {
			input: expr.Eq("path", `a'b\c`),
			want:  "`path` = 'a''b\\\\c'",
		},
		"bool_true": {
			input: expr.Eq("active", true),
			want:  "`active` = TRUE",
		},
		"bool_false": {
			input: expr.Eq("active", false),
			want:  "`active` = FALSE",
		},
	}

	for name, tc := range tcs {
		t.Run(name, func(t *testing.T) {
			got, err := NewMySQLDriver().Render(tc.input)
			if err != nil {
				t.Fatalf("got an unexpected error when rendering: %v", err)
			}
			if tc.want != got {
				t.Fatalf(errTemplate, "generated sql does not match", tc.want, got)
			}
		})
	}
}

func TestMySQLDriverParam(t *testing.T) {
	type tc struct {
		input      *expr.Expression
		wantStr    string
		wantParams []any
	}

	tcs := map[string]tc{
		"bool_true_param": {
			input:      expr.Eq("active", true),
			wantStr:    "`active` = ?",
			wantParams: []any{true},
		},
		"bool_false_param": {
			input:      expr.Eq("active", false),
			wantStr:    "`active` = ?",
			wantParams: []any{false},
		},
		"string_range_inclusive_param": {
			input:      expr.Rang("a", "foo", "bar", true),
			wantStr:    "`a` BETWEEN ? AND ?",
			wantParams: []any{"foo", "bar"},
		},
		"string_range_exclusive_param": {
			input:      expr.Rang("a", "foo", "bar", false),
			wantStr:    "`a` > ? AND `a` < ?",
			wantParams: []any{"foo", "bar"},
		},
		"like_alternation_param": {
			input:      expr.LIKE("a", "*(b|d)*"),
			wantStr:    "`a` REGEXP ?",
			wantParams: []any{"^(.*(b|d).*)$"},
		},
		"like_plain_wildcard_param": {
			input:      expr.LIKE("a", "b*"),
			wantStr:    "`a` LIKE ? ESCAPE '#'",
			wantParams: []any{"b%"},
		},
	}

	for name, tc := range tcs {
		t.Run(name, func(t *testing.T) {
			gotStr, gotParams, err := NewMySQLDriver().RenderParam(tc.input)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if gotStr != tc.wantStr {
				t.Fatalf(errTemplate, "generated sql does not match", tc.wantStr, gotStr)
			}
			if len(gotParams) != len(tc.wantParams) {
				t.Fatalf("param count: want %d, got %d", len(tc.wantParams), len(gotParams))
			}
			for i := range gotParams {
				if gotParams[i] != tc.wantParams[i] {
					t.Fatalf("param[%d]: want %v (%T), got %v (%T)",
						i, tc.wantParams[i], tc.wantParams[i], gotParams[i], gotParams[i])
				}
			}
		})
	}
}
