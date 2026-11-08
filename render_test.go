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

// TestRenderKnownQuirksStaySafe pins inputs that render wrong SQL but keep
// every user-supplied term escaped or bound. They are separate bugs from
// issue #68 and are not fixed here; the pins show where the boundary is and
// should be updated when those bugs are fixed.
func TestRenderKnownQuirksStaySafe(t *testing.T) {
	tcs := map[string]struct {
		input        string
		defaultField string
		render       string
		want         string
		wantErr      bool
	}{
		// The $N rewrite also replaces a ? inside a quoted column name.
		"question_mark_in_column": {input: `a?b:c`, render: "param_postgres", want: `"a$1b" = $2`},
		// An escaped \* is meant to match a literal *, but becomes \%.
		"escaped_star_in_wildcard": {input: `a:\*x*`, render: "postgres", want: `"a" SIMILAR TO '\%x%'`},
		// A field group compares the column to a boolean expression.
		"field_group_and": {input: `a:("x' OR 1=1 --" AND c)`, render: "postgres", want: `"a" = ('x'' OR 1=1 --' AND 'c')`},
		"field_group_gt":  {input: `a:>("x' OR 1=1 --" OR c)`, render: "postgres", want: `"a" > ('x'' OR 1=1 --' OR 'c')`},
		// The default field only reaches a bare wildcard, regex or null when
		// it is the whole query, so here the wildcard stands alone...
		"default_field_skips_wildcard": {input: `x OR foo*`, defaultField: "default", render: "postgres", want: `("default" = 'x') OR 'foo*'`},
		// ...and the null fails to render.
		"default_field_skips_null": {input: `x OR null`, defaultField: "default", render: "postgres", wantErr: true},
	}

	for name, tc := range tcs {
		t.Run(name, func(t *testing.T) {
			var opts []Opt
			if tc.defaultField != "" {
				opts = append(opts, WithDefaultField(tc.defaultField))
			}
			got, err := renderers[tc.render](tc.input, opts...)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected an error, got SQL %q", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tc.want {
				t.Fatalf("\nwant %s\ngot  %s", tc.want, got)
			}
		})
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
	_, params, err := ToParameterizedPostgres(`0:[1 TO 2] AND 7:>3`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if want := []any{1, 2, 3}; !reflect.DeepEqual(params, want) {
		t.Fatalf("want params %#v, got %#v", want, params)
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
