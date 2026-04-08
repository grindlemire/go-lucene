package driver

// Dialect captures the operations that differ between SQL databases.
// Base calls into a Dialect for the operators that have database-specific
// semantics (Like, Range, standalone wildcard, pattern escaping, bool literals).
// Simple operators (And, Or, Equals, comparisons, In, List, Not) are handled
// by Base via the RenderFNs map and do not flow through the dialect.
type Dialect interface {
	// RenderLike renders a wildcard or regex match (non-parameterized form).
	// isRegex is true when the right side came from a /regex/ literal.
	RenderLike(left, right string, isRegex bool) (string, error)

	// RenderLikeParam is the parameterized variant. params is the slice of
	// values bound for the right-hand side; the dialect may rewrite them
	// in place if needed.
	RenderLikeParam(left, right string, params []any, isRegex bool) (string, error)

	// RenderRange renders a BETWEEN-style expression (non-parameterized).
	RenderRange(left, right string) (string, error)

	// RenderRangeParam is the parameterized variant.
	RenderRangeParam(left, right string, params []any) (string, error)

	// RenderStandaloneWild handles `field:*` — the "field has any value" case.
	RenderStandaloneWild(left string) (string, error)

	// EscapeLikePattern transforms a raw lucene wildcard string into the
	// dialect's pattern syntax. Called by Base before parameter binding.
	EscapeLikePattern(pattern string) string

	// SerializeBool converts a Go bool to its SQL literal form.
	SerializeBool(b bool) string
}

// defaultDialect is used by Base when no Dialect has been set on a driver
// (e.g., custom drivers built against the pre-dialect API). It preserves
// the historical Postgres-flavored behavior that such drivers inherited.
var defaultDialect Dialect = postgresDialect{}
