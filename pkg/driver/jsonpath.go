package driver

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

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

// JSONPathSQL is the SQL that reads one key inside a JSON column. Every field
// must be set; rendering returns an error for an empty one.
type JSONPathSQL struct {
	// Text reads the value as text: a string as itself, a number as its digits
	// and a boolean as true or false. It is NULL for a missing key or a JSON
	// null.
	Text string
	// Value reads the value for equality and IN with plain text: a term that
	// is not a number, true, false or null. It can be a plainer accessor than
	// Text, so that an index on it applies, but for those terms it must give
	// the same result as Text, also for a JSON null. Text itself is a valid
	// Value.
	Value string
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

func (b Base) renderJSONPath(p expr.JSONPath) (JSONPathSQL, error) {
	jd, ok := b.dialect().(JSONPathDialect)
	if !ok {
		return JSONPathSQL{}, fmt.Errorf("dialect %T does not support json paths (%s)", b.dialect(), p)
	}
	if err := p.Validate(); err != nil {
		return JSONPathSQL{}, err
	}
	col, err := b.quoteColumn(p.Column)
	if err != nil {
		return JSONPathSQL{}, err
	}
	sql, err := jd.RenderJSONPath(col, p.Path)
	if err != nil {
		return JSONPathSQL{}, err
	}
	for _, f := range []struct{ name, sql string }{{"Text", sql.Text}, {"Value", sql.Value}, {"IsNumber", sql.IsNumber}, {"Number", sql.Number}} {
		if strings.TrimSpace(f.sql) == "" {
			return JSONPathSQL{}, fmt.Errorf("dialect %T returned an empty %s for json path %s", b.dialect(), f.name, p)
		}
	}
	return sql, nil
}

// jsonPathLeft returns the JSON path on the left side of e, if there is one.
func jsonPathLeft(e *expr.Expression) (expr.JSONPath, bool) {
	lit, ok := e.Left.(*expr.Expression)
	if !ok || lit.Op != expr.Literal {
		return expr.JSONPath{}, false
	}
	p, ok := lit.Left.(expr.JSONPath)
	return p, ok
}

type compareFunc func(e *expr.Expression, left, nullLeft string) (string, []any, error)

// renderJSONPathCompare renders e, whose left side is the JSON path p.
//
// Equality and IN with plain text compare sql.Value, so that an index on the
// accessor applies. Any other comparison with text, or a range or IN list that
// mixes numbers and text, compares the value as text, with any number written
// as text.
//
// Equality and IN with numbers match a JSON number by value and any other
// value by its text, so labels.version:2 matches both 2 and "2".
//
// A numeric comparison (>, <, a numeric range) holds only for a JSON number.
// It is FALSE for any other present value, so NOT matches it, and NULL for a
// missing key or a JSON null, the same as for a NULL column.
func (b Base) renderJSONPathCompare(e *expr.Expression, p expr.JSONPath, compare compareFunc) (string, []any, error) {
	sql, err := b.renderJSONPath(p)
	if err != nil {
		return "", nil, err
	}
	if comparesPlainText(e) {
		return compare(e, sql.Value, sql.Value)
	}
	asText := numbersAsText(e)
	if !comparesNumber(e) {
		return compare(asText, sql.Text, sql.Text)
	}
	num, params, err := compare(e, sql.Number, sql.Text)
	if err != nil {
		return "", nil, err
	}
	if e.Op != expr.Equals && e.Op != expr.In {
		return fmt.Sprintf("CASE WHEN %s THEN %s WHEN %s IS NOT NULL THEN FALSE END", sql.IsNumber, num, sql.Text), params, nil
	}
	text, textParams, err := compare(asText, sql.Text, sql.Text)
	if err != nil {
		return "", nil, err
	}
	return fmt.Sprintf("CASE WHEN %s THEN %s ELSE %s END", sql.IsNumber, num, text), append(params, textParams...), nil
}

// numbersAsText returns a copy of e with each number on its right side, in a
// literal, a range bound or an IN list, replaced by its text.
func numbersAsText(e *expr.Expression) *expr.Expression {
	c := *e
	switch r := e.Right.(type) {
	case *expr.RangeBoundary:
		rb := *r
		rb.Min, rb.Max = numberAsText(r.Min), numberAsText(r.Max)
		c.Right = &rb
	case *expr.Expression:
		if items, ok := r.Left.([]*expr.Expression); ok && r.Op == expr.List {
			list := *r
			texts := make([]*expr.Expression, len(items))
			for i, item := range items {
				texts[i] = numberAsText(item).(*expr.Expression)
			}
			list.Left = texts
			c.Right = &list
		} else {
			c.Right = numberAsText(r)
		}
	}
	return &c
}

// numberAsText turns a number, or a literal that holds one, into text. A
// literal keeps the text it was written as, so 1.0 stays 1.0 and does not
// become 1.
func numberAsText(in any) any {
	v := in
	lit, isLit := in.(*expr.Expression)
	if isLit {
		if lit.Op != expr.Literal {
			return in
		}
		v = lit.Left
	}
	if !isNumber(v) {
		return in
	}
	text := ""
	if isLit {
		text = lit.NumberText()
	}
	if text == "" {
		s, _, err := formatNumber(v, 'f')
		if err != nil {
			return in
		}
		text = s
	}
	if !isLit {
		return text
	}
	c := *lit
	c.Left = text
	return &c
}

// comparesPlainText reports whether e is an equality or IN test whose terms
// are all plain text.
func comparesPlainText(e *expr.Expression) bool {
	switch e.Op {
	case expr.Equals:
		return isPlainText(e.Right)
	case expr.In:
		items, _, ok := partitionNullsFromList(e.Right)
		if !ok || len(items) == 0 {
			return false
		}
		for _, item := range items {
			if !isPlainText(item) {
				return false
			}
		}
		return true
	default:
		return false
	}
}

// isPlainText reports whether in is a string literal that cannot equal the
// text of a JSON number, true, false or null.
func isPlainText(in any) bool {
	lit, ok := in.(*expr.Expression)
	if !ok || lit.Op != expr.Literal {
		return false
	}
	s, ok := lit.Left.(string)
	if !ok {
		return false
	}
	if _, err := strconv.ParseFloat(s, 64); err == nil {
		return false
	}
	for _, word := range []string{"true", "false", "null"} {
		if strings.EqualFold(s, word) {
			return false
		}
	}
	return true
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
		return (minOpen || isNumber(minVal)) && (maxOpen || isNumber(maxVal))
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
	return isNumber(in)
}

// isNumber reports whether v is a Go integer or float of any size.
func isNumber(v any) bool {
	_, ok, _ := formatNumber(v, 'f')
	return ok
}
