package fuzz

import (
	"reflect"
	"testing"

	"github.com/grindlemire/go-lucene"
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
		// JSON paths on the columns a and labels, which the fuzzer registers
		// with WithJSONFields (issue #56)
		`a.b:c AND labels.x.y:>5 AND a.b:[1 TO 10] AND a.b:(x OR y)`,
		`a.b\ c:x OR "labels.a b":fo* OR labels.a\:b:null`,
		`labels.it\'s:(x AND -y) OR "labels.a[0]":1.0 OR "a.k*":/re/`,
		`labels.AND:x OR labels.5:x OR a.null:x OR "a.é":x`,
	}
	for _, tc := range tcs {
		f.Add(tc)
	}

	f.Fuzz(func(t *testing.T, in string) {
		requireRoundTrip(t, in)
		requireRoundTrip(t, in, lucene.WithDefaultField("default"))
		requireRoundTrip(t, in, lucene.WithJSONFields("a", "labels"))
	})
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
