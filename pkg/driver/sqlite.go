package driver

import (
	"fmt"
	"strings"

	"github.com/grindlemire/go-lucene/pkg/lucene/expr"
)

// SQLiteDriver transforms a parsed lucene expression to a SQLite SQL filter.
//
// Notable differences from the Postgres driver:
//   - Wildcards render as GLOB (case-sensitive, Unix glob syntax).
//     Lucene's * and ? map 1:1 to GLOB's * and ?.
//   - GLOB has no escape mechanism, so a literal * or ? inside a wildcard
//     pattern cannot be matched. Use the regex form (field:/.../) if you
//     need to match those characters literally.
//   - Regex (/pattern/) renders as REGEXP. SQLite does not define regexp()
//     by default — the caller must register a regexp() function on their
//     connection (see modernc.org/sqlite.RegisterDeterministicScalarFunction
//     or the sqlite_regex build tag for mattn/go-sqlite3) or they will get
//     a "no such function: regexp" error at query time.
//   - Standalone `field:*` renders as `"field" IS NOT NULL`.
//   - Parameter placeholders are `?` (no rewriting).
//   - Bool literals serialize as 1/0.
type SQLiteDriver struct {
	Base
}

// NewSQLiteDriver creates a new driver that will output SQLite filter strings
// from parsed lucene expressions.
func NewSQLiteDriver() SQLiteDriver {
	fns := map[expr.Operator]RenderFN{}
	for op, sharedFN := range Shared {
		fns[op] = sharedFN
	}
	return SQLiteDriver{
		Base: Base{
			RenderFNs: fns,
			Dialect:   sqliteDialect{},
		},
	}
}

// sqliteDialect implements Dialect for SQLite.
type sqliteDialect struct{}

func (sqliteDialect) RenderLike(left, right string, isRegex bool) (string, error) {
	if isRegex {
		return fmt.Sprintf("%s REGEXP %s", left, right), nil
	}
	return fmt.Sprintf("%s GLOB %s", left, right), nil
}

func (sqliteDialect) RenderStandaloneWild(left string) (string, error) {
	return fmt.Sprintf("%s IS NOT NULL", left), nil
}

// PrepareLikePattern is the identity function for SQLite. Lucene's wildcard
// syntax (* and ?) is already the same as GLOB's, so no translation is needed,
// and SQLite never falls back to regex based on pattern content.
// Note that GLOB has no escape mechanism — patterns containing a literal * or ?
// cannot be expressed via the wildcard form; users must fall back to the regex
// form for that.
func (sqliteDialect) PrepareLikePattern(pattern string) (string, bool) {
	return pattern, false
}

func (sqliteDialect) EscapeStringLiteral(s string) string {
	return fmt.Sprintf("'%s'", strings.ReplaceAll(s, "'", "''"))
}

func (sqliteDialect) SerializeBool(b bool) string {
	if b {
		return "1"
	}
	return "0"
}

func (sqliteDialect) BoolParam(b bool) any {
	if b {
		return 1
	}
	return 0
}

func (sqliteDialect) QuoteColumn(name string) (string, error) {
	if strings.ContainsRune(name, '"') {
		return "", fmt.Errorf("column name contains a double quote: %q", name)
	}
	return `"` + name + `"`, nil
}

// RenderJSONPath reads the value as text, so a text comparison matches the
// same rows as on Postgres and MySQL: json_extract alone returns a JSON number
// as a number and true/false as 1/0. The numeric read is json_extract, and the
// type check matters for it, because SQLite orders every text value above every
// number.
func (d sqliteDialect) RenderJSONPath(column string, path []string) (JSONPathSQL, error) {
	jsonPath := d.EscapeStringLiteral(dollarJSONPath(path))
	value := fmt.Sprintf("json_extract(%s, %s)", column, jsonPath)
	jsonType := fmt.Sprintf("json_type(%s, %s)", column, jsonPath)
	return JSONPathSQL{
		Text:     fmt.Sprintf("CASE %s WHEN 'true' THEN 'true' WHEN 'false' THEN 'false' ELSE CAST(%s AS TEXT) END", jsonType, value),
		Value:    value,
		IsNumber: fmt.Sprintf("%s IN ('integer', 'real')", jsonType),
		Number:   value,
	}, nil
}
