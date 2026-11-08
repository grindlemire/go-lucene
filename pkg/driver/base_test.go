package driver

import (
	"encoding/json"
	"math"
	"strings"
	"testing"
	"time"

	"github.com/grindlemire/go-lucene/pkg/lucene/expr"
)

type renderer interface {
	Render(e *expr.Expression) (string, error)
	RenderParam(e *expr.Expression) (string, []any, error)
}

// allDrivers lists every built-in driver. MariaDB uses the MySQL driver.
var allDrivers = map[string]renderer{
	"postgres": NewPostgresDriver(),
	"mysql":    NewMySQLDriver(),
	"sqlite":   NewSQLiteDriver(),
}

func rangeOf(min, max any) *expr.Expression {
	return &expr.Expression{
		Left:  expr.Lit(expr.Column("a")),
		Op:    expr.Range,
		Right: &expr.RangeBoundary{Min: min, Max: max, Inclusive: true},
	}
}

func mustUnmarshal(t *testing.T, raw string) *expr.Expression {
	t.Helper()
	var e expr.Expression
	if err := json.Unmarshal([]byte(raw), &e); err != nil {
		t.Fatalf("unmarshal %s: %v", raw, err)
	}
	return &e
}

// TestRenderRejectsNonTermRangeBound covers ASTs that never went through
// Validate: hand-built ones and ones decoded from JSON. A range bound that is
// not a single term used to render its left half with %v, which wrote Lucene
// text straight into the SQL (issue #68).
func TestRenderRejectsNonTermRangeBound(t *testing.T) {
	bounds := map[string]*expr.Expression{
		"and_min":        rangeOf(expr.AND(expr.Lit(`" AND 1) OR 1=1 --`), expr.Lit("z")), expr.Lit("q")),
		"or_max":         rangeOf(expr.Lit(1), expr.OR(expr.Lit(3), expr.Lit(4))),
		"not_min":        rangeOf(expr.NOT(expr.Lit(1)), expr.Lit(3)),
		"must_min":       rangeOf(expr.MUST(expr.Lit(1)), expr.Lit(3)),
		"must_not_min":   rangeOf(expr.MUSTNOT(expr.Lit("x")), expr.Lit(3)),
		"equals_max":     rangeOf(expr.Lit(1), expr.Eq("b", "c")),
		"fuzzy_min":      rangeOf(expr.FUZZY(expr.Lit(1)), expr.Lit(3)),
		"boost_max":      rangeOf(expr.Lit(1), expr.BOOST(expr.Lit(3), 2.0)),
		"range_max":      rangeOf(expr.Lit(1), expr.Rang("b", 1, 2, true)),
		"list_max":       rangeOf(expr.Lit(1), expr.LIST(expr.Lit(3), expr.Lit(4))),
		"nested_literal": rangeOf(expr.Lit(expr.Lit(`" OR 1=1 --`)), expr.Lit("z")),
		"raw_expression": rangeOf(expr.Lit("x"), expr.Expression{Op: expr.Literal, Left: "z"}),
		"raw_map":        rangeOf(map[string]any{"x": "1') OR 1=1 --"}, 3),
		"raw_slice":      rangeOf([]any{"1') OR 1=1 --"}, 3),
		"helper_and":     expr.Rang("a", expr.AND(expr.Lit("x"), expr.Lit("y")), expr.Lit("z"), true),
		"helper_eq_max":  expr.Rang("a", 1, expr.Eq("b", "c"), false),
		"unbounded_and":  rangeOf(expr.WILD("*"), expr.AND(expr.Lit("x"), expr.Lit("y"))),
	}

	jsonBounds := map[string]string{
		"json_object_min": `{"left":"a","operator":"RANGE","right":{"min":{"x":"1') OR 1=1 --"},"max":2,"inclusive":true}}`,
		"json_array_max":  `{"left":"a","operator":"RANGE","right":{"min":1,"max":["1') OR 1=1 --"],"inclusive":true}}`,
		"json_null_min":   `{"left":"a","operator":"RANGE","right":{"min":null,"max":2,"inclusive":true}}`,
	}
	for name, raw := range jsonBounds {
		bounds[name] = mustUnmarshal(t, raw)
	}

	for name, e := range bounds {
		for dname, d := range allDrivers {
			t.Run(name+"/"+dname+"/render", func(t *testing.T) {
				if got, err := d.Render(e); err == nil {
					t.Fatalf("expected error rendering %#v, got SQL %q", e, got)
				}
			})
			t.Run(name+"/"+dname+"/render_param", func(t *testing.T) {
				if got, params, err := d.RenderParam(e); err == nil {
					t.Fatalf("expected error rendering %#v, got SQL %q with params %#v", e, got, params)
				}
			})
		}
	}
}

// TestRenderRejectsUnsupportedValues covers values that serialize used to
// print with %v: AST nodes in a value position, JSON containers, Go types
// with no SQL literal form, and non-finite floats.
func TestRenderRejectsUnsupportedValues(t *testing.T) {
	type stringer struct{ S string }

	// Rejected by both Render and RenderParam: AST pieces in a value slot.
	// encoding/json produces these when a "right" object has min/max keys.
	both := map[string]*expr.Expression{
		"json_equals_boundary":  mustUnmarshal(t, `{"left":"a","operator":"EQUALS","right":{"min":"x\" OR 1=1 --","max":"z","inclusive":true}}`),
		"json_greater_boundary": mustUnmarshal(t, `{"left":"a","operator":"GREATER","right":{"min":"x","max":"z","inclusive":true}}`),
		"json_in_boundary":      mustUnmarshal(t, `{"left":"a","operator":"IN","right":{"min":"x","max":"z","inclusive":true}}`),
		"json_like_boundary":    mustUnmarshal(t, `{"left":"a","operator":"LIKE","right":{"min":"x","max":"z","inclusive":true}}`),
		"equals_raw_boundary":   expr.Eq("a", &expr.RangeBoundary{Min: expr.Lit(`x" OR 1=1 --`), Max: expr.Lit("z")}),
	}
	for name, e := range both {
		for dname, d := range allDrivers {
			t.Run(name+"/"+dname+"/render", func(t *testing.T) {
				if got, err := d.Render(e); err == nil {
					t.Fatalf("expected error rendering %#v, got SQL %q", e, got)
				}
			})
			t.Run(name+"/"+dname+"/render_param", func(t *testing.T) {
				if got, params, err := d.RenderParam(e); err == nil {
					t.Fatalf("expected error rendering %#v, got SQL %q with params %#v", e, got, params)
				}
			})
		}
	}

	// Rejected by Render only. A parameter is never spliced into the SQL, and
	// types like time.Time or driver.Valuer implementations are legitimate
	// bind values, so RenderParam keeps passing them through.
	renderOnly := map[string]*expr.Expression{
		"struct_value":      expr.Eq("a", expr.Lit(stringer{"x') OR 1=1 --"})),
		"json_number_value": expr.Eq("a", expr.Lit(json.Number("1) OR (1=1"))),
		"time_value":        expr.Eq("a", expr.Lit(time.Unix(0, 0).UTC())),
		"nan_value":         expr.Eq("a", expr.Lit(math.NaN())),
		"inf_value":         expr.GREATER("a", expr.Lit(math.Inf(1))),
		"neg_inf_in_list":   expr.IN("a", expr.LIST(expr.Lit(1), expr.Lit(math.Inf(-1)))),
		"nan_range_bound":   expr.Rang("a", math.NaN(), 1, true),
		"inf_range_bound":   expr.Rang("a", 1, math.Inf(1), true),
		"struct_range":      expr.Rang("a", stringer{"x"}, stringer{"y"}, true),
		"time_range":        expr.Rang("a", time.Unix(0, 0).UTC(), time.Unix(60, 0).UTC(), true),
	}
	for name, e := range renderOnly {
		for dname, d := range allDrivers {
			t.Run(name+"/"+dname+"/render", func(t *testing.T) {
				if got, err := d.Render(e); err == nil {
					t.Fatalf("expected error rendering %#v, got SQL %q", e, got)
				}
			})
		}
	}
}

// TestRenderParamPassesThroughBindValues pins that values a database driver
// can bind keep working in the parameterized path.
func TestRenderParamPassesThroughBindValues(t *testing.T) {
	t0, t1 := time.Unix(0, 0).UTC(), time.Unix(60, 0).UTC()
	got, params, err := NewPostgresDriver().RenderParam(expr.Rang("ts", t0, t1, true))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if want := `"ts" BETWEEN $1 AND $2`; got != want {
		t.Fatalf(errTemplate, "generated sql does not match", want, got)
	}
	if len(params) != 2 || params[0] != t0 || params[1] != t1 {
		t.Fatalf("want params [%v %v], got %#v", t0, t1, params)
	}

	got, params, err = NewPostgresDriver().RenderParam(expr.Eq("a", expr.Lit(math.Inf(1))))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if want := `"a" = $1`; got != want || len(params) != 1 || params[0] != math.Inf(1) {
		t.Fatalf("want %q with [+Inf], got %q with %#v", want, got, params)
	}
}

// TestRenderRangeBoundRejectsNullByteAndInvalidUTF8 checks that string and
// column range bounds get the same literal checks as every other string the
// non-parameterized renderers write. Invalid UTF-8 matters on MySQL with a
// multibyte client charset such as GBK, where 0xbf followed by a backslash
// is one character and can swallow the escape in front of a closing quote.
func TestRenderRangeBoundRejectsNullByteAndInvalidUTF8(t *testing.T) {
	cases := map[string]*expr.Expression{
		"null_byte_min":      expr.Rang("a", "x\x00", "z", true),
		"null_byte_max":      expr.Rang("a", "x", "z\x00", false),
		"invalid_utf8_min":   expr.Rang("a", "x\xbf\\", "z", true),
		"invalid_utf8_open":  expr.Rang("a", "*", "\xbf\\", true),
		"null_byte_column":   expr.Rang("a", expr.Column("lo\x00"), expr.Column("hi"), true),
		"invalid_utf8_colum": expr.Rang("a", expr.Column("lo"), expr.Column("hi\xbf"), true),
	}
	for name, e := range cases {
		for dname, d := range allDrivers {
			t.Run(name+"/"+dname, func(t *testing.T) {
				if got, err := d.Render(e); err == nil {
					t.Fatalf("expected error rendering %#v, got SQL %q", e, got)
				}
			})
		}
	}
}

// TestRenderRawValueChecks covers hand-built ASTs that put a bare string or
// column straight into Left/Right instead of wrapping it in a Literal. Those
// skip the Literal render function, which is where strings are normally
// checked for NUL bytes and invalid UTF-8.
func TestRenderRawValueChecks(t *testing.T) {
	cases := map[string]*expr.Expression{
		"raw_string_right": {Left: expr.Lit(expr.Column("a")), Op: expr.Equals, Right: "x\xbf\\"},
		"raw_string_nul":   {Left: expr.Lit(expr.Column("a")), Op: expr.Equals, Right: "x\x00"},
		"raw_column_left":  {Left: expr.Column("a\xbf"), Op: expr.Equals, Right: expr.Lit("x")},
		"raw_column_nul":   {Left: expr.Column("a\x00"), Op: expr.Equals, Right: expr.Lit("x")},
	}
	for name, e := range cases {
		for dname, d := range allDrivers {
			t.Run(name+"/"+dname+"/render", func(t *testing.T) {
				if got, err := d.Render(e); err == nil {
					t.Fatalf("expected error rendering %#v, got SQL %q", e, got)
				}
			})
		}
	}

	// Columns are always written into the SQL, even in parameterized mode.
	for _, name := range []string{"raw_column_left", "raw_column_nul"} {
		e := cases[name]
		for dname, d := range allDrivers {
			t.Run(name+"/"+dname+"/render_param", func(t *testing.T) {
				if got, params, err := d.RenderParam(e); err == nil {
					t.Fatalf("expected error rendering %#v, got SQL %q with params %#v", e, got, params)
				}
			})
		}
	}
}

// noPanic runs fn and turns a panic into a test failure, so one bad case
// does not abort the rest of the table.
func noPanic(t *testing.T, fn func()) {
	t.Helper()
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("panicked: %v", r)
		}
	}()
	fn()
}

// TestRenderLikePattern checks that the right side of LIKE is a single term
// holding a string (or a column) in both render modes. RenderParam used to
// panic on a non-string pattern, and both modes rewrote compound right sides
// differently.
func TestRenderLikePattern(t *testing.T) {
	invalid := map[string]*expr.Expression{
		"int_wildcard":  expr.LIKE("a", expr.WILD(5)),
		"int_literal":   {Left: expr.Lit(expr.Column("a")), Op: expr.Like, Right: expr.Lit(5)},
		"json_int":      mustUnmarshal(t, `{"left":"a","operator":"LIKE","right":5}`),
		"json_float":    mustUnmarshal(t, `{"left":"a","operator":"LIKE","right":1.5}`),
		"bool_literal":  {Left: expr.Lit(expr.Column("a")), Op: expr.Like, Right: expr.Lit(true)},
		"int_regexp":    expr.LIKE("a", expr.REGEXP(5)),
		"compound":      expr.LIKE("a", expr.AND(expr.WILD("x*"), expr.Lit("y"))),
		"compound_or":   expr.LIKE("a", expr.OR(expr.Lit(1), expr.WILD("y*"))),
		"equals_inside": expr.LIKE("a", expr.Eq("b", "c")),
	}
	for name, e := range invalid {
		for dname, d := range allDrivers {
			t.Run("invalid/"+name+"/"+dname+"/render", func(t *testing.T) {
				noPanic(t, func() {
					if got, err := d.Render(e); err == nil {
						t.Fatalf("expected error rendering %#v, got SQL %q", e, got)
					}
				})
			})
			t.Run("invalid/"+name+"/"+dname+"/render_param", func(t *testing.T) {
				noPanic(t, func() {
					if got, params, err := d.RenderParam(e); err == nil {
						t.Fatalf("expected error rendering %#v, got SQL %q with params %#v", e, got, params)
					}
				})
			})
		}
	}

	type want struct{ render, param string }
	valid := map[string]struct {
		input  *expr.Expression
		want   map[string]want
		params []any
	}{
		// LIKE with a plain string literal still renders, with no wildcards.
		"plain_string": {
			input: expr.LIKE("a", "x_y"),
			want: map[string]want{
				"postgres": {`"a" SIMILAR TO 'x\_y'`, `"a" SIMILAR TO $1`},
				"mysql":    {"`a` LIKE 'x#_y' ESCAPE '#'", "`a` LIKE ? ESCAPE '#'"},
				"sqlite":   {`"a" GLOB 'x_y'`, `"a" GLOB ?`},
			},
		},
		// A column on the right compares against another column's value.
		"column_pattern": {
			input: expr.LIKE("a", expr.Column("b")),
			want: map[string]want{
				"postgres": {`"a" SIMILAR TO "b"`, `"a" SIMILAR TO "b"`},
				"mysql":    {"`a` LIKE `b` ESCAPE '#'", "`a` LIKE `b` ESCAPE '#'"},
				"sqlite":   {`"a" GLOB "b"`, `"a" GLOB "b"`},
			},
		},
		"standalone_star": {
			input: expr.LIKE("a", "*"),
			want: map[string]want{
				"postgres": {`"a" SIMILAR TO '%'`, `"a" SIMILAR TO '%'`},
				"mysql":    {"`a` IS NOT NULL", "`a` IS NOT NULL"},
				"sqlite":   {`"a" IS NOT NULL`, `"a" IS NOT NULL`},
			},
		},
	}
	for name, tc := range valid {
		for dname, d := range allDrivers {
			w := tc.want[dname]
			t.Run("valid/"+name+"/"+dname, func(t *testing.T) {
				got, err := d.Render(tc.input)
				if err != nil {
					t.Fatalf("Render: unexpected error: %v", err)
				}
				if got != w.render {
					t.Fatalf(errTemplate, "Render sql does not match", w.render, got)
				}
				got, _, err = d.RenderParam(tc.input)
				if err != nil {
					t.Fatalf("RenderParam: unexpected error: %v", err)
				}
				if got != w.param {
					t.Fatalf(errTemplate, "RenderParam sql does not match", w.param, got)
				}
			})
		}
	}
}

// TestMySQLRegexFallbackEscaping checks that the regex built by the MySQL
// LIKE fallback reaches MySQL unchanged in both modes. The non-parameterized
// SQL is a string literal, which MySQL unescapes under the default sql_mode,
// so every backslash in the regex has to be doubled there. The parameterized
// value is the regex itself.
func TestMySQLRegexFallbackEscaping(t *testing.T) {
	tcs := map[string]struct {
		pattern string
		render  string
		param   string
	}{
		"dot":       {`a.b|(c)*`, `'^(a\\.b|(c).*)$'`, `^(a\.b|(c).*)$`},
		"caret":     {`^a|(b)`, `'^(\\^a|(b))$'`, `^(\^a|(b))$`},
		"dollar":    {`a$|(b)`, `'^(a\\$|(b))$'`, `^(a\$|(b))$`},
		"backslash": {`a\b|(c)`, `'^(a\\\\b|(c))$'`, `^(a\\b|(c))$`},
		"quote":     {`it's|(x)`, `'^(it''s|(x))$'`, `^(it's|(x))$`},
	}
	d := NewMySQLDriver()
	for name, tc := range tcs {
		t.Run(name, func(t *testing.T) {
			e := expr.LIKE("a", tc.pattern)
			got, err := d.Render(e)
			if err != nil {
				t.Fatalf("Render: unexpected error: %v", err)
			}
			if want := "`a` REGEXP " + tc.render; got != want {
				t.Fatalf(errTemplate, "Render sql does not match", want, got)
			}
			got, params, err := d.RenderParam(e)
			if err != nil {
				t.Fatalf("RenderParam: unexpected error: %v", err)
			}
			if want := "`a` REGEXP ?"; got != want {
				t.Fatalf(errTemplate, "RenderParam sql does not match", want, got)
			}
			if len(params) != 1 || params[0] != tc.param {
				t.Fatalf("want params [%q], got %#v", tc.param, params)
			}
		})
	}
}

// TestRenderSafeValueBoundary pins inputs on the safe side of the checks
// above, so tightening them does not start rejecting valid ASTs, and shows
// how each dialect escapes the strings that do get written into SQL.
func TestRenderSafeValueBoundary(t *testing.T) {
	type want struct{ postgres, mysql, sqlite string }

	tcs := map[string]struct {
		input *expr.Expression
		want  want
	}{
		"int64_range": {
			input: expr.Rang("a", int64(1), int64(5), true),
			want:  want{`"a" BETWEEN 1 AND 5`, "`a` BETWEEN 1 AND 5", `"a" BETWEEN 1 AND 5`},
		},
		"uint8_range": {
			input: expr.Rang("a", uint8(1), uint8(5), false),
			want:  want{`"a" > 1 AND "a" < 5`, "`a` > 1 AND `a` < 5", `"a" > 1 AND "a" < 5`},
		},
		"float32_equals": {
			input: expr.Eq("a", expr.Lit(float32(1.5))),
			want:  want{`"a" = 1.5`, "`a` = 1.5", `"a" = 1.5`},
		},
		"large_float_equals": {
			input: expr.Eq("a", expr.Lit(1e21)),
			want:  want{`"a" = 1e+21`, "`a` = 1e+21", `"a" = 1e+21`},
		},
		"int64_equals": {
			input: expr.Eq("a", expr.Lit(int64(-7))),
			want:  want{`"a" = -7`, "`a` = -7", `"a" = -7`},
		},
		"bool_range": {
			input: expr.Rang("a", false, true, true),
			want:  want{`"a" BETWEEN false AND true`, "`a` BETWEEN FALSE AND TRUE", `"a" BETWEEN 0 AND 1`},
		},
		"column_range": {
			input: expr.Rang("a", expr.Column("lo"), expr.Column("hi"), true),
			want:  want{`"a" BETWEEN "lo" AND "hi"`, "`a` BETWEEN `lo` AND `hi`", `"a" BETWEEN "lo" AND "hi"`},
		},
		"regexp_range": {
			input: expr.Rang("a", expr.REGEXP("/x/"), expr.REGEXP("/y/"), true),
			want:  want{`"a" BETWEEN '/x/' AND '/y/'`, "`a` BETWEEN '/x/' AND '/y/'", `"a" BETWEEN '/x/' AND '/y/'`},
		},
		"quote_and_backslash_range": {
			input: expr.Rang("a", `x'y\`, `z'--`, true),
			want: want{
				`"a" BETWEEN 'x''y\' AND 'z''--'`,
				"`a` BETWEEN 'x''y\\\\' AND 'z''--'",
				`"a" BETWEEN 'x''y\' AND 'z''--'`,
			},
		},
		"quote_in_list": {
			input: expr.IN("a", expr.LIST(expr.Lit(`x' OR 1=1 --`), expr.Lit(`y\`))),
			want: want{
				`"a" IN ('x'' OR 1=1 --', 'y\')`,
				"`a` IN ('x'' OR 1=1 --', 'y\\\\')",
				`"a" IN ('x'' OR 1=1 --', 'y\')`,
			},
		},
		"quote_in_comparison": {
			input: expr.GREATER("a", expr.Lit(`x' OR 1=1 --`)),
			want:  want{`"a" > 'x'' OR 1=1 --'`, "`a` > 'x'' OR 1=1 --'", `"a" > 'x'' OR 1=1 --'`},
		},
		"quote_in_wildcard": {
			input: expr.LIKE("a", `x'_\*`),
			want: want{
				`"a" SIMILAR TO 'x''\_\%'`,
				"`a` LIKE 'x''#_\\\\%' ESCAPE '#'",
				`"a" GLOB 'x''_\*'`,
			},
		},
		"quote_in_mysql_regex_fallback": {
			input: expr.LIKE("a", `x'\(y)*`),
			want: want{
				`"a" SIMILAR TO 'x''\(y)%'`,
				"`a` REGEXP '^(x''\\\\\\\\(y).*)$'",
				`"a" GLOB 'x''\(y)*'`,
			},
		},
		// The MySQL fallback escapes . as \. in the regex. The SQL string
		// literal doubles that backslash so MySQL's string parser leaves
		// \. for the regex engine instead of reducing it to a bare dot.
		"mysql_regex_fallback_dot": {
			input: expr.LIKE("a", `a.b|(c)*`),
			want: want{
				`"a" SIMILAR TO 'a.b|(c)%'`,
				"`a` REGEXP '^(a\\\\.b|(c).*)$'",
				`"a" GLOB 'a.b|(c)*'`,
			},
		},
		"quote_in_regexp": {
			input: expr.LIKE("a", expr.REGEXP(`/x'\d/`)),
			want:  want{`"a" ~ 'x''\d'`, "`a` REGEXP 'x''\\\\d'", `"a" REGEXP 'x''\d'`},
		},
		// a:("x' --" AND y) renders as an equality against a boolean
		// expression. The SQL is wrong but every term is still escaped.
		"compound_equals_right": {
			input: expr.Eq("a", expr.AND(expr.Lit(`x' OR 1=1 --`), expr.Lit("y"))),
			want: want{
				`"a" = ('x'' OR 1=1 --' AND 'y')`,
				"`a` = ('x'' OR 1=1 --' AND 'y')",
				`"a" = ('x'' OR 1=1 --' AND 'y')`,
			},
		},
		"column_with_other_quote_chars": {
			input: expr.Eq("a`b\\c", "x"),
			want:  want{"\"a`b\\c\" = 'x'", "", "\"a`b\\c\" = 'x'"},
		},
		"column_with_double_quote": {
			input: expr.Eq(`a"b\c`, "x"),
			want:  want{"", "`a\"b\\c` = 'x'", ""},
		},
	}

	for name, tc := range tcs {
		for dname, d := range allDrivers {
			want := map[string]string{"postgres": tc.want.postgres, "mysql": tc.want.mysql, "sqlite": tc.want.sqlite}[dname]
			t.Run(name+"/"+dname, func(t *testing.T) {
				got, err := d.Render(tc.input)
				if want == "" {
					// The dialect cannot quote this identifier and must refuse it.
					if err == nil {
						t.Fatalf("expected error, got SQL %q", got)
					}
					if !strings.Contains(err.Error(), "column name contains") {
						t.Fatalf("expected a column quoting error, got: %v", err)
					}
					return
				}
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if got != want {
					t.Fatalf(errTemplate, "generated sql does not match", want, got)
				}
			})
		}
	}
}
