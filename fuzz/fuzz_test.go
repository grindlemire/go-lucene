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
	})
}

func validateRender(t *testing.T, e *expr.Expression) {
	f, err := driver.NewPostgresDriver().Render(e)
	if err != nil {
		// Ignore errors that are expected.
		if strings.Contains(err.Error(), "unable to render operator") ||
			strings.Contains(err.Error(), "literal contains invalid utf8") ||
			strings.Contains(err.Error(), "literal contains null byte") ||
			strings.Contains(err.Error(), "column name contains a double quote") ||
			strings.Contains(err.Error(), "column name is empty") ||
			strings.Contains(err.Error(), "the BETWEEN operator needs a two item list in the right hand side") {
			return
		}

		t.Fatal(err)
	}

	j, err := pg_query.ParseToJSON("SELECT * FROM test WHERE a = b AND (" + f + ")")
	if err != nil {
		t.Fatal(err)
	}

	if strings.Contains(j, "CommentStmt") {
		t.Fatal("CommentStmt found")
	}
}
