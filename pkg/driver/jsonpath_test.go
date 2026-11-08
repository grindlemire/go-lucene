package driver

import (
	"strings"
	"testing"

	"github.com/grindlemire/go-lucene/pkg/lucene/expr"
)

func path(col string, segs ...string) expr.JSONPath {
	return expr.JSONPath{Column: expr.Column(col), Path: segs}
}

func TestJSONPathSegmentEscaping(t *testing.T) {
	type tc struct {
		input    *expr.Expression
		postgres string
		mysql    string
		sqlite   string
	}

	tcs := map[string]tc{
		"single_quote": {
			input:    expr.Eq(path("labels", "it's"), "x"),
			postgres: `"labels"->>'it''s' = 'x'`,
			mysql:    "CASE WHEN JSON_TYPE(JSON_EXTRACT(`labels`, '$.\"it''s\"')) <> 'NULL' THEN JSON_UNQUOTE(JSON_EXTRACT(`labels`, '$.\"it''s\"')) END = 'x'",
			sqlite:   `json_extract("labels", '$."it''s"') = 'x'`,
		},
		"injection_attempt": {
			input:    expr.Eq(path("labels", "a') OR 1=1 --"), "x"),
			postgres: `"labels"->>'a'') OR 1=1 --' = 'x'`,
			mysql:    "CASE WHEN JSON_TYPE(JSON_EXTRACT(`labels`, '$.\"a'') OR 1=1 --\"')) <> 'NULL' THEN JSON_UNQUOTE(JSON_EXTRACT(`labels`, '$.\"a'') OR 1=1 --\"')) END = 'x'",
			sqlite:   `json_extract("labels", '$."a'') OR 1=1 --"') = 'x'`,
		},
		"dot_in_segment": {
			input:    expr.Eq(path("labels", "a.b"), "x"),
			postgres: `"labels"->>'a.b' = 'x'`,
			mysql:    "CASE WHEN JSON_TYPE(JSON_EXTRACT(`labels`, '$.\"a.b\"')) <> 'NULL' THEN JSON_UNQUOTE(JSON_EXTRACT(`labels`, '$.\"a.b\"')) END = 'x'",
			sqlite:   `json_extract("labels", '$."a.b"') = 'x'`,
		},
		"brackets_in_segment": {
			input:    expr.Eq(path("labels", "a[0]"), "x"),
			postgres: `"labels"->>'a[0]' = 'x'`,
			mysql:    "CASE WHEN JSON_TYPE(JSON_EXTRACT(`labels`, '$.\"a[0]\"')) <> 'NULL' THEN JSON_UNQUOTE(JSON_EXTRACT(`labels`, '$.\"a[0]\"')) END = 'x'",
			sqlite:   `json_extract("labels", '$."a[0]"') = 'x'`,
		},
		"leading_digit": {
			input:    expr.Eq(path("labels", "1st"), "x"),
			postgres: `"labels"->>'1st' = 'x'`,
			mysql:    "CASE WHEN JSON_TYPE(JSON_EXTRACT(`labels`, '$.\"1st\"')) <> 'NULL' THEN JSON_UNQUOTE(JSON_EXTRACT(`labels`, '$.\"1st\"')) END = 'x'",
			sqlite:   `json_extract("labels", '$."1st"') = 'x'`,
		},
	}

	for name, tc := range tcs {
		t.Run(name, func(t *testing.T) {
			drivers := []struct {
				name string
				fn   func(*expr.Expression) (string, error)
				want string
			}{
				{"postgres", NewPostgresDriver().Render, tc.postgres},
				{"mysql", NewMySQLDriver().Render, tc.mysql},
				{"sqlite", NewSQLiteDriver().Render, tc.sqlite},
			}
			for _, d := range drivers {
				got, err := d.fn(tc.input)
				if err != nil {
					t.Fatalf("%s: unexpected error: %v", d.name, err)
				}
				if got != d.want {
					t.Errorf("%s:\n got: %s\nwant: %s", d.name, got, d.want)
				}
			}
		})
	}
}

func TestJSONPathRejectsUnsafeSegments(t *testing.T) {
	tcs := map[string]expr.JSONPath{
		"double_quote":  path("labels", `a"b`),
		"backslash":     path("labels", `a\b`),
		"null_byte":     path("labels", "a\x00b"),
		"tab":           path("labels", "a\tb"),
		"invalid_utf8":  path("labels", "a\xffb"),
		"empty_segment": path("labels", "a", ""),
		"no_segments":   path("labels"),
		"empty_column":  path("", "a"),
	}

	for name, p := range tcs {
		t.Run(name, func(t *testing.T) {
			e := expr.Eq(p, "x")
			if _, err := NewPostgresDriver().Render(e); err == nil {
				t.Errorf("postgres: expected an error")
			}
			if _, err := NewMySQLDriver().Render(e); err == nil {
				t.Errorf("mysql: expected an error")
			}
			if _, _, err := NewSQLiteDriver().RenderParam(e); err == nil {
				t.Errorf("sqlite parameterized: expected an error")
			}
		})
	}
}

// noJSONDialect exposes only the Dialect methods, so it lacks RenderJSONPath.
type noJSONDialect struct{ Dialect }

func TestJSONPathUnsupportedDialect(t *testing.T) {
	d := Base{RenderFNs: Shared, Dialect: noJSONDialect{postgresDialect{}}}
	_, err := d.Render(expr.Eq(path("labels", "a"), "x"))
	if err == nil || !strings.Contains(err.Error(), "does not support json paths") {
		t.Fatalf("expected unsupported dialect error, got %v", err)
	}
}

// emptyJSONDialect returns the fragments it holds, some of them empty.
type emptyJSONDialect struct {
	postgresDialect
	sql JSONPathSQL
}

func (d emptyJSONDialect) RenderJSONPath(string, []string) (JSONPathSQL, error) { return d.sql, nil }

func TestJSONPathRejectsEmptyDialectFragments(t *testing.T) {
	full := JSONPathSQL{Text: "t", Value: "v", IsNumber: "n", Number: "x"}
	tcs := map[string]JSONPathSQL{
		"Text":     {Value: full.Value, IsNumber: full.IsNumber, Number: full.Number},
		"Value":    {Text: full.Text, Value: "\t", IsNumber: full.IsNumber, Number: full.Number},
		"IsNumber": {Text: full.Text, Value: full.Value, IsNumber: " ", Number: full.Number},
		"Number":   {Text: full.Text, Value: full.Value, IsNumber: full.IsNumber},
	}
	for name, sql := range tcs {
		t.Run(name, func(t *testing.T) {
			d := Base{RenderFNs: Shared, Dialect: emptyJSONDialect{sql: sql}}
			_, err := d.Render(expr.GREATER(path("labels", "a"), 1))
			if err == nil || !strings.Contains(err.Error(), "empty "+name) {
				t.Fatalf("expected an empty %s error, got %v", name, err)
			}
		})
	}
	d := Base{RenderFNs: Shared, Dialect: emptyJSONDialect{sql: full}}
	if _, err := d.Render(expr.GREATER(path("labels", "a"), 1)); err != nil {
		t.Fatalf("complete fragments: unexpected error %v", err)
	}
}

func TestJSONPathComparesSizedNumbers(t *testing.T) {
	d := NewPostgresDriver()
	v := path("labels", "v")
	for _, n := range []any{int8(5), int16(5), int32(5), int64(5), uint(5), uint8(5), uint16(5), uint32(5), uint64(5), float32(5)} {
		for name, e := range map[string]*expr.Expression{
			"equals":  expr.Eq(v, n),
			"greater": expr.GREATER(v, n),
			"in":      expr.IN(v, expr.LIST(expr.Lit(n), expr.Lit(n))),
			"range":   expr.Rang(v, n, n, true),
		} {
			got, err := d.Render(e)
			if err != nil {
				t.Fatalf("%T %s: unexpected error %v", n, name, err)
			}
			if !strings.HasPrefix(got, "CASE WHEN json_typeof") {
				t.Errorf("%T %s: want the numeric CASE form, got %s", n, name, got)
			}
		}
		got, err := d.Render(expr.IN(v, expr.LIST(expr.Lit(n), expr.Lit("abc"))))
		if err != nil {
			t.Fatalf("%T mixed in: unexpected error %v", n, err)
		}
		if want := `"labels"->>'v' IN ('5', 'abc')`; got != want {
			t.Errorf("%T mixed in:\n got: %s\nwant: %s", n, got, want)
		}
		got, err = d.Render(expr.Rang(v, n, "abc", true))
		if err != nil {
			t.Fatalf("%T mixed range: unexpected error %v", n, err)
		}
		if want := `"labels"->>'v' BETWEEN '5' AND 'abc'`; got != want {
			t.Errorf("%T mixed range:\n got: %s\nwant: %s", n, got, want)
		}
	}
}

func TestJSONPathChecksColumnName(t *testing.T) {
	d := NewPostgresDriver()
	for _, col := range []string{"a\x00b", "a\xffb"} {
		if _, err := d.Render(expr.Eq(path(col, "k"), "x")); err == nil {
			t.Errorf("column %q: expected an error", col)
		}
	}
}
