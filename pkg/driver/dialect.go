package driver

// Dialect captures the operations that differ between SQL databases.
// Base calls into a Dialect for the operators that have database-specific
// semantics (Like, Range, standalone wildcard, pattern escaping, bool literals).
// Simple operators (And, Or, Equals, comparisons, In, List, Not) are handled
// by Base via the RenderFNs map and do not flow through the dialect.
type Dialect interface {
	// RenderLike renders a wildcard or regex match (non-parameterized form).
	// isRegex is true when the right side came from a /regex/ literal OR
	// when PrepareLikePattern asked for the regex path.
	RenderLike(left, right string, isRegex bool) (string, error)

	// RenderStandaloneWild handles `field:*` — the "field has any value" case.
	RenderStandaloneWild(left string) (string, error)

	// PrepareLikePattern transforms a raw lucene wildcard pattern into the
	// dialect's pattern syntax and reports whether the resulting expression
	// should be rendered via the regex path (REGEXP / ~) instead of the
	// wildcard path (LIKE / SIMILAR TO / GLOB). Replaces EscapeLikePattern.
	PrepareLikePattern(pattern string) (transformed string, useRegex bool)

	// EscapeStringLiteral escapes a string for safe embedding in a SQL
	// literal and returns the resulting SQL fragment INCLUDING the surrounding
	// single quotes. Used by Base on every non-parameterized path that emits
	// a string-valued literal (plain strings, regex patterns, string range
	// bounds).
	EscapeStringLiteral(s string) string

	// SerializeBool converts a Go bool to its SQL literal form.
	SerializeBool(b bool) string

	// BoolParam returns the parameter value for a boolean. Databases with
	// native bool support (Postgres) return the Go bool directly; databases
	// that store bools as integers (SQLite) return 1 or 0.
	BoolParam(b bool) any

	// QuoteColumn wraps a column name in the dialect's identifier quoting
	// characters. Most databases use SQL-standard double quotes ("col"),
	// but MySQL uses backticks (`col`). Returns an error if the name
	// contains the quoting character itself.
	QuoteColumn(name string) (string, error)
}

// defaultDialect is used by Base when no Dialect has been set on a driver
// (e.g., custom drivers built against the pre-dialect API). It preserves
// the historical Postgres-flavored behavior that such drivers inherited.
var defaultDialect Dialect = postgresDialect{}
