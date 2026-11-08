package driver

import (
	"reflect"
	"testing"

	"github.com/grindlemire/go-lucene/pkg/lucene/expr"
)

// TestRenderSpreadsFieldGroup checks that an EQUALS whose value is a group of
// terms, built in code or read from JSON, renders the way the parser reads
// a:(...): the field goes to every term in the group. These used to render as
// a comparison of the column with a boolean, such as "a" = ('x' AND 'y').
func TestRenderSpreadsFieldGroup(t *testing.T) {
	labels := expr.Lit(expr.JSONPath{Column: "labels", Path: []string{"k"}})
	fieldless := &expr.Expression{Op: expr.Range, Right: &expr.RangeBoundary{Min: expr.Lit(1), Max: expr.Lit(5), Inclusive: true}}

	tcs := map[string]struct {
		input  *expr.Expression
		render string
		param  string
		params []any
	}{
		"and": {
			input:  expr.Eq("a", expr.AND("x", "y")),
			render: `("a" = 'x') AND ("a" = 'y')`,
			param:  `("a" = $1) AND ("a" = $2)`,
			params: []any{"x", "y"},
		},
		// An OR of terms is an IN, as a:(x OR y) is.
		"or": {
			input:  expr.Eq("a", expr.OR("x", "y")),
			render: `"a" IN ('x', 'y')`,
			param:  `"a" IN ($1, $2)`,
			params: []any{"x", "y"},
		},
		"or_null": {
			input:  expr.Eq("a", expr.OR("x", expr.NULL())),
			render: `("a" = 'x' OR "a" IS NULL)`,
			param:  `("a" = $1 OR "a" IS NULL)`,
			params: []any{"x"},
		},
		"or_wildcard": {
			input:  expr.Eq("a", expr.OR(expr.WILD("x*"), "y")),
			render: `("a" SIMILAR TO 'x%') OR ("a" = 'y')`,
			param:  `("a" SIMILAR TO $1) OR ("a" = $2)`,
			params: []any{"x%", "y"},
		},
		"not": {
			input:  expr.Eq("a", expr.NOT(expr.Lit("x"))),
			render: `NOT("a" = 'x')`,
			param:  `NOT("a" = $1)`,
			params: []any{"x"},
		},
		"must_not_null": {
			input:  expr.Eq("a", expr.MUSTNOT(expr.NULL())),
			render: `"a" IS NOT NULL`,
			param:  `"a" IS NOT NULL`,
		},
		"must": {
			input:  expr.Eq("a", expr.MUST(expr.Lit("x"))),
			render: `"a" = 'x'`,
			param:  `"a" = $1`,
			params: []any{"x"},
		},
		"nested": {
			input:  expr.Eq("a", expr.AND("x", expr.OR("y", expr.NOT(expr.Lit("z"))))),
			render: `("a" = 'x') AND (("a" = 'y') OR (NOT("a" = 'z')))`,
			param:  `("a" = $1) AND (("a" = $2) OR (NOT("a" = $3)))`,
			params: []any{"x", "y", "z"},
		},
		"range": {
			input:  expr.Eq("a", fieldless),
			render: `"a" >= 1 AND "a" <= 5`,
			param:  `"a" >= $1 AND "a" <= $2`,
			params: []any{1, 5},
		},
		// A term with its own field keeps it, as in a:(b:c OR d).
		"own_field": {
			input:  expr.Eq("a", expr.OR(expr.Eq("b", "c"), "d")),
			render: `("b" = 'c') OR ("a" = 'd')`,
			param:  `("b" = $1) OR ("a" = $2)`,
			params: []any{"c", "d"},
		},
		"json_path": {
			input:  &expr.Expression{Op: expr.Equals, Left: labels, Right: expr.AND("x", "y")},
			render: `("labels"->>'k' = 'x') AND ("labels"->>'k' = 'y')`,
			param:  `("labels"->>'k' = $1) AND ("labels"->>'k' = $2)`,
			params: []any{"x", "y"},
		},
		"from_json": {
			input:  mustUnmarshal(t, `{"left":"a","operator":"EQUALS","right":{"left":"x","operator":"AND","right":{"left":"y","operator":"NOT"}}}`),
			render: `("a" = 'x') AND (NOT("a" = 'y'))`,
			param:  `("a" = $1) AND (NOT("a" = $2))`,
			params: []any{"x", "y"},
		},
	}

	d := NewPostgresDriver()
	for name, tc := range tcs {
		t.Run(name, func(t *testing.T) {
			got, err := d.Render(tc.input)
			if err != nil {
				t.Fatalf("Render: unexpected error: %v", err)
			}
			if got != tc.render {
				t.Fatalf(errTemplate, "Render sql does not match", tc.render, got)
			}
			got, params, err := d.RenderParam(tc.input)
			if err != nil {
				t.Fatalf("RenderParam: unexpected error: %v", err)
			}
			if got != tc.param {
				t.Fatalf(errTemplate, "RenderParam sql does not match", tc.param, got)
			}
			if len(params) != len(tc.params) || (len(params) > 0 && !reflect.DeepEqual(params, tc.params)) {
				t.Fatalf("want params %#v, got %#v", tc.params, params)
			}
		})
	}
}
