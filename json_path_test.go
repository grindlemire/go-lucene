package lucene

import (
	"encoding/json"
	"math"
	"reflect"
	"strings"
	"testing"

	"github.com/grindlemire/go-lucene/pkg/driver"
	"github.com/grindlemire/go-lucene/pkg/lucene/expr"
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
			mysql:     "CASE WHEN JSON_TYPE(JSON_EXTRACT(`labels`, '$.category')) <> 'NULL' THEN JSON_UNQUOTE(JSON_EXTRACT(`labels`, '$.category')) END = 'fruit'",
			sqlite:    `json_extract("labels", '$.category') = 'fruit'`,
			pgParams:  []any{"fruit"},
			sqlParams: []any{"fruit"},
		},
		"nested_path": {
			input:     `metadata.address.city:Boston`,
			postgres:  `"metadata"->'address'->>'city' = 'Boston'`,
			mysql:     "CASE WHEN JSON_TYPE(JSON_EXTRACT(`metadata`, '$.address.city')) <> 'NULL' THEN JSON_UNQUOTE(JSON_EXTRACT(`metadata`, '$.address.city')) END = 'Boston'",
			sqlite:    `json_extract("metadata", '$.address.city') = 'Boston'`,
			pgParams:  []any{"Boston"},
			sqlParams: []any{"Boston"},
		},
		"null": {
			input:    `labels.category:null`,
			postgres: `"labels"->>'category' IS NULL`,
			mysql:    "CASE WHEN JSON_TYPE(JSON_EXTRACT(`labels`, '$.category')) <> 'NULL' THEN JSON_UNQUOTE(JSON_EXTRACT(`labels`, '$.category')) END IS NULL",
			sqlite:   `CASE json_type("labels", '$.category') WHEN 'true' THEN 'true' WHEN 'false' THEN 'false' ELSE CAST(json_extract("labels", '$.category') AS TEXT) END IS NULL`,
		},
		"not_null": {
			input:    `NOT labels.category:null`,
			postgres: `"labels"->>'category' IS NOT NULL`,
			mysql:    "CASE WHEN JSON_TYPE(JSON_EXTRACT(`labels`, '$.category')) <> 'NULL' THEN JSON_UNQUOTE(JSON_EXTRACT(`labels`, '$.category')) END IS NOT NULL",
			sqlite:   `CASE json_type("labels", '$.category') WHEN 'true' THEN 'true' WHEN 'false' THEN 'false' ELSE CAST(json_extract("labels", '$.category') AS TEXT) END IS NOT NULL`,
		},
		"equals_number": {
			input:     `labels.count:5`,
			postgres:  `CASE WHEN json_typeof(("labels"->'count')::json) = 'number' AND length("labels"->>'count') <= 1000 AND "labels"->>'count' ~ '^-?[0-9]+([.][0-9]+)?([eE][+-]?[0-9]{1,4})?$' THEN ("labels"->>'count')::numeric = 5 ELSE "labels"->>'count' = '5' END`,
			mysql:     "CASE WHEN JSON_TYPE(JSON_EXTRACT(`labels`, '$.count')) IN ('INTEGER', 'UNSIGNED INTEGER', 'DOUBLE', 'DECIMAL') THEN JSON_EXTRACT(`labels`, '$.count') = 5 ELSE CASE WHEN JSON_TYPE(JSON_EXTRACT(`labels`, '$.count')) <> 'NULL' THEN JSON_UNQUOTE(JSON_EXTRACT(`labels`, '$.count')) END = '5' END",
			sqlite:    `CASE WHEN json_type("labels", '$.count') IN ('integer', 'real') THEN json_extract("labels", '$.count') = 5 ELSE CASE json_type("labels", '$.count') WHEN 'true' THEN 'true' WHEN 'false' THEN 'false' ELSE CAST(json_extract("labels", '$.count') AS TEXT) END = '5' END`,
			pgParams:  []any{5, "5"},
			sqlParams: []any{5, "5"},
		},
		"greater_number": {
			input:     `labels.count:>5`,
			postgres:  `CASE WHEN json_typeof(("labels"->'count')::json) = 'number' AND length("labels"->>'count') <= 1000 AND "labels"->>'count' ~ '^-?[0-9]+([.][0-9]+)?([eE][+-]?[0-9]{1,4})?$' THEN ("labels"->>'count')::numeric > 5 WHEN "labels"->>'count' IS NOT NULL THEN FALSE END`,
			mysql:     "CASE WHEN JSON_TYPE(JSON_EXTRACT(`labels`, '$.count')) IN ('INTEGER', 'UNSIGNED INTEGER', 'DOUBLE', 'DECIMAL') THEN JSON_EXTRACT(`labels`, '$.count') > 5 WHEN CASE WHEN JSON_TYPE(JSON_EXTRACT(`labels`, '$.count')) <> 'NULL' THEN JSON_UNQUOTE(JSON_EXTRACT(`labels`, '$.count')) END IS NOT NULL THEN FALSE END",
			sqlite:    `CASE WHEN json_type("labels", '$.count') IN ('integer', 'real') THEN json_extract("labels", '$.count') > 5 WHEN CASE json_type("labels", '$.count') WHEN 'true' THEN 'true' WHEN 'false' THEN 'false' ELSE CAST(json_extract("labels", '$.count') AS TEXT) END IS NOT NULL THEN FALSE END`,
			pgParams:  []any{5},
			sqlParams: []any{5},
		},
		"less_eq_number": {
			input:     `labels.count:<=5`,
			postgres:  `CASE WHEN json_typeof(("labels"->'count')::json) = 'number' AND length("labels"->>'count') <= 1000 AND "labels"->>'count' ~ '^-?[0-9]+([.][0-9]+)?([eE][+-]?[0-9]{1,4})?$' THEN ("labels"->>'count')::numeric <= 5 WHEN "labels"->>'count' IS NOT NULL THEN FALSE END`,
			mysql:     "CASE WHEN JSON_TYPE(JSON_EXTRACT(`labels`, '$.count')) IN ('INTEGER', 'UNSIGNED INTEGER', 'DOUBLE', 'DECIMAL') THEN JSON_EXTRACT(`labels`, '$.count') <= 5 WHEN CASE WHEN JSON_TYPE(JSON_EXTRACT(`labels`, '$.count')) <> 'NULL' THEN JSON_UNQUOTE(JSON_EXTRACT(`labels`, '$.count')) END IS NOT NULL THEN FALSE END",
			sqlite:    `CASE WHEN json_type("labels", '$.count') IN ('integer', 'real') THEN json_extract("labels", '$.count') <= 5 WHEN CASE json_type("labels", '$.count') WHEN 'true' THEN 'true' WHEN 'false' THEN 'false' ELSE CAST(json_extract("labels", '$.count') AS TEXT) END IS NOT NULL THEN FALSE END`,
			pgParams:  []any{5},
			sqlParams: []any{5},
		},
		"greater_string": {
			input:     `labels.name:>m`,
			postgres:  `"labels"->>'name' > 'm'`,
			mysql:     "CASE WHEN JSON_TYPE(JSON_EXTRACT(`labels`, '$.name')) <> 'NULL' THEN JSON_UNQUOTE(JSON_EXTRACT(`labels`, '$.name')) END > 'm'",
			sqlite:    `CASE json_type("labels", '$.name') WHEN 'true' THEN 'true' WHEN 'false' THEN 'false' ELSE CAST(json_extract("labels", '$.name') AS TEXT) END > 'm'`,
			pgParams:  []any{"m"},
			sqlParams: []any{"m"},
		},
		"range_number": {
			input:     `labels.count:[1 TO 10]`,
			postgres:  `CASE WHEN json_typeof(("labels"->'count')::json) = 'number' AND length("labels"->>'count') <= 1000 AND "labels"->>'count' ~ '^-?[0-9]+([.][0-9]+)?([eE][+-]?[0-9]{1,4})?$' THEN ("labels"->>'count')::numeric >= 1 AND ("labels"->>'count')::numeric <= 10 WHEN "labels"->>'count' IS NOT NULL THEN FALSE END`,
			mysql:     "CASE WHEN JSON_TYPE(JSON_EXTRACT(`labels`, '$.count')) IN ('INTEGER', 'UNSIGNED INTEGER', 'DOUBLE', 'DECIMAL') THEN JSON_EXTRACT(`labels`, '$.count') >= 1 AND JSON_EXTRACT(`labels`, '$.count') <= 10 WHEN CASE WHEN JSON_TYPE(JSON_EXTRACT(`labels`, '$.count')) <> 'NULL' THEN JSON_UNQUOTE(JSON_EXTRACT(`labels`, '$.count')) END IS NOT NULL THEN FALSE END",
			sqlite:    `CASE WHEN json_type("labels", '$.count') IN ('integer', 'real') THEN json_extract("labels", '$.count') >= 1 AND json_extract("labels", '$.count') <= 10 WHEN CASE json_type("labels", '$.count') WHEN 'true' THEN 'true' WHEN 'false' THEN 'false' ELSE CAST(json_extract("labels", '$.count') AS TEXT) END IS NOT NULL THEN FALSE END`,
			pgParams:  []any{1, 10},
			sqlParams: []any{1, 10},
		},
		"range_open_number": {
			input:     `labels.count:{5 TO *}`,
			postgres:  `CASE WHEN json_typeof(("labels"->'count')::json) = 'number' AND length("labels"->>'count') <= 1000 AND "labels"->>'count' ~ '^-?[0-9]+([.][0-9]+)?([eE][+-]?[0-9]{1,4})?$' THEN ("labels"->>'count')::numeric > 5 WHEN "labels"->>'count' IS NOT NULL THEN FALSE END`,
			mysql:     "CASE WHEN JSON_TYPE(JSON_EXTRACT(`labels`, '$.count')) IN ('INTEGER', 'UNSIGNED INTEGER', 'DOUBLE', 'DECIMAL') THEN JSON_EXTRACT(`labels`, '$.count') > 5 WHEN CASE WHEN JSON_TYPE(JSON_EXTRACT(`labels`, '$.count')) <> 'NULL' THEN JSON_UNQUOTE(JSON_EXTRACT(`labels`, '$.count')) END IS NOT NULL THEN FALSE END",
			sqlite:    `CASE WHEN json_type("labels", '$.count') IN ('integer', 'real') THEN json_extract("labels", '$.count') > 5 WHEN CASE json_type("labels", '$.count') WHEN 'true' THEN 'true' WHEN 'false' THEN 'false' ELSE CAST(json_extract("labels", '$.count') AS TEXT) END IS NOT NULL THEN FALSE END`,
			pgParams:  []any{5},
			sqlParams: []any{5},
		},
		"range_string": {
			input:     `labels.name:[a TO m]`,
			postgres:  `"labels"->>'name' BETWEEN 'a' AND 'm'`,
			mysql:     "CASE WHEN JSON_TYPE(JSON_EXTRACT(`labels`, '$.name')) <> 'NULL' THEN JSON_UNQUOTE(JSON_EXTRACT(`labels`, '$.name')) END BETWEEN 'a' AND 'm'",
			sqlite:    `CASE json_type("labels", '$.name') WHEN 'true' THEN 'true' WHEN 'false' THEN 'false' ELSE CAST(json_extract("labels", '$.name') AS TEXT) END BETWEEN 'a' AND 'm'`,
			pgParams:  []any{"a", "m"},
			sqlParams: []any{"a", "m"},
		},
		"in_list": {
			input:     `labels.category:(fruit OR veg)`,
			postgres:  `"labels"->>'category' IN ('fruit', 'veg')`,
			mysql:     "CASE WHEN JSON_TYPE(JSON_EXTRACT(`labels`, '$.category')) <> 'NULL' THEN JSON_UNQUOTE(JSON_EXTRACT(`labels`, '$.category')) END IN ('fruit', 'veg')",
			sqlite:    `json_extract("labels", '$.category') IN ('fruit', 'veg')`,
			pgParams:  []any{"fruit", "veg"},
			sqlParams: []any{"fruit", "veg"},
		},
		"in_list_numbers": {
			input:     `labels.count:(1 OR 2)`,
			postgres:  `CASE WHEN json_typeof(("labels"->'count')::json) = 'number' AND length("labels"->>'count') <= 1000 AND "labels"->>'count' ~ '^-?[0-9]+([.][0-9]+)?([eE][+-]?[0-9]{1,4})?$' THEN ("labels"->>'count')::numeric IN (1, 2) ELSE "labels"->>'count' IN ('1', '2') END`,
			mysql:     "CASE WHEN JSON_TYPE(JSON_EXTRACT(`labels`, '$.count')) IN ('INTEGER', 'UNSIGNED INTEGER', 'DOUBLE', 'DECIMAL') THEN JSON_EXTRACT(`labels`, '$.count') IN (1, 2) ELSE CASE WHEN JSON_TYPE(JSON_EXTRACT(`labels`, '$.count')) <> 'NULL' THEN JSON_UNQUOTE(JSON_EXTRACT(`labels`, '$.count')) END IN ('1', '2') END",
			sqlite:    `CASE WHEN json_type("labels", '$.count') IN ('integer', 'real') THEN json_extract("labels", '$.count') IN (1, 2) ELSE CASE json_type("labels", '$.count') WHEN 'true' THEN 'true' WHEN 'false' THEN 'false' ELSE CAST(json_extract("labels", '$.count') AS TEXT) END IN ('1', '2') END`,
			pgParams:  []any{1, 2, "1", "2"},
			sqlParams: []any{1, 2, "1", "2"},
		},
		"in_list_numbers_with_null": {
			input:     `labels.count:(1 OR null)`,
			postgres:  `CASE WHEN json_typeof(("labels"->'count')::json) = 'number' AND length("labels"->>'count') <= 1000 AND "labels"->>'count' ~ '^-?[0-9]+([.][0-9]+)?([eE][+-]?[0-9]{1,4})?$' THEN (("labels"->>'count')::numeric = 1 OR "labels"->>'count' IS NULL) ELSE ("labels"->>'count' = '1' OR "labels"->>'count' IS NULL) END`,
			mysql:     "CASE WHEN JSON_TYPE(JSON_EXTRACT(`labels`, '$.count')) IN ('INTEGER', 'UNSIGNED INTEGER', 'DOUBLE', 'DECIMAL') THEN (JSON_EXTRACT(`labels`, '$.count') = 1 OR CASE WHEN JSON_TYPE(JSON_EXTRACT(`labels`, '$.count')) <> 'NULL' THEN JSON_UNQUOTE(JSON_EXTRACT(`labels`, '$.count')) END IS NULL) ELSE (CASE WHEN JSON_TYPE(JSON_EXTRACT(`labels`, '$.count')) <> 'NULL' THEN JSON_UNQUOTE(JSON_EXTRACT(`labels`, '$.count')) END = '1' OR CASE WHEN JSON_TYPE(JSON_EXTRACT(`labels`, '$.count')) <> 'NULL' THEN JSON_UNQUOTE(JSON_EXTRACT(`labels`, '$.count')) END IS NULL) END",
			sqlite:    `CASE WHEN json_type("labels", '$.count') IN ('integer', 'real') THEN (json_extract("labels", '$.count') = 1 OR CASE json_type("labels", '$.count') WHEN 'true' THEN 'true' WHEN 'false' THEN 'false' ELSE CAST(json_extract("labels", '$.count') AS TEXT) END IS NULL) ELSE (CASE json_type("labels", '$.count') WHEN 'true' THEN 'true' WHEN 'false' THEN 'false' ELSE CAST(json_extract("labels", '$.count') AS TEXT) END = '1' OR CASE json_type("labels", '$.count') WHEN 'true' THEN 'true' WHEN 'false' THEN 'false' ELSE CAST(json_extract("labels", '$.count') AS TEXT) END IS NULL) END`,
			pgParams:  []any{1, "1"},
			sqlParams: []any{1, "1"},
		},
		"range_mixed_bounds": {
			input:     `labels.x:[1 TO m]`,
			postgres:  `"labels"->>'x' BETWEEN '1' AND 'm'`,
			mysql:     "CASE WHEN JSON_TYPE(JSON_EXTRACT(`labels`, '$.x')) <> 'NULL' THEN JSON_UNQUOTE(JSON_EXTRACT(`labels`, '$.x')) END BETWEEN '1' AND 'm'",
			sqlite:    `CASE json_type("labels", '$.x') WHEN 'true' THEN 'true' WHEN 'false' THEN 'false' ELSE CAST(json_extract("labels", '$.x') AS TEXT) END BETWEEN '1' AND 'm'`,
			pgParams:  []any{"1", "m"},
			sqlParams: []any{"1", "m"},
		},
		"in_list_mixed": {
			input:     `labels.x:(1 OR abc)`,
			postgres:  `"labels"->>'x' IN ('1', 'abc')`,
			mysql:     "CASE WHEN JSON_TYPE(JSON_EXTRACT(`labels`, '$.x')) <> 'NULL' THEN JSON_UNQUOTE(JSON_EXTRACT(`labels`, '$.x')) END IN ('1', 'abc')",
			sqlite:    `CASE json_type("labels", '$.x') WHEN 'true' THEN 'true' WHEN 'false' THEN 'false' ELSE CAST(json_extract("labels", '$.x') AS TEXT) END IN ('1', 'abc')`,
			pgParams:  []any{"1", "abc"},
			sqlParams: []any{"1", "abc"},
		},
		"in_list_with_null": {
			input:     `labels.category:(fruit OR null)`,
			postgres:  `("labels"->>'category' = 'fruit' OR "labels"->>'category' IS NULL)`,
			mysql:     "(CASE WHEN JSON_TYPE(JSON_EXTRACT(`labels`, '$.category')) <> 'NULL' THEN JSON_UNQUOTE(JSON_EXTRACT(`labels`, '$.category')) END = 'fruit' OR CASE WHEN JSON_TYPE(JSON_EXTRACT(`labels`, '$.category')) <> 'NULL' THEN JSON_UNQUOTE(JSON_EXTRACT(`labels`, '$.category')) END IS NULL)",
			sqlite:    `(json_extract("labels", '$.category') = 'fruit' OR json_extract("labels", '$.category') IS NULL)`,
			pgParams:  []any{"fruit"},
			sqlParams: []any{"fruit"},
		},
		"like_wildcard": {
			input:       `labels.category:fru*`,
			postgres:    `"labels"->>'category' SIMILAR TO 'fru%'`,
			mysql:       "CASE WHEN JSON_TYPE(JSON_EXTRACT(`labels`, '$.category')) <> 'NULL' THEN JSON_UNQUOTE(JSON_EXTRACT(`labels`, '$.category')) END LIKE 'fru%' ESCAPE '#'",
			sqlite:      `CASE json_type("labels", '$.category') WHEN 'true' THEN 'true' WHEN 'false' THEN 'false' ELSE CAST(json_extract("labels", '$.category') AS TEXT) END GLOB 'fru*'`,
			pgParams:    []any{"fru%"},
			sqlParams:   []any{"fru*"},
			mysqlParams: []any{"fru%"},
		},
		"regex": {
			input:     `labels.color:/re.*/`,
			postgres:  `"labels"->>'color' ~ 're.*'`,
			mysql:     "CASE WHEN JSON_TYPE(JSON_EXTRACT(`labels`, '$.color')) <> 'NULL' THEN JSON_UNQUOTE(JSON_EXTRACT(`labels`, '$.color')) END REGEXP 're.*'",
			sqlite:    `CASE json_type("labels", '$.color') WHEN 'true' THEN 'true' WHEN 'false' THEN 'false' ELSE CAST(json_extract("labels", '$.color') AS TEXT) END REGEXP 're.*'`,
			pgParams:  []any{"re.*"},
			sqlParams: []any{"re.*"},
		},
		"not_greater_number": {
			input:     `-labels.count:>5`,
			postgres:  `NOT(CASE WHEN json_typeof(("labels"->'count')::json) = 'number' AND length("labels"->>'count') <= 1000 AND "labels"->>'count' ~ '^-?[0-9]+([.][0-9]+)?([eE][+-]?[0-9]{1,4})?$' THEN ("labels"->>'count')::numeric > 5 WHEN "labels"->>'count' IS NOT NULL THEN FALSE END)`,
			mysql:     "NOT(CASE WHEN JSON_TYPE(JSON_EXTRACT(`labels`, '$.count')) IN ('INTEGER', 'UNSIGNED INTEGER', 'DOUBLE', 'DECIMAL') THEN JSON_EXTRACT(`labels`, '$.count') > 5 WHEN CASE WHEN JSON_TYPE(JSON_EXTRACT(`labels`, '$.count')) <> 'NULL' THEN JSON_UNQUOTE(JSON_EXTRACT(`labels`, '$.count')) END IS NOT NULL THEN FALSE END)",
			sqlite:    `NOT(CASE WHEN json_type("labels", '$.count') IN ('integer', 'real') THEN json_extract("labels", '$.count') > 5 WHEN CASE json_type("labels", '$.count') WHEN 'true' THEN 'true' WHEN 'false' THEN 'false' ELSE CAST(json_extract("labels", '$.count') AS TEXT) END IS NOT NULL THEN FALSE END)`,
			pgParams:  []any{5},
			sqlParams: []any{5},
		},
		"combined_with_plain_column": {
			input:     `name:apple AND labels.category:fruit`,
			postgres:  `("name" = 'apple') AND ("labels"->>'category' = 'fruit')`,
			mysql:     "(`name` = 'apple') AND (CASE WHEN JSON_TYPE(JSON_EXTRACT(`labels`, '$.category')) <> 'NULL' THEN JSON_UNQUOTE(JSON_EXTRACT(`labels`, '$.category')) END = 'fruit')",
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
		"number_keeps_written_text": {
			input:     `labels.v:1.0`,
			postgres:  `CASE WHEN json_typeof(("labels"->'v')::json) = 'number' AND length("labels"->>'v') <= 1000 AND "labels"->>'v' ~ '^-?[0-9]+([.][0-9]+)?([eE][+-]?[0-9]{1,4})?$' THEN ("labels"->>'v')::numeric = 1 ELSE "labels"->>'v' = '1.0' END`,
			mysql:     "CASE WHEN JSON_TYPE(JSON_EXTRACT(`labels`, '$.v')) IN ('INTEGER', 'UNSIGNED INTEGER', 'DOUBLE', 'DECIMAL') THEN JSON_EXTRACT(`labels`, '$.v') = 1 ELSE CASE WHEN JSON_TYPE(JSON_EXTRACT(`labels`, '$.v')) <> 'NULL' THEN JSON_UNQUOTE(JSON_EXTRACT(`labels`, '$.v')) END = '1.0' END",
			sqlite:    `CASE WHEN json_type("labels", '$.v') IN ('integer', 'real') THEN json_extract("labels", '$.v') = 1 ELSE CASE json_type("labels", '$.v') WHEN 'true' THEN 'true' WHEN 'false' THEN 'false' ELSE CAST(json_extract("labels", '$.v') AS TEXT) END = '1.0' END`,
			pgParams:  []any{1.0, "1.0"},
			sqlParams: []any{1.0, "1.0"},
		},
		"leading_zero_keeps_written_text": {
			input:     `labels.v:02134`,
			postgres:  `CASE WHEN json_typeof(("labels"->'v')::json) = 'number' AND length("labels"->>'v') <= 1000 AND "labels"->>'v' ~ '^-?[0-9]+([.][0-9]+)?([eE][+-]?[0-9]{1,4})?$' THEN ("labels"->>'v')::numeric = 2134 ELSE "labels"->>'v' = '02134' END`,
			mysql:     "CASE WHEN JSON_TYPE(JSON_EXTRACT(`labels`, '$.v')) IN ('INTEGER', 'UNSIGNED INTEGER', 'DOUBLE', 'DECIMAL') THEN JSON_EXTRACT(`labels`, '$.v') = 2134 ELSE CASE WHEN JSON_TYPE(JSON_EXTRACT(`labels`, '$.v')) <> 'NULL' THEN JSON_UNQUOTE(JSON_EXTRACT(`labels`, '$.v')) END = '02134' END",
			sqlite:    `CASE WHEN json_type("labels", '$.v') IN ('integer', 'real') THEN json_extract("labels", '$.v') = 2134 ELSE CASE json_type("labels", '$.v') WHEN 'true' THEN 'true' WHEN 'false' THEN 'false' ELSE CAST(json_extract("labels", '$.v') AS TEXT) END = '02134' END`,
			pgParams:  []any{2134, "02134"},
			sqlParams: []any{2134, "02134"},
		},
		"exponent_keeps_written_text": {
			input:     `labels.v:1e3`,
			postgres:  `CASE WHEN json_typeof(("labels"->'v')::json) = 'number' AND length("labels"->>'v') <= 1000 AND "labels"->>'v' ~ '^-?[0-9]+([.][0-9]+)?([eE][+-]?[0-9]{1,4})?$' THEN ("labels"->>'v')::numeric = 1000 ELSE "labels"->>'v' = '1e3' END`,
			mysql:     "CASE WHEN JSON_TYPE(JSON_EXTRACT(`labels`, '$.v')) IN ('INTEGER', 'UNSIGNED INTEGER', 'DOUBLE', 'DECIMAL') THEN JSON_EXTRACT(`labels`, '$.v') = 1000 ELSE CASE WHEN JSON_TYPE(JSON_EXTRACT(`labels`, '$.v')) <> 'NULL' THEN JSON_UNQUOTE(JSON_EXTRACT(`labels`, '$.v')) END = '1e3' END",
			sqlite:    `CASE WHEN json_type("labels", '$.v') IN ('integer', 'real') THEN json_extract("labels", '$.v') = 1000 ELSE CASE json_type("labels", '$.v') WHEN 'true' THEN 'true' WHEN 'false' THEN 'false' ELSE CAST(json_extract("labels", '$.v') AS TEXT) END = '1e3' END`,
			pgParams:  []any{1000.0, "1e3"},
			sqlParams: []any{1000.0, "1e3"},
		},
		"mixed_range_keeps_written_text": {
			input:     `labels.v:[2.10 TO abc]`,
			postgres:  `"labels"->>'v' BETWEEN '2.10' AND 'abc'`,
			mysql:     "CASE WHEN JSON_TYPE(JSON_EXTRACT(`labels`, '$.v')) <> 'NULL' THEN JSON_UNQUOTE(JSON_EXTRACT(`labels`, '$.v')) END BETWEEN '2.10' AND 'abc'",
			sqlite:    `CASE json_type("labels", '$.v') WHEN 'true' THEN 'true' WHEN 'false' THEN 'false' ELSE CAST(json_extract("labels", '$.v') AS TEXT) END BETWEEN '2.10' AND 'abc'`,
			pgParams:  []any{"2.10", "abc"},
			sqlParams: []any{"2.10", "abc"},
		},
		"mixed_in_keeps_written_text": {
			input:     `labels.v:(1.50 OR abc)`,
			postgres:  `"labels"->>'v' IN ('1.50', 'abc')`,
			mysql:     "CASE WHEN JSON_TYPE(JSON_EXTRACT(`labels`, '$.v')) <> 'NULL' THEN JSON_UNQUOTE(JSON_EXTRACT(`labels`, '$.v')) END IN ('1.50', 'abc')",
			sqlite:    `CASE json_type("labels", '$.v') WHEN 'true' THEN 'true' WHEN 'false' THEN 'false' ELSE CAST(json_extract("labels", '$.v') AS TEXT) END IN ('1.50', 'abc')`,
			pgParams:  []any{"1.50", "abc"},
			sqlParams: []any{"1.50", "abc"},
		},
		"quoted_number_reads_text_form": {
			input:     `labels.v:"5"`,
			postgres:  `"labels"->>'v' = '5'`,
			mysql:     "CASE WHEN JSON_TYPE(JSON_EXTRACT(`labels`, '$.v')) <> 'NULL' THEN JSON_UNQUOTE(JSON_EXTRACT(`labels`, '$.v')) END = '5'",
			sqlite:    `CASE json_type("labels", '$.v') WHEN 'true' THEN 'true' WHEN 'false' THEN 'false' ELSE CAST(json_extract("labels", '$.v') AS TEXT) END = '5'`,
			pgParams:  []any{"5"},
			sqlParams: []any{"5"},
		},
		"quoted_null_reads_text_form": {
			input:     `labels.v:"null"`,
			postgres:  `"labels"->>'v' = 'null'`,
			mysql:     "CASE WHEN JSON_TYPE(JSON_EXTRACT(`labels`, '$.v')) <> 'NULL' THEN JSON_UNQUOTE(JSON_EXTRACT(`labels`, '$.v')) END = 'null'",
			sqlite:    `CASE json_type("labels", '$.v') WHEN 'true' THEN 'true' WHEN 'false' THEN 'false' ELSE CAST(json_extract("labels", '$.v') AS TEXT) END = 'null'`,
			pgParams:  []any{"null"},
			sqlParams: []any{"null"},
		},
		"bool_word_reads_text_form": {
			input:     `labels.v:TRUE`,
			postgres:  `"labels"->>'v' = 'TRUE'`,
			mysql:     "CASE WHEN JSON_TYPE(JSON_EXTRACT(`labels`, '$.v')) <> 'NULL' THEN JSON_UNQUOTE(JSON_EXTRACT(`labels`, '$.v')) END = 'TRUE'",
			sqlite:    `CASE json_type("labels", '$.v') WHEN 'true' THEN 'true' WHEN 'false' THEN 'false' ELSE CAST(json_extract("labels", '$.v') AS TEXT) END = 'TRUE'`,
			pgParams:  []any{"TRUE"},
			sqlParams: []any{"TRUE"},
		},
		"in_list_plain_text": {
			input:     `labels.color:(red OR blue)`,
			postgres:  `"labels"->>'color' IN ('red', 'blue')`,
			mysql:     "CASE WHEN JSON_TYPE(JSON_EXTRACT(`labels`, '$.color')) <> 'NULL' THEN JSON_UNQUOTE(JSON_EXTRACT(`labels`, '$.color')) END IN ('red', 'blue')",
			sqlite:    `json_extract("labels", '$.color') IN ('red', 'blue')`,
			pgParams:  []any{"red", "blue"},
			sqlParams: []any{"red", "blue"},
		},
		// A number that a float64 can't hold is written into the SQL, as on a
		// plain column. Bound as a string, MySQL and SQLite would compare it
		// with the JSON value as text.
		"equals_number_too_big_for_float": {
			input:     `labels.count:9223372036854775808`,
			postgres:  `CASE WHEN json_typeof(("labels"->'count')::json) = 'number' AND length("labels"->>'count') <= 1000 AND "labels"->>'count' ~ '^-?[0-9]+([.][0-9]+)?([eE][+-]?[0-9]{1,4})?$' THEN ("labels"->>'count')::numeric = 9223372036854775808 ELSE "labels"->>'count' = '9223372036854775808' END`,
			mysql:     "CASE WHEN JSON_TYPE(JSON_EXTRACT(`labels`, '$.count')) IN ('INTEGER', 'UNSIGNED INTEGER', 'DOUBLE', 'DECIMAL') THEN JSON_EXTRACT(`labels`, '$.count') = 9223372036854775808 ELSE CASE WHEN JSON_TYPE(JSON_EXTRACT(`labels`, '$.count')) <> 'NULL' THEN JSON_UNQUOTE(JSON_EXTRACT(`labels`, '$.count')) END = '9223372036854775808' END",
			sqlite:    `CASE WHEN json_type("labels", '$.count') IN ('integer', 'real') THEN json_extract("labels", '$.count') = 9223372036854775808 ELSE CASE json_type("labels", '$.count') WHEN 'true' THEN 'true' WHEN 'false' THEN 'false' ELSE CAST(json_extract("labels", '$.count') AS TEXT) END = '9223372036854775808' END`,
			pgParams:  []any{"9223372036854775808"},
			sqlParams: []any{"9223372036854775808"},
		},
		"range_number_too_big_for_float": {
			input:     `labels.count:[1 TO 9223372036854775808]`,
			postgres:  `CASE WHEN json_typeof(("labels"->'count')::json) = 'number' AND length("labels"->>'count') <= 1000 AND "labels"->>'count' ~ '^-?[0-9]+([.][0-9]+)?([eE][+-]?[0-9]{1,4})?$' THEN ("labels"->>'count')::numeric >= 1 AND ("labels"->>'count')::numeric <= 9223372036854775808 WHEN "labels"->>'count' IS NOT NULL THEN FALSE END`,
			mysql:     "CASE WHEN JSON_TYPE(JSON_EXTRACT(`labels`, '$.count')) IN ('INTEGER', 'UNSIGNED INTEGER', 'DOUBLE', 'DECIMAL') THEN JSON_EXTRACT(`labels`, '$.count') >= 1 AND JSON_EXTRACT(`labels`, '$.count') <= 9223372036854775808 WHEN CASE WHEN JSON_TYPE(JSON_EXTRACT(`labels`, '$.count')) <> 'NULL' THEN JSON_UNQUOTE(JSON_EXTRACT(`labels`, '$.count')) END IS NOT NULL THEN FALSE END",
			sqlite:    `CASE WHEN json_type("labels", '$.count') IN ('integer', 'real') THEN json_extract("labels", '$.count') >= 1 AND json_extract("labels", '$.count') <= 9223372036854775808 WHEN CASE json_type("labels", '$.count') WHEN 'true' THEN 'true' WHEN 'false' THEN 'false' ELSE CAST(json_extract("labels", '$.count') AS TEXT) END IS NOT NULL THEN FALSE END`,
			pgParams:  []any{1},
			sqlParams: []any{1},
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
	want := `(CASE WHEN json_typeof(("labels"->'count')::json) = 'number' AND length("labels"->>'count') <= 1000 AND "labels"->>'count' ~ '^-?[0-9]+([.][0-9]+)?([eE][+-]?[0-9]{1,4})?$' THEN ("labels"->>'count')::numeric > $1 WHEN "labels"->>'count' IS NOT NULL THEN FALSE END) AND ("labels"->>'category' = $2)`
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

// TestStringQuotesJSONPathFields checks that String() quotes a JSON-path
// field that would not lex back as one word. It used to print the dotted
// name as is, so labels.a\ b:x printed as labels.a b:x, which parses as
// labels.a AND b:x.
func TestStringQuotesJSONPathFields(t *testing.T) {
	runStringCases(t, map[string]stringCase{
		"plain":            {`labels.category:fruit`, `labels.category:fruit`},
		"nested":           {`metadata.address.city:Boston`, `metadata.address.city:Boston`},
		"escaped_space":    {`labels.a\ b:x`, `"labels.a b":x`},
		"quoted_space":     {`"labels.a b":x`, `"labels.a b":x`},
		"colon":            {`labels.a\:b:x`, `"labels.a:b":x`},
		"single_quote":     {`labels.it\'s:x`, `"labels.it's":x`},
		"brackets":         {`"labels.a[0]":x`, `"labels.a[0]":x`},
		"escaped_brackets": {`labels.a\[0\]:x`, `"labels.a[0]":x`},
		"wildcard_chars":   {`"labels.a*b?":x`, `"labels.a*b?":x`},
		"keyword_key":      {`labels.AND:x`, `labels.AND:x`},
		"number_key":       {`labels.5:x`, `labels.5:x`},
		"unicode_key":      {`labels.é:x`, `labels.é:x`},
		"spaced_column":    {`"my col.k":x`, `"my col.k":x`},
		"compare":          {`labels.a\ b:>5`, `"labels.a b":>5`},
		"range":            {`labels.a\ b:[1 TO 5]`, `"labels.a b":[1 TO 5]`},
		"in":               {`labels.a\ b:(x OR y)`, `"labels.a b":(x OR y)`},
		"like":             {`labels.a\ b:fo*`, `"labels.a b":fo*`},
		"null":             {`labels.a\ b:null`, `"labels.a b":null`},
		"field_group":      {`labels.a\ b:(x AND -y)`, `"labels.a b":x AND -"labels.a b":y`},
	}, WithJSONFields("labels", "metadata", "my col"))
}

// TestStringHandBuiltJSONPath checks that String() prints a JSON path built
// in code or read from JSON as a field that parses back to the same path. A
// key that contains a dot has no spelling in a query, so it prints with an
// escaped dot, which Parse rejects rather than reading another path.
func TestStringHandBuiltJSONPath(t *testing.T) {
	opt := WithJSONFields("labels", "my col")
	path := func(column string, keys ...string) *expr.Expression {
		return expr.Lit(expr.JSONPath{Column: expr.Column(column), Path: keys})
	}
	fromJSON := func(raw string) *expr.Expression {
		var e expr.Expression
		if err := json.Unmarshal([]byte(raw), &e); err != nil {
			t.Fatalf("unmarshal %s: %v", raw, err)
		}
		return &e
	}

	roundTrips := map[string]struct {
		input *expr.Expression
		want  string
	}{
		"plain":       {expr.Eq(path("labels", "k"), "x"), `labels.k:x`},
		"space":       {expr.Eq(path("labels", "a b"), "x"), `"labels.a b":x`},
		"nested":      {expr.Eq(path("my col", "k", "x y"), "x"), `"my col.k.x y":x`},
		"quote":       {expr.Eq(path("labels", "it's"), "x"), `"labels.it's":x`},
		"compare":     {expr.GREATER(path("labels", "a b"), 5), `"labels.a b":>5`},
		"range":       {expr.Rang(path("labels", "a b"), 1, 5, true), `"labels.a b":[1 TO 5]`},
		"in":          {expr.IN(path("labels", "a b"), expr.LIST(expr.Lit("x"), expr.Lit("y"))), `"labels.a b":(x OR y)`},
		"like":        {expr.LIKE(path("labels", "a b"), expr.WILD("fo*")), `"labels.a b":fo*`},
		"json":        {fromJSON(`{"left":{"json_path":{"column":"labels","path":["a b","c:d"]}},"operator":"EQUALS","right":"x"}`), `"labels.a b.c:d":x`},
		"json_number": {fromJSON(`{"left":{"json_path":{"column":"labels","path":["v w"]}},"operator":"EQUALS","right":{"number":{"value":1,"text":"1.0"}}}`), `"labels.v w":1.0`},
	}
	for name, tc := range roundTrips {
		t.Run(name, func(t *testing.T) {
			got := tc.input.String()
			if got != tc.want {
				t.Fatalf("String():\n got: %s\nwant: %s", got, tc.want)
			}
			parsed, err := Parse(got, opt)
			if err != nil {
				t.Fatalf("Parse(%q): %v", got, err)
			}
			if !reflect.DeepEqual(parsed, tc.input) {
				t.Fatalf(errTemplate, "String() parses back to a different expression", tc.input, parsed)
			}
		})
	}

	dotted := map[string]struct {
		input *expr.Expression
		want  string
	}{
		"dot":           {expr.Eq(path("labels", "a.b"), "x"), `labels.a\.b:x`},
		"dot_and_space": {expr.Eq(path("labels", "a b.c"), "x"), `"labels.a b\\.c":x`},
	}
	for name, tc := range dotted {
		t.Run(name, func(t *testing.T) {
			got := tc.input.String()
			if got != tc.want {
				t.Fatalf("String():\n got: %s\nwant: %s", got, tc.want)
			}
			if parsed, err := Parse(got, opt); err == nil {
				t.Fatalf("Parse(%q): expected an error, got %#v", got, parsed)
			}
		})
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
		{"mysql", ToMySQL, "CASE WHEN JSON_TYPE(JSON_EXTRACT(`labels`, '$.\"it''s\"')) <> 'NULL' THEN JSON_UNQUOTE(JSON_EXTRACT(`labels`, '$.\"it''s\"')) END = 'x'"},
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

func TestJSONPathRejectsBadSegmentsInQuery(t *testing.T) {
	opt := WithJSONFields("labels")
	for _, input := range []string{`labels.:x`, `labels..a:x`, `labels.a.:x`, "labels.a\\\tb:x", `labels.a\"b:x`} {
		if _, err := Parse(input, opt); err == nil {
			t.Errorf("Parse(%q): expected an error", input)
		}
	}
}

func TestJSONPathRejectsEscapedDotInKey(t *testing.T) {
	opt := WithJSONFields("labels")
	// = and a field in parentheses are checked as : is, and so is a wildcard
	// field, which drops its escapes.
	for _, input := range []string{`labels.a\.b:x`, `labels\.a:x`, `name:y AND labels.a.b\.c:>1`,
		`labels\.a=x`, `(labels\.a):x`, `((labels.a\.b)):x`, `labels\.a*:x`, `labels\.a*=x`, `(labels\.a*):x`} {
		_, err := Parse(input, opt)
		if err == nil || !strings.Contains(err.Error(), "cannot contain a dot") {
			t.Errorf("Parse(%q): expected an escaped dot error, got %v", input, err)
		}
	}

	// An escaped dot in a value, in a quoted field, or on a column that is not
	// registered keeps its meaning.
	for _, input := range []string{`labels.a:x\.y`, `"labels.a":x`, `other.a\.b:x`} {
		if _, err := Parse(input, opt); err != nil {
			t.Errorf("Parse(%q): unexpected error %v", input, err)
		}
	}
}

func TestNumberKeepsWrittenTextOnlyWhenItDiffers(t *testing.T) {
	got, err := Parse(`a:5 AND b:2.5`)
	if err != nil {
		t.Fatal(err)
	}
	want := expr.AND(expr.Eq("a", 5), expr.Eq("b", 2.5))
	if !reflect.DeepEqual(got, want) {
		t.Errorf("a number written as it prints must equal expr.Lit:\n got: %#v\nwant: %#v", got, want)
	}

	for input, text := range map[string]string{`a:1.0`: "1.0", `a:02134`: "02134", `a:1e3`: "1e3"} {
		e, err := Parse(input)
		if err != nil {
			t.Fatal(err)
		}
		lit, _ := e.Right.(*expr.Expression)
		if lit == nil || lit.NumberText() != text {
			t.Errorf("Parse(%q): want number text %q, got %#v", input, text, e.Right)
		}
	}

	// Text that is not a decimal number equal to the value is dropped, so
	// String can never print it as another query.
	for _, tc := range []struct {
		v    any
		text string
	}{
		{1, "1 OR b:c"},
		{1, "2"},
		{1.5, "1.50x"},
		{1.5, "+1.5"},
		{"x", "1"},
		// 1e-400 parses to 0 only because a float64 can't hold it.
		{0.0, "1e-400"},
		// An integer compares exactly, not through a float64, which reads
		// 9007199254740993 as 9007199254740992.
		{9007199254740992, "9007199254740993"},
		{int64(9007199254740992), "9007199254740993"},
		{uint64(9007199254740992), "9007199254740993.0"},
		{int64(math.MaxInt64), "9223372036854775808"},
	} {
		if text := expr.NumberLit(tc.v, tc.text).NumberText(); text != "" {
			t.Errorf("NumberLit(%#v, %q): want no text, got %q", tc.v, tc.text, text)
		}
	}
	for _, tc := range []struct {
		v    any
		text string
	}{
		{int64(5), "05"},
		{int64(9007199254740993), "09007199254740993"},
		{uint64(math.MaxUint64), "18446744073709551615.0"},
		{1000, "1e3"},
	} {
		if text := expr.NumberLit(tc.v, tc.text).NumberText(); text != tc.text {
			t.Errorf("NumberLit(%#v, %q): want text %q, got %q", tc.v, tc.text, tc.text, text)
		}
	}

	// A value changed after NumberLit no longer has the text, so String,
	// JSON and the drivers all use the new value.
	lit := expr.NumberLit(1.0, "1.0")
	lit.Left = 2.0
	if text := lit.NumberText(); text != "" {
		t.Errorf("changed value: want no text, got %q", text)
	}
	if got := expr.Eq("a", lit).String(); got != "a:2.0" {
		t.Errorf("changed value: want String() a:2.0, got %s", got)
	}
	raw, err := json.Marshal(lit)
	wantJSON, _ := json.Marshal(expr.Lit(2.0))
	if err != nil || string(raw) != string(wantJSON) {
		t.Errorf("changed value: want JSON %s, got %s (%v)", wantJSON, raw, err)
	}
}

func TestJSONPathFoldsCaseByDottedName(t *testing.T) {
	got, err := ToPostgres(`labels.category:Fruit AND labels.other:Fruit`,
		WithJSONFields("labels"), WithCaseInsensitiveFields("labels.category"))
	if err != nil {
		t.Fatal(err)
	}
	want := `(LOWER("labels"->>'category') = LOWER('Fruit')) AND ("labels"->>'other' = 'Fruit')`
	if got != want {
		t.Errorf("\n got: %s\nwant: %s", got, want)
	}
}

// TestJSONPathEmptySegmentErrorNamesPlainPath checks that the empty segment
// error prints the path's plain dotted name once, not the quoted String().
func TestJSONPathEmptySegmentErrorNamesPlainPath(t *testing.T) {
	err := expr.JSONPath{Column: "labels", Path: []string{"a b", ""}}.Validate()
	if want := `json path has an empty segment: "labels.a b."`; err == nil || err.Error() != want {
		t.Fatalf("\n got: %v\nwant: %s", err, want)
	}
}

// TestJSONPathFoldsCaseByDottedNameWithSpecialKey checks that a key with a
// space or a colon is listed by its plain dotted name, with no quotes, even
// though String() quotes the field.
func TestJSONPathFoldsCaseByDottedNameWithSpecialKey(t *testing.T) {
	tcs := map[string]struct {
		query string
		field string
		want  string
	}{
		"space":         {`"labels.a b":X`, "labels.a b", `LOWER("labels"->>'a b') = LOWER('X')`},
		"escaped_space": {`labels.a\ b:X`, "labels.a b", `LOWER("labels"->>'a b') = LOWER('X')`},
		"colon":         {`labels.a\:b:X`, "labels.a:b", `LOWER("labels"->>'a:b') = LOWER('X')`},
		"nested":        {`"labels.a b.c:d":X*`, "labels.a b.c:d", `LOWER("labels"->'a b'->>'c:d') SIMILAR TO LOWER('X%')`},
	}
	for name, tc := range tcs {
		t.Run(name, func(t *testing.T) {
			got, err := ToPostgres(tc.query, WithJSONFields("labels"), WithCaseInsensitiveFields(tc.field))
			if err != nil {
				t.Fatal(err)
			}
			if got != tc.want {
				t.Errorf("\n got: %s\nwant: %s", got, tc.want)
			}
		})
	}
}

func TestWithJSONFieldsRejectsBadColumnNames(t *testing.T) {
	for _, name := range []string{"t.labels", ".labels", ""} {
		_, err := Parse(`labels.a:x`, WithJSONFields("ok", name))
		if err == nil || !strings.Contains(err.Error(), "WithJSONFields") {
			t.Errorf("WithJSONFields(%q): expected an error, got %v", name, err)
		}
	}
}

func TestJSONPathJSONRoundTrip(t *testing.T) {
	inputs := []string{
		`labels.category:fruit`,
		`metadata.address.city:Boston AND name:x`,
		`labels.count:[1 TO 10]`,
		`labels.category:(fruit OR null)`,
		`labels.v:1.0`,
		`labels.v:02134`,
		`labels.v:1e3`,
		`labels.v:[1.0 TO 2.10]`,
		`labels.v:(1.0 OR 2)`,
	}
	for _, input := range inputs {
		t.Run(input, func(t *testing.T) {
			first, err := Parse(input, WithJSONFields("labels", "metadata"))
			if err != nil {
				t.Fatalf("parse: %v", err)
			}
			raw, err := json.Marshal(first)
			if err != nil {
				t.Fatalf("marshal: %v", err)
			}
			var second expr.Expression
			if err := json.Unmarshal(raw, &second); err != nil {
				t.Fatalf("unmarshal %s: %v", raw, err)
			}
			if !reflect.DeepEqual(first, &second) {
				t.Fatalf("round trip through JSON changed the expression:\n got: %#v\nwant: %#v", &second, first)
			}
			want, _, err := driver.NewPostgresDriver().RenderParam(first)
			if err != nil {
				t.Fatalf("render: %v", err)
			}
			got, _, err := driver.NewPostgresDriver().RenderParam(&second)
			if err != nil {
				t.Fatalf("render after round trip: %v", err)
			}
			if got != want {
				t.Fatalf("round trip through JSON changed the SQL:\n got: %s\nwant: %s", got, want)
			}
		})
	}
}

func TestPostgresPlaceholdersSkipQuotedText(t *testing.T) {
	tcs := map[string]struct {
		input string
		want  string
	}{
		"question_mark_in_json_key": {`labels.a?b:x AND name:y`, `("labels"->>'a?b' = $1) AND ("name" = $2)`},
		"question_mark_in_column":   {`a?b:x AND name:y`, `("a?b" = $1) AND ("name" = $2)`},
		"numeric_guard_regex":       {`labels.n:>5`, `CASE WHEN json_typeof(("labels"->'n')::json) = 'number' AND length("labels"->>'n') <= 1000 AND "labels"->>'n' ~ '^-?[0-9]+([.][0-9]+)?([eE][+-]?[0-9]{1,4})?$' THEN ("labels"->>'n')::numeric > $1 WHEN "labels"->>'n' IS NOT NULL THEN FALSE END`},
	}
	for name, tc := range tcs {
		t.Run(name, func(t *testing.T) {
			got, _, err := ToParameterizedPostgres(tc.input, WithJSONFields("labels"))
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tc.want {
				t.Errorf("\n got: %s\nwant: %s", got, tc.want)
			}
		})
	}
}

func TestJSONPathJSONRejectsMalformedLeaf(t *testing.T) {
	tcs := map[string]string{
		"null":               `{"left":{"json_path":null},"operator":"EQUALS","right":"x"}`,
		"string":             `{"left":{"json_path":"labels.a"},"operator":"EQUALS","right":"x"}`,
		"empty":              `{"left":{"json_path":{}},"operator":"EQUALS","right":"x"}`,
		"no_path":            `{"left":{"json_path":{"column":"labels"}},"operator":"EQUALS","right":"x"}`,
		"extra_keys":         `{"left":{"json_path":{"column":"labels","path":["a"]},"operator":"AND"},"operator":"EQUALS","right":"x"}`,
		"number_no_text":     `{"left":"a","operator":"EQUALS","right":{"number":{"value":1}}}`,
		"number_no_value":    `{"left":"a","operator":"EQUALS","right":{"number":{"text":"1.0"}}}`,
		"number_string":      `{"left":"a","operator":"EQUALS","right":{"number":{"value":"x","text":"1.0"}}}`,
		"number_int_text":    `{"left":"a","operator":"EQUALS","right":{"number":{"value":1.5,"text":"02"}}}`,
		"number_extra_keys":  `{"left":"a","operator":"EQUALS","right":{"number":{"value":1,"text":"1.0"},"operator":"AND"}}`,
		"number_in_list":     `{"left":"a","operator":"IN","right":{"left":[{"number":{"value":1}}],"operator":"LIST"}}`,
		"number_injection":   `{"left":"a","operator":"EQUALS","right":{"number":{"value":1,"text":"1 OR b:c"}}}`,
		"number_mismatch":    `{"left":"a","operator":"EQUALS","right":{"number":{"value":1,"text":"2.0"}}}`,
		"number_underflow":   `{"left":"a","operator":"EQUALS","right":{"number":{"value":0,"text":"1e-400"}}}`,
		"number_int_rounds":  `{"left":"a","operator":"EQUALS","right":{"number":{"value":9007199254740992,"text":"9007199254740993"}}}`,
		"number_range_bound": `{"left":"a","operator":"RANGE","right":{"min":{"number":{"value":1}},"max":2,"inclusive":true}}`,
	}
	for name, raw := range tcs {
		t.Run(name, func(t *testing.T) {
			var e expr.Expression
			if err := json.Unmarshal([]byte(raw), &e); err == nil {
				t.Fatalf("expected an error, got %#v", &e)
			}
		})
	}
}
