package fuzz

import (
	"reflect"
	"strings"
	"testing"

	"github.com/grindlemire/go-lucene"
	"github.com/grindlemire/go-lucene/pkg/lucene/expr"
)

// FuzzStringRoundTrip checks that String() prints a parsed query as text
// that parses back to the same expression.
func FuzzStringRoundTrip(f *testing.F) {
	tcs := []string{
		// from FuzzPostgresDriver
		"A:B AND C:D",
		"+foo OR (NOT(B))",
		"NOT(b:c)",
		"z:[* TO 10]",
		"(+a:b -c:d) OR (z:[1 TO *] NOT(foo))",
		`+bbq:"woo yay"`,
		`(a:b)^10`,
		`a:foo~`,
		`a:{"x y" TO *}`,
		`a:inf OR b:[NaN TO 1]`,
		`0:1 AND 1.5:>2 AND 007:[1 TO *]`,
		`a:[null TO 1]`,
		`a:(Foo OR 5 OR null) AND A:F?o*`,
		`-a:"O'Brien" OR default:(x OR y)`,
		// operators (issue #70)
		`a:>5 AND b:<=-2.5 AND c:>="x y"`,
		`a:fo* OR a:f?o OR a:\*x* OR a:/re.*/ OR a:/foo bar/`,
		`a:(b OR 1 OR 2.0 OR "x y" OR null)`,
		`-a:(b OR c)^2 AND k4:a*~10`,
		// terms that need quotes
		`a:"5" b:"null" c:"AND" d:"fo*" e:"" f:"a:b"`,
		`a:"C:\temp" b:"say \"hi\"" c:'it\'s' d:a\:b`,
		`"foo bar":x AND foo\ bar:y AND "AND":z`,
		`a:5.0 b:1e3 c:1e21 d:[0.5 TO 5.0]`,
		// grouping
		`a:1 AND (b:2 OR (c:3 AND -(d:4 OR e:5)))`,
		`a:(b:c) OR a:(b:[1 TO 5]) OR -(5) OR -(1*)`,
		`a:b^0.5 OR a:b^1.25 OR a:b~0 OR (a:b*~0)^0.5`,
		`NOT a:/re.*/ AND +(+a:1) AND -(NOT a:1)`,
	}
	for _, tc := range tcs {
		f.Add(tc)
	}

	f.Fuzz(func(t *testing.T, in string) {
		requireRoundTrip(t, in)
		requireRoundTrip(t, in, lucene.WithDefaultField("default"))
	})
}

func requireRoundTrip(t *testing.T, in string, opts ...lucene.Opt) {
	t.Helper()
	first, err := lucene.Parse(in, opts...)
	if err != nil {
		// Ignore invalid expressions.
		return
	}
	if hasKnownParseMismatch(first) {
		return
	}

	rendered := first.String()
	second, err := lucene.Parse(rendered, opts...)
	if err != nil {
		t.Fatalf("String() of %q is %q, which does not parse: %v", in, rendered, err)
	}
	if !reflect.DeepEqual(first, second) {
		t.Fatalf("String() of %q is %q, which parses to a different expression:\n    first  %#v\n    second %#v", in, rendered, first, second)
	}
}

// hasKnownParseMismatch reports expressions that String() prints correctly
// but that the parser builds differently from the printed text. These are
// parser bugs, not String() bugs:
//
//   - A number used as a field stays a number unless a : follows it
//     directly. 1=1 and (0):* keep the number, while 1:1 and 0:* turn it
//     into a column, and String() always prints the second form.
//   - With a default field, a query that is a single quoted term such as
//     "fo*" becomes an EQUALS whose value is a wildcard or regex, where
//     default:fo* parses as LIKE.
//   - The lexer accepts a backslash at the very end of the input, so a*\
//     parses as the wildcard a*\. Anywhere else that backslash escapes the
//     next character, so there is no way to print the wildcard inside a
//     larger query.
func hasKnownParseMismatch(e *expr.Expression) bool {
	if e == nil {
		return false
	}
	if s, ok := e.Left.(string); ok && e.Op == expr.Wild {
		trailing := len(s) - len(strings.TrimRight(s, `\`))
		if trailing%2 == 1 {
			return true
		}
	}
	switch e.Op {
	case expr.Equals, expr.Like, expr.In, expr.Range,
		expr.Greater, expr.Less, expr.GreaterEq, expr.LessEq:
		if l, ok := e.Left.(*expr.Expression); ok {
			switch l.Left.(type) {
			case int, float64:
				return true
			}
		}
	}
	if e.Op == expr.Equals {
		if r, ok := e.Right.(*expr.Expression); ok && (r.Op == expr.Wild || r.Op == expr.Regexp) {
			return true
		}
	}
	for _, sub := range []any{e.Left, e.Right} {
		if s, ok := sub.(*expr.Expression); ok && hasKnownParseMismatch(s) {
			return true
		}
	}
	return false
}
