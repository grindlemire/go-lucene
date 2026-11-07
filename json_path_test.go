package lucene

import (
	"reflect"
	"testing"
)

func TestJSONPathEndToEnd(t *testing.T) {
	type tc struct {
		input     string
		postgres  string
		mysql     string
		sqlite    string
		pgParams  []any
		sqlParams []any
		// mysqlParams overrides sqlParams where MySQL binds a different value.
		mysqlParams []any
	}

	tcs := map[string]tc{
		"equals": {
			input:     `labels.category:fruit`,
			postgres:  `"labels"->>'category' = 'fruit'`,
			mysql:     "JSON_UNQUOTE(JSON_EXTRACT(`labels`, '$.category')) = 'fruit'",
			sqlite:    `json_extract("labels", '$.category') = 'fruit'`,
			pgParams:  []any{"fruit"},
			sqlParams: []any{"fruit"},
		},
		"nested_path": {
			input:     `metadata.address.city:Boston`,
			postgres:  `"metadata"->'address'->>'city' = 'Boston'`,
			mysql:     "JSON_UNQUOTE(JSON_EXTRACT(`metadata`, '$.address.city')) = 'Boston'",
			sqlite:    `json_extract("metadata", '$.address.city') = 'Boston'`,
			pgParams:  []any{"Boston"},
			sqlParams: []any{"Boston"},
		},
		"null": {
			input:    `labels.category:null`,
			postgres: `"labels"->>'category' IS NULL`,
			mysql:    "JSON_UNQUOTE(JSON_EXTRACT(`labels`, '$.category')) IS NULL",
			sqlite:   `json_extract("labels", '$.category') IS NULL`,
		},
		"not_null": {
			input:    `NOT labels.category:null`,
			postgres: `"labels"->>'category' IS NOT NULL`,
			mysql:    "JSON_UNQUOTE(JSON_EXTRACT(`labels`, '$.category')) IS NOT NULL",
			sqlite:   `json_extract("labels", '$.category') IS NOT NULL`,
		},
		"equals_number": {
			input:     `labels.count:5`,
			postgres:  `("labels"->>'count')::numeric = 5`,
			mysql:     "JSON_UNQUOTE(JSON_EXTRACT(`labels`, '$.count')) = 5",
			sqlite:    `json_extract("labels", '$.count') = 5`,
			pgParams:  []any{5},
			sqlParams: []any{5},
		},
		"greater_number": {
			input:     `labels.count:>5`,
			postgres:  `("labels"->>'count')::numeric > 5`,
			mysql:     "JSON_UNQUOTE(JSON_EXTRACT(`labels`, '$.count')) > 5",
			sqlite:    `json_extract("labels", '$.count') > 5`,
			pgParams:  []any{5},
			sqlParams: []any{5},
		},
		"less_eq_number": {
			input:     `labels.count:<=5`,
			postgres:  `("labels"->>'count')::numeric <= 5`,
			mysql:     "JSON_UNQUOTE(JSON_EXTRACT(`labels`, '$.count')) <= 5",
			sqlite:    `json_extract("labels", '$.count') <= 5`,
			pgParams:  []any{5},
			sqlParams: []any{5},
		},
		"greater_string": {
			input:     `labels.name:>m`,
			postgres:  `"labels"->>'name' > 'm'`,
			mysql:     "JSON_UNQUOTE(JSON_EXTRACT(`labels`, '$.name')) > 'm'",
			sqlite:    `json_extract("labels", '$.name') > 'm'`,
			pgParams:  []any{"m"},
			sqlParams: []any{"m"},
		},
		"range_number": {
			input:     `labels.count:[1 TO 10]`,
			postgres:  `("labels"->>'count')::numeric >= 1 AND ("labels"->>'count')::numeric <= 10`,
			mysql:     "JSON_UNQUOTE(JSON_EXTRACT(`labels`, '$.count')) >= 1 AND JSON_UNQUOTE(JSON_EXTRACT(`labels`, '$.count')) <= 10",
			sqlite:    `json_extract("labels", '$.count') >= 1 AND json_extract("labels", '$.count') <= 10`,
			pgParams:  []any{1, 10},
			sqlParams: []any{1, 10},
		},
		"range_open_number": {
			input:     `labels.count:{5 TO *}`,
			postgres:  `("labels"->>'count')::numeric > 5`,
			mysql:     "JSON_UNQUOTE(JSON_EXTRACT(`labels`, '$.count')) > 5",
			sqlite:    `json_extract("labels", '$.count') > 5`,
			pgParams:  []any{5},
			sqlParams: []any{5},
		},
		"range_string": {
			input:     `labels.name:[a TO m]`,
			postgres:  `"labels"->>'name' BETWEEN 'a' AND 'm'`,
			mysql:     "JSON_UNQUOTE(JSON_EXTRACT(`labels`, '$.name')) BETWEEN 'a' AND 'm'",
			sqlite:    `json_extract("labels", '$.name') BETWEEN 'a' AND 'm'`,
			pgParams:  []any{"a", "m"},
			sqlParams: []any{"a", "m"},
		},
		"in_list": {
			input:     `labels.category:(fruit OR veg)`,
			postgres:  `"labels"->>'category' IN ('fruit', 'veg')`,
			mysql:     "JSON_UNQUOTE(JSON_EXTRACT(`labels`, '$.category')) IN ('fruit', 'veg')",
			sqlite:    `json_extract("labels", '$.category') IN ('fruit', 'veg')`,
			pgParams:  []any{"fruit", "veg"},
			sqlParams: []any{"fruit", "veg"},
		},
		"in_list_numbers": {
			input:     `labels.count:(1 OR 2)`,
			postgres:  `("labels"->>'count')::numeric IN (1, 2)`,
			mysql:     "JSON_UNQUOTE(JSON_EXTRACT(`labels`, '$.count')) IN (1, 2)",
			sqlite:    `json_extract("labels", '$.count') IN (1, 2)`,
			pgParams:  []any{1, 2},
			sqlParams: []any{1, 2},
		},
		"in_list_with_null": {
			input:     `labels.category:(fruit OR null)`,
			postgres:  `("labels"->>'category' = 'fruit' OR "labels"->>'category' IS NULL)`,
			mysql:     "(JSON_UNQUOTE(JSON_EXTRACT(`labels`, '$.category')) = 'fruit' OR JSON_UNQUOTE(JSON_EXTRACT(`labels`, '$.category')) IS NULL)",
			sqlite:    `(json_extract("labels", '$.category') = 'fruit' OR json_extract("labels", '$.category') IS NULL)`,
			pgParams:  []any{"fruit"},
			sqlParams: []any{"fruit"},
		},
		"like_wildcard": {
			input:       `labels.category:fru*`,
			postgres:    `"labels"->>'category' SIMILAR TO 'fru%'`,
			mysql:       "JSON_UNQUOTE(JSON_EXTRACT(`labels`, '$.category')) LIKE 'fru%' ESCAPE '#'",
			sqlite:      `json_extract("labels", '$.category') GLOB 'fru*'`,
			pgParams:    []any{"fru%"},
			sqlParams:   []any{"fru*"},
			mysqlParams: []any{"fru%"},
		},
		"combined_with_plain_column": {
			input:     `name:apple AND labels.category:fruit`,
			postgres:  `("name" = 'apple') AND ("labels"->>'category' = 'fruit')`,
			mysql:     "(`name` = 'apple') AND (JSON_UNQUOTE(JSON_EXTRACT(`labels`, '$.category')) = 'fruit')",
			sqlite:    `("name" = 'apple') AND (json_extract("labels", '$.category') = 'fruit')`,
			pgParams:  []any{"apple", "fruit"},
			sqlParams: []any{"apple", "fruit"},
		},
		"registered_column_without_dot": {
			input:     `labels:fruit`,
			postgres:  `"labels" = 'fruit'`,
			mysql:     "`labels` = 'fruit'",
			sqlite:    `"labels" = 'fruit'`,
			pgParams:  []any{"fruit"},
			sqlParams: []any{"fruit"},
		},
		"unregistered_dotted_column": {
			input:     `other.category:fruit`,
			postgres:  `"other.category" = 'fruit'`,
			mysql:     "`other.category` = 'fruit'",
			sqlite:    `"other.category" = 'fruit'`,
			pgParams:  []any{"fruit"},
			sqlParams: []any{"fruit"},
		},
	}

	opt := WithJSONFields("labels", "metadata")
	for name, tc := range tcs {
		t.Run(name, func(t *testing.T) {
			renders := []struct {
				dialect string
				fn      func(string, ...Opt) (string, error)
				want    string
			}{
				{"postgres", ToPostgres, tc.postgres},
				{"mysql", ToMySQL, tc.mysql},
				{"sqlite", ToSQLite, tc.sqlite},
			}
			for _, r := range renders {
				got, err := r.fn(tc.input, opt)
				if err != nil {
					t.Fatalf("%s: unexpected error: %v", r.dialect, err)
				}
				if got != r.want {
					t.Errorf("%s:\n got: %s\nwant: %s", r.dialect, got, r.want)
				}
			}

			mysqlParams := tc.sqlParams
			if tc.mysqlParams != nil {
				mysqlParams = tc.mysqlParams
			}
			params := []struct {
				dialect string
				fn      func(string, ...Opt) (string, []any, error)
				want    []any
			}{
				{"postgres", ToParameterizedPostgres, tc.pgParams},
				{"mysql", ToParameterizedMySQL, mysqlParams},
				{"sqlite", ToParameterizedSQLite, tc.sqlParams},
			}
			for _, p := range params {
				_, got, err := p.fn(tc.input, opt)
				if err != nil {
					t.Fatalf("%s parameterized: unexpected error: %v", p.dialect, err)
				}
				if len(got) == 0 && len(p.want) == 0 {
					continue
				}
				if !reflect.DeepEqual(got, p.want) {
					t.Errorf("%s parameterized params:\n got: %#v\nwant: %#v", p.dialect, got, p.want)
				}
			}
		})
	}
}

func TestJSONPathParameterizedPostgres(t *testing.T) {
	got, params, err := ToParameterizedPostgres(`labels.count:>5 AND labels.category:fruit`, WithJSONFields("labels"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := `(("labels"->>'count')::numeric > $1) AND ("labels"->>'category' = $2)`
	if got != want {
		t.Errorf("\n got: %s\nwant: %s", got, want)
	}
	if !reflect.DeepEqual(params, []any{5, "fruit"}) {
		t.Errorf("params: got %#v", params)
	}
}

func TestJSONPathWithoutOptionKeepsDottedColumn(t *testing.T) {
	got, err := ToPostgres(`labels.category:fruit`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if want := `"labels.category" = 'fruit'`; got != want {
		t.Errorf("\n got: %s\nwant: %s", got, want)
	}
}

func TestJSONPathStringRoundTrip(t *testing.T) {
	e, err := Parse(`labels.category:fruit`, WithJSONFields("labels"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got, want := e.String(), `labels.category:fruit`; got != want {
		t.Errorf("String(): got %s, want %s", got, want)
	}
}

func TestJSONPathEscapedQuoteInQuery(t *testing.T) {
	opt := WithJSONFields("labels")
	tcs := []struct {
		dialect string
		fn      func(string, ...Opt) (string, error)
		want    string
	}{
		{"postgres", ToPostgres, `"labels"->>'it''s' = 'x'`},
		{"mysql", ToMySQL, "JSON_UNQUOTE(JSON_EXTRACT(`labels`, '$.\"it''s\"')) = 'x'"},
		{"sqlite", ToSQLite, `json_extract("labels", '$."it''s"') = 'x'`},
	}
	for _, tc := range tcs {
		got, err := tc.fn(`labels.it\'s:x`, opt)
		if err != nil {
			t.Fatalf("%s: unexpected error: %v", tc.dialect, err)
		}
		if got != tc.want {
			t.Errorf("%s:\n got: %s\nwant: %s", tc.dialect, got, tc.want)
		}
	}

	if _, err := ToPostgres(`labels.a\"b:x`, opt); err == nil {
		t.Errorf("expected an error for a double quote in a json path segment")
	}
}
