package driver

import (
	"fmt"
	"strings"

	"github.com/grindlemire/go-lucene/internal/wildcard"
	"github.com/grindlemire/go-lucene/pkg/lucene/expr"
)

// PostgresDriver transforms a parsed lucene expression to a postgres sql filter.
type PostgresDriver struct {
	Base
}

// NewPostgresDriver creates a new driver that will output postgres filter strings from parsed lucene expressions.
func NewPostgresDriver() PostgresDriver {
	fns := map[expr.Operator]RenderFN{}
	for op, sharedFN := range Shared {
		fns[op] = sharedFN
	}
	return PostgresDriver{
		Base{
			RenderFNs: fns,
			Dialect:   postgresDialect{},
		},
	}
}

// RenderParam will render the expression into a parameterized query using PostgreSQL's $N placeholder format.
// The returned string will contain $1, $2, $3, etc. placeholders and the params will contain the values
// that should be passed to the query.
func (p PostgresDriver) RenderParam(e *expr.Expression) (s string, params []any, err error) {
	// First, use the base implementation to get the result with ? placeholders
	str, params, err := p.Base.RenderParam(e)
	if err != nil {
		return s, params, err
	}

	return numberPlaceholders(str), params, nil
}

// numberPlaceholders converts each ? placeholder to $N. A ? inside a quoted
// string literal or identifier is text, not a placeholder, so it is kept. A
// doubled quote inside a literal closes and reopens it, which keeps the state.
func numberPlaceholders(str string) string {
	paramIndex := 1
	result := strings.Builder{}
	var quote byte
	for i := 0; i < len(str); i++ {
		c := str[i]
		switch {
		case quote != 0:
			if c == quote {
				quote = 0
			}
		case c == '\'' || c == '"':
			quote = c
		case c == '?':
			result.WriteString(fmt.Sprintf("$%d", paramIndex))
			paramIndex++
			continue
		}
		result.WriteByte(c)
	}
	return result.String()
}

// postgresDialect implements Dialect for PostgreSQL. It is a lift-and-shift
// of the behavior previously baked into Base — SIMILAR TO for wildcards,
// ~ for regex, SIMILAR TO '%' for standalone wildcard, Postgres-style
// %/_ pattern escaping, and true/false bool literals.
type postgresDialect struct{}

func (postgresDialect) RenderLike(left, right string, isRegex bool) (string, error) {
	if isRegex {
		return fmt.Sprintf("%s ~ %s", left, right), nil
	}
	return fmt.Sprintf("%s SIMILAR TO %s", left, right), nil
}

func (postgresDialect) RenderStandaloneWild(left string) (string, error) {
	return fmt.Sprintf("%s SIMILAR TO '%%'", left), nil
}

// similarToOperators are the characters SIMILAR TO gives a meaning to, apart
// from its wildcards % and _. In a pattern built in code, an unescaped one
// keeps that meaning, so *(a|b)* matches text that contains a or b.
const similarToOperators = "|*+?{}()[]"

// bracketMetachars are the characters a regex bracket expression gives a
// meaning to. Postgres and the MySQL REGEXP fallback keep the backslash on an
// escaped one inside brackets, so x[\^a] stays the set of ^ and a.
const bracketMetachars = `\]^-[`

// PrepareLikePattern writes a Lucene wildcard pattern for SIMILAR TO, whose
// escape character is the backslash. Writing \ before an alphanumeric
// character would make a regex escape such as \d, so only the characters
// SIMILAR TO gives a meaning to are escaped. Inside a bracket expression,
// which only a pattern built in code has, every character is a member of the
// set: * and ? are not wildcards there, and only bracketMetachars keep their
// backslash.
func (postgresDialect) PrepareLikePattern(pattern string) (string, bool) {
	var b strings.Builder
	b.Grow(len(pattern) + 4)
	chars := wildcard.Split(pattern)
	inBrackets := wildcard.InBrackets(chars)
	for i, c := range chars {
		switch {
		case inBrackets[i] && c.Escaped && c.In(bracketMetachars):
			b.WriteString(`\` + c.Text)
		case inBrackets[i]:
			b.WriteString(c.Text)
		case c.IsWildcard() && c.Text == "*":
			b.WriteByte('%')
		case c.IsWildcard():
			b.WriteByte('_')
		case c.In(`%_\`), c.Escaped && c.In(similarToOperators):
			b.WriteString(`\` + c.Text)
		default:
			b.WriteString(c.Text)
		}
	}
	return b.String(), false
}

func (postgresDialect) EscapeStringLiteral(s string) string {
	return fmt.Sprintf("'%s'", strings.ReplaceAll(s, "'", "''"))
}

func (postgresDialect) SerializeBool(b bool) string {
	if b {
		return "true"
	}
	return "false"
}

func (postgresDialect) BoolParam(b bool) any { return b }

func (postgresDialect) QuoteColumn(name string) (string, error) {
	if strings.ContainsRune(name, '"') {
		return "", fmt.Errorf("column name contains a double quote: %q", name)
	}
	return `"` + name + `"`, nil
}

// jsonNumberPattern matches the text of a JSON number whose exponent has at
// most four digits. With the maxJSONNumberLength check this keeps the numeric
// cast inside its limits (131072 digits before the point, 16383 after). The
// pattern has no backslash, so it means the same with or without
// standard_conforming_strings.
const jsonNumberPattern = `^-?[0-9]+([.][0-9]+)?([eE][+-]?[0-9]{1,4})?$`

const maxJSONNumberLength = 1000

// RenderJSONPath chains -> through intermediate objects and ends with ->> for
// the text leaf: "metadata"->'address'->>'city'. ->> already yields text, so it
// cannot tell the string "5" from the number 5; IsNumber checks the JSON type
// with json_typeof, which reads json and jsonb columns alike. The numeric read
// is a cast, so it runs only where IsNumber holds.
func (d postgresDialect) RenderJSONPath(column string, path []string) (JSONPathSQL, error) {
	var parent strings.Builder
	parent.WriteString(column)
	for _, seg := range path[:len(path)-1] {
		parent.WriteString("->" + d.EscapeStringLiteral(seg))
	}
	leaf := d.EscapeStringLiteral(path[len(path)-1])
	text := parent.String() + "->>" + leaf
	value := parent.String() + "->" + leaf
	return JSONPathSQL{
		Text:  text,
		Value: text,
		IsNumber: fmt.Sprintf("json_typeof((%s)::json) = 'number' AND length(%s) <= %d AND %s ~ '%s'",
			value, text, maxJSONNumberLength, text, jsonNumberPattern),
		Number: fmt.Sprintf("(%s)::numeric", text),
	}, nil
}
