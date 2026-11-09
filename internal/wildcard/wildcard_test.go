package wildcard

import (
	"reflect"
	"strings"
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

func TestInBrackets(t *testing.T) {
	tcs := map[string]string{
		"x[ab]y":  "..11..",
		`x[a\]]y`: "..11..",
		"x[]a]y":  "..11..",
		"x[^]a]y": "..111..",
		"x[ab":    "....",
		`x\[ab]`:  ".....",
		"[a][b]":  ".1..1.",
		"[*?]*":   ".11..",
		"":        "",
	}
	for in, want := range tcs {
		var got strings.Builder
		for _, in := range InBrackets(Split(in)) {
			if in {
				got.WriteByte('1')
			} else {
				got.WriteByte('.')
			}
		}
		if got.String() != want {
			t.Errorf("InBrackets(Split(%q)) = %s, want %s", in, got.String(), want)
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

// HasWildcard and Unescape must give the same answer as reading the pattern
// through Split, which is the definition of an escape.
func TestScansMatchSplit(t *testing.T) {
	inputs := []string{
		"", "a", "*", "?", `\`, `\\`, `\*`, `\?`, `a\`, `a\\`, `a\\*`, `a\\\*`,
		`fo*`, `f?o`, `fo\*bar*`, `0190a1b2\-c3d4\-7e5f`, `a\ b\:c`, `\é*`, `é\*`,
		"\\\xff\xfe", "\xff*", `[a\]]*`, `\[*\]`, `日本\語?`,
	}
	for _, in := range inputs {
		wantWild := false
		var want strings.Builder
		for _, c := range Split(in) {
			wantWild = wantWild || c.IsWildcard()
			want.WriteString(c.Text)
		}
		if got := HasWildcard(in); got != wantWild {
			t.Errorf("HasWildcard(%q) = %v, want %v", in, got, wantWild)
		}
		if got := Unescape(in); got != want.String() {
			t.Errorf("Unescape(%q) = %q, want %q", in, got, want.String())
		}
	}
}

// The parser calls both on every term, so neither may allocate a slice per
// character. A term without a backslash unescapes to itself.
func TestScansDoNotAllocate(t *testing.T) {
	escaped := `0190a1b2\-c3d4\-7e5f\-8a9b\-0c1d2e3f4a5b`
	if n := testing.AllocsPerRun(100, func() { HasWildcard(escaped) }); n != 0 {
		t.Errorf("HasWildcard allocates %v times, want 0", n)
	}
	if n := testing.AllocsPerRun(100, func() { Unescape("plain-term") }); n != 0 {
		t.Errorf("Unescape of a term without escapes allocates %v times, want 0", n)
	}
	if n := testing.AllocsPerRun(100, func() { Unescape(escaped) }); n > 1 {
		t.Errorf("Unescape allocates %v times, want at most 1", n)
	}
}
