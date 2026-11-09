package driver

import (
	"encoding/json"
	"math"
	"reflect"
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
		"ast_slice":      expr.Rang("a", expr.Lit([]*expr.Expression{expr.Lit("x")}), 5, true),
		"ast_value_list": rangeOf(expr.Lit([]expr.Expression{*expr.Lit("x")}), expr.Lit(5)),
		"typed_nil_min":  rangeOf((*expr.Expression)(nil), expr.Lit(3)),
		"typed_nil_max":  rangeOf(expr.Lit(1), (*expr.Expression)(nil)),
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
				noPanic(t, func() {
					got, err := d.Render(e)
					if err == nil {
						t.Fatalf("expected error rendering %#v, got SQL %q", e, got)
					}
					if !strings.Contains(err.Error(), "must be a single term") {
						t.Fatalf("expected a single-term range bound error, got: %v", err)
					}
				})
			})
			t.Run(name+"/"+dname+"/render_param", func(t *testing.T) {
				noPanic(t, func() {
					got, params, err := d.RenderParam(e)
					if err == nil {
						t.Fatalf("expected error rendering %#v, got SQL %q with params %#v", e, got, params)
					}
					if !strings.Contains(err.Error(), "must be a single term") {
						t.Fatalf("expected a single-term range bound error, got: %v", err)
					}
				})
			})
		}
	}
}

// TestRangeBoundErrorArticle checks the driver's compound-bound error uses
// "an" before an operator name that starts with a vowel.
func TestRangeBoundErrorArticle(t *testing.T) {
	tcs := map[string]struct {
		bound any
		want  string
	}{
		"and": {expr.AND(expr.Lit(1), expr.Lit(2)), "range bound must be a single term, not an AND expression"},
		"not": {expr.NOT(expr.Lit(1)), "range bound must be a single term, not a NOT expression"},
	}
	for name, tc := range tcs {
		t.Run(name, func(t *testing.T) {
			_, err := NewPostgresDriver().Render(rangeOf(tc.bound, expr.Lit(3)))
			if err == nil || err.Error() != tc.want {
				t.Fatalf("\nwant %s\ngot  %v", tc.want, err)
			}
		})
	}
}

// TestRenderRejectsUnsupportedValues covers values that serialize used to
// print with %v: AST nodes in a value position, JSON containers, Go types
// with no SQL literal form, and non-finite floats.
func TestRenderRejectsUnsupportedValues(t *testing.T) {
	type stringer struct{ S string }

	// Rejected by both Render and RenderParam: AST pieces in a value slot.
	// encoding/json produces these when a "right" object has min/max keys.
	const boundaryValue = "unsupported value of type *expr.RangeBoundary"
	both := map[string]struct {
		input   *expr.Expression
		wantErr string
	}{
		"json_equals_boundary": {
			mustUnmarshal(t, `{"left":"a","operator":"EQUALS","right":{"min":"x\" OR 1=1 --","max":"z","inclusive":true}}`),
			boundaryValue,
		},
		"json_greater_boundary": {
			mustUnmarshal(t, `{"left":"a","operator":"GREATER","right":{"min":"x","max":"z","inclusive":true}}`),
			boundaryValue,
		},
		"json_in_boundary": {
			mustUnmarshal(t, `{"left":"a","operator":"IN","right":{"min":"x","max":"z","inclusive":true}}`),
			boundaryValue,
		},
		"json_like_boundary": {
			mustUnmarshal(t, `{"left":"a","operator":"LIKE","right":{"min":"x","max":"z","inclusive":true}}`),
			"LIKE pattern must be a single term, not *expr.RangeBoundary",
		},
		"equals_raw_boundary": {
			expr.Eq("a", &expr.RangeBoundary{Min: expr.Lit(`x" OR 1=1 --`), Max: expr.Lit("z")}),
			boundaryValue,
		},
	}
	for name, tc := range both {
		for dname, d := range allDrivers {
			t.Run(name+"/"+dname+"/render", func(t *testing.T) {
				got, err := d.Render(tc.input)
				if err == nil {
					t.Fatalf("expected error rendering %#v, got SQL %q", tc.input, got)
				}
				if !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("expected an error containing %q, got: %v", tc.wantErr, err)
				}
			})
			t.Run(name+"/"+dname+"/render_param", func(t *testing.T) {
				got, params, err := d.RenderParam(tc.input)
				if err == nil {
					t.Fatalf("expected error rendering %#v, got SQL %q with params %#v", tc.input, got, params)
				}
				if !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("expected an error containing %q, got: %v", tc.wantErr, err)
				}
			})
		}
	}

	// Rejected by Render only. A parameter is never spliced into the SQL, and
	// types like time.Time or driver.Valuer implementations are legitimate
	// bind values, so RenderParam keeps passing them through.
	renderOnly := map[string]struct {
		input   *expr.Expression
		wantErr string
	}{
		"struct_value":      {expr.Eq("a", expr.Lit(stringer{"x') OR 1=1 --"})), "unsupported value of type driver.stringer"},
		"json_number_value": {expr.Eq("a", expr.Lit(json.Number("1) OR (1=1"))), "unsupported value of type json.Number"},
		"time_value":        {expr.Eq("a", expr.Lit(time.Unix(0, 0).UTC())), "unsupported value of type time.Time"},
		"nan_value":         {expr.Eq("a", expr.Lit(math.NaN())), "NaN has no SQL literal form"},
		"inf_value":         {expr.GREATER("a", expr.Lit(math.Inf(1))), "+Inf has no SQL literal form"},
		"neg_inf_in_list":   {expr.IN("a", expr.LIST(expr.Lit(1), expr.Lit(math.Inf(-1)))), "-Inf has no SQL literal form"},
		"nan_range_bound":   {expr.Rang("a", math.NaN(), 1, true), "NaN has no SQL literal form"},
		"inf_range_bound":   {expr.Rang("a", 1, math.Inf(1), true), "+Inf has no SQL literal form"},
		"struct_range":      {expr.Rang("a", stringer{"x"}, stringer{"y"}, true), "unsupported range bound value of type driver.stringer"},
		"time_range":        {expr.Rang("a", time.Unix(0, 0).UTC(), time.Unix(60, 0).UTC(), true), "unsupported range bound value of type time.Time"},
	}
	for name, tc := range renderOnly {
		for dname, d := range allDrivers {
			t.Run(name+"/"+dname+"/render", func(t *testing.T) {
				got, err := d.Render(tc.input)
				if err == nil {
					t.Fatalf("expected error rendering %#v, got SQL %q", tc.input, got)
				}
				if !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("expected an error containing %q, got: %v", tc.wantErr, err)
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

// TestNumberThatLosesDigitsIsWritten checks hand-built and JSON-decoded
// number literals whose float value rounds their text. Both render modes write
// the text into the SQL; one whose float holds its text exactly renders and
// binds the float as before.
func TestNumberThatLosesDigitsIsWritten(t *testing.T) {
	big := expr.NumberLit(9223372036854775808.0, "9223372036854775808")
	// changed returns that literal with its value replaced after NumberLit
	// checked the text. The text no longer spells the value, so the driver
	// renders the value.
	changed := func(v any) *expr.Expression {
		lit := expr.NumberLit(9223372036854775808.0, "9223372036854775808")
		lit.Left = v
		return lit
	}
	tcs := map[string]struct {
		input      *expr.Expression
		want       string
		wantParam  string
		wantParams []any
	}{
		"float64": {
			input:     expr.Eq("a", big),
			want:      `"a" = 9223372036854775808`,
			wantParam: `"a" = 9223372036854775808`,
		},
		"float32": {
			input:     expr.Eq("a", expr.NumberLit(float32(16777217), "16777217")),
			want:      `"a" = 16777217`,
			wantParam: `"a" = 16777217`,
		},
		"json": {
			input:     mustUnmarshal(t, `{"left":"a","operator":"EQUALS","right":{"number":{"value":9223372036854775808,"text":"9223372036854775808"}}}`),
			want:      `"a" = 9223372036854775808`,
			wantParam: `"a" = 9223372036854775808`,
		},
		// A bare JSON number keeps its digits, as it does in a query.
		"json_bare": {
			input:     mustUnmarshal(t, `{"left":"a","operator":"EQUALS","right":9223372036854775808}`),
			want:      `"a" = 9223372036854775808`,
			wantParam: `"a" = 9223372036854775808`,
		},
		"json_bare_range_bound": {
			input:      mustUnmarshal(t, `{"left":"a","operator":"RANGE","right":{"min":123456789012345678.5,"max":5,"inclusive":true}}`),
			want:       `"a" >= 123456789012345678.5 AND "a" <= 5`,
			wantParam:  `"a" >= 123456789012345678.5 AND "a" <= $1`,
			wantParams: []any{5},
		},
		"json_bare_list_item": {
			input:      mustUnmarshal(t, `{"left":"a","operator":"IN","right":{"left":[12345678901234567890123,2],"operator":"LIST"}}`),
			want:       `"a" IN (12345678901234567890123, 2)`,
			wantParam:  `"a" IN (12345678901234567890123, $1)`,
			wantParams: []any{2},
		},
		"range_bound": {
			input:      rangeOf(big, 5),
			want:       `"a" >= 9223372036854775808 AND "a" <= 5`,
			wantParam:  `"a" >= 9223372036854775808 AND "a" <= $1`,
			wantParams: []any{5},
		},
		"exact": {
			input:      expr.Eq("a", expr.NumberLit(1000.0, "1e3")),
			want:       `"a" = 1000`,
			wantParam:  `"a" = $1`,
			wantParams: []any{1000.0},
		},
		"value_changed": {
			input:      expr.Eq("a", changed(5.0)),
			want:       `"a" = 5`,
			wantParam:  `"a" = $1`,
			wantParams: []any{5.0},
		},
		"value_changed_to_float32": {
			input:      expr.Eq("a", changed(float32(1.5))),
			want:       `"a" = 1.5`,
			wantParam:  `"a" = $1`,
			wantParams: []any{float32(1.5)},
		},
		"value_changed_to_int": {
			input:      expr.Eq("a", changed(7)),
			want:       `"a" = 7`,
			wantParam:  `"a" = $1`,
			wantParams: []any{7},
		},
		"bound_value_changed": {
			input:      rangeOf(changed(1e19), 5),
			want:       `"a" >= 10000000000000000000 AND "a" <= 5`,
			wantParam:  `"a" >= $1 AND "a" <= $2`,
			wantParams: []any{1e19, 5},
		},
	}
	for name, tc := range tcs {
		t.Run(name, func(t *testing.T) {
			d := NewPostgresDriver()
			got, err := d.Render(tc.input)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tc.want {
				t.Fatalf(errTemplate, "generated sql does not match", tc.want, got)
			}
			got, params, err := d.RenderParam(tc.input)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tc.wantParam {
				t.Fatalf(errTemplate, "generated sql does not match", tc.wantParam, got)
			}
			if len(params) != len(tc.wantParams) || (len(params) > 0 && !reflect.DeepEqual(params, tc.wantParams)) {
				t.Fatalf("want params %#v, got %#v", tc.wantParams, params)
			}
		})
	}
}

// TestNamedNumericType pins how a named integer type is handled. Render only
// writes the built-in numeric types as SQL numbers, so it rejects one rather
// than guessing at a format; RenderParam binds the value unchanged and leaves
// the conversion to the database driver.
func TestNamedNumericType(t *testing.T) {
	type status int

	tcs := map[string]struct {
		input     *expr.Expression
		renderErr string
		params    []any
	}{
		"equals": {
			input:     expr.Eq("status", status(1)),
			renderErr: "unsupported value of type",
			params:    []any{status(1)},
		},
		"range": {
			input:     expr.Rang("status", status(1), status(3), true),
			renderErr: "unsupported range bound value of type",
			params:    []any{status(1), status(3)},
		},
	}
	for name, tc := range tcs {
		for dname, d := range allDrivers {
			t.Run(name+"/"+dname+"/render", func(t *testing.T) {
				got, err := d.Render(tc.input)
				if err == nil {
					t.Fatalf("expected error rendering %#v, got SQL %q", tc.input, got)
				}
				if !strings.Contains(err.Error(), tc.renderErr) {
					t.Fatalf("expected an error containing %q, got: %v", tc.renderErr, err)
				}
			})
			t.Run(name+"/"+dname+"/render_param", func(t *testing.T) {
				got, params, err := d.RenderParam(tc.input)
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if !reflect.DeepEqual(params, tc.params) {
					t.Fatalf("want params %#v, got %#v (sql: %s)", tc.params, params, got)
				}
			})
		}
	}
}

// TestRenderRangeBoundRejectsNullByteAndInvalidUTF8 checks that string and
// column range bounds get the same NUL and UTF-8 checks as every other string
// the non-parameterized renderers write.
//
// The UTF-8 check does not make ToMySQL safe under a GBK, Big5, or SJIS
// connection charset. Valid UTF-8 can end in a byte that starts a two-byte
// character in those charsets: 丿 is E4 B8 BF, and BF followed by the
// backslash ToMySQL uses for escaping is one GBK character, which swallows
// the escape. ToMySQL assumes a utf8mb4 connection.
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
		"backslash": {`a\\b|(c)`, `'^(a\\\\b|(c))$'`, `^(a\\b|(c))$`},
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

// TestRenderColumnRangeBound checks that a column used as a range bound is
// compared as a column in both modes. RenderParam used to bind it as a
// parameter, so the database compared against the column's name as a string.
func TestRenderColumnRangeBound(t *testing.T) {
	type want struct {
		render, param string
		params        []any
	}
	tcs := map[string]struct {
		input *expr.Expression
		want  map[string]want
	}{
		"column_range": {
			input: expr.Rang("a", expr.Column("lo"), expr.Column("hi"), true),
			want: map[string]want{
				"postgres": {`"a" BETWEEN "lo" AND "hi"`, `"a" BETWEEN "lo" AND "hi"`, nil},
				"mysql":    {"`a` BETWEEN `lo` AND `hi`", "`a` BETWEEN `lo` AND `hi`", nil},
				"sqlite":   {`"a" BETWEEN "lo" AND "hi"`, `"a" BETWEEN "lo" AND "hi"`, nil},
			},
		},
		"column_and_number": {
			input: expr.Rang("a", expr.Lit(expr.Column("b")), 5, true),
			want: map[string]want{
				"postgres": {`"a" >= "b" AND "a" <= 5`, `"a" >= "b" AND "a" <= $1`, []any{5}},
				"mysql":    {"`a` >= `b` AND `a` <= 5", "`a` >= `b` AND `a` <= ?", []any{5}},
				"sqlite":   {`"a" >= "b" AND "a" <= 5`, `"a" >= "b" AND "a" <= ?`, []any{5}},
			},
		},
		"number_and_column_exclusive": {
			input: expr.Rang("a", 1, expr.Column("b"), false),
			want: map[string]want{
				"postgres": {`"a" > 1 AND "a" < "b"`, `"a" > $1 AND "a" < "b"`, []any{1}},
				"mysql":    {"`a` > 1 AND `a` < `b`", "`a` > ? AND `a` < `b`", []any{1}},
				"sqlite":   {`"a" > 1 AND "a" < "b"`, `"a" > ? AND "a" < "b"`, []any{1}},
			},
		},
		"open_min_column_max": {
			input: expr.Rang("a", "*", expr.Column("hi"), true),
			want: map[string]want{
				"postgres": {`"a" <= "hi"`, `"a" <= "hi"`, nil},
				"mysql":    {"`a` <= `hi`", "`a` <= `hi`", nil},
				"sqlite":   {`"a" <= "hi"`, `"a" <= "hi"`, nil},
			},
		},
		"column_min_open_max": {
			input: expr.Rang("a", expr.Column("lo"), "*", false),
			want: map[string]want{
				"postgres": {`"a" > "lo"`, `"a" > "lo"`, nil},
				"mysql":    {"`a` > `lo`", "`a` > `lo`", nil},
				"sqlite":   {`"a" > "lo"`, `"a" > "lo"`, nil},
			},
		},
	}
	for name, tc := range tcs {
		for dname, d := range allDrivers {
			w := tc.want[dname]
			t.Run(name+"/"+dname, func(t *testing.T) {
				got, err := d.Render(tc.input)
				if err != nil {
					t.Fatalf("Render: unexpected error: %v", err)
				}
				if got != w.render {
					t.Fatalf(errTemplate, "Render sql does not match", w.render, got)
				}
				got, params, err := d.RenderParam(tc.input)
				if err != nil {
					t.Fatalf("RenderParam: unexpected error: %v", err)
				}
				if got != w.param {
					t.Fatalf(errTemplate, "RenderParam sql does not match", w.param, got)
				}
				if len(params) != len(w.params) || (len(params) > 0 && !reflect.DeepEqual(params, w.params)) {
					t.Fatalf("want params %#v, got %#v", w.params, params)
				}
			})
		}
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
			input: expr.LIKE("a", `x'_\\*`),
			want: want{
				`"a" SIMILAR TO 'x''\_\\%'`,
				"`a` LIKE 'x''#_\\\\%' ESCAPE '#'",
				`"a" GLOB 'x''_\*'`,
			},
		},
		"quote_in_mysql_regex_fallback": {
			input: expr.LIKE("a", `x'\\(y)*`),
			want: want{
				`"a" SIMILAR TO 'x''\\(y)%'`,
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
		// A hand-built EQUALS whose value is an AND gets its field on each
		// term, as a:(x AND y) does, and each term is still escaped. It used
		// to render as "a" = ('x'' OR 1=1 --' AND 'y').
		"compound_equals_right": {
			input: expr.Eq("a", expr.AND(expr.Lit(`x' OR 1=1 --`), expr.Lit("y"))),
			want: want{
				`("a" = 'x'' OR 1=1 --') AND ("a" = 'y')`,
				"(`a` = 'x'' OR 1=1 --') AND (`a` = 'y')",
				`("a" = 'x'' OR 1=1 --') AND ("a" = 'y')`,
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
