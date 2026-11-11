package lucene

import (
	"fmt"
	"strings"
	"testing"
)

// BenchmarkParseDeepNesting parses a group nested many levels deep. Each reduce
// checks whether it is inside a field's value; that check must not grow with
// the nesting depth, or parse time grows with its square.
func BenchmarkParseDeepNesting(b *testing.B) {
	const depth = 10000
	query := strings.Repeat("(", depth) + "x" + strings.Repeat(")", depth)

	for _, tc := range []struct {
		name string
		opts []Opt
	}{
		{name: "no_default_field"},
		{name: "default_field", opts: []Opt{WithDefaultField("default")}},
	} {
		b.Run(tc.name, func(b *testing.B) {
			for b.Loop() {
				if _, err := Parse(query, tc.opts...); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

// BenchmarkParseLongGroup parses a field group of many escaped values, the
// shape of a batched lookup by id. Every term is checked for a wildcard and
// unescaped, so that check must not allocate per character.
func BenchmarkParseLongGroup(b *testing.B) {
	for _, n := range []int{15, 100, 1000} {
		query := longGroup(n)
		b.Run(fmt.Sprint(n), func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				if _, err := Parse(query); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

// TestParseAllocsPerTerm caps the allocations per term when parsing a long
// field group. A count does not vary between machines the way time does, so
// this check can run in CI. The budget leaves room for small changes but
// fails when a change adds work to every term. Parse allocates about 22.5
// times per term on Go 1.25 and 21.5 on Go 1.27, and going back to Split in
// HasWildcard or Unescape adds about 5.
func TestParseAllocsPerTerm(t *testing.T) {
	if testing.CoverMode() != "" {
		t.Skip("coverage instrumentation changes the allocation count")
	}
	const terms, budget = 100, 26
	query := longGroup(terms)
	allocs := testing.AllocsPerRun(20, func() {
		if _, err := Parse(query); err != nil {
			t.Fatal(err)
		}
	})
	if perTerm := allocs / terms; perTerm > budget {
		t.Fatalf("Parse allocates %.1f times per term, budget %d", perTerm, budget)
	}
}

// longGroup returns a field group of n escaped ids joined by OR.
func longGroup(n int) string {
	ids := make([]string, n)
	for i := range ids {
		ids[i] = fmt.Sprintf(`0190a1b2\-c3d4\-7e5f\-8a9b\-%012d`, i)
	}
	return "id:(" + strings.Join(ids, " OR ") + ")"
}
