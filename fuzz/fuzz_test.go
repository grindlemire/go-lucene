package fuzz

import (
	"strings"
	"testing"

	"github.com/grindlemire/go-lucene"
	"github.com/grindlemire/go-lucene/pkg/driver"
	"github.com/grindlemire/go-lucene/pkg/lucene/expr"
	pg_query "github.com/pganalyze/pg_query_go/v4"
)

func FuzzPostgresDriver(f *testing.F) {
	tcs := []string{
		"A:B AND C:D",
		"+foo OR (NOT(B))",
		"A:bar",
		"NOT(b:c)",
		"z:[* TO 10]",
		"x:[10 TO *] AND NOT(y:[1 TO 5]",
		"(+a:b -c:d) OR (z:[1 TO *] NOT(foo))",
		`+bbq:"woo yay"`,
		`-bbq:"woo"`,
		`(a:b)^10`,
		`a:foo~`,
		`a.b:c AND a.b.c:>5`,
		`a.b:[1 TO 10] OR a.b:(x OR y)`,
		`a.it\'s:x AND NOT a.b:null`,
		`a.b:1.0 AND a.b:(02 OR x) AND a.b:[1e3 TO z]`,
		`a.b\.c:x OR a.b:"5"`,
		`a.b:FoO* AND a.b:(X OR y)`,
		// issue #68: compound range bounds
		`0:[0 0 TO 0]`,
		`a:["\" AND 1) OR 1=1 --" z TO q]`,
		`a:[1 TO (b:c)]`,
		`a:{"x y" TO *}`,
		`a:inf OR b:[NaN TO 1]`,
		`0:1 AND 1.5:>2 AND 007:[1 TO *]`,
		// issue #77: a number used as a field with = or in parentheses
		`1=1 OR (0):* OR ((007)):x OR -1=2`,
		// null is rejected at render time in these positions
		`a:[null TO 1]`,
		`a:>null`,
		`null`,
		// issue #57: case-insensitive fields
		`a:(Foo OR 5 OR null) AND A:F?o*`,
		`-a:"O'Brien" OR default:(x OR y)`,
		// issue #77: numbers a float64 can't hold
		`a:9223372036854775808 OR a:[1e-400 TO 0.1000000000000000055511151231257827] OR a.b:(-12345678901234567891 OR 2)`,
		// + inside a term is part of it
		`a:c++ OR a+b:1+2 OR (x)+y:[a+ TO b+c] OR -1+2`,
	}
	for _, tc := range tcs {
		f.Add(tc)
	}

	f.Fuzz(func(t *testing.T, in string) {
		e, err := lucene.Parse(in)
		if err != nil {
			// Ignore invalid expressions.
			return
		}

		validateRender(t, e)

		// Test the default field option.
		e, err = lucene.Parse(in, lucene.WithDefaultField("default"))
		if err != nil {
			// Ignore invalid expressions.
			return
		}

		validateRender(t, e)

		// Test JSON paths on a registered column.
		e, err = lucene.Parse(in, lucene.WithJSONFields("a", "A"))
		if err != nil {
			// Ignore invalid expressions.
			return
		}

		validateRender(t, e)
	})
}

func validateRender(t *testing.T, e *expr.Expression) {
	validateRenderWith(t, driver.NewPostgresDriver(), e)

	// Fold case on the columns the seeds use.
	folded := driver.NewPostgresDriver()
	folded.CaseInsensitiveFields = []string{"a", "A", "a.b", "default"}
	validateRenderWith(t, folded, e)
}

func validateRenderWith(t *testing.T, d driver.PostgresDriver, e *expr.Expression) {
	f, err := d.Render(e)
	if err != nil {
		// Ignore errors that are expected.
		if strings.Contains(err.Error(), "unable to render operator") ||
			strings.Contains(err.Error(), "literal contains invalid utf8") ||
			strings.Contains(err.Error(), "literal contains null byte") ||
			strings.Contains(err.Error(), "column name contains a double quote") ||
			strings.Contains(err.Error(), "column name is empty") ||
			strings.Contains(err.Error(), "null is not allowed as a range bound") ||
			strings.Contains(err.Error(), "cannot be used with null") ||
			strings.Contains(err.Error(), "null cannot be rendered as a standalone value") ||
			strings.Contains(err.Error(), "the BETWEEN operator needs a two item list in the right hand side") {
			return
		}

		t.Fatal(err)
	}

	requireSingleSelect(t, f)

	// Render accepted this expression, so RenderParam must too: the two
	// renderers share their validation, and a mismatch means one of them
	// is checking something the other is not.
	p, params, err := d.RenderParam(e)
	if err != nil {
		t.Fatalf("RenderParam failed after Render succeeded: %v (sql: %s)", err, f)
	}
	for i, param := range params {
		switch param.(type) {
		case *expr.Expression, expr.Expression, []*expr.Expression, []expr.Expression,
			*expr.RangeBoundary, expr.RangeBoundary:
			t.Fatalf("param %d is an AST node, not a value: %#v (sql: %s)", i, param, p)
		}
	}
	requireSingleSelect(t, p)
}

// requireSingleSelect fails unless the rendered filter, placed between two
// sentinel conditions, parses as one SELECT whose WHERE clause is
// `before = 1 AND (filter) AND after = 2`. A filter that breaks out of a
// literal or identifier must close the paren to do anything useful, which
// adds a statement, adds an AND arm, changes the top-level operator, or
// comments out the trailing sentinel. A statement count alone misses the
// last three: `"\" AND 1) OR 1=1 --"` still parses as one SELECT.
func requireSingleSelect(t *testing.T, filter string) {
	t.Helper()
	query := "SELECT * FROM test WHERE before = 1 AND (" + filter + ") AND after = 2"
	tree, err := pg_query.Parse(query)
	if err != nil {
		t.Fatalf("%v (sql: %s)", err, query)
	}
	if n := len(tree.GetStmts()); n != 1 {
		t.Fatalf("rendered SQL parsed as %d statements (sql: %s)", n, query)
	}
	sel := tree.GetStmts()[0].GetStmt().GetSelectStmt()
	if sel == nil {
		t.Fatalf("rendered SQL is not a SELECT (sql: %s)", query)
	}
	where := sel.GetWhereClause().GetBoolExpr()
	if where == nil || where.GetBoolop() != pg_query.BoolExprType_AND_EXPR || len(where.GetArgs()) != 3 {
		t.Fatalf("filter escaped its parentheses (sql: %s)", query)
	}
	args := where.GetArgs()
	if leftColumn(args[0]) != "before" || leftColumn(args[2]) != "after" {
		t.Fatalf("filter escaped its parentheses (sql: %s)", query)
	}
}

// leftColumn returns the column name on the left of a simple `col = value`
// expression, or "" if n is not one.
func leftColumn(n *pg_query.Node) string {
	fields := n.GetAExpr().GetLexpr().GetColumnRef().GetFields()
	if len(fields) != 1 {
		return ""
	}
	return fields[0].GetString_().GetSval()
}
