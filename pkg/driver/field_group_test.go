package driver

import (
	"fmt"
	"reflect"
	"strings"
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
		// A value with a field of its own keeps it, as a:(b:1) does, and the
		// outer field is dropped. These used to render as "a" = ("b" = 1).
		"own_field_value": {
			input:  expr.Eq("a", expr.Eq("b", 1)),
			render: `"b" = 1`,
			param:  `"b" = $1`,
			params: []any{1},
		},
		"own_field_range": {
			input:  expr.Eq("a", expr.Rang("b", 1, 5, true)),
			render: `"b" >= 1 AND "b" <= 5`,
			param:  `"b" >= $1 AND "b" <= $2`,
			params: []any{1, 5},
		},
		"own_field_compare": {
			input:  expr.Eq("a", expr.GREATER("b", 1)),
			render: `"b" > 1`,
			param:  `"b" > $1`,
			params: []any{1},
		},
		"own_field_in": {
			input:  expr.Eq("a", expr.IN("b", expr.LIST(expr.Lit("x"), expr.Lit("y")))),
			render: `"b" IN ('x', 'y')`,
			param:  `"b" IN ($1, $2)`,
			params: []any{"x", "y"},
		},
		"own_field_like": {
			input:  expr.Eq("a", expr.LIKE("b", expr.WILD("x*"))),
			render: `"b" SIMILAR TO 'x%'`,
			param:  `"b" SIMILAR TO $1`,
			params: []any{"x%"},
		},
		"json_path": {
			input:  &expr.Expression{Op: expr.Equals, Left: labels, Right: expr.AND("x", "y")},
			render: `("labels"->>'k' = 'x') AND ("labels"->>'k' = 'y')`,
			param:  `("labels"->>'k' = $1) AND ("labels"->>'k' = $2)`,
			params: []any{"x", "y"},
		},
		// The JSON of a range with no field has "left":null.
		"range_from_json": {
			input:  mustUnmarshal(t, `{"left":"a","operator":"EQUALS","right":{"left":{"left":null,"operator":"RANGE","right":{"min":1,"max":5,"inclusive":true}},"operator":"OR","right":"x"}}`),
			render: `("a" >= 1 AND "a" <= 5) OR ("a" = 'x')`,
			param:  `("a" >= $1 AND "a" <= $2) OR ("a" = $3)`,
			params: []any{1, 5, "x"},
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
			// Rendering builds a new tree and leaves the caller's as it was.
			before := fmt.Sprintf("%#v", tc.input)
			defer func() {
				if after := fmt.Sprintf("%#v", tc.input); after != before {
					t.Errorf("rendering changed the input:\n    before %s\n    after  %s", before, after)
				}
			}()

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

// TestRenderRejectsMisplacedGroup checks the shapes where spreading a field
// makes no sense. The parser rejects a:>(x OR y) with "must be a single
// term", and the drivers now reject the same shapes when built in code. They
// used to render SQL such as "a" > ('x' OR 'y') or "a" IN 'x' OR 'y'.
func TestRenderRejectsMisplacedGroup(t *testing.T) {
	labels := expr.Lit(expr.JSONPath{Column: "labels", Path: []string{"k"}})
	fieldless := &expr.Expression{Op: expr.Range, Right: &expr.RangeBoundary{Min: expr.Lit(1), Max: expr.Lit(5), Inclusive: true}}

	tcs := map[string]struct {
		input   *expr.Expression
		wantErr string
	}{
		"greater_or":      {expr.GREATER("a", expr.OR("x", "y")), "the value in a GREATER comparison must be a single term, not an OR expression"},
		"greater_eq_and":  {expr.GREATEREQ("a", expr.AND("x", "y")), "the value in a GREATER_EQ comparison must be a single term, not an AND expression"},
		"less_not":        {expr.LESS("a", expr.NOT(expr.Lit(1))), "the value in a LESS comparison must be a single term, not a NOT expression"},
		"less_eq_range":   {expr.LESSEQ("a", fieldless), "the value in a LESS_EQ comparison must be a single term, not a RANGE expression"},
		"json_greater_or": {&expr.Expression{Op: expr.Greater, Left: labels, Right: expr.OR(1, 2)}, "the value in a GREATER comparison must be a single term, not an OR expression"},
		"like_or":         {expr.LIKE("a", expr.OR(expr.WILD("x*"), expr.WILD("y*"))), "LIKE pattern must be a single term, not an OR expression"},
		"equals_list":     {expr.Eq("a", expr.LIST(expr.Lit("x"), expr.Lit("y"))), "the value in an EQUALS must be a single term, not a LIST expression"},
		"in_not_list":     {expr.IN("a", expr.OR("x", "y")), "the right side of an IN must be a LIST, not an OR expression"},
		"in_list_and":     {expr.IN("a", expr.LIST(expr.AND("x", "y"))), "an IN list item must be a single term, not an AND expression"},
		"range_no_field":  {fieldless, "a range needs a field, as in field:[a TO b]"},
		"field_is_and":    {&expr.Expression{Op: expr.Equals, Left: expr.AND("x", "y"), Right: expr.Lit("z")}, "the field of an EQUALS must be a single term, not an AND expression"},
		"field_is_or":     {expr.GREATER(expr.OR("x", "y"), 1), "the field of a GREATER must be a single term, not an OR expression"},
		"from_json":       {mustUnmarshal(t, `{"left":"a","operator":"GREATER","right":{"left":"x","operator":"OR","right":"y"}}`), "must be a single term, not an OR expression"},
		// A value or field with a field of its own is not a single term
		// either. These used to render "a" > ("b" = 1) and the like.
		"greater_equals":  {expr.GREATER("a", expr.Eq("b", 1)), "the value in a GREATER comparison must be a single term, not an EQUALS expression"},
		"less_own_range":  {expr.LESS("a", expr.Rang("b", 1, 5, true)), "the value in a LESS comparison must be a single term, not a RANGE expression"},
		"in_list_equals":  {expr.IN("a", expr.LIST(expr.Eq("b", "c"))), "an IN list item must be a single term, not an EQUALS expression"},
		"field_is_equals": {&expr.Expression{Op: expr.Equals, Left: expr.Eq("b", "c"), Right: expr.Lit("z")}, "the field of an EQUALS must be a single term, not an EQUALS expression"},
		// A field operator with no field used to render ( = 'x') AND ( = 'y').
		"equals_no_field":      {&expr.Expression{Op: expr.Equals, Right: expr.AND("x", "y")}, "an EQUALS needs a field, as in field:value"},
		"equals_term_no_field": {&expr.Expression{Op: expr.Equals, Right: expr.Lit("x")}, "an EQUALS needs a field, as in field:value"},
		"like_no_field":        {&expr.Expression{Op: expr.Like, Right: expr.WILD("x*")}, "a LIKE needs a field, as in field:value*"},
		"in_no_field":          {&expr.Expression{Op: expr.In, Right: expr.LIST(expr.Lit("x"), expr.Lit("y"))}, "an IN needs a field, as in field:(a OR b)"},
		"greater_no_field":     {&expr.Expression{Op: expr.Greater, Right: expr.Lit(1)}, "a GREATER needs a field, as in field:>value"},
		"less_eq_no_field":     {&expr.Expression{Op: expr.LessEq, Right: expr.Lit(1)}, "a LESS_EQ needs a field, as in field:<=value"},
	}

	for name, tc := range tcs {
		for dname, d := range allDrivers {
			t.Run(name+"/"+dname+"/render", func(t *testing.T) {
				noPanic(t, func() {
					got, err := d.Render(tc.input)
					if err == nil {
						t.Fatalf("expected error rendering %#v, got SQL %q", tc.input, got)
					}
					if !strings.Contains(err.Error(), tc.wantErr) {
						t.Fatalf("expected an error containing %q, got: %v", tc.wantErr, err)
					}
				})
			})
			t.Run(name+"/"+dname+"/render_param", func(t *testing.T) {
				noPanic(t, func() {
					got, params, err := d.RenderParam(tc.input)
					if err == nil {
						t.Fatalf("expected error rendering %#v, got SQL %q with params %#v", tc.input, got, params)
					}
					if !strings.Contains(err.Error(), tc.wantErr) {
						t.Fatalf("expected an error containing %q, got: %v", tc.wantErr, err)
					}
				})
			})
		}
	}
}
