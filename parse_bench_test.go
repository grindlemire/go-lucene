package lucene

import (
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
