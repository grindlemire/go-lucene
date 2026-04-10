package driver

import (
	"fmt"
	"strings"

	"github.com/grindlemire/go-lucene/pkg/lucene/expr"
)

// PostgresDriver transforms a parsed lucene expression to a postgres sql filter.
type PostgresDriver struct {
	Base
}

// NewPostgresDriver creates a new driver that will output postgres filter strings from parsed lucene expressions.
func NewPostgresDriver() PostgresDriver {
	fns := map[expr.Operator]RenderFN{}
	for op, sharedFN := range Shared {
		fns[op] = sharedFN
	}
	return PostgresDriver{
		Base{
			RenderFNs: fns,
			Dialect:   postgresDialect{},
		},
	}
}

// RenderParam will render the expression into a parameterized query using PostgreSQL's $N placeholder format.
// The returned string will contain $1, $2, $3, etc. placeholders and the params will contain the values
// that should be passed to the query.
func (p PostgresDriver) RenderParam(e *expr.Expression) (s string, params []any, err error) {
	// First, use the base implementation to get the result with ? placeholders
	str, params, err := p.Base.RenderParam(e)
	if err != nil {
		return s, params, err
	}

	// Then convert ? placeholders to $N format
	paramIndex := 1
	result := strings.Builder{}
	i := 0
	for i < len(str) {
		if str[i] == '?' {
			result.WriteString(fmt.Sprintf("$%d", paramIndex))
			paramIndex++
		} else {
			result.WriteByte(str[i])
		}
		i++
	}

	return result.String(), params, nil
}

// postgresDialect implements Dialect for PostgreSQL. It is a lift-and-shift
// of the behavior previously baked into Base — SIMILAR TO for wildcards,
// ~ for regex, SIMILAR TO '%' for standalone wildcard, Postgres-style
// %/_ pattern escaping, and true/false bool literals.
type postgresDialect struct{}

func (postgresDialect) RenderLike(left, right string, isRegex bool) (string, error) {
	if isRegex {
		return fmt.Sprintf("%s ~ %s", left, right), nil
	}
	return fmt.Sprintf("%s SIMILAR TO %s", left, right), nil
}

func (postgresDialect) RenderStandaloneWild(left string) (string, error) {
	return fmt.Sprintf("%s SIMILAR TO '%%'", left), nil
}

func (postgresDialect) EscapeLikePattern(pattern string) string {
	pattern = strings.ReplaceAll(pattern, "%", `\%`)
	pattern = strings.ReplaceAll(pattern, "_", `\_`)
	pattern = strings.ReplaceAll(pattern, "*", "%")
	pattern = strings.ReplaceAll(pattern, "?", "_")
	return pattern
}

func (postgresDialect) SerializeBool(b bool) string {
	if b {
		return "true"
	}
	return "false"
}
