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

// escaper puts a backslash before each character a pattern built in code
// gives a meaning to: the escape itself, the wildcards, and the SIMILAR TO
// operators.
var escaper = strings.NewReplacer(
	`\`, `\\`, `*`, `\*`, `?`, `\?`,
	`|`, `\|`, `(`, `\(`, `)`, `\)`, `[`, `\[`, `]`, `\]`, `{`, `\{`, `}`, `\}`, `+`, `\+`,
)

// Escape returns a pattern that matches the text s and nothing else.
func Escape(s string) string {
	return escaper.Replace(s)
}

// InBrackets reports, for each character, whether it sits inside a bracket
// expression: between an unescaped [ and the unescaped ] that closes it,
// not counting those two. As in POSIX, a ] right after the [ or [^ is a
// member of the set. A [ with no ] to close it starts no bracket expression.
// Only a pattern built in code has one, since the lexer splits a word at [.
func InBrackets(chars []Char) []bool {
	in := make([]bool, len(chars))
	bare := func(i int, text string) bool {
		return i < len(chars) && !chars[i].Escaped && chars[i].Text == text
	}
	for i := 0; i < len(chars); i++ {
		if !bare(i, "[") {
			continue
		}
		end := i + 1
		if bare(end, "^") {
			end++
		}
		if bare(end, "]") {
			end++
		}
		for end < len(chars) && !bare(end, "]") {
			end++
		}
		if end == len(chars) {
			continue
		}
		for k := i + 1; k < end; k++ {
			in[k] = true
		}
		i = end
	}
	return in
}

// HasWildcard reports whether pattern has an unescaped * or ?.
//
// It reads the bytes directly instead of going through Split: the parser
// calls it on every term. A backslash escapes the byte after it, and a
// multi-byte character never holds a byte equal to *, ? or \.
func HasWildcard(pattern string) bool {
	for i := 0; i < len(pattern); i++ {
		switch pattern[i] {
		case '\\':
			i++
		case '*', '?':
			return true
		}
	}
	return false
}

// Unescape returns pattern with its escapes removed: the text it matches
// when every character is read literally. A backslash at the end stands for
// itself, as in Split.
func Unescape(pattern string) string {
	if strings.IndexByte(pattern, '\\') < 0 {
		return pattern
	}
	var b strings.Builder
	b.Grow(len(pattern))
	for i := 0; i < len(pattern); i++ {
		if pattern[i] == '\\' && i+1 < len(pattern) {
			i++
		}
		b.WriteByte(pattern[i])
	}
	return b.String()
}
