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
	fns := map[expr.Operator]RenderFN{
		expr.Literal: literal,
	}

	for op, sharedFN := range Shared {
		_, found := fns[op]
		if !found {
			fns[op] = sharedFN
		}
	}

	return PostgresDriver{
		Base{
			RenderFNs: fns,
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
