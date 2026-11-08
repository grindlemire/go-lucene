package wildcard

import (
	"reflect"
	"testing"
)

func TestSplit(t *testing.T) {
	tcs := map[string]struct {
		in   string
		want []Char
	}{
		"plain":             {"ab", []Char{{"a", false}, {"b", false}}},
		"wildcards":         {"a*?", []Char{{"a", false}, {"*", false}, {"?", false}}},
		"escaped_star":      {`a\*`, []Char{{"a", false}, {"*", true}}},
		"escaped_escape":    {`a\\*`, []Char{{"a", false}, {`\`, true}, {"*", false}}},
		"escaped_letter":    {`\a`, []Char{{"a", true}}},
		"trailing_escape":   {`a\`, []Char{{"a", false}, {`\`, true}}},
		"escaped_multibyte": {`\é*`, []Char{{"é", true}, {"*", false}}},
		"invalid_utf8":      {"\\\xff\xfe", []Char{{"\xff", true}, {"\xfe", false}}},
		"empty":             {"", nil},
	}
	for name, tc := range tcs {
		t.Run(name, func(t *testing.T) {
			if got := Split(tc.in); !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("Split(%q)\n    want %#v\n    got  %#v", tc.in, tc.want, got)
			}
		})
	}
}

func TestHasWildcard(t *testing.T) {
	tcs := map[string]bool{
		"fo*":      true,
		"f?o":      true,
		`fo\*`:     false,
		`fo\?`:     false,
		`fo\\*`:    true,
		`fo\*bar*`: true,
		"foo":      false,
		`fo\`:      false,
	}
	for in, want := range tcs {
		if got := HasWildcard(in); got != want {
			t.Errorf("HasWildcard(%q) = %v, want %v", in, got, want)
		}
	}
}

func TestJoin(t *testing.T) {
	tcs := map[string]string{
		`fo\*bar*`: `fo\*bar*`,
		`a\\b?`:    `a\\b?`,
		`\ab`:      `\ab`,
		`a\`:       `a\\`,
		"":         "",
	}
	for in, want := range tcs {
		if got := Join(Split(in)); got != want {
			t.Errorf("Join(Split(%q)) = %q, want %q", in, got, want)
		}
	}
}

func TestUnescape(t *testing.T) {
	tcs := map[string]string{
		`fo\*`:    "fo*",
		`a\\b`:    `a\b`,
		`a\ b\:c`: "a b:c",
		`fo*`:     "fo*",
		`a\`:      `a\`,
		"\\\xffx": "\xffx",
		"":        "",
	}
	for in, want := range tcs {
		if got := Unescape(in); got != want {
			t.Errorf("Unescape(%q) = %q, want %q", in, got, want)
		}
	}
}
