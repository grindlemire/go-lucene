package driver

import (
	"fmt"
	"strings"

	"github.com/grindlemire/go-lucene/pkg/lucene/expr"
)

// MySQLDriver transforms a parsed lucene expression to a MySQL SQL filter.
//
// Notable differences from the Postgres driver:
//   - Identifiers quote with backticks (`col`), never double quotes. Column
//     names containing a backtick are rejected at render time.
//   - Wildcard patterns without SIMILAR-TO-only metacharacters render as
//     LIKE ... ESCAPE '#' (sql_mode-portable: works under default mode,
//     ANSI_QUOTES, and NO_BACKSLASH_ESCAPES).
//   - Wildcard patterns that contain |, (), [], {}, or + render as REGEXP
//     with an anchored ^(...)$ translation so the matching semantics line
//     up with the Postgres SIMILAR TO path.
//   - Regex (/pattern/) renders as REGEXP.
//   - Standalone `field:*` renders as "`field` IS NOT NULL".
//   - Parameter placeholders are `?` (no rewriting).
//   - Bool literals serialize as TRUE / FALSE; bool params pass through as
//     Go bool (go-sql-driver/mysql converts natively).
//   - String literals in the non-parameterized path double both single
//     quotes and backslashes. See README for the NO_BACKSLASH_ESCAPES caveat.
type MySQLDriver struct {
	Base
}

// NewMySQLDriver creates a new driver that will output MySQL filter strings
// from parsed lucene expressions.
func NewMySQLDriver() MySQLDriver {
	fns := map[expr.Operator]RenderFN{}
	for op, sharedFN := range Shared {
		fns[op] = sharedFN
	}
	return MySQLDriver{
		Base: Base{
			RenderFNs: fns,
			Dialect:   mysqlDialect{},
		},
	}
}

// mysqlDialect implements Dialect for MySQL.
type mysqlDialect struct{}

// similarToOnlyMetachars are the pattern characters that have meaning in
// Postgres SIMILAR TO but not in MySQL LIKE. Their presence in a Lucene
// wildcard pattern forces the dialect to fall back to REGEXP so alternation
// and grouping semantics are preserved.
const similarToOnlyMetachars = "|()[]{}+"

func (mysqlDialect) RenderLike(left, right string, isRegex bool) (string, error) {
	if isRegex {
		return fmt.Sprintf("%s REGEXP %s", left, right), nil
	}
	return fmt.Sprintf("%s LIKE %s ESCAPE '#'", left, right), nil
}

func (mysqlDialect) RenderStandaloneWild(left string) (string, error) {
	return fmt.Sprintf("%s IS NOT NULL", left), nil
}

// PrepareLikePattern returns the pattern transformed for LIKE (and useRegex=false)
// when the pattern contains only simple wildcards, or transformed for REGEXP
// (and useRegex=true) when it contains SIMILAR-TO-only metacharacters.
func (mysqlDialect) PrepareLikePattern(pattern string) (string, bool) {
	if strings.ContainsAny(pattern, similarToOnlyMetachars) {
		return luceneWildcardToRegex(pattern), true
	}
	return luceneWildcardToLike(pattern), false
}

// luceneWildcardToLike translates Lucene wildcard syntax to MySQL LIKE syntax
// using '#' as the escape character. '\' would collide with MySQL's string-literal
// backslash meta-escape under default sql_mode.
func luceneWildcardToLike(pattern string) string {
	pattern = strings.ReplaceAll(pattern, `#`, `##`)
	pattern = strings.ReplaceAll(pattern, `%`, `#%`)
	pattern = strings.ReplaceAll(pattern, `_`, `#_`)
	pattern = strings.ReplaceAll(pattern, `*`, `%`)
	pattern = strings.ReplaceAll(pattern, `?`, `_`)
	return pattern
}

// luceneWildcardToRegex translates a Lucene wildcard pattern (which may
// contain SIMILAR-TO-style alternation, grouping, or character classes) into
// an anchored POSIX regex.
//
// The body is wrapped in a capturing group `^(...)$` so the anchors apply
// to the whole pattern. Without the group, raw alternation like `foo|bar`
// would parse as `(^foo)|(bar$)` because `|` has lower precedence than the
// anchors in POSIX regex — diverging from the Postgres SIMILAR TO
// whole-string semantics this method preserves. A capturing group (rather
// than a non-capturing `(?:...)`) is used because the latter is a Perl
// extension not guaranteed by POSIX ERE; MySQL 5.7's Henry Spencer engine
// may reject it. The capture is unused so the extra allocation is
// negligible.
func luceneWildcardToRegex(pattern string) string {
	var b strings.Builder
	b.Grow(len(pattern) + 8)
	b.WriteString("^(")
	for _, r := range pattern {
		switch r {
		case '*':
			b.WriteString(".*")
		case '?':
			b.WriteByte('.')
		// SIMILAR TO metacharacters we retain as regex metacharacters.
		case '|', '(', ')', '[', ']', '{', '}', '+':
			b.WriteRune(r)
		// Regex-only metacharacters escaped so they behave like literals
		// (matching the SIMILAR TO semantics of the Postgres path).
		case '.', '^', '$', '\\':
			b.WriteByte('\\')
			b.WriteRune(r)
		default:
			b.WriteRune(r)
		}
	}
	b.WriteString(")$")
	return b.String()
}

func (mysqlDialect) EscapeStringLiteral(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `'`, `''`)
	return "'" + s + "'"
}

func (mysqlDialect) SerializeBool(b bool) string {
	if b {
		return "TRUE"
	}
	return "FALSE"
}

// BoolParam passes the Go bool through directly. go-sql-driver/mysql converts
// bool parameters to the wire protocol's integer form automatically.
func (mysqlDialect) BoolParam(b bool) any { return b }

func (mysqlDialect) QuoteColumn(name string) (string, error) {
	if len(name) == 0 {
		return "", fmt.Errorf("column name is empty")
	}
	if strings.ContainsRune(name, 0) {
		return "", fmt.Errorf("column name contains a null byte: %q", name)
	}
	if strings.ContainsRune(name, '`') {
		return "", fmt.Errorf("column name contains a backtick: %q", name)
	}
	return "`" + name + "`", nil
}
