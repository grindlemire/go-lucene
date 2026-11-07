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
			mysql:    "JSON_UNQUOTE(JSON_EXTRACT(`labels`, '$.\"it''s\"')) = 'x'",
			sqlite:   `json_extract("labels", '$."it''s"') = 'x'`,
		},
		"injection_attempt": {
			input:    expr.Eq(path("labels", "a') OR 1=1 --"), "x"),
			postgres: `"labels"->>'a'') OR 1=1 --' = 'x'`,
			mysql:    "JSON_UNQUOTE(JSON_EXTRACT(`labels`, '$.\"a'') OR 1=1 --\"')) = 'x'",
			sqlite:   `json_extract("labels", '$."a'') OR 1=1 --"') = 'x'`,
		},
		"dot_in_segment": {
			input:    expr.Eq(path("labels", "a.b"), "x"),
			postgres: `"labels"->>'a.b' = 'x'`,
			mysql:    "JSON_UNQUOTE(JSON_EXTRACT(`labels`, '$.\"a.b\"')) = 'x'",
			sqlite:   `json_extract("labels", '$."a.b"') = 'x'`,
		},
		"brackets_in_segment": {
			input:    expr.Eq(path("labels", "a[0]"), "x"),
			postgres: `"labels"->>'a[0]' = 'x'`,
			mysql:    "JSON_UNQUOTE(JSON_EXTRACT(`labels`, '$.\"a[0]\"')) = 'x'",
			sqlite:   `json_extract("labels", '$."a[0]"') = 'x'`,
		},
		"leading_digit": {
			input:    expr.Eq(path("labels", "1st"), "x"),
			postgres: `"labels"->>'1st' = 'x'`,
			mysql:    "JSON_UNQUOTE(JSON_EXTRACT(`labels`, '$.\"1st\"')) = 'x'",
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
