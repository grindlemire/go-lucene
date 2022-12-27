package driver

import "github.com/grindlemire/go-lucene/pkg/lucene/expr"

// SQLDriver transforms a parsed lucene expression to a sql filter.
type SQLDriver struct {
	base
}

// NewSQLDriver creates a new driver that will output a parsed lucene expression as a SQL filter.
func NewSQLDriver() SQLDriver {
	fns := map[expr.Operator]renderFN{
		expr.Literal: literal,
	}

	for op, sharedFN := range shared {
		_, found := fns[op]
		if !found {
			fns[op] = sharedFN
		}
	}

	return SQLDriver{
		base{
			renderFNs: fns,
		},
	}
}
