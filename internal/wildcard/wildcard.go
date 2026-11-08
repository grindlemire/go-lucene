// Package wildcard reads the value of a Lucene wildcard term. In it * matches
// any run of characters, ? matches one character, and a backslash makes the
// next character literal, so fo\*bar* matches text that starts with fo*bar.
package wildcard

import (
	"strings"
	"unicode/utf8"
)

// Char is one character of a wildcard pattern.
type Char struct {
	// Text is the character: one UTF-8 rune, or a single byte that is not
	// valid UTF-8. A backslash that escapes it is not part of it.
	Text string
	// Escaped is true when a backslash came before the character, which
	// makes it literal.
	Escaped bool
}

// IsWildcard reports whether c is an unescaped * or ?.
func (c Char) IsWildcard() bool {
	return !c.Escaped && (c.Text == "*" || c.Text == "?")
}

// In reports whether c is one of the ASCII characters in set.
func (c Char) In(set string) bool {
	return len(c.Text) == 1 && strings.IndexByte(set, c.Text[0]) >= 0
}

// Split returns the characters of pattern. A backslash at the end has
// nothing to escape, so it is a literal backslash, as in Lucene.
func Split(pattern string) []Char {
	var chars []Char
	for i := 0; i < len(pattern); {
		escaped := false
		if pattern[i] == '\\' && i+1 < len(pattern) {
			escaped = true
			i++
		} else if pattern[i] == '\\' {
			// The escape at the end stands for itself.
			escaped = true
		}
		_, n := utf8.DecodeRuneInString(pattern[i:])
		chars = append(chars, Char{Text: pattern[i : i+n], Escaped: escaped})
		i += n
	}
	return chars
}

// Join writes chars back as a pattern, with a backslash before each escaped
// character.
func Join(chars []Char) string {
	var b strings.Builder
	for _, c := range chars {
		if c.Escaped {
			b.WriteByte('\\')
		}
		b.WriteString(c.Text)
	}
	return b.String()
}

// HasWildcard reports whether pattern has an unescaped * or ?.
func HasWildcard(pattern string) bool {
	for _, c := range Split(pattern) {
		if c.IsWildcard() {
			return true
		}
	}
	return false
}

// Unescape returns pattern with its escapes removed: the text it matches
// when every character is read literally.
func Unescape(pattern string) string {
	var b strings.Builder
	b.Grow(len(pattern))
	for _, c := range Split(pattern) {
		b.WriteString(c.Text)
	}
	return b.String()
}
