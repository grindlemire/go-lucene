package driver

import "github.com/grindlemire/go-lucene/pkg/lucene/expr"

// PostgresDriver transforms a parsed lucene expression to a sql filter.
type PostgresDriver struct {
	base
}

// NewPostgresDriver creates a new driver that will output a parsed lucene expression as a SQL filter.
func NewPostgresDriver() PostgresDriver {
	fns := map[expr.Operator]renderFN{
		expr.Literal: literal,
	}

	for op, sharedFN := range shared {
		_, found := fns[op]
		if !found {
			fns[op] = sharedFN
		}
	}

	return PostgresDriver{
		base{
			renderFNs: fns,
		},
	}
}
