package driver

import (
	"testing"

	"github.com/grindlemire/go-lucene/pkg/lucene/expr"
)

const errTemplate = "%s:\n    wanted %s\n    got    %s"

func TestSQLDriver(t *testing.T) {
	type tc struct {
		input *expr.Expression
		want  string
	}

	tcs := map[string]tc{
		"simple_equals": {
			input: expr.Eq("a", 5),
			want:  "a = 5",
		},
		// "simple_and": {
		// 	input: expr.AND(expr.Eq("a", 5), expr.Eq("b", "foo")),
		// 	want:  `a = 5 AND b = "foo"`,
		// },
		// "simple_or": {
		// 	input: expr.OR(expr.Eq("a", 5), expr.Eq("b", "foo")),
		// 	want:  `a = 5 OR b = "foo"`,
		// },
	}

	for name, tc := range tcs {
		t.Run(name, func(t *testing.T) {
			got, err := NewSQLDriver().Render(tc.input)
			if err != nil {
				t.Fatalf("got an unexpected error when rendering: %v", err)
			}

			if tc.want != got {
				t.Fatalf(errTemplate, "generated sql does not match", tc.want, got)
			}
		})
	}
}
