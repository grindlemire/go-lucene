package driver

import (
	"fmt"

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

func (sqliteDialect) RenderLikeParam(left, right string, params []any, isRegex bool) (string, error) {
	if isRegex {
		return fmt.Sprintf("%s REGEXP %s", left, right), nil
	}
	return fmt.Sprintf("%s GLOB %s", left, right), nil
}

func (sqliteDialect) RenderRange(left, right string) (string, error) {
	return rang(left, right)
}

func (sqliteDialect) RenderRangeParam(left, right string, params []any) (string, error) {
	return rangParam(left, right, params)
}

func (sqliteDialect) RenderStandaloneWild(left string) (string, error) {
	return fmt.Sprintf("%s IS NOT NULL", left), nil
}

// EscapeLikePattern is the identity function for SQLite. Lucene's wildcard
// syntax (* and ?) is already the same as GLOB's, so no translation is needed.
// Note that GLOB has no escape mechanism — patterns containing a literal * or ?
// cannot be expressed via the wildcard form; users must fall back to the regex
// form for that.
func (sqliteDialect) EscapeLikePattern(pattern string) string {
	return pattern
}

func (sqliteDialect) SerializeBool(b bool) string {
	if b {
		return "1"
	}
	return "0"
}
