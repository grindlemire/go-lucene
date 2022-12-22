package expr

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

const (
	errTemplate     = "%s:\n    wanted %#v\n    got    %#v"
	jsonErrTemplate = "%s:\n    wanted %s\n    got    %s"
)

func TestExprJSON(t *testing.T) {
	type tc struct {
		input string
		want  *Expression
	}

	tcs := map[string]tc{
		"flat_literal": {
			input: `"a"`,
			want:  Lit("a"),
		},
		"flat_equals": {
			input: `{"left": "a", "operator": "EQUALS", "right": "b"}`,
			want:  Eq(Lit("a"), Lit("b")),
		},
		"flat_regexp": {
			input: `{
				"left": "a",
				"operator": "EQUALS",
				"right": "/b [c]/"
			  }`,
			want: Eq(Lit("a"), REGEXP("/b [c]/")),
		},
		"flat_inclusive_range": {
			input: `{
				"left": 1,
				"operator": "RANGE",
				"right": 2
			  }`,
			want: Rang(Lit(1), Lit(2), true),
		},
		"flat_exclusive_range": {
			input: `{
				"left": 1,
				"operator": "RANGE",
				"right": 2,
				"exclusive": true
			  }`,
			want: Rang(Lit(1), Lit(2), false),
		},
		"flat_must": {
			input: `{
				"left": "a",
				"operator": "MUST"
			}`,
			want: MUST(Lit("a")),
		},
	}

	for name, tc := range tcs {
		t.Run(name, func(t *testing.T) {
			got := &Expression{}
			err := json.Unmarshal([]byte(tc.input), got)
			if err != nil {
				t.Fatalf("expected no error during unmarshal but got [%s]", err)
			}

			// if !reflect.DeepEqual(tc.want, got) {
			// 	t.Fatalf(errTemplate, "parsed expression doesn't match", tc.want, got)
			// }
			require.Equal(t, tc.want, got)

			gotSerialized, err := json.Marshal(got)
			if err != nil {
				t.Fatalf("expected no error during marshal but got [%s]", err)
			}

			if !jsonEqual(string(gotSerialized), tc.input) {
				t.Fatalf(
					jsonErrTemplate,
					"serialized expressions don't match",
					stripWhitespace(tc.input),
					stripWhitespace(string(gotSerialized)),
				)
			}
		})
	}
}

func jsonEqual(got string, want string) bool {
	return stripWhitespace(got) == stripWhitespace(want)
}

func stripWhitespace(in string) string {
	return strings.Join(strings.Fields(in), "")
}
