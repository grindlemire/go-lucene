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
			sqlite:   `CASE json_type("labels", '$."it''s"') WHEN 'true' THEN 'true' WHEN 'false' THEN 'false' ELSE CAST(json_extract("labels", '$."it''s"') AS TEXT) END = 'x'`,
		},
		"injection_attempt": {
			input:    expr.Eq(path("labels", "a') OR 1=1 --"), "x"),
			postgres: `"labels"->>'a'') OR 1=1 --' = 'x'`,
			mysql:    "CASE WHEN JSON_TYPE(JSON_EXTRACT(`labels`, '$.\"a'') OR 1=1 --\"')) <> 'NULL' THEN JSON_UNQUOTE(JSON_EXTRACT(`labels`, '$.\"a'') OR 1=1 --\"')) END = 'x'",
			sqlite:   `CASE json_type("labels", '$."a'') OR 1=1 --"') WHEN 'true' THEN 'true' WHEN 'false' THEN 'false' ELSE CAST(json_extract("labels", '$."a'') OR 1=1 --"') AS TEXT) END = 'x'`,
		},
		"dot_in_segment": {
			input:    expr.Eq(path("labels", "a.b"), "x"),
			postgres: `"labels"->>'a.b' = 'x'`,
			mysql:    "CASE WHEN JSON_TYPE(JSON_EXTRACT(`labels`, '$.\"a.b\"')) <> 'NULL' THEN JSON_UNQUOTE(JSON_EXTRACT(`labels`, '$.\"a.b\"')) END = 'x'",
			sqlite:   `CASE json_type("labels", '$."a.b"') WHEN 'true' THEN 'true' WHEN 'false' THEN 'false' ELSE CAST(json_extract("labels", '$."a.b"') AS TEXT) END = 'x'`,
		},
		"brackets_in_segment": {
			input:    expr.Eq(path("labels", "a[0]"), "x"),
			postgres: `"labels"->>'a[0]' = 'x'`,
			mysql:    "CASE WHEN JSON_TYPE(JSON_EXTRACT(`labels`, '$.\"a[0]\"')) <> 'NULL' THEN JSON_UNQUOTE(JSON_EXTRACT(`labels`, '$.\"a[0]\"')) END = 'x'",
			sqlite:   `CASE json_type("labels", '$."a[0]"') WHEN 'true' THEN 'true' WHEN 'false' THEN 'false' ELSE CAST(json_extract("labels", '$."a[0]"') AS TEXT) END = 'x'`,
		},
		"leading_digit": {
			input:    expr.Eq(path("labels", "1st"), "x"),
			postgres: `"labels"->>'1st' = 'x'`,
			mysql:    "CASE WHEN JSON_TYPE(JSON_EXTRACT(`labels`, '$.\"1st\"')) <> 'NULL' THEN JSON_UNQUOTE(JSON_EXTRACT(`labels`, '$.\"1st\"')) END = 'x'",
			sqlite:   `CASE json_type("labels", '$."1st"') WHEN 'true' THEN 'true' WHEN 'false' THEN 'false' ELSE CAST(json_extract("labels", '$."1st"') AS TEXT) END = 'x'`,
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
	full := JSONPathSQL{Text: "t", IsNumber: "n", Number: "v"}
	tcs := map[string]JSONPathSQL{
		"Text":     {IsNumber: full.IsNumber, Number: full.Number},
		"IsNumber": {Text: full.Text, IsNumber: " ", Number: full.Number},
		"Number":   {Text: full.Text, IsNumber: full.IsNumber},
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
