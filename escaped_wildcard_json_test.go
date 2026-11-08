package lucene

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/grindlemire/go-lucene/pkg/lucene/expr"
)

// TestEscapedWildcardJSONRoundTrip checks that a term holding a literal * or ?
// stays a plain term through JSON. A JSON string with a * or ? decodes as a
// wildcard, so such a term used to come back as one: a:"fo*" and a:fo\*
// turned into the wildcard fo*.
func TestEscapedWildcardJSONRoundTrip(t *testing.T) {
	inputs := []string{
		`a:fo\*`,
		`a:"fo*"`,
		`a:"f?o"`,
		`a:"/re/"`,
		`a:(fo\* OR b)`,
		`a:["fo*" TO z]`,
		`a:\*x*`,
		`a:fo\*bar*`,
		`fo\*:x`,
	}
	for _, input := range inputs {
		t.Run(input, func(t *testing.T) {
			first, err := Parse(input)
			if err != nil {
				t.Fatalf("parse: %v", err)
			}
			raw, err := json.Marshal(first)
			if err != nil {
				t.Fatalf("marshal: %v", err)
			}
			var second expr.Expression
			if err := json.Unmarshal(raw, &second); err != nil {
				t.Fatalf("unmarshal %s: %v", raw, err)
			}
			if !reflect.DeepEqual(first, &second) {
				t.Fatalf("round trip through JSON %s changed the expression:\n got: %#v\nwant: %#v", raw, &second, first)
			}
		})
	}
}
