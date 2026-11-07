package driver

import (
	"fmt"
	"regexp"
	"strings"
	"unicode"
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
	// UTF-8 without a double quote, backslash or control character, but is
	// otherwise raw user input and must be escaped.
	RenderJSONPath(column string, path []string) (JSONPathSQL, error)
}

// JSONPathSQL is the SQL that reads one key inside a JSON column.
type JSONPathSQL struct {
	// Text reads the value as text. It is NULL for a missing key or a JSON null.
	Text string
	// IsNumber is true when the value is a JSON number.
	IsNumber string
	// Number reads the value as a number. Base evaluates it only where IsNumber
	// is true, so it may fail for other values.
	Number string
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
	case strings.ContainsAny(seg, `"\`):
		return fmt.Errorf("json path segment contains a double quote or backslash: %q", seg)
	case strings.ContainsFunc(seg, unicode.IsControl):
		return fmt.Errorf("json path segment contains a control character: %q", seg)
	}
	return nil
}

func (b Base) renderJSONPath(p expr.JSONPath) (JSONPathSQL, error) {
	jd, ok := b.dialect().(JSONPathDialect)
	if !ok {
		return JSONPathSQL{}, fmt.Errorf("dialect %T does not support json paths (%s)", b.dialect(), p)
	}
	if len(p.Column) == 0 {
		return JSONPathSQL{}, fmt.Errorf("column name is empty")
	}
	if len(p.Path) == 0 {
		return JSONPathSQL{}, fmt.Errorf("json path on column %q is empty", p.Column)
	}
	for _, seg := range p.Path {
		if err := validateJSONPathSegment(seg); err != nil {
			return JSONPathSQL{}, err
		}
	}
	col, err := b.dialect().QuoteColumn(string(p.Column))
	if err != nil {
		return JSONPathSQL{}, err
	}
	return jd.RenderJSONPath(col, p.Path)
}

func noGuard(cmp string) string { return cmp }

// renderLeft renders the left side of e when it is a JSON path. nullLeft is the
// text read for an IS NULL test. When e compares with a number, which only the
// full expression can tell, left reads the value as a number and guard wraps
// the rendered comparison: it holds only for a JSON number, it is FALSE for any
// other present value (so NOT matches it), and it is NULL for a missing key or
// a JSON null, the same as for a NULL column.
func (b Base) renderLeft(e *expr.Expression) (left, nullLeft string, guard func(string) string, isPath bool, err error) {
	lit, ok := e.Left.(*expr.Expression)
	if !ok || lit.Op != expr.Literal {
		return "", "", noGuard, false, nil
	}
	p, ok := lit.Left.(expr.JSONPath)
	if !ok {
		return "", "", noGuard, false, nil
	}
	sql, err := b.renderJSONPath(p)
	if err != nil || !comparesNumber(e) {
		return sql.Text, sql.Text, noGuard, true, err
	}
	guard = func(cmp string) string {
		return fmt.Sprintf("CASE WHEN %s THEN %s WHEN %s IS NOT NULL THEN FALSE END", sql.IsNumber, cmp, sql.Text)
	}
	return sql.Number, sql.Text, guard, true, nil
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
		minVal, minOpen, _ := extractBoundValue(boundary.Min)
		maxVal, maxOpen, _ := extractBoundValue(boundary.Max)
		if minOpen && maxOpen {
			return false
		}
		return (minOpen || isNumericBound(minVal)) && (maxOpen || isNumericBound(maxVal))
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
