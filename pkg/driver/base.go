package driver

import (
	"fmt"
	"math"
	"math/big"
	"regexp"
	"strconv"
	"strings"

	"github.com/grindlemire/go-lucene/internal/wildcard"
	"github.com/grindlemire/go-lucene/pkg/lucene/expr"
	"github.com/grindlemire/go-lucene/pkg/lucene/reduce"
)

// stripRegexpDelimiters removes surrounding /.../ delimiters from a Lucene
// regexp literal, returning the inner pattern.
func stripRegexpDelimiters(s string) string {
	if len(s) >= 2 && s[0] == '/' && s[len(s)-1] == '/' {
		return s[1 : len(s)-1]
	}
	return s
}

// isNullExpr returns true if the value is a *expr.Expression with Op == Null.
func isNullExpr(in any) bool {
	e, ok := in.(*expr.Expression)
	return ok && e.Op == expr.Null
}

// isNullEquals returns true if the value is an Expression of the form
// Equals(left, Null). Used to collapse Not/MustNot wrappers into IS NOT NULL.
func isNullEquals(in any) bool {
	e, ok := in.(*expr.Expression)
	if !ok || e.Op != expr.Equals {
		return false
	}
	return isNullExpr(e.Right)
}

// partitionNullsFromList walks the elements of an IN-list right-side (which is
// always a List Expression whose Left is []*expr.Expression) and returns the
// non-null members and how many nulls were found.
func partitionNullsFromList(right any) (nonNulls []*expr.Expression, nullCount int, ok bool) {
	list, isList := right.(*expr.Expression)
	if !isList || list.Op != expr.List {
		return nil, 0, false
	}
	items, isSlice := list.Left.([]*expr.Expression)
	if !isSlice {
		return nil, 0, false
	}
	for _, item := range items {
		if item != nil && item.Op == expr.Null {
			nullCount++
			continue
		}
		nonNulls = append(nonNulls, item)
	}
	return nonNulls, nullCount, true
}

// Shared is the set of render functions for operators whose SQL is identical
// across dialects (And, Or, Not, Equals, comparisons, In, List, Must, Wild).
// Custom drivers embed these by copying Shared into their RenderFNs map.
//
// Database-specific operators — Like, Range, and Regexp — are intentionally
// not in this map. They are dispatched through the Dialect interface
// (for Like) or handled directly by Base (for Range and Regexp), so custom
// drivers customize them by providing a Dialect implementation rather than
// by overriding a RenderFN entry.
var Shared = map[expr.Operator]RenderFN{
	expr.Literal:   literal,
	expr.And:       basicCompound(expr.And),
	expr.Or:        basicCompound(expr.Or),
	expr.Not:       basicWrap(expr.Not),
	expr.Equals:    equals,
	expr.Must:      noop,                // must doesn't really translate to sql
	expr.MustNot:   basicWrap(expr.Not), // must not is really just a negation
	expr.Wild:      literal,
	expr.Greater:   greater,
	expr.GreaterEq: greaterEq,
	expr.Less:      less,
	expr.LessEq:    lessEq,
	expr.In:        inFn,
	expr.List:      list,
}

// Base is the base driver that is embedded in each driver
type Base struct {
	RenderFNs map[expr.Operator]RenderFN
	// Dialect captures database-specific rendering for Like, Range, standalone
	// wildcard, pattern escaping, and bool literals. If nil, Base falls back to
	// a Postgres-compatible default to preserve backwards compatibility for
	// custom drivers built against the pre-dialect API.
	Dialect Dialect
	// CaseInsensitiveFields lists the columns whose string matches ignore
	// case. On these columns, equality, IN lists and wildcards with a string
	// value wrap both sides in LOWER() and keep the dialect's operator.
	// Numbers, regexes, ranges, comparisons, field:* and null checks are not
	// changed. Names match exactly. The lucene.WithCaseInsensitiveFields
	// option sets this for the lucene.ToX functions.
	CaseInsensitiveFields []string
}

// dialect returns the configured dialect, falling back to defaultDialect if
// the Base was constructed without one (the historical extension API).
func (b Base) dialect() Dialect {
	if b.Dialect == nil {
		return defaultDialect
	}
	return b.Dialect
}

// RenderParam will render the expression into a parameterized query. The returned string will contain placeholders
// and the params will contain the values that should be passed to the query.
func (b Base) RenderParam(e *expr.Expression) (s string, params []any, err error) {
	if e == nil {
		return "", params, nil
	}

	if e.Op == expr.Null {
		return "", nil, fmt.Errorf("null cannot be rendered as a standalone value")
	}

	e, err = fieldShape(e)
	if err != nil {
		return "", nil, err
	}

	// Standalone Regexp expression: strip /.../ delimiters and return as a
	// parameterized value. This mirrors what serializeParams does for nested
	// Regexp sub-expressions.
	if e.Op == expr.Regexp {
		s, _ := e.Left.(string)
		s = stripRegexpDelimiters(s)
		if err := validateStringLiteral(s); err != nil {
			return "", nil, err
		}
		return "?", []any{s}, nil
	}

	// A number that a float64 can't hold goes into the SQL as written. No Go
	// value the drivers bind holds it: a float64 rounds it, and a string is
	// compared as text against a JSON value in MySQL and SQLite.
	if text, ok := lostDigits(e); ok {
		return string(text), nil, nil
	}

	// Not/MustNot wrapping Equals(field, Null) -> IS NOT NULL.
	if (e.Op == expr.Not || e.Op == expr.MustNot) && isNullEquals(e.Left) {
		inner, _ := e.Left.(*expr.Expression)
		col, cparams, err := b.serializeParams(inner.Left)
		if err != nil {
			return "", cparams, err
		}
		return fmt.Sprintf("%s IS NOT NULL", col), cparams, nil
	}

	if p, ok := jsonPathLeft(e); ok {
		return b.renderJSONPathCompare(e, p, b.renderCompareParam)
	}

	left, lparams, err := b.serializeParams(e.Left)
	if err != nil {
		return s, params, err
	}
	str, rparams, err := b.renderCompareParam(e, left, left)
	return str, append(lparams, rparams...), err
}

// renderCompareParam renders e once its left side is rendered. nullLeft is the
// left side for an IS NULL test.
func (b Base) renderCompareParam(e *expr.Expression, left, nullLeft string) (s string, params []any, err error) {
	d := b.dialect()

	// Range: access typed boundary directly, skip serializing right side
	if e.Op == expr.Range {
		boundary, ok := e.Right.(*expr.RangeBoundary)
		if !ok {
			return "", nil, fmt.Errorf("range operator requires *expr.RangeBoundary, got %T", e.Right)
		}
		str, rangeParams, err := b.renderRangeParam(left, boundary)
		if err != nil {
			return "", nil, err
		}
		return str, rangeParams, nil
	}

	// Null right-hand side handling. Intercept before serializing the right
	// side so dialects don't have to handle null themselves.
	if isNullExpr(e.Right) {
		switch e.Op {
		case expr.Equals:
			return fmt.Sprintf("%s IS NULL", nullLeft), nil, nil
		case expr.Greater, expr.Less, expr.GreaterEq, expr.LessEq:
			return "", nil, fmt.Errorf(
				"comparison operator %s cannot be used with null; use field:null for IS NULL",
				e.Op,
			)
		}
	}

	if e.Op == expr.In && b.foldsCase(e.Left) {
		str, inParams, ok, err := b.renderFoldedIn(left, e.Right, b.serializeParams)
		if err != nil {
			return "", nil, err
		}
		if ok {
			return str, inParams, nil
		}
	}

	// IN with null members -> partition into (IN (...) OR IS NULL).
	if e.Op == expr.In {
		nonNulls, nullCount, ok := partitionNullsFromList(e.Right)
		if ok && nullCount > 0 {
			switch len(nonNulls) {
			case 0:
				return fmt.Sprintf("%s IS NULL", nullLeft), nil, nil
			case 1:
				rhs, rparams, err := b.serializeParams(nonNulls[0])
				if err != nil {
					return "", nil, err
				}
				return fmt.Sprintf("(%s = %s OR %s IS NULL)", left, rhs, nullLeft),
					rparams, nil
			default:
				// Hand-roll the List rather than calling expr.LIST: that constructor
				// takes ...any and would require boxing the []*expr.Expression slice.
				inList := &expr.Expression{Op: expr.List, Left: nonNulls}
				inStr, inParams, err := b.serializeParams(inList)
				if err != nil {
					return "", nil, err
				}
				return fmt.Sprintf("(%s IN %s OR %s IS NULL)", left, inStr, nullLeft),
					inParams, nil
			}
		}
	}

	var pattern string
	var hasPattern bool
	if e.Op == expr.Like {
		pattern, hasPattern, err = likeOperand(e.Right)
		if err != nil {
			return "", nil, err
		}
	}

	right, rparams, err := b.serializeParams(e.Right)
	if err != nil {
		return s, params, err
	}

	// Standalone wildcard on a Like operator: `field:*`. Route through the
	// dialect so each database can decide how to represent "any value".
	if right == "'*'" && e.Op == expr.Like {
		str, err := d.RenderStandaloneWild(left)
		return str, nil, err
	}

	// Detect regex (Lucene /regex/) vs. wildcard and let the dialect escape
	// the wildcard pattern however it needs to. The dialect may additionally
	// signal via PrepareLikePattern that a wildcard pattern should be rendered
	// through the regex path (MySQL does this for patterns containing
	// alternation, grouping, or character classes).
	isRegex := false
	if e.Op == expr.Like {
		if rightExpr, ok := e.Right.(*expr.Expression); ok && rightExpr.Op == expr.Regexp {
			isRegex = true
		}
		if hasPattern && len(rparams) == 1 {
			transformed, useRegex := d.PrepareLikePattern(pattern)
			rparams[0] = transformed
			if useRegex {
				isRegex = true
			}
		}
	}

	params = rparams

	if e.Op != expr.Not &&
		e.Op != expr.List &&
		e.Op != expr.In &&
		e.Op != expr.Literal &&
		e.Op != expr.Must &&
		e.Op != expr.MustNot {
		if !b.isSimple(e.Left) {
			left = "(" + left + ")"
		}
		if !b.isSimple(e.Right) {
			right = "(" + right + ")"
		}
	}

	if b.foldsCaseCompare(e, hasPattern) {
		left, right = lower(left), lower(right)
	}

	if e.Op == expr.Like {
		str, err := d.RenderLike(left, right, isRegex)
		return str, params, err
	}

	fn, ok := b.RenderFNs[e.Op]
	if !ok {
		return s, params, fmt.Errorf("unable to render operator [%s]", e.Op)
	}

	str, err := fn(left, right)
	if err != nil {
		return "", nil, err
	}
	return str, params, nil
}

// Render will render the expression based on the renderFNs provided by the driver.
func (b Base) Render(e *expr.Expression) (s string, err error) {
	if e == nil {
		return "", nil
	}

	if e.Op == expr.Null {
		return "", fmt.Errorf("null cannot be rendered as a standalone value")
	}

	e, err = fieldShape(e)
	if err != nil {
		return "", err
	}

	// Standalone Regexp expression: strip /.../ delimiters and return as a
	// single-quoted literal. This mirrors what serialize does for nested
	// Regexp sub-expressions.
	if e.Op == expr.Regexp {
		s, _ := e.Left.(string)
		s = stripRegexpDelimiters(s)
		if err := validateStringLiteral(s); err != nil {
			return "", err
		}
		return b.dialect().EscapeStringLiteral(s), nil
	}

	if text, ok := lostDigits(e); ok {
		return string(text), nil
	}

	// Not/MustNot wrapping Equals(field, Null) -> IS NOT NULL.
	if (e.Op == expr.Not || e.Op == expr.MustNot) && isNullEquals(e.Left) {
		inner, _ := e.Left.(*expr.Expression)
		col, err := b.serialize(inner.Left)
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("%s IS NOT NULL", col), nil
	}

	if p, ok := jsonPathLeft(e); ok {
		str, _, err := b.renderJSONPathCompare(e, p, b.renderCompareInline)
		return str, err
	}

	left, err := b.serialize(e.Left)
	if err != nil {
		return s, err
	}
	return b.renderCompare(e, left, left)
}

// renderCompareInline adapts renderCompare to the signature that
// renderJSONPathCompare shares with renderCompareParam.
func (b Base) renderCompareInline(e *expr.Expression, left, nullLeft string) (string, []any, error) {
	s, err := b.renderCompare(e, left, nullLeft)
	return s, nil, err
}

// renderCompare renders e once its left side is rendered. nullLeft is the left
// side for an IS NULL test.
func (b Base) renderCompare(e *expr.Expression, left, nullLeft string) (s string, err error) {
	d := b.dialect()

	// Range: access typed boundary directly, skip serializing right side
	if e.Op == expr.Range {
		boundary, ok := e.Right.(*expr.RangeBoundary)
		if !ok {
			return "", fmt.Errorf("range operator requires *expr.RangeBoundary, got %T", e.Right)
		}
		str, err := b.renderRange(left, boundary)
		if err != nil {
			return "", err
		}
		return str, nil
	}

	// Null right-hand side handling. Intercept before serializing the right
	// side so dialects don't have to handle null themselves.
	if isNullExpr(e.Right) {
		switch e.Op {
		case expr.Equals:
			return fmt.Sprintf("%s IS NULL", nullLeft), nil
		case expr.Greater, expr.Less, expr.GreaterEq, expr.LessEq:
			return "", fmt.Errorf(
				"comparison operator %s cannot be used with null; use field:null for IS NULL",
				e.Op,
			)
		}
	}

	if e.Op == expr.In && b.foldsCase(e.Left) {
		ser := func(v any) (string, []any, error) {
			s, err := b.serialize(v)
			return s, nil, err
		}
		str, _, ok, err := b.renderFoldedIn(left, e.Right, ser)
		if err != nil {
			return "", err
		}
		if ok {
			return str, nil
		}
	}

	// IN with null members -> partition into (IN (...) OR IS NULL).
	if e.Op == expr.In {
		nonNulls, nullCount, ok := partitionNullsFromList(e.Right)
		if ok && nullCount > 0 {
			switch len(nonNulls) {
			case 0:
				return fmt.Sprintf("%s IS NULL", nullLeft), nil
			case 1:
				rhs, err := b.serialize(nonNulls[0])
				if err != nil {
					return "", err
				}
				return fmt.Sprintf("(%s = %s OR %s IS NULL)", left, rhs, nullLeft), nil
			default:
				// Hand-roll the List rather than calling expr.LIST: that constructor
				// takes ...any and would require boxing the []*expr.Expression slice.
				inList := &expr.Expression{Op: expr.List, Left: nonNulls}
				inStr, err := b.serialize(inList)
				if err != nil {
					return "", err
				}
				return fmt.Sprintf("(%s IN %s OR %s IS NULL)", left, inStr, nullLeft), nil
			}
		}
	}

	var pattern string
	var hasPattern bool
	if e.Op == expr.Like {
		pattern, hasPattern, err = likeOperand(e.Right)
		if err != nil {
			return "", err
		}
	}

	right, err := b.serialize(e.Right)
	if err != nil {
		return s, err
	}

	// Standalone wildcard on a Like operator: `field:*`. Route through the
	// dialect so each database can decide how to represent "any value".
	if right == "'*'" && e.Op == expr.Like {
		return d.RenderStandaloneWild(left)
	}

	// Detect regex (Lucene /regex/) vs. wildcard and let the dialect transform
	// the pattern and optionally flip to the regex path. Positioned before
	// paren-wrap to stay symmetric with RenderParam. The dialect transforms
	// the raw pattern, the same text RenderParam binds, and the result is
	// escaped afterwards: escaping first would leave any backslash the
	// transform adds (MySQL's \. in the regex fallback) unescaped.
	isRegex := false
	if e.Op == expr.Like {
		if rightExpr, ok := e.Right.(*expr.Expression); ok && rightExpr.Op == expr.Regexp {
			isRegex = true
		}
		if hasPattern {
			transformed, useRegex := d.PrepareLikePattern(pattern)
			right = d.EscapeStringLiteral(transformed)
			if useRegex {
				isRegex = true
			}
		}
	}

	if e.Op != expr.Not &&
		e.Op != expr.List &&
		e.Op != expr.In &&
		e.Op != expr.Literal &&
		e.Op != expr.Must &&
		e.Op != expr.MustNot {
		if !b.isSimple(e.Left) {
			left = "(" + left + ")"
		}
		if !b.isSimple(e.Right) {
			right = "(" + right + ")"
		}
	}

	if b.foldsCaseCompare(e, hasPattern) {
		left, right = lower(left), lower(right)
	}

	if e.Op == expr.Like {
		return d.RenderLike(left, right, isRegex)
	}

	fn, ok := b.RenderFNs[e.Op]
	if !ok {
		return s, fmt.Errorf("unable to render operator [%s]", e.Op)
	}

	str, err := fn(left, right)
	if err != nil {
		return "", err
	}
	return str, nil
}

// fieldShape checks that a field operator has a field and a value it can
// compare, and spreads a field over a group of terms. Render does not run
// expr.Validate, so a tree built in code or read from JSON can hold a group
// where the parser only puts a single term.
//
// An EQUALS whose value is not a single term goes through reduce.FieldGroup,
// as the parser's a:(...) does, so it renders as the parsed query does. A
// group gets the field on every term in it, as in a:(x AND y), and a value
// with a field of its own keeps it, so expr.Eq("a", expr.Eq("b", 1)) is
// b = 1, as a:(b:1) is. A comparison, an IN list item and a field compare
// one value, so anything but a single term there is an error, as
// a:>(x OR y) is for the parser and expr.Validate. The right side of an IN
// must be a LIST. A LIKE pattern is checked by likeOperand.
func fieldShape(e *expr.Expression) (*expr.Expression, error) {
	switch e.Op {
	case expr.Equals, expr.Like, expr.In, expr.Range, expr.Greater, expr.Less, expr.GreaterEq, expr.LessEq:
	default:
		return e, nil
	}

	if e.Left == nil {
		if e.Op == expr.Range {
			return nil, fmt.Errorf("a range needs a field, as in field:[a TO b]")
		}
		return nil, fmt.Errorf("%s %s needs a field, as in %s", article(e.Op), e.Op, fieldExamples[e.Op])
	}
	if field, ok := e.Left.(*expr.Expression); ok && !isSingleTerm(field) {
		return nil, fmt.Errorf("the field of %s %s must be a single term, not %s %s expression", article(e.Op), e.Op, article(field.Op), field.Op)
	}

	value, ok := e.Right.(*expr.Expression)
	if !ok {
		return e, nil
	}
	switch e.Op {
	case expr.Equals:
		if isSingleTerm(value) {
			return e, nil
		}
		if value.Op == expr.List {
			return nil, fmt.Errorf("the value in an EQUALS must be a single term, not a LIST expression")
		}
		field, ok := e.Left.(*expr.Expression)
		if !ok {
			field = &expr.Expression{Op: expr.Literal, Left: e.Left}
		}
		return reduce.FieldGroup(field, value), nil
	case expr.In:
		if value.Op != expr.List {
			return nil, fmt.Errorf("the right side of an IN must be a LIST, not %s %s expression", article(value.Op), value.Op)
		}
		items, _ := value.Left.([]*expr.Expression)
		for _, item := range items {
			if item != nil && !isSingleTerm(item) {
				return nil, fmt.Errorf("an IN list item must be a single term, not %s %s expression", article(item.Op), item.Op)
			}
		}
	case expr.Greater, expr.Less, expr.GreaterEq, expr.LessEq:
		if !isSingleTerm(value) {
			return nil, fmt.Errorf("the value in %s %s comparison must be a single term, not %s %s expression", article(e.Op), e.Op, article(value.Op), value.Op)
		}
	}
	return e, nil
}

// fieldExamples shows each field operator with its field, for the error when
// the field is missing.
var fieldExamples = map[expr.Operator]string{
	expr.Equals:    "field:value",
	expr.Like:      "field:value*",
	expr.In:        "field:(a OR b)",
	expr.Greater:   "field:>value",
	expr.Less:      "field:<value",
	expr.GreaterEq: "field:>=value",
	expr.LessEq:    "field:<=value",
}

// isSingleTerm reports whether e is a word, phrase, number, wildcard, regex or
// null. An empty expression also counts, so it renders as it did before.
func isSingleTerm(e *expr.Expression) bool {
	switch e.Op {
	case expr.Undefined, expr.Literal, expr.Wild, expr.Regexp, expr.Null:
		return true
	default:
		return false
	}
}

func (b Base) isSimple(in any) bool {
	switch v := in.(type) {
	case *expr.Expression:
		return v.Op == expr.Undefined || v.Op == expr.Literal || v.Op == expr.Regexp || v.Op == expr.Wild
	case expr.Column, expr.JSONPath:
		return true
	case nil:
		return true
	case string, int, float64:
		return true
	default:
		return false
	}
}

func (b Base) serialize(in any) (s string, err error) {
	if in == nil {
		return "", nil
	}

	switch v := in.(type) {
	case *expr.Expression:
		if v.Op == expr.Regexp {
			s, _ := v.Left.(string)
			s = stripRegexpDelimiters(s)
			if err := validateStringLiteral(s); err != nil {
				return "", err
			}
			return b.dialect().EscapeStringLiteral(s), nil
		}
		return b.Render(v)
	case []*expr.Expression:
		strs := []string{}
		for _, e := range v {
			s, err = b.Render(e)
			if err != nil {
				return s, err
			}
			strs = append(strs, s)
		}
		return strings.Join(strs, ", "), nil
	case expr.Column:
		return b.quoteColumn(v)
	case expr.JSONPath:
		sql, err := b.renderJSONPath(v)
		return sql.Text, err
	case string:
		if err := validateStringLiteral(v); err != nil {
			return "", err
		}
		return b.dialect().EscapeStringLiteral(v), nil
	case bool:
		return b.dialect().SerializeBool(v), nil
	default:
		if s, ok, err := formatNumber(v, 'g'); ok {
			return s, err
		}
		return "", fmt.Errorf("unsupported value of type %T", v)
	}
}

// quoteColumn checks a column name and quotes it for the dialect. Column names
// are always written into the SQL, so they get the same NUL and UTF-8 checks
// as string literals.
func (b Base) quoteColumn(c expr.Column) (string, error) {
	if len(c) == 0 {
		return "", fmt.Errorf("column name is empty")
	}
	if err := validateStringLiteral(string(c)); err != nil {
		return "", err
	}
	return b.dialect().QuoteColumn(string(c))
}

// formatNumber renders a Go integer or float as a SQL numeric literal, using
// floatFmt as the strconv format for floats. ok is false when v is not a
// number. NaN and the infinities are rejected: they would print as NaN or
// +Inf, which SQL reads as column references.
func formatNumber(v any, floatFmt byte) (s string, ok bool, err error) {
	switch n := v.(type) {
	case int:
		return strconv.Itoa(n), true, nil
	case int8:
		return strconv.FormatInt(int64(n), 10), true, nil
	case int16:
		return strconv.FormatInt(int64(n), 10), true, nil
	case int32:
		return strconv.FormatInt(int64(n), 10), true, nil
	case int64:
		return strconv.FormatInt(n, 10), true, nil
	case uint:
		return strconv.FormatUint(uint64(n), 10), true, nil
	case uint8:
		return strconv.FormatUint(uint64(n), 10), true, nil
	case uint16:
		return strconv.FormatUint(uint64(n), 10), true, nil
	case uint32:
		return strconv.FormatUint(uint64(n), 10), true, nil
	case uint64:
		return strconv.FormatUint(n, 10), true, nil
	case float32:
		return formatFloat(float64(n), floatFmt, 32)
	case float64:
		return formatFloat(n, floatFmt, 64)
	case numberText:
		return string(n), true, nil
	default:
		return "", false, nil
	}
}

// numberText is the text a number was written as, for a number that its Go
// value does not hold. lostDigits only returns one that is a plain decimal
// number, so it is safe to write into the SQL.
type numberText string

// plainDecimal matches a plain decimal number, with an optional minus, digits
// with an optional dot or a dot and digits, and an optional exponent, such as
// -1.5 or 9223372036854775808. That is SQL's numeric literal syntax, so text
// that matches it reads as one number in the SQL.
var plainDecimal = regexp.MustCompile(`^-?([0-9]+\.?[0-9]*|\.[0-9]+)([eE][+-]?[0-9]+)?$`)

// lostDigits returns the text that e, a number literal, was written as when
// its float value is a different number. A float64 holds
// 9223372036854775808 as 9223372036854776000, for example, and
// 0.1000000000000000055511151231257827 as 0.1. Such a number has to reach
// the database as its text for the database to compare it as written.
//
// The text goes into the SQL unquoted, so lostDigits checks it here rather
// than relying on NumberLit: it must be a plain decimal number, and it must
// still parse to the value, which a caller can change after NumberLit
// checked it.
func lostDigits(e *expr.Expression) (numberText, bool) {
	text := e.NumberText()
	if e.Op != expr.Literal || text == "" || !plainDecimal.MatchString(text) {
		return "", false
	}
	var held string
	switch f := e.Left.(type) {
	case float64:
		parsed, err := strconv.ParseFloat(text, 64)
		if err != nil || parsed != f || f == 0 {
			return "", false
		}
		held = strconv.FormatFloat(f, 'g', -1, 64)
	case float32:
		parsed, err := strconv.ParseFloat(text, 32)
		if err != nil || float32(parsed) != f || f == 0 {
			return "", false
		}
		held = strconv.FormatFloat(float64(f), 'g', -1, 32)
	default:
		// An integer renders as its own digits, so its text never reaches
		// the SQL.
		return "", false
	}
	// The text parsed to a float that is neither 0 nor infinite, so its
	// magnitude is within a float's range, and parsing it exactly costs time
	// in proportion to its length.
	written, ok := new(big.Rat).SetString(text)
	if !ok {
		return "", false
	}
	value, ok := new(big.Rat).SetString(held)
	if !ok || written.Cmp(value) == 0 {
		return "", false
	}
	return numberText(text), true
}

func formatFloat(f float64, floatFmt byte, bitSize int) (string, bool, error) {
	if math.IsNaN(f) || math.IsInf(f, 0) {
		return "", true, fmt.Errorf("%v has no SQL literal form; pass it as a parameter instead", f)
	}
	return strconv.FormatFloat(f, floatFmt, -1, bitSize), true, nil
}

// likeOperand checks the right side of a LIKE and returns its raw pattern.
// The right side must be one term: a wildcard or literal holding a string
// (hasPattern is true), a regexp holding a string, or a literal column,
// which is compared as-is. Anything else, such as a number or a compound
// expression, has no pattern text to translate and is an error.
func likeOperand(right any) (pattern string, hasPattern bool, err error) {
	switch v := right.(type) {
	case string:
		return v, true, nil
	case expr.Column:
		return "", false, nil
	case *expr.Expression:
		switch v.Op {
		case expr.Wild, expr.Literal, expr.Regexp:
		default:
			return "", false, fmt.Errorf("LIKE pattern must be a single term, not %s %s expression", article(v.Op), v.Op)
		}
		switch left := v.Left.(type) {
		case string:
			return left, v.Op != expr.Regexp, nil
		case expr.Column:
			if v.Op == expr.Literal {
				return "", false, nil
			}
		}
		return "", false, fmt.Errorf("LIKE pattern must be a string, not %T", v.Left)
	default:
		return "", false, fmt.Errorf("LIKE pattern must be a single term, not %T", right)
	}
}

func (b Base) serializeParams(in any) (s string, params []any, err error) {
	if in == nil {
		return "", params, nil
	}

	switch v := in.(type) {
	case *expr.Expression:
		if v.Op == expr.Regexp {
			s, _ := v.Left.(string)
			s = stripRegexpDelimiters(s)
			if err := validateStringLiteral(s); err != nil {
				return "", nil, err
			}
			return "?", []any{s}, nil
		}
		return b.RenderParam(v)
	case []*expr.Expression:
		strs := []string{}
		for _, e := range v {
			s, eparams, err := b.RenderParam(e)
			if err != nil {
				return s, params, err
			}
			strs = append(strs, s)
			params = append(params, eparams...)
		}
		return strings.Join(strs, ", "), params, nil
	case expr.Column:
		quoted, err := b.quoteColumn(v)
		if err != nil {
			return "", params, err
		}
		return quoted, params, nil
	case expr.JSONPath:
		sql, err := b.renderJSONPath(v)
		return sql.Text, params, err
	case bool:
		return "?", []any{b.dialect().BoolParam(v)}, nil
	case string:
		// if we have a '*' then we don't want to insert a param since
		// it can be used either in a regexp or a range operator.
		if v == "*" {
			return "'*'", params, nil
		}

		// escape single quotes with double single quotes
		return "?", []any{v}, nil
	default:
		// Binding an AST piece or a JSON map/slice would send the database
		// a Go struct, map, or slice instead of a term.
		if expr.IsASTOrJSONContainer(v) {
			return "", nil, fmt.Errorf("unsupported value of type %T", v)
		}
		return "?", []any{v}, nil
	}
}

// extractBoundValue unwraps a range boundary value from its Expression wrapper.
// Returns the bound's Go value, which can be any non-AST, non-container value, and whether the bound is unbounded (*).
// A number that its Go value does not hold is returned as its numberText.
// A bound must be a single term. Render does not run Validate, so a hand-built
// or JSON-decoded AST can still carry a compound bound here.
func extractBoundValue(bound any) (val any, unbounded bool, err error) {
	val = bound
	if e, ok := bound.(*expr.Expression); ok && e != nil {
		switch e.Op {
		case expr.Null:
			return nil, false, fmt.Errorf("null is not allowed as a range bound; use field:null for IS NULL")
		case expr.Wild:
			// Only a bare * leaves this side open. Any other wildcard bound,
			// such as ab*, is literal text, as in Lucene, so its escapes are
			// dropped: f\o* is fo*.
			s, ok := e.Left.(string)
			if ok && s == "*" {
				return nil, true, nil
			}
			val = e.Left
			if ok {
				val = wildcard.Unescape(s)
			}
		case expr.Literal, expr.Regexp:
			val = e.Left
			if text, ok := lostDigits(e); ok {
				val = text
			}
		default:
			return nil, false, fmt.Errorf("range bound must be a single term, not %s %s expression", article(e.Op), e.Op)
		}
	}
	if expr.IsASTOrJSONContainer(val) {
		return nil, false, fmt.Errorf("range bound must be a single term, not %T", val)
	}
	return val, false, nil
}

// article returns "an" for an operator name that starts with a vowel and
// "a" otherwise.
func article(op expr.Operator) string {
	if name := op.String(); name != "" && strings.ContainsRune("AEIOU", rune(name[0])) {
		return "an"
	}
	return "a"
}

// formatRangeValue renders a range bound value as a SQL literal.
func (b Base) formatRangeValue(val any) (string, error) {
	switch v := val.(type) {
	case string:
		if err := validateStringLiteral(v); err != nil {
			return "", err
		}
		return b.dialect().EscapeStringLiteral(v), nil
	case bool:
		return b.dialect().SerializeBool(v), nil
	case expr.Column:
		return b.quoteColumn(v)
	}
	if s, ok, err := formatNumber(val, 'f'); ok {
		return s, err
	}
	return "", fmt.Errorf("unsupported range bound value of type %T", val)
}

// isNumericBound checks whether a range bound value is numeric.
func isNumericBound(val any) bool {
	switch val.(type) {
	case int, float64, numberText:
		return true
	default:
		return false
	}
}

func (b Base) renderRange(left string, boundary *expr.RangeBoundary) (string, error) {
	minVal, minUnbounded, err := extractBoundValue(boundary.Min)
	if err != nil {
		return "", err
	}
	maxVal, maxUnbounded, err := extractBoundValue(boundary.Max)
	if err != nil {
		return "", err
	}
	inclusive := boundary.Inclusive

	if minUnbounded && maxUnbounded {
		return "1=1", nil
	}

	if minUnbounded {
		maxStr, err := b.formatRangeValue(maxVal)
		if err != nil {
			return "", err
		}
		if inclusive {
			return fmt.Sprintf("%s <= %s", left, maxStr), nil
		}
		return fmt.Sprintf("%s < %s", left, maxStr), nil
	}

	if maxUnbounded {
		minStr, err := b.formatRangeValue(minVal)
		if err != nil {
			return "", err
		}
		if inclusive {
			return fmt.Sprintf("%s >= %s", left, minStr), nil
		}
		return fmt.Sprintf("%s > %s", left, minStr), nil
	}

	minStr, err := b.formatRangeValue(minVal)
	if err != nil {
		return "", err
	}
	maxStr, err := b.formatRangeValue(maxVal)
	if err != nil {
		return "", err
	}

	if isNumericBound(minVal) || isNumericBound(maxVal) {
		if inclusive {
			return fmt.Sprintf("%s >= %s AND %s <= %s", left, minStr, left, maxStr), nil
		}
		return fmt.Sprintf("%s > %s AND %s < %s", left, minStr, left, maxStr), nil
	}

	if inclusive {
		return fmt.Sprintf("%s BETWEEN %s AND %s", left, minStr, maxStr), nil
	}
	return fmt.Sprintf("%s > %s AND %s < %s", left, minStr, left, maxStr), nil
}

func (b Base) renderRangeParam(left string, boundary *expr.RangeBoundary) (string, []any, error) {
	minVal, minUnbounded, err := extractBoundValue(boundary.Min)
	if err != nil {
		return "", nil, err
	}
	maxVal, maxUnbounded, err := extractBoundValue(boundary.Max)
	if err != nil {
		return "", nil, err
	}
	inclusive := boundary.Inclusive

	if minUnbounded && maxUnbounded {
		return "1=1", nil, nil
	}

	var minSQL, maxSQL string
	var params []any
	if !minUnbounded {
		if minSQL, params, err = b.rangeParam(minVal, params); err != nil {
			return "", nil, err
		}
	}
	if !maxUnbounded {
		if maxSQL, params, err = b.rangeParam(maxVal, params); err != nil {
			return "", nil, err
		}
	}

	if minUnbounded {
		if inclusive {
			return fmt.Sprintf("%s <= %s", left, maxSQL), params, nil
		}
		return fmt.Sprintf("%s < %s", left, maxSQL), params, nil
	}

	if maxUnbounded {
		if inclusive {
			return fmt.Sprintf("%s >= %s", left, minSQL), params, nil
		}
		return fmt.Sprintf("%s > %s", left, minSQL), params, nil
	}

	if isNumericBound(minVal) || isNumericBound(maxVal) {
		if inclusive {
			return fmt.Sprintf("%s >= %s AND %s <= %s", left, minSQL, left, maxSQL), params, nil
		}
		return fmt.Sprintf("%s > %s AND %s < %s", left, minSQL, left, maxSQL), params, nil
	}

	if inclusive {
		return fmt.Sprintf("%s BETWEEN %s AND %s", left, minSQL, maxSQL), params, nil
	}
	return fmt.Sprintf("%s > %s AND %s < %s", left, minSQL, left, maxSQL), params, nil
}

// rangeParam renders one range bound for the parameterized path. A column is
// written into the SQL as a quoted identifier, as Render does, so both modes
// compare against the column. So is a number that its Go value does not hold,
// as RenderParam does for one outside a range. Anything else becomes a
// placeholder and is appended to params.
func (b Base) rangeParam(val any, params []any) (string, []any, error) {
	switch v := val.(type) {
	case expr.Column:
		quoted, err := b.quoteColumn(v)
		return quoted, params, err
	case numberText:
		return string(v), params, nil
	}
	return "?", append(params, val), nil
}
