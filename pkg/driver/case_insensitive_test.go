package driver

import (
	"reflect"
	"testing"

	"github.com/grindlemire/go-lucene/pkg/lucene/expr"
)

// TestCaseInsensitiveFieldsOnDrivers sets Base.CaseInsensitiveFields directly,
// the way a caller that renders a parsed expression itself would.
func TestCaseInsensitiveFieldsOnDrivers(t *testing.T) {
	type want struct {
		render string
		param  string
		params []any
	}

	postgres := NewPostgresDriver()
	postgres.CaseInsensitiveFields = []string{"a"}
	mysql := NewMySQLDriver()
	mysql.CaseInsensitiveFields = []string{"a"}
	sqlite := NewSQLiteDriver()
	sqlite.CaseInsensitiveFields = []string{"a"}
	// A custom driver with no Dialect gets the Postgres-flavored default and
	// folds case without implementing anything new.
	custom := Base{RenderFNs: Shared, CaseInsensitiveFields: []string{"a"}}

	drivers := map[string]renderer{
		"postgres": postgres,
		"mysql":    mysql,
		"sqlite":   sqlite,
		"custom":   custom,
	}

	tcs := map[string]struct {
		input *expr.Expression
		want  map[string]want
	}{
		"wildcard": {
			input: expr.LIKE("a", "Foo*"),
			want: map[string]want{
				"postgres": {`LOWER("a") SIMILAR TO LOWER('Foo%')`, `LOWER("a") SIMILAR TO LOWER($1)`, []any{"Foo%"}},
				"mysql":    {"LOWER(`a`) LIKE LOWER('Foo%') ESCAPE '#'", "LOWER(`a`) LIKE LOWER(?) ESCAPE '#'", []any{"Foo%"}},
				"sqlite":   {`LOWER("a") GLOB LOWER('Foo*')`, `LOWER("a") GLOB LOWER(?)`, []any{"Foo*"}},
				"custom":   {`LOWER("a") SIMILAR TO LOWER('Foo%')`, `LOWER("a") SIMILAR TO LOWER(?)`, []any{"Foo%"}},
			},
		},
		// MySQL sends a pattern with SIMILAR TO metacharacters through
		// REGEXP. The folded form keeps that operator.
		"regexp_fallback": {
			input: expr.LIKE("a", "Foo|(Bar)*"),
			want: map[string]want{
				"postgres": {`LOWER("a") SIMILAR TO LOWER('Foo|(Bar)%')`, `LOWER("a") SIMILAR TO LOWER($1)`, []any{"Foo|(Bar)%"}},
				"mysql":    {"LOWER(`a`) REGEXP LOWER('^(Foo|(Bar).*)$')", "LOWER(`a`) REGEXP LOWER(?)", []any{"^(Foo|(Bar).*)$"}},
				"sqlite":   {`LOWER("a") GLOB LOWER('Foo|(Bar)*')`, `LOWER("a") GLOB LOWER(?)`, []any{"Foo|(Bar)*"}},
				"custom":   {`LOWER("a") SIMILAR TO LOWER('Foo|(Bar)%')`, `LOWER("a") SIMILAR TO LOWER(?)`, []any{"Foo|(Bar)%"}},
			},
		},
		"equality": {
			input: expr.Eq("a", "Foo"),
			want: map[string]want{
				"postgres": {`LOWER("a") = LOWER('Foo')`, `LOWER("a") = LOWER($1)`, []any{"Foo"}},
				"mysql":    {"LOWER(`a`) = LOWER('Foo')", "LOWER(`a`) = LOWER(?)", []any{"Foo"}},
				"sqlite":   {`LOWER("a") = LOWER('Foo')`, `LOWER("a") = LOWER(?)`, []any{"Foo"}},
				"custom":   {`LOWER("a") = LOWER('Foo')`, `LOWER("a") = LOWER(?)`, []any{"Foo"}},
			},
		},
		"in_list": {
			input: expr.IN("a", expr.LIST(expr.Lit("Foo"), expr.Lit(5), expr.Lit("Bar"))),
			want: map[string]want{
				"postgres": {`(LOWER("a") IN (LOWER('Foo'), LOWER('Bar')) OR "a" = 5)`, `(LOWER("a") IN (LOWER($1), LOWER($2)) OR "a" = $3)`, []any{"Foo", "Bar", 5}},
				"mysql":    {"(LOWER(`a`) IN (LOWER('Foo'), LOWER('Bar')) OR `a` = 5)", "(LOWER(`a`) IN (LOWER(?), LOWER(?)) OR `a` = ?)", []any{"Foo", "Bar", 5}},
				"sqlite":   {`(LOWER("a") IN (LOWER('Foo'), LOWER('Bar')) OR "a" = 5)`, `(LOWER("a") IN (LOWER(?), LOWER(?)) OR "a" = ?)`, []any{"Foo", "Bar", 5}},
				"custom":   {`(LOWER("a") IN (LOWER('Foo'), LOWER('Bar')) OR "a" = 5)`, `(LOWER("a") IN (LOWER(?), LOWER(?)) OR "a" = ?)`, []any{"Foo", "Bar", 5}},
			},
		},
		// Only strings fold: a bool, a number and a column stay as they are.
		"bool_unchanged": {
			input: expr.Eq("a", true),
			want: map[string]want{
				"postgres": {`"a" = true`, `"a" = $1`, []any{true}},
				"mysql":    {"`a` = TRUE", "`a` = ?", []any{true}},
				"sqlite":   {`"a" = 1`, `"a" = ?`, []any{1}},
				"custom":   {`"a" = true`, `"a" = ?`, []any{true}},
			},
		},
		"number_unchanged": {
			input: expr.Eq("a", 5),
			want: map[string]want{
				"postgres": {`"a" = 5`, `"a" = $1`, []any{5}},
				"mysql":    {"`a` = 5", "`a` = ?", []any{5}},
				"sqlite":   {`"a" = 5`, `"a" = ?`, []any{5}},
				"custom":   {`"a" = 5`, `"a" = ?`, []any{5}},
			},
		},
		"column_unchanged": {
			input: expr.Eq("a", expr.Lit(expr.Column("b"))),
			want: map[string]want{
				"postgres": {`"a" = "b"`, `"a" = "b"`, nil},
				"mysql":    {"`a` = `b`", "`a` = `b`", nil},
				"sqlite":   {`"a" = "b"`, `"a" = "b"`, nil},
				"custom":   {`"a" = "b"`, `"a" = "b"`, nil},
			},
		},
		"unlisted_unchanged": {
			input: expr.LIKE("b", "Foo*"),
			want: map[string]want{
				"postgres": {`"b" SIMILAR TO 'Foo%'`, `"b" SIMILAR TO $1`, []any{"Foo%"}},
				"mysql":    {"`b` LIKE 'Foo%' ESCAPE '#'", "`b` LIKE ? ESCAPE '#'", []any{"Foo%"}},
				"sqlite":   {`"b" GLOB 'Foo*'`, `"b" GLOB ?`, []any{"Foo*"}},
				"custom":   {`"b" SIMILAR TO 'Foo%'`, `"b" SIMILAR TO ?`, []any{"Foo%"}},
			},
		},
	}

	for name, tc := range tcs {
		for dname, d := range drivers {
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
				if len(params) != 0 || len(w.params) != 0 {
					if !reflect.DeepEqual(params, w.params) {
						t.Fatalf("params\n got: %#v\nwant: %#v", params, w.params)
					}
				}
			})
		}
	}
}
