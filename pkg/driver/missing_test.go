package driver

import (
	"strings"
	"testing"

	"github.com/grindlemire/go-lucene/pkg/lucene/expr"
)

// TestRenderMissingOperand covers trees built in code or read from JSON with
// an operand left out, as a nil or a nil *expr.Expression. They used to panic
// or render broken SQL such as "a" = .
func TestRenderMissingOperand(t *testing.T) {
	var none *expr.Expression
	col := expr.Lit(expr.Column("a"))
	tcs := map[string]struct {
		input   *expr.Expression
		wantErr string
	}{
		"eq_nil":             {expr.Eq("a", nil), "an EQUALS needs a value"},
		"eq_typed_nil":       {expr.Eq("a", none), "an EQUALS needs a value"},
		"like_typed_nil":     {expr.LIKE("a", none), "a LIKE needs a value"},
		"greater_typed_nil":  {expr.GREATER("a", none), "a GREATER needs a value"},
		"in_nil":             {expr.IN("a", nil), "an IN needs a value"},
		"in_string":          {expr.IN("a", "x"), "the right side of an IN must be a LIST, not string"},
		"in_empty_list":      {expr.IN("a", expr.LIST()), "a LIST needs at least one value"},
		"in_nil_item":        {expr.IN("a", expr.LIST("x", none)), "a LIST item is nil"},
		"and_nil":            {expr.AND(expr.Eq("a", "b"), nil), "an AND needs an expression on each side"},
		"and_typed_nil":      {expr.AND(expr.Eq("a", "b"), none), "an AND needs an expression on each side"},
		"or_typed_nil":       {expr.OR(none, expr.Eq("a", "b")), "an OR needs an expression on each side"},
		"not_typed_nil":      {expr.NOT(none), "a NOT needs an expression"},
		"must_typed_nil":     {expr.MUST(none), "a MUST needs an expression"},
		"must_not_typed_nil": {expr.MUSTNOT(none), "a MUST_NOT needs an expression"},
		"literal_nil":        {expr.Lit(nil), "a LITERAL needs a value"},
		"field_typed_nil":    {expr.Eq(none, "b"), "an EQUALS needs a field"},
		"nested":             {expr.OR(expr.Eq("a", "b"), expr.AND(expr.Eq("c", "d"), none)), "an AND needs an expression on each side"},

		// Struct literals skip the constructors, so a nil *expr.Expression
		// stays typed.
		"struct_and":   {&expr.Expression{Op: expr.And, Left: expr.Eq("a", "b"), Right: none}, "an AND needs an expression on each side"},
		"struct_like":  {&expr.Expression{Op: expr.Like, Left: col, Right: none}, "a LIKE needs a value"},
		"struct_range": {&expr.Expression{Op: expr.Range, Left: col, Right: (*expr.RangeBoundary)(nil)}, "a RANGE needs bounds"},
		"struct_list":  {&expr.Expression{Op: expr.In, Left: col, Right: &expr.Expression{Op: expr.List, Left: []*expr.Expression{none}}}, "a LIST item is nil"},

		"json_and": {mustUnmarshal(t, `{"left":{"left":"a","operator":"EQUALS","right":"b"},"operator":"AND"}`), "an AND needs an expression on each side"},
		"json_eq":  {mustUnmarshal(t, `{"left":"a","operator":"EQUALS"}`), "an EQUALS needs a value"},
		"json_in":  {mustUnmarshal(t, `{"left":"a","operator":"IN","right":{"left":[],"operator":"LIST"}}`), "a LIST needs at least one value"},
	}
	for name, tc := range tcs {
		for dname, d := range allDrivers {
			t.Run(name+"/"+dname+"/render", func(t *testing.T) {
				got, err := d.Render(tc.input)
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("expected an error containing %q, got SQL %q and error %v", tc.wantErr, got, err)
				}
			})
			t.Run(name+"/"+dname+"/render_param", func(t *testing.T) {
				got, params, err := d.RenderParam(tc.input)
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("expected an error containing %q, got SQL %q with params %#v and error %v", tc.wantErr, got, params, err)
				}
			})
		}
	}
}

// TestRenderListOfValues covers LIST built from plain values, which used to
// panic. Each value is matched exactly, even one with a * in it.
func TestRenderListOfValues(t *testing.T) {
	got, params, err := NewPostgresDriver().RenderParam(expr.IN("a", expr.LIST("x", 5, "y*")))
	if err != nil {
		t.Fatal(err)
	}
	if want := `"a" IN ($1, $2, $3)`; got != want {
		t.Fatalf("want %s, got %s", want, got)
	}
	if len(params) != 3 || params[0] != "x" || params[1] != 5 || params[2] != "y*" {
		t.Fatalf("unexpected params %#v", params)
	}
}
