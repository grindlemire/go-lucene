package lucene

import (
	"reflect"
	"strings"
	"testing"
)

// renderers lists every public render entry point, with the parameterized
// ones adapted to the same shape.
var renderers = map[string]func(string, ...Opt) (string, error){
	"postgres": ToPostgres,
	"mysql":    ToMySQL,
	"mariadb":  ToMariaDB,
	"sqlite":   ToSQLite,
	"param_postgres": func(in string, opts ...Opt) (string, error) {
		s, _, err := ToParameterizedPostgres(in, opts...)
		return s, err
	},
	"param_mysql": func(in string, opts ...Opt) (string, error) {
		s, _, err := ToParameterizedMySQL(in, opts...)
		return s, err
	},
	"param_mariadb": func(in string, opts ...Opt) (string, error) {
		s, _, err := ToParameterizedMariaDB(in, opts...)
		return s, err
	},
	"param_sqlite": func(in string, opts ...Opt) (string, error) {
		s, _, err := ToParameterizedSQLite(in, opts...)
		return s, err
	},
}

// TestRangeBoundInjectionErrors is the end-to-end check for issue #68. The
// first input used to render as
//
//	"a" BETWEEN "\" AND 1) OR 1=1 --" AND 'q'
//
// on Postgres and SQLite: the bound was the AND of two terms, and its left
// half was printed as Lucene text, unescaped. The parameterized renderers
// bound the *expr.Expression itself as a parameter.
func TestRangeBoundInjectionErrors(t *testing.T) {
	inputs := map[string]string{
		"double_quote_breakout": `a:["\" AND 1) OR 1=1 --" z TO q]`,
		"single_quote_breakout": `a:['\' AND 1) OR 1=1 --' z TO q]`,
		"breakout_in_max":       `a:[q TO ("\" AND 1) OR 1=1 --" z)]`,
		"field_in_bound":        `a:[1 TO ("x\" OR 1=1 --":c)]`,
	}
	for name, input := range inputs {
		for rname, render := range renderers {
			for variant, opts := range map[string][]Opt{"no_default": nil, "default_field": {WithDefaultField("default")}} {
				t.Run(name+"/"+rname+"/"+variant, func(t *testing.T) {
					got, err := render(input, opts...)
					if err == nil {
						t.Fatalf("expected an error for %q, got SQL %q", input, got)
					}
					if !strings.Contains(err.Error(), "must be a single term") {
						t.Fatalf("expected a single-term range bound error for %q, got: %v", input, err)
					}
				})
			}
		}
	}
}

// TestRenderEscapedWildcardChars is the end-to-end check for escaped
// wildcard characters (issue #77). An escaped \* used to stay a wildcard:
// a:\*x* rendered "a" SIMILAR TO '\%x%' and a:fo\* rendered a LIKE.
func TestRenderEscapedWildcardChars(t *testing.T) {
	tcs := map[string]struct {
		input string
		want  map[string]string
	}{
		"only_escaped_star": {
			input: `a:fo\*`,
			want: map[string]string{
				"postgres": `"a" = 'fo*'`,
				"mysql":    "`a` = 'fo*'",
				"sqlite":   `"a" = 'fo*'`,
			},
		},
		"only_escaped_question": {
			input: `a:fo\?`,
			want: map[string]string{
				"postgres": `"a" = 'fo?'`,
				"mysql":    "`a` = 'fo?'",
				"sqlite":   `"a" = 'fo?'`,
			},
		},
		"escaped_star_in_wildcard": {
			input: `a:\*x*`,
			want: map[string]string{
				"postgres": `"a" SIMILAR TO '\*x%'`,
				"mysql":    "`a` LIKE '*x%' ESCAPE '#'",
				"sqlite":   `"a" GLOB '[*]x*'`,
			},
		},
		"escaped_question_in_wildcard": {
			input: `a:x\?*`,
			want: map[string]string{
				"postgres": `"a" SIMILAR TO 'x\?%'`,
				"mysql":    "`a` LIKE 'x?%' ESCAPE '#'",
				"sqlite":   `"a" GLOB 'x[?]*'`,
			},
		},
		"escaped_bracket_in_wildcard": {
			input: `a:x\[*`,
			want: map[string]string{
				"postgres": `"a" SIMILAR TO 'x\[%'`,
				"mysql":    "`a` LIKE 'x[%' ESCAPE '#'",
				"sqlite":   `"a" GLOB 'x[[]*'`,
			},
		},
		// The + of an exponent is a literal +. It used to reach SIMILAR TO
		// bare, where it means "one or more", and sent MySQL to REGEXP.
		"exponent_plus_in_wildcard": {
			input: `a:1e+5*`,
			want: map[string]string{
				"postgres": `"a" SIMILAR TO '1e\+5%'`,
				"mysql":    "`a` LIKE '1e+5%' ESCAPE '#'",
				"sqlite":   `"a" GLOB '1e+5*'`,
			},
		},
		"escaped_space_in_wildcard": {
			input: `a:foo\ bar*`,
			want: map[string]string{
				"postgres": `"a" SIMILAR TO 'foo bar%'`,
				"mysql":    "`a` LIKE 'foo bar%' ESCAPE '#'",
				"sqlite":   `"a" GLOB 'foo bar*'`,
			},
		},
	}
	for name, tc := range tcs {
		for dname, want := range tc.want {
			t.Run(name+"/"+dname, func(t *testing.T) {
				got, err := renderers[dname](tc.input)
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if got != want {
					t.Fatalf("\nwant %s\ngot  %s", want, got)
				}
			})
		}
	}
}

// TestDefaultFieldReachesEveryTerm is the end-to-end check for the default
// field part of issue #77. With a default field, x OR null used to fail with
// "null cannot be rendered as a standalone value", x OR foo* rendered
// ("default" = 'x') OR 'foo*', and "fo*" parsed as a wildcard.
func TestDefaultFieldReachesEveryTerm(t *testing.T) {
	tcs := map[string]struct {
		input string
		want  map[string]string
	}{
		"or_null": {
			input: `x OR null`,
			want: map[string]string{
				"postgres":       `("default" = 'x') OR ("default" IS NULL)`,
				"mysql":          "(`default` = 'x') OR (`default` IS NULL)",
				"mariadb":        "(`default` = 'x') OR (`default` IS NULL)",
				"sqlite":         `("default" = 'x') OR ("default" IS NULL)`,
				"param_postgres": `("default" = $1) OR ("default" IS NULL)`,
				"param_mysql":    "(`default` = ?) OR (`default` IS NULL)",
				"param_mariadb":  "(`default` = ?) OR (`default` IS NULL)",
				"param_sqlite":   `("default" = ?) OR ("default" IS NULL)`,
			},
		},
		"not_null": {
			input: `x -null`,
			want: map[string]string{
				"postgres":       `("default" = 'x') AND ("default" IS NOT NULL)`,
				"mysql":          "(`default` = 'x') AND (`default` IS NOT NULL)",
				"sqlite":         `("default" = 'x') AND ("default" IS NOT NULL)`,
				"param_postgres": `("default" = $1) AND ("default" IS NOT NULL)`,
			},
		},
		"or_wildcard": {
			input: `x OR foo*`,
			want: map[string]string{
				"postgres": `("default" = 'x') OR ("default" SIMILAR TO 'foo%')`,
				"mysql":    "(`default` = 'x') OR (`default` LIKE 'foo%' ESCAPE '#')",
				"mariadb":  "(`default` = 'x') OR (`default` LIKE 'foo%' ESCAPE '#')",
				"sqlite":   `("default" = 'x') OR ("default" GLOB 'foo*')`,
			},
		},
		"wildcard": {
			input: `foo*`,
			want: map[string]string{
				"postgres": `"default" SIMILAR TO 'foo%'`,
				"mysql":    "`default` LIKE 'foo%' ESCAPE '#'",
				"sqlite":   `"default" GLOB 'foo*'`,
			},
		},
		"range": {
			input: `x OR [1 TO 5]`,
			want: map[string]string{
				"postgres":       `("default" = 'x') OR ("default" >= 1 AND "default" <= 5)`,
				"mysql":          "(`default` = 'x') OR (`default` >= 1 AND `default` <= 5)",
				"sqlite":         `("default" = 'x') OR ("default" >= 1 AND "default" <= 5)`,
				"param_postgres": `("default" = $1) OR ("default" >= $2 AND "default" <= $3)`,
				"param_mysql":    "(`default` = ?) OR (`default` >= ? AND `default` <= ?)",
				"param_sqlite":   `("default" = ?) OR ("default" >= ? AND "default" <= ?)`,
			},
		},
		"quoted_star": {
			input: `"fo*"`,
			want: map[string]string{
				"postgres":       `"default" = 'fo*'`,
				"mysql":          "`default` = 'fo*'",
				"sqlite":         `"default" = 'fo*'`,
				"param_postgres": `"default" = $1`,
				"param_mysql":    "`default` = ?",
				"param_sqlite":   `"default" = ?`,
			},
		},
	}

	for name, tc := range tcs {
		for rname, want := range tc.want {
			t.Run(name+"/"+rname, func(t *testing.T) {
				got, err := renderers[rname](tc.input, WithDefaultField("default"))
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if got != want {
					t.Fatalf("\nwant %s\ngot  %s", want, got)
				}
			})
		}
	}
}

// TestDefaultFieldInsideFieldGroup is the end-to-end check for issue #67. A
// field group like a:(x OR y) binds field a to every bare term inside the
// parentheses, so the default field must not be applied to those terms. With
// a default field, a:(x OR null) used to fail with "null cannot be rendered
// as a standalone value", and a:(x OR y) rendered as
// "a" = (("default" = 'x') OR ("default" = 'y')).
func TestDefaultFieldInsideFieldGroup(t *testing.T) {
	tcs := map[string]struct {
		input string
		want  map[string]string
	}{
		"group_with_null": {
			input: `a:(x OR null)`,
			want: map[string]string{
				"postgres":       `("a" = 'x' OR "a" IS NULL)`,
				"mysql":          "(`a` = 'x' OR `a` IS NULL)",
				"mariadb":        "(`a` = 'x' OR `a` IS NULL)",
				"sqlite":         `("a" = 'x' OR "a" IS NULL)`,
				"param_postgres": `("a" = $1 OR "a" IS NULL)`,
				"param_mysql":    "(`a` = ? OR `a` IS NULL)",
				"param_mariadb":  "(`a` = ? OR `a` IS NULL)",
				"param_sqlite":   `("a" = ? OR "a" IS NULL)`,
			},
		},
		"group_of_terms": {
			input: `a:(x OR y)`,
			want: map[string]string{
				"postgres":       `"a" IN ('x', 'y')`,
				"mysql":          "`a` IN ('x', 'y')",
				"sqlite":         `"a" IN ('x', 'y')`,
				"param_postgres": `"a" IN ($1, $2)`,
				"param_mysql":    "`a` IN (?, ?)",
				"param_sqlite":   `"a" IN (?, ?)`,
			},
		},
		"group_with_phrase": {
			input: `a:("foo bar" OR baz)`,
			want: map[string]string{
				"postgres":       `"a" IN ('foo bar', 'baz')`,
				"mysql":          "`a` IN ('foo bar', 'baz')",
				"sqlite":         `"a" IN ('foo bar', 'baz')`,
				"param_postgres": `"a" IN ($1, $2)`,
			},
		},
		"default_still_applies_outside_group": {
			input: `a:(x OR null) AND z`,
			want: map[string]string{
				"postgres":       `(("a" = 'x' OR "a" IS NULL)) AND ("default" = 'z')`,
				"mysql":          "((`a` = 'x' OR `a` IS NULL)) AND (`default` = 'z')",
				"sqlite":         `(("a" = 'x' OR "a" IS NULL)) AND ("default" = 'z')`,
				"param_postgres": `(("a" = $1 OR "a" IS NULL)) AND ("default" = $2)`,
				"param_mysql":    "((`a` = ? OR `a` IS NULL)) AND (`default` = ?)",
			},
		},
		"bare_group_gets_default": {
			input: `(x OR y)`,
			want: map[string]string{
				"postgres":       `("default" = 'x') OR ("default" = 'y')`,
				"mysql":          "(`default` = 'x') OR (`default` = 'y')",
				"sqlite":         `("default" = 'x') OR ("default" = 'y')`,
				"param_postgres": `("default" = $1) OR ("default" = $2)`,
			},
		},
		"must_not_group": {
			input: `-a:(x OR y)`,
			want: map[string]string{
				"postgres":       `NOT("a" IN ('x', 'y'))`,
				"mysql":          "NOT(`a` IN ('x', 'y'))",
				"sqlite":         `NOT("a" IN ('x', 'y'))`,
				"param_postgres": `NOT("a" IN ($1, $2))`,
			},
		},
		"must_group": {
			input: `+a:(x OR null)`,
			want: map[string]string{
				"postgres":       `("a" = 'x' OR "a" IS NULL)`,
				"mysql":          "(`a` = 'x' OR `a` IS NULL)",
				"param_postgres": `("a" = $1 OR "a" IS NULL)`,
			},
		},
	}

	for name, tc := range tcs {
		for rname, want := range tc.want {
			t.Run(name+"/"+rname, func(t *testing.T) {
				got, err := renderers[rname](tc.input, WithDefaultField("default"))
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if got != want {
					t.Fatalf("\nwant %s\ngot  %s", want, got)
				}
			})
		}
	}
}

// TestFieldGroupAppliesFieldToEachTerm is the end-to-end check for the field
// group part of issue #77. Only a plain OR of terms used to get the group's
// field, so a:(x AND y) rendered as "a" = ('x' AND 'y'), and
// a:(x OR [1 TO 5]) failed to parse.
func TestFieldGroupAppliesFieldToEachTerm(t *testing.T) {
	tcs := map[string]struct {
		input string
		want  map[string]string
	}{
		"and": {
			input: `a:(x AND y)`,
			want: map[string]string{
				"postgres":       `("a" = 'x') AND ("a" = 'y')`,
				"mysql":          "(`a` = 'x') AND (`a` = 'y')",
				"mariadb":        "(`a` = 'x') AND (`a` = 'y')",
				"sqlite":         `("a" = 'x') AND ("a" = 'y')`,
				"param_postgres": `("a" = $1) AND ("a" = $2)`,
				"param_mysql":    "(`a` = ?) AND (`a` = ?)",
				"param_mariadb":  "(`a` = ?) AND (`a` = ?)",
				"param_sqlite":   `("a" = ?) AND ("a" = ?)`,
			},
		},
		"must_not": {
			input: `a:(x -y)`,
			want: map[string]string{
				"postgres":       `("a" = 'x') AND (NOT("a" = 'y'))`,
				"mysql":          "(`a` = 'x') AND (NOT(`a` = 'y'))",
				"sqlite":         `("a" = 'x') AND (NOT("a" = 'y'))`,
				"param_postgres": `("a" = $1) AND (NOT("a" = $2))`,
				"param_mysql":    "(`a` = ?) AND (NOT(`a` = ?))",
				"param_sqlite":   `("a" = ?) AND (NOT("a" = ?))`,
			},
		},
		"wildcard": {
			input: `a:(x OR foo*)`,
			want: map[string]string{
				"postgres": `("a" = 'x') OR ("a" SIMILAR TO 'foo%')`,
				"mysql":    "(`a` = 'x') OR (`a` LIKE 'foo%' ESCAPE '#')",
				"sqlite":   `("a" = 'x') OR ("a" GLOB 'foo*')`,
			},
		},
		"range": {
			input: `a:(x OR [1 TO 5])`,
			want: map[string]string{
				"postgres":       `("a" = 'x') OR ("a" >= 1 AND "a" <= 5)`,
				"mysql":          "(`a` = 'x') OR (`a` >= 1 AND `a` <= 5)",
				"mariadb":        "(`a` = 'x') OR (`a` >= 1 AND `a` <= 5)",
				"sqlite":         `("a" = 'x') OR ("a" >= 1 AND "a" <= 5)`,
				"param_postgres": `("a" = $1) OR ("a" >= $2 AND "a" <= $3)`,
				"param_mysql":    "(`a` = ?) OR (`a` >= ? AND `a` <= ?)",
				"param_sqlite":   `("a" = ?) OR ("a" >= ? AND "a" <= ?)`,
			},
		},
		"null": {
			input: `a:(x AND null)`,
			want: map[string]string{
				"postgres":       `("a" = 'x') AND ("a" IS NULL)`,
				"mysql":          "(`a` = 'x') AND (`a` IS NULL)",
				"sqlite":         `("a" = 'x') AND ("a" IS NULL)`,
				"param_postgres": `("a" = $1) AND ("a" IS NULL)`,
			},
		},
		"quote_in_term": {
			input: `a:("x' OR 1=1 --" AND c)`,
			want: map[string]string{
				"postgres":       `("a" = 'x'' OR 1=1 --') AND ("a" = 'c')`,
				"mysql":          "(`a` = 'x'' OR 1=1 --') AND (`a` = 'c')",
				"sqlite":         `("a" = 'x'' OR 1=1 --') AND ("a" = 'c')`,
				"param_postgres": `("a" = $1) AND ("a" = $2)`,
			},
		},
		// Unchanged: a plain OR of terms is still an IN list.
		"or_of_terms": {
			input: `a:(x OR y)`,
			want: map[string]string{
				"postgres":       `"a" IN ('x', 'y')`,
				"param_postgres": `"a" IN ($1, $2)`,
			},
		},
	}

	for name, tc := range tcs {
		for rname, want := range tc.want {
			for variant, opts := range map[string][]Opt{"no_default": nil, "default_field": {WithDefaultField("default")}} {
				t.Run(name+"/"+rname+"/"+variant, func(t *testing.T) {
					got, err := renderers[rname](tc.input, opts...)
					if err != nil {
						t.Fatalf("unexpected error: %v", err)
					}
					if got != want {
						t.Fatalf("\nwant %s\ngot  %s", want, got)
					}
				})
			}
		}
	}
}

// TestCompareGroupRenderError checks that every renderer refuses a
// comparison on a group. a:>(x OR y) used to render "a" > ('x' OR 'y').
func TestCompareGroupRenderError(t *testing.T) {
	for rname, render := range renderers {
		t.Run(rname, func(t *testing.T) {
			got, err := render(`a:>("x' OR 1=1 --" OR c)`)
			if err == nil {
				t.Fatalf("expected an error, got SQL %q", got)
			}
			if !strings.Contains(err.Error(), "must be a single term") {
				t.Fatalf("expected a single-term error, got: %v", err)
			}
		})
	}
}

// TestRangeWildcardBound checks that only a bare * leaves a side of a range
// open. Any other wildcard bound is literal text, as in Lucene. a:[ab* TO c]
// used to drop its minimum and render as "a" <= 'c'.
func TestRangeWildcardBound(t *testing.T) {
	tcs := map[string]struct {
		input  string
		want   map[string]string
		params []any
	}{
		"wildcard_bound_is_literal": {
			input: `a:[ab* TO c]`,
			want: map[string]string{
				"postgres":       `"a" BETWEEN 'ab*' AND 'c'`,
				"mysql":          "`a` BETWEEN 'ab*' AND 'c'",
				"sqlite":         `"a" BETWEEN 'ab*' AND 'c'`,
				"param_postgres": `"a" BETWEEN $1 AND $2`,
				"param_mysql":    "`a` BETWEEN ? AND ?",
			},
			params: []any{"ab*", "c"},
		},
		"wildcard_max_exclusive": {
			input: `a:{a TO b?}`,
			want: map[string]string{
				"postgres":       `"a" > 'a' AND "a" < 'b?'`,
				"param_postgres": `"a" > $1 AND "a" < $2`,
			},
			params: []any{"a", "b?"},
		},
		"open_min": {
			input: `a:[* TO b]`,
			want: map[string]string{
				"postgres":       `"a" <= 'b'`,
				"param_postgres": `"a" <= $1`,
			},
			params: []any{"b"},
		},
		"open_max_exclusive": {
			input: `a:{b TO *}`,
			want: map[string]string{
				"postgres":       `"a" > 'b'`,
				"param_postgres": `"a" > $1`,
			},
			params: []any{"b"},
		},
		"fully_open": {
			input: `a:{* TO *}`,
			want: map[string]string{
				"postgres":       `1=1`,
				"param_postgres": `1=1`,
			},
		},
	}
	for name, tc := range tcs {
		for rname, want := range tc.want {
			t.Run(name+"/"+rname, func(t *testing.T) {
				got, err := renderers[rname](tc.input)
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if got != want {
					t.Fatalf("\nwant %s\ngot  %s", want, got)
				}
			})
		}
		t.Run(name+"/params", func(t *testing.T) {
			_, params, err := ToParameterizedPostgres(tc.input)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(params) != len(tc.params) || (len(params) > 0 && !reflect.DeepEqual(params, tc.params)) {
				t.Fatalf("want params %#v, got %#v", tc.params, params)
			}
		})
	}
}

// TestNumericFieldRendersAsColumn checks that a numeric-looking field name is
// quoted as an identifier in every dialect and never bound as a parameter.
// 0:1 used to render as `0 = 1`, and `$1 = $2` with params [0 1].
func TestNumericFieldRendersAsColumn(t *testing.T) {
	tcs := map[string]struct {
		input        string
		defaultField string
		want         map[string]string
	}{
		"numeric_field": {
			input: `0:1`,
			want: map[string]string{
				"postgres":       `"0" = 1`,
				"mysql":          "`0` = 1",
				"sqlite":         `"0" = 1`,
				"param_postgres": `"0" = $1`,
				"param_mysql":    "`0` = ?",
				"param_sqlite":   `"0" = ?`,
			},
		},
		"numeric_field_range": {
			input: `0:[1 TO 2]`,
			want: map[string]string{
				"postgres":       `"0" >= 1 AND "0" <= 2`,
				"mysql":          "`0` >= 1 AND `0` <= 2",
				"param_postgres": `"0" >= $1 AND "0" <= $2`,
			},
		},
		"numeric_field_compare": {
			input: `1.5:>2`,
			want: map[string]string{
				"postgres":       `"1.5" > 2`,
				"param_postgres": `"1.5" > $1`,
			},
		},
		"numeric_field_in": {
			input: `0:(1 OR 2)`,
			want: map[string]string{
				"postgres":       `"0" IN (1, 2)`,
				"param_postgres": `"0" IN ($1, $2)`,
			},
		},
		"numeric_field_wildcard": {
			input: `0:1*`,
			want: map[string]string{
				"postgres": `"0" SIMILAR TO '1%'`,
				"mysql":    "`0` LIKE '1%' ESCAPE '#'",
				"sqlite":   `"0" GLOB '1*'`,
			},
		},
		"numeric_field_default_field": {
			input:        `0:1 2`,
			defaultField: "df",
			want: map[string]string{
				"postgres":       `("0" = 1) AND ("df" = 2)`,
				"param_postgres": `("0" = $1) AND ("df" = $2)`,
			},
		},
		// 1=1 used to render 1 = 1, which matches every row.
		"numeric_field_equals_sign": {
			input: `1=1`,
			want: map[string]string{
				"postgres":       `"1" = 1`,
				"mysql":          "`1` = 1",
				"mariadb":        "`1` = 1",
				"sqlite":         `"1" = 1`,
				"param_postgres": `"1" = $1`,
				"param_mysql":    "`1` = ?",
				"param_mariadb":  "`1` = ?",
				"param_sqlite":   `"1" = ?`,
			},
		},
		"numeric_field_in_parens": {
			input: `(0):*`,
			want: map[string]string{
				"postgres":       `"0" SIMILAR TO '%'`,
				"mysql":          "`0` IS NOT NULL",
				"sqlite":         `"0" IS NOT NULL`,
				"param_postgres": `"0" SIMILAR TO '%'`,
				"param_mysql":    "`0` IS NOT NULL",
				"param_sqlite":   `"0" IS NOT NULL`,
			},
		},
		"numeric_field_equals_or": {
			input: `a:x OR 1=1`,
			want: map[string]string{
				"postgres":       `("a" = 'x') OR ("1" = 1)`,
				"mysql":          "(`a` = 'x') OR (`1` = 1)",
				"sqlite":         `("a" = 'x') OR ("1" = 1)`,
				"param_postgres": `("a" = $1) OR ("1" = $2)`,
			},
		},
	}
	for name, tc := range tcs {
		for rname, want := range tc.want {
			t.Run(name+"/"+rname, func(t *testing.T) {
				var opts []Opt
				if tc.defaultField != "" {
					opts = append(opts, WithDefaultField(tc.defaultField))
				}
				got, err := renderers[rname](tc.input, opts...)
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if got != want {
					t.Fatalf("\nwant %s\ngot  %s", want, got)
				}
			})
		}
	}

	// The field name must never travel as a bind value.
	_, params, err := ToParameterizedPostgres(`0:[1 TO 2] AND 7:>3 AND 8=4 AND (9):5`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if want := []any{1, 2, 3, 4, 5}; !reflect.DeepEqual(params, want) {
		t.Fatalf("want params %#v, got %#v", want, params)
	}
}

// TestNumberSpellingsRender is the end-to-end check for the lexer part of
// issue #77. a:1E+3 used to render ("a" = '1E') AND (3), and a:.5 failed to
// parse.
func TestNumberSpellingsRender(t *testing.T) {
	tcs := map[string]struct {
		input  string
		want   map[string]string
		params []any
	}{
		"exponent_plus": {
			input: `a:1E+3`,
			want: map[string]string{
				"postgres":       `"a" = 1000`,
				"mysql":          "`a` = 1000",
				"mariadb":        "`a` = 1000",
				"sqlite":         `"a" = 1000`,
				"param_postgres": `"a" = $1`,
				"param_mysql":    "`a` = ?",
				"param_mariadb":  "`a` = ?",
				"param_sqlite":   `"a" = ?`,
			},
			params: []any{1000.0},
		},
		"leading_dot_range": {
			input: `a:[-.5 TO .5]`,
			want: map[string]string{
				"postgres":       `"a" >= -0.5 AND "a" <= 0.5`,
				"mysql":          "`a` >= -0.5 AND `a` <= 0.5",
				"sqlite":         `"a" >= -0.5 AND "a" <= 0.5`,
				"param_postgres": `"a" >= $1 AND "a" <= $2`,
				"param_mysql":    "`a` >= ? AND `a` <= ?",
				"param_sqlite":   `"a" >= ? AND "a" <= ?`,
			},
			params: []any{-0.5, 0.5},
		},
	}

	for name, tc := range tcs {
		for rname, want := range tc.want {
			t.Run(name+"/"+rname, func(t *testing.T) {
				got, err := renderers[rname](tc.input)
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if got != want {
					t.Fatalf("\nwant %s\ngot  %s", want, got)
				}
			})
		}
		t.Run(name+"/params", func(t *testing.T) {
			_, params, err := ToParameterizedPostgres(tc.input)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !reflect.DeepEqual(params, tc.params) {
				t.Fatalf("want params %#v, got %#v", tc.params, params)
			}
		})
	}
}

// TestNonFiniteWordsRenderAsStrings checks that inf and NaN reach the SQL as
// quoted strings. They used to parse as floats and render as a bare +Inf or
// NaN, which SQL reads as a column reference.
func TestNonFiniteWordsRenderAsStrings(t *testing.T) {
	tcs := map[string]struct {
		input string
		want  map[string]string
	}{
		"inf_equals": {
			input: `a:inf`,
			want: map[string]string{
				"postgres": `"a" = 'inf'`,
				"mysql":    "`a` = 'inf'",
				"sqlite":   `"a" = 'inf'`,
			},
		},
		"nan_range": {
			input: `a:[NaN TO 1]`,
			want: map[string]string{
				"postgres": `"a" >= 'NaN' AND "a" <= 1`,
				"mysql":    "`a` >= 'NaN' AND `a` <= 1",
				"sqlite":   `"a" >= 'NaN' AND "a" <= 1`,
			},
		},
		"inf_field": {
			input: `inf:1`,
			want: map[string]string{
				"postgres": `"inf" = 1`,
				"mysql":    "`inf` = 1",
				"sqlite":   `"inf" = 1`,
			},
		},
	}
	for name, tc := range tcs {
		for rname, want := range tc.want {
			t.Run(name+"/"+rname, func(t *testing.T) {
				got, err := renderers[rname](tc.input)
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if got != want {
					t.Fatalf("\nwant %s\ngot  %s", want, got)
				}
			})
		}
	}
}

// TestNumbersThatLoseDigitsRenderAsWritten is the end-to-end check for numbers
// that a float64 can't hold, from issue #77. a:9223372036854775808 used to
// render "a" = 9.223372036854776e+18 and bind that float64, so the database
// compared against 9223372036854776000. Such a number now reaches the SQL as
// written, in both modes. No Go value the drivers bind holds it, so
// RenderParam writes it into the SQL rather than binding it.
func TestNumbersThatLoseDigitsRenderAsWritten(t *testing.T) {
	tcs := map[string]struct {
		input  string
		want   map[string]string
		params []any
	}{
		"int_overflow": {
			input: `a:9223372036854775808`,
			want:  inlineSQL(`"a" = 9223372036854775808`),
		},
		"negative_int_overflow": {
			input: `a:-9223372036854775809`,
			want:  inlineSQL(`"a" = -9223372036854775809`),
		},
		"too_many_digits": {
			input: `a:0.1000000000000000055511151231257827`,
			want:  inlineSQL(`"a" = 0.1000000000000000055511151231257827`),
		},
		"comparison": {
			input: `a:>=12345678901234567891`,
			want:  inlineSQL(`"a" >= 12345678901234567891`),
		},
		"range": {
			input: `a:[9223372036854775808 TO 12345678901234567891]`,
			want:  inlineSQL(`"a" >= 9223372036854775808 AND "a" <= 12345678901234567891`),
		},
		"open_range": {
			input: `a:{* TO 9223372036854775808}`,
			want:  inlineSQL(`"a" < 9223372036854775808`),
		},
		"range_with_param": {
			input: `a:[1 TO 9223372036854775808]`,
			want: map[string]string{
				"postgres":       `"a" >= 1 AND "a" <= 9223372036854775808`,
				"sqlite":         `"a" >= 1 AND "a" <= 9223372036854775808`,
				"mysql":          "`a` >= 1 AND `a` <= 9223372036854775808",
				"param_postgres": `"a" >= $1 AND "a" <= 9223372036854775808`,
				"param_sqlite":   `"a" >= ? AND "a" <= 9223372036854775808`,
				"param_mysql":    "`a` >= ? AND `a` <= 9223372036854775808",
			},
			params: []any{1},
		},
		"in_list": {
			input: `a:(9223372036854775808 OR 2)`,
			want: map[string]string{
				"postgres":       `"a" IN (9223372036854775808, 2)`,
				"sqlite":         `"a" IN (9223372036854775808, 2)`,
				"mysql":          "`a` IN (9223372036854775808, 2)",
				"param_postgres": `"a" IN (9223372036854775808, $1)`,
				"param_sqlite":   `"a" IN (9223372036854775808, ?)`,
				"param_mysql":    "`a` IN (9223372036854775808, ?)",
			},
			params: []any{2},
		},
		"placeholders_stay_numbered": {
			input: `a:1 AND b:9223372036854775808 AND c:2`,
			want: map[string]string{
				"postgres":       `(("a" = 1) AND ("b" = 9223372036854775808)) AND ("c" = 2)`,
				"param_postgres": `(("a" = $1) AND ("b" = 9223372036854775808)) AND ("c" = $2)`,
			},
			params: []any{1, 2},
		},
		// 1e20 is exactly a float64, so it binds as before.
		"exact_float_bound": {
			input: `a:[9223372036854775808 TO 1e20]`,
			want: map[string]string{
				"postgres":       `"a" >= 9223372036854775808 AND "a" <= 100000000000000000000`,
				"param_postgres": `"a" >= 9223372036854775808 AND "a" <= $1`,
			},
			params: []any{1e20},
		},
		"default_field": {
			input: `9223372036854775808`,
			want:  inlineSQL(`"f" = 9223372036854775808`),
		},
	}
	for name, tc := range tcs {
		opts := []Opt{WithDefaultField("f")}
		for rname, want := range tc.want {
			t.Run(name+"/"+rname, func(t *testing.T) {
				got, err := renderers[rname](tc.input, opts...)
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if got != want {
					t.Fatalf("\nwant %s\ngot  %s", want, got)
				}
			})
		}
		for rname, render := range paramRenderers {
			t.Run(name+"/"+rname+"_params", func(t *testing.T) {
				_, params, err := render(tc.input, opts...)
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if len(params) != len(tc.params) || (len(params) > 0 && !reflect.DeepEqual(params, tc.params)) {
					t.Fatalf("want params %#v, got %#v", tc.params, params)
				}
			})
		}
	}
}

// inlineSQL gives the same SQL for every renderer in both modes, for SQL with
// no bind parameters where MySQL and MariaDB differ only in quoting columns
// with backticks.
func inlineSQL(postgres string) map[string]string {
	want := sameSQL(postgres)
	for name, sql := range sameSQL(postgres) {
		want["param_"+name] = sql
	}
	return want
}

// paramRenderers lists the parameterized render entry points with their
// bind parameters.
var paramRenderers = map[string]func(string, ...Opt) (string, []any, error){
	"param_postgres": ToParameterizedPostgres,
	"param_mysql":    ToParameterizedMySQL,
	"param_mariadb":  ToParameterizedMariaDB,
	"param_sqlite":   ToParameterizedSQLite,
}

// checkNumberRendering renders input with every dialect, in both modes. want
// maps the non-parameterized renderers to their SQL; every parameterized
// renderer must bind wantParams.
func checkNumberRendering(t *testing.T, input string, want map[string]string, wantParams []any, opts ...Opt) {
	t.Helper()
	for rname, wantSQL := range want {
		got, err := renderers[rname](input, opts...)
		if err != nil {
			t.Errorf("%s: unexpected error: %v", rname, err)
			continue
		}
		if got != wantSQL {
			t.Errorf("%s:\nwant %s\ngot  %s", rname, wantSQL, got)
		}
	}
	for rname, render := range paramRenderers {
		_, params, err := render(input, opts...)
		if err != nil {
			t.Errorf("%s: unexpected error: %v", rname, err)
			continue
		}
		if !reflect.DeepEqual(params, wantParams) {
			t.Errorf("%s:\nwant params %#v\ngot  params %#v", rname, wantParams, params)
		}
	}
}

// sameSQL gives the expected SQL for each non-parameterized renderer, for SQL
// where Postgres and SQLite agree and MySQL and MariaDB differ only in quoting
// columns with backticks.
func sameSQL(postgres string) map[string]string {
	mysql := strings.ReplaceAll(postgres, `"`, "`")
	return map[string]string{
		"postgres": postgres,
		"sqlite":   postgres,
		"mysql":    mysql,
		"mariadb":  mysql,
	}
}

// TestNonDecimalNumbersRenderAsStrings is the end-to-end check for the rest of
// issue #69: hex floats and underscore digit separators used to parse as
// numbers, so a:0x1p3 rendered "a" = 8 and a:1_000 rendered "a" = 1000. They
// now reach the SQL as the text the query wrote.
func TestNonDecimalNumbersRenderAsStrings(t *testing.T) {
	tcs := map[string]struct {
		input      string
		want       map[string]string
		wantParams []any
	}{
		"hex_float":          {`a:0x1p3`, sameSQL(`"a" = '0x1p3'`), []any{"0x1p3"}},
		"negative_hex_float": {`a:-0x1p-2`, sameSQL(`"a" = '-0x1p-2'`), []any{"-0x1p-2"}},
		"digit_separator":    {`a:1_000`, sameSQL(`"a" = '1_000'`), []any{"1_000"}},
		"range_bound":        {`a:[1 TO 1_000]`, sameSQL(`"a" >= 1 AND "a" <= '1_000'`), []any{1, "1_000"}},
		"comparison":         {`a:>0x1p3`, sameSQL(`"a" > '0x1p3'`), []any{"0x1p3"}},
		"in_list":            {`a:(1_000 OR 2)`, sameSQL(`"a" IN ('1_000', 2)`), []any{"1_000", 2}},
	}
	for name, tc := range tcs {
		t.Run(name, func(t *testing.T) {
			checkNumberRendering(t, tc.input, tc.want, tc.wantParams)
		})
	}

	t.Run("default_field", func(t *testing.T) {
		checkNumberRendering(t, `0x1p3`, sameSQL(`"f" = '0x1p3'`), []any{"0x1p3"}, WithDefaultField("f"))
	})
}

// TestPlusInsideTermRenders checks that a + inside a term stays in it. a:be+3
// used to parse as a:be AND +3 and render ("a" = 'be') AND (3), and a:c++
// failed to parse. 1+2 is text, not a number.
func TestPlusInsideTermRenders(t *testing.T) {
	tcs := map[string]struct {
		input      string
		want       map[string]string
		wantParams []any
	}{
		"after_word":   {`a:be+3`, sameSQL(`"a" = 'be+3'`), []any{"be+3"}},
		"after_number": {`a:1+2`, sameSQL(`"a" = '1+2'`), []any{"1+2"}},
		"at_end":       {`a:c++`, sameSQL(`"a" = 'c++'`), []any{"c++"}},
		"in_field":     {`a+b:c`, sameSQL(`"a+b" = 'c'`), []any{"c"}},
		"leading_plus": {`a:b +c:d`, sameSQL(`("a" = 'b') AND ("c" = 'd')`), []any{"b", "d"}},
	}
	for name, tc := range tcs {
		t.Run(name, func(t *testing.T) {
			checkNumberRendering(t, tc.input, tc.want, tc.wantParams)
		})
	}
}

// TestPlusInsideWildcardIsLiteral checks that a + inside a wildcard term
// matches a +. The lexer keeps it in the term, and the parser escapes it, so
// Postgres SIMILAR TO and MySQL don't read it as "one or more". Before the
// parser escaped it, a:c++* rendered "a" SIMILAR TO 'c++%', which Postgres
// rejects, and MySQL sent the pattern to REGEXP.
func TestPlusInsideWildcardIsLiteral(t *testing.T) {
	tcs := map[string]struct {
		input  string
		want   map[string]string
		params map[string][]any
	}{
		"star_after_plus": {
			input: `a:c++*`,
			want: map[string]string{
				"postgres":       `"a" SIMILAR TO 'c\+\+%'`,
				"mysql":          "`a` LIKE 'c++%' ESCAPE '#'",
				"mariadb":        "`a` LIKE 'c++%' ESCAPE '#'",
				"sqlite":         `"a" GLOB 'c++*'`,
				"param_postgres": `"a" SIMILAR TO $1`,
				"param_mysql":    "`a` LIKE ? ESCAPE '#'",
				"param_mariadb":  "`a` LIKE ? ESCAPE '#'",
				"param_sqlite":   `"a" GLOB ?`,
			},
			params: map[string][]any{
				"param_postgres": {`c\+\+%`},
				"param_mysql":    {"c++%"},
				"param_mariadb":  {"c++%"},
				"param_sqlite":   {"c++*"},
			},
		},
		"question_after_plus": {
			input: `a:a+b?`,
			want: map[string]string{
				"postgres":       `"a" SIMILAR TO 'a\+b_'`,
				"mysql":          "`a` LIKE 'a+b_' ESCAPE '#'",
				"mariadb":        "`a` LIKE 'a+b_' ESCAPE '#'",
				"sqlite":         `"a" GLOB 'a+b?'`,
				"param_postgres": `"a" SIMILAR TO $1`,
				"param_mysql":    "`a` LIKE ? ESCAPE '#'",
				"param_mariadb":  "`a` LIKE ? ESCAPE '#'",
				"param_sqlite":   `"a" GLOB ?`,
			},
			params: map[string][]any{
				"param_postgres": {`a\+b_`},
				"param_mysql":    {"a+b_"},
				"param_mariadb":  {"a+b_"},
				"param_sqlite":   {"a+b?"},
			},
		},
		"plus_between_stars": {
			input: `a:*+*`,
			want: map[string]string{
				"postgres":       `"a" SIMILAR TO '%\+%'`,
				"mysql":          "`a` LIKE '%+%' ESCAPE '#'",
				"mariadb":        "`a` LIKE '%+%' ESCAPE '#'",
				"sqlite":         `"a" GLOB '*+*'`,
				"param_postgres": `"a" SIMILAR TO $1`,
				"param_mysql":    "`a` LIKE ? ESCAPE '#'",
				"param_mariadb":  "`a` LIKE ? ESCAPE '#'",
				"param_sqlite":   `"a" GLOB ?`,
			},
			params: map[string][]any{
				"param_postgres": {`%\+%`},
				"param_mysql":    {"%+%"},
				"param_mariadb":  {"%+%"},
				"param_sqlite":   {"*+*"},
			},
		},
	}
	for name, tc := range tcs {
		for rname, want := range tc.want {
			t.Run(name+"/"+rname, func(t *testing.T) {
				got, err := renderers[rname](tc.input)
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if got != want {
					t.Fatalf("\nwant %s\ngot  %s", want, got)
				}
			})
		}
		for rname, render := range paramRenderers {
			t.Run(name+"/"+rname+"_params", func(t *testing.T) {
				_, params, err := render(tc.input)
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if !reflect.DeepEqual(params, tc.params[rname]) {
					t.Fatalf("want params %#v, got %#v", tc.params[rname], params)
				}
			})
		}
	}

	// JSON paths and case-insensitive fields render the same pattern.
	for name, tc := range map[string]struct {
		input string
		opt   Opt
		want  map[string]string
	}{
		"json_path": {
			input: `labels.v:c++*`,
			opt:   WithJSONFields("labels"),
			want: map[string]string{
				"postgres": `"labels"->>'v' SIMILAR TO 'c\+\+%'`,
				"mysql":    "CASE WHEN JSON_TYPE(JSON_EXTRACT(`labels`, '$.v')) <> 'NULL' THEN JSON_UNQUOTE(JSON_EXTRACT(`labels`, '$.v')) END LIKE 'c++%' ESCAPE '#'",
				"sqlite":   `CASE json_type("labels", '$.v') WHEN 'true' THEN 'true' WHEN 'false' THEN 'false' ELSE CAST(json_extract("labels", '$.v') AS TEXT) END GLOB 'c++*'`,
			},
		},
		"case_insensitive": {
			input: `a:C++*`,
			opt:   WithCaseInsensitiveFields("a"),
			want: map[string]string{
				"postgres": `LOWER("a") SIMILAR TO LOWER('C\+\+%')`,
				"mysql":    "LOWER(`a`) LIKE LOWER('C++%') ESCAPE '#'",
				"sqlite":   `LOWER("a") GLOB LOWER('C++*')`,
			},
		},
	} {
		for rname, want := range tc.want {
			t.Run(name+"/"+rname, func(t *testing.T) {
				got, err := renderers[rname](tc.input, tc.opt)
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if got != want {
					t.Fatalf("\nwant %s\ngot  %s", want, got)
				}
			})
		}
	}
}

// TestDecimalNumberFormsRender pins the SQL for each spelling of a plain
// decimal number, in both render modes. Leading zeros are dropped, an exponent
// is folded into the value. A number that a float64 can't hold keeps its
// written text instead; TestNumbersThatLoseDigitsRenderAsWritten covers it.
func TestDecimalNumberFormsRender(t *testing.T) {
	tcs := map[string]struct {
		input      string
		want       map[string]string
		wantParams []any
	}{
		"negative_zero":  {`a:-0`, sameSQL(`"a" = 0`), []any{0}},
		"leading_zeros":  {`a:007`, sameSQL(`"a" = 7`), []any{7}},
		"trailing_dot":   {`a:5.`, sameSQL(`"a" = 5`), []any{5.0}},
		"exponent":       {`a:1e3`, sameSQL(`"a" = 1000`), []any{1000.0}},
		"upper_exponent": {`a:1E3`, sameSQL(`"a" = 1000`), []any{1000.0}},
		"fraction":       {`a:1.5e-3`, sameSQL(`"a" = 0.0015`), []any{0.0015}},
		"float_overflow": {`a:1e400`, sameSQL(`"a" = '1e400'`), []any{"1e400"}},
		"hex_int":        {`sku:0x1F`, sameSQL(`"sku" = '0x1F'`), []any{"0x1F"}},
		"escaped_digits": {`a:\007`, sameSQL(`"a" = '007'`), []any{"007"}},
		"range":          {`a:[007 TO 1e3]`, sameSQL(`"a" >= 7 AND "a" <= 1000`), []any{7, 1000.0}},

		// A nonzero number too small for a float64 used to render as 0.
		"float_underflow": {`a:1e-400`, sameSQL(`"a" = '1e-400'`), []any{"1e-400"}},
		"zero_exponent":   {`a:0e-400`, sameSQL(`"a" = 0`), []any{0.0}},
	}
	for name, tc := range tcs {
		t.Run(name, func(t *testing.T) {
			checkNumberRendering(t, tc.input, tc.want, tc.wantParams)
		})
	}
}
