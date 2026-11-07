package driver

import (
	"fmt"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/grindlemire/go-lucene/pkg/lucene/expr"
)

// JSONPathDialect is implemented by dialects that can read a key inside a JSON
// column. It is separate from Dialect so that custom dialects written before
// JSON path support keep compiling; rendering an expr.JSONPath with a dialect
// that lacks it returns an error.
type JSONPathDialect interface {
	// RenderJSONPath renders read access to path inside the JSON column. column
	// is already quoted by QuoteColumn. Each path segment is non-empty valid
	// UTF-8 without a double quote, backslash or null byte, but is otherwise
	// raw user input and must be escaped. numeric is true when the value is
	// compared with a number, so dialects that extract text should cast.
	RenderJSONPath(column string, path []string, numeric bool) (string, error)
}

// bareJSONPathKey matches keys that need no quoting in a '$.a.b' path.
var bareJSONPathKey = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

// dollarJSONPath builds the '$.a.b' path used by MySQL and SQLite. Keys that are
// not identifier-shaped are double-quoted ($."a.b"), so dots, brackets, spaces
// and quotes inside a key cannot change the path.
func dollarJSONPath(path []string) string {
	var b strings.Builder
	b.WriteString("$")
	for _, seg := range path {
		b.WriteString(".")
		if bareJSONPathKey.MatchString(seg) {
			b.WriteString(seg)
		} else {
			b.WriteString(`"` + seg + `"`)
		}
	}
	return b.String()
}

func validateJSONPathSegment(seg string) error {
	switch {
	case seg == "":
		return fmt.Errorf("json path has an empty segment")
	case !utf8.ValidString(seg):
		return fmt.Errorf("json path segment is not valid UTF-8: %q", seg)
	case strings.ContainsAny(seg, "\"\\\x00"):
		return fmt.Errorf("json path segment contains a double quote, backslash or null byte: %q", seg)
	}
	return nil
}

func (b Base) renderJSONPath(p expr.JSONPath, numeric bool) (string, error) {
	jd, ok := b.dialect().(JSONPathDialect)
	if !ok {
		return "", fmt.Errorf("dialect %T does not support json paths (%s)", b.dialect(), p)
	}
	if len(p.Column) == 0 {
		return "", fmt.Errorf("column name is empty")
	}
	if len(p.Path) == 0 {
		return "", fmt.Errorf("json path on column %q is empty", p.Column)
	}
	for _, seg := range p.Path {
		if err := validateJSONPathSegment(seg); err != nil {
			return "", err
		}
	}
	col, err := b.dialect().QuoteColumn(string(p.Column))
	if err != nil {
		return "", err
	}
	return jd.RenderJSONPath(col, p.Path, numeric)
}

// renderLeft renders the left side of e. A JSON path is cast to a number when
// the right side is numeric, which only the full expression can tell.
func (b Base) renderLeft(e *expr.Expression) (string, bool, error) {
	lit, ok := e.Left.(*expr.Expression)
	if !ok || lit.Op != expr.Literal {
		return "", false, nil
	}
	p, ok := lit.Left.(expr.JSONPath)
	if !ok {
		return "", false, nil
	}
	s, err := b.renderJSONPath(p, comparesNumber(e))
	return s, true, err
}

// comparesNumber reports whether e compares its left side with a number.
func comparesNumber(e *expr.Expression) bool {
	switch e.Op {
	case expr.Equals, expr.Greater, expr.Less, expr.GreaterEq, expr.LessEq:
		return isNumberLiteral(e.Right)
	case expr.Range:
		boundary, ok := e.Right.(*expr.RangeBoundary)
		if !ok {
			return false
		}
		minVal, _, _ := extractBoundValue(boundary.Min)
		maxVal, _, _ := extractBoundValue(boundary.Max)
		return isNumericBound(minVal) || isNumericBound(maxVal)
	case expr.In:
		items, _, ok := partitionNullsFromList(e.Right)
		if !ok || len(items) == 0 {
			return false
		}
		for _, item := range items {
			if !isNumberLiteral(item) {
				return false
			}
		}
		return true
	default:
		return false
	}
}

func isNumberLiteral(in any) bool {
	if e, ok := in.(*expr.Expression); ok {
		if e.Op != expr.Literal {
			return false
		}
		in = e.Left
	}
	return isNumericBound(in)
}
