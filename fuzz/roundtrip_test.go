package fuzz

import (
	"reflect"
	"strings"
	"testing"

	"github.com/grindlemire/go-lucene"
	"github.com/grindlemire/go-lucene/pkg/driver"
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
		// parser fixes from issue #77
		`1=1 OR (0):* OR (007):x OR "fo*" OR "/re/"`,
		`a:(x AND -y) OR a:(x OR [1 TO 5]) OR a:(b:c OR d*)`,
		`x OR null OR foo* OR /re/ OR [1 TO 5]`,
		`a:1E+3 OR a:[-.5 TO .5] OR a:.5e+1`,
		`a:9223372036854775808 OR a:[1e-400 TO 0.1000000000000000055511151231257827] OR a:-0e-400`,
		// + inside a term is part of it
		`a:c++ OR a+b:1+2 OR (x)+y:[a+ TO b+c] OR -1+2 OR a:"+x"`,
	}
	for _, tc := range tcs {
		f.Add(tc)
	}

	f.Fuzz(func(t *testing.T, in string) {
		requireRoundTrip(t, in)
		requireRoundTrip(t, in, lucene.WithDefaultField("default"))
	})
}

// FuzzBuiltWildcardString checks String() on a wildcard built in code, which
// holds only its value. The text must parse back to a wildcard that renders
// the same SQL pattern.
func FuzzBuiltWildcardString(f *testing.F) {
	for _, tc := range []string{"foo bar*", `fo\*bar*`, "a:b?", "-5*", "-x*", `a*\`, "1e+5*", "é€*", `it's "x"*`, "/x*/"} {
		f.Add(tc)
	}
	d := driver.NewPostgresDriver()
	f.Fuzz(func(t *testing.T, pattern string) {
		// A pattern with no * or ? prints as a plain term, and SIMILAR TO
		// operators have no Lucene syntax, so String() cannot keep them.
		if !hasWildcard(pattern) || strings.ContainsAny(pattern, "|()[]{}+") {
			return
		}
		// A LIKE reads a literal on its right as a pattern too.
		for _, built := range []*expr.Expression{
			expr.LIKE("a", expr.WILD(pattern)),
			expr.LIKE("a", expr.Lit(pattern)),
		} {
			rendered := built.String()
			parsed, err := lucene.Parse(rendered)
			if err != nil {
				t.Fatalf("String() of %#v is %q, which does not parse: %v", built, rendered, err)
			}
			_, want, err := d.RenderParam(built)
			if err != nil {
				return
			}
			_, got, err := d.RenderParam(parsed)
			if err != nil {
				t.Fatalf("String() of %#v is %q, which does not render: %v", built, rendered, err)
			}
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("String() of %#v is %q, which renders %#v instead of %#v", built, rendered, got, want)
			}
		}
	})
}

// hasWildcard reports whether pattern has a * or ? that no backslash escapes.
func hasWildcard(pattern string) bool {
	for i := 0; i < len(pattern); i++ {
		switch pattern[i] {
		case '\\':
			i++
		case '*', '?':
			return true
		}
	}
	return false
}

func requireRoundTrip(t *testing.T, in string, opts ...lucene.Opt) {
	t.Helper()
	first, err := lucene.Parse(in, opts...)
	if err != nil {
		// Ignore invalid expressions.
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
