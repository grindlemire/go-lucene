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
		ids := make([]string, n)
		for i := range ids {
			ids[i] = fmt.Sprintf(`0190a1b2\-c3d4\-7e5f\-8a9b\-%012d`, i)
		}
		query := "id:(" + strings.Join(ids, " OR ") + ")"

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
