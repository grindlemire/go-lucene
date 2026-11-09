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

		// A group in a field's value is rebuilt with the field on each term,
		// which dropped a missing operand or panicked.
		"group_or":         {expr.Eq("a", expr.OR("x", nil)), "an OR needs an expression on each side"},
		"group_and":        {expr.Eq("a", expr.AND("x", nil)), "an AND needs an expression on each side"},
		"group_not":        {expr.Eq("a", expr.NOT(nil)), "a NOT needs an expression"},
		"group_must":       {expr.Eq("a", expr.MUST(nil)), "a MUST needs an expression"},
		"group_must_not":   {expr.Eq("a", expr.MUSTNOT(nil)), "a MUST_NOT needs an expression"},
		"group_nested":     {expr.Eq("a", expr.OR("x", expr.AND("y", expr.NOT(nil)))), "a NOT needs an expression"},
		"group_struct_or":  {expr.Eq("a", &expr.Expression{Op: expr.Or, Left: expr.Lit("x"), Right: none}), "an OR needs an expression on each side"},
		"group_struct_not": {expr.Eq("a", &expr.Expression{Op: expr.Not, Left: none}), "a NOT needs an expression"},
		"json_group_or":    {mustUnmarshal(t, `{"left":"a","operator":"EQUALS","right":{"left":"x","operator":"OR"}}`), "an OR needs an expression on each side"},
		"json_path_in_nil": {expr.IN(expr.Lit(expr.JSONPath{Column: "labels", Path: []string{"k"}}), expr.LIST("x", nil)), "a LIST item is nil"},
		"json_path_in_num": {expr.IN(expr.Lit(expr.JSONPath{Column: "labels", Path: []string{"k"}}), expr.LIST(1, nil)), "a LIST item is nil"},
		"nested_regexp":    {expr.AND(expr.Eq("a", "b"), expr.REGEXP(nil)), "a REGEXP needs a value"},
		"group_in_field":   {expr.Eq("a", expr.OR("y", expr.IN(nil, expr.LIST("x")))), "an IN needs a field"},
		"is_not_null":      {expr.NOT(expr.Eq(nil, expr.NULL())), "an EQUALS needs a field"},
		"list_not_slice":   {&expr.Expression{Op: expr.List, Left: "x"}, "a LIST holds a []*expr.Expression, not string"},
		"json_path_and":    {expr.AND(expr.Lit(expr.JSONPath{Column: "labels", Path: []string{"k"}}), expr.LIST(none)), "a LIST item is nil"},

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

// TestRenderMissingOperandFolded covers a nil IN item on a case-insensitive
// field, which the case folding renderer read before it was rendered.
func TestRenderMissingOperandFolded(t *testing.T) {
	pg, my, lite := NewPostgresDriver(), NewMySQLDriver(), NewSQLiteDriver()
	pg.CaseInsensitiveFields = []string{"a"}
	my.CaseInsensitiveFields = []string{"a"}
	lite.CaseInsensitiveFields = []string{"a"}
	for dname, d := range map[string]renderer{"postgres": pg, "mysql": my, "sqlite": lite} {
		t.Run(dname, func(t *testing.T) {
			noPanic(t, func() {
				in := expr.IN("a", expr.LIST("x", nil))
				if _, err := d.Render(in); err == nil || err.Error() != "a LIST item is nil" {
					t.Fatalf("Render: expected a nil item error, got: %v", err)
				}
				if _, _, err := d.RenderParam(in); err == nil || err.Error() != "a LIST item is nil" {
					t.Fatalf("RenderParam: expected a nil item error, got: %v", err)
				}
			})
		})
	}
}

// TestRenderNilRight covers a nil *expr.Expression on the right of an
// operator that takes no right side. It counts as no right side, as nil
// does, and used to panic.
func TestRenderNilRight(t *testing.T) {
	var none *expr.Expression
	tcs := map[string]struct {
		input *expr.Expression
		want  string
	}{
		"not":     {&expr.Expression{Op: expr.Not, Left: expr.Eq("a", "b"), Right: none}, `NOT("a" = 'b')`},
		"literal": {&expr.Expression{Op: expr.Literal, Left: "x", Right: none}, `'x'`},
	}
	for name, tc := range tcs {
		t.Run(name, func(t *testing.T) {
			noPanic(t, func() {
				got, err := NewPostgresDriver().Render(tc.input)
				if err != nil || got != tc.want {
					t.Fatalf("want %s, got %q and error %v", tc.want, got, err)
				}
			})
		})
	}
}
