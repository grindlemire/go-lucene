package driver

import "github.com/grindlemire/go-lucene/pkg/lucene/expr"

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
