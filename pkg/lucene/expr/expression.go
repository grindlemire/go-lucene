package expr

import (
	"bytes"
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/grindlemire/go-lucene/internal/wildcard"
)

// Lucene Grammar:
// E ->
// 		E:E
// 		(E)
// 		+E
// 		-E
// 		E~E
// 		E^E
// 		NOT E
//      E AND E
// 		E OR E
// 		id
// 		[id TO id]

// Added grammar to be compatible with elastic lucene
// See https://www.elastic.co/guide/en/elasticsearch/reference/8.9/query-dsl-query-string-query.html#query-string-syntax
//      E:>E
//      E:>=E
//      E:<E
//      E:<=E

// Expression is an interface over all the different types of expressions
// that we can parse out of lucene
type Expression struct {
	Left  any      `json:"left"`
	Op    Operator `json:"-"`
	Right any      `json:"right,omitempty"`

	// these are operator specific states we have to track
	boostPower    float64
	fuzzyDistance int
	numberText    string
}

// RangeBoundary represents the boundary conditions for a range operator
type RangeBoundary struct {
	Min       any  `json:"min"`
	Max       any  `json:"max"`
	Inclusive bool `json:"inclusive"`
}

func (e Expression) String() string {
	if e.Op == Undefined {
		return ""
	}
	renderer, found := renderers[e.Op]
	if !found {
		return "ERROR: unable to render string for unsupported operator"
	}
	return renderer(&e, false)
}

// GoString prints a verbose string representation. Useful for debugging exactly
// what types were parsed. You can print this format using %#v
func (e Expression) GoString() string {
	if e.Op == Undefined {
		return ""
	}
	renderer, found := renderers[e.Op]
	if !found {
		return "ERROR: unable to render gostring for unsupported operator"
	}
	return renderer(&e, true)
}

// Lit represents a literal expression
func Lit(in any) *Expression {
	return Expr(in, Literal)
}

// NumberLit returns a literal for the number v, written as text in the query,
// such as 1.0 or 02134. A driver that compares the number as text uses text,
// and String prints it. text is kept only when it differs from how v prints,
// so NumberLit(5, "5") equals Lit(5). It is also dropped when it is not a
// plain decimal number equal to v, so it can never print as another query.
func NumberLit(v any, text string) *Expression {
	e := Lit(v)
	if text != numberString(v) && spellsNumber(text, v) {
		e.numberText = text
	}
	return e
}

// numberSpelling matches a number as the lexer reads it: an optional minus,
// digits with an optional dot or a dot and digits, and an optional exponent.
var numberSpelling = regexp.MustCompile(`^-?([0-9]+\.?[0-9]*|\.[0-9]+)([eE][+-]?[0-9]+)?$`)

// spellsNumber reports whether text is a plain decimal number equal to v.
func spellsNumber(text string, v any) bool {
	if !numberSpelling.MatchString(text) {
		return false
	}
	f, err := strconv.ParseFloat(text, 64)
	if err != nil {
		return false
	}
	switch n := v.(type) {
	case int:
		return f == float64(n)
	case int8:
		return f == float64(n)
	case int16:
		return f == float64(n)
	case int32:
		return f == float64(n)
	case int64:
		return f == float64(n)
	case uint:
		return f == float64(n)
	case uint8:
		return f == float64(n)
	case uint16:
		return f == float64(n)
	case uint32:
		return f == float64(n)
	case uint64:
		return f == float64(n)
	case float32:
		return float32(f) == n
	case float64:
		return f == n
	default:
		return false
	}
}

// numberString prints v the way the SQL drivers print a number as text.
func numberString(v any) string {
	switch n := v.(type) {
	case float64:
		return strconv.FormatFloat(n, 'f', -1, 64)
	case float32:
		return strconv.FormatFloat(float64(n), 'f', -1, 32)
	default:
		return fmt.Sprint(v)
	}
}

// NumberText returns the text that a NumberLit was written as. It is empty
// when the number prints as written.
func (e *Expression) NumberText() string {
	return e.numberText
}

// WILD represents a literal wildcard expression. Its value uses Lucene
// wildcard syntax: * matches any run of characters, ? matches one character,
// and a backslash makes the next character literal, so fo\*bar* matches text
// that starts with fo*bar.
func WILD(in any) *Expression {
	return Expr(in, Wild)
}

// EscapeWildcard returns a wildcard pattern that matches the text s and
// nothing else, for building a pattern in code from text such as user input.
// It puts a backslash before each backslash, * and ?, and before each of
// | ( ) [ ] { } and +, which keep their SIMILAR TO meaning in a pattern built
// in code. Add wildcards around the result, as in
// LIKE("name", EscapeWildcard(prefix)+"*").
func EscapeWildcard(s string) string {
	return wildcard.Escape(s)
}

// REGEXP represents a literal regular expression
func REGEXP(in any) *Expression {
	return Expr(in, Regexp)
}

// NULL represents a SQL null literal (the bare `null` keyword in lucene).
// It is a leaf expression with no value in Left or Right.
func NULL() *Expression {
	e := ptr(empty())
	e.Op = Null
	return e
}

// Eq creates a new EQUALS expression
func Eq(a any, b any) *Expression {
	return Expr(a, Equals, b)
}

func GREATER(a any, b any) *Expression {
	return Expr(a, Greater, b)
}

func LESS(a any, b any) *Expression {
	return Expr(a, Less, b)
}

func GREATEREQ(a any, b any) *Expression {
	return Expr(a, GreaterEq, b)
}

func LESSEQ(a any, b any) *Expression {
	return Expr(a, LessEq, b)
}

// LIKE creates a LIKE expression. b uses Lucene wildcard syntax (* and ?, not
// SQL % and _), with a backslash before a * or ? that should match itself.
func LIKE(a any, b any) *Expression {
	return Expr(a, Like, b)
}

func IN(a any, b any) *Expression {
	return Expr(a, In, b)
}

func LIST(a ...any) *Expression {
	return Expr(a, List)
}

// AND creates an AND expression
func AND(a, b any) *Expression {
	return Expr(a, And, b)
}

// OR creates a new OR expression
func OR(a, b any) *Expression {
	return Expr(a, Or, b)
}

// Rang creates a new range expression
func Rang(term any, min, max any, inclusive bool) *Expression {
	return Expr(term, Range, min, max, inclusive)
}

// NOT wraps an expression in a Not
func NOT(e any) *Expression {
	return Expr(e, Not)
}

// MUST wraps an expression in a Must
func MUST(e any) *Expression {
	return Expr(e, Must)
}

// MUSTNOT wraps an expression in a MustNot
func MUSTNOT(e any) *Expression {
	return Expr(e, MustNot)
}

// BOOST wraps an expression in a boost
func BOOST(e any, power ...float64) *Expression {
	if len(power) > 0 {
		return Expr(e, Boost, power[0])
	}
	return Expr(e, Boost)
}

// FUZZY wraps an expression in a fuzzy
func FUZZY(e any, distance ...int) *Expression {
	if len(distance) > 0 {
		return Expr(e, Fuzzy, distance[0])
	}
	return Expr(e, Fuzzy)
}

// IsExpr checks if the input is an expression
func IsExpr(in any) bool {
	_, isExpr := in.(*Expression)
	return isExpr
}

// IsASTOrJSONContainer reports whether v is a piece of an AST (an Expression,
// a slice of them, or a RangeBoundary), nil, or a map or slice left over from
// JSON decoding. None of these is a value that can stand in for a single
// term. Validate and the SQL drivers share this check so they reject the
// same things.
func IsASTOrJSONContainer(v any) bool {
	switch v.(type) {
	case nil, *Expression, Expression, []*Expression, []Expression,
		*RangeBoundary, RangeBoundary, map[string]any, []any:
		return true
	default:
		return false
	}
}

// Validate validates the expression is correctly structured.
func Validate(in any) (err error) {
	e, isExpr := in.(*Expression)
	if !isExpr {
		// if we don't have an expression we must be in a leaf node
		return nil
	}

	fn, found := validators[e.Op]
	if !found {
		return fmt.Errorf("unsupported operator %v", e.Op)
	}
	err = fn(e)
	if err != nil {
		return err
	}

	err = Validate(e.Left)
	if err != nil {
		return err
	}

	return Validate(e.Right)
}

// Column represents a column in sql. It will not be escaped by quotes in the sql rendering
type Column string

// GoString is a debug print for the column type
func (c Column) GoString() string {
	return fmt.Sprintf("COLUMN(%s)", c)
}

// JSONPath addresses a key inside a JSON column, e.g. labels.category is
// Column "labels" with Path ["category"]. The parser produces it in column
// position for columns registered with lucene.WithJSONFields.
type JSONPath struct {
	Column Column   `json:"column"`
	Path   []string `json:"path"`
}

// jsonPathLeaf is the JSON form of a JSONPath leaf. The "json_path" key tells
// it apart from a nested expression object.
type jsonPathLeaf struct {
	JSONPath *jsonPathFields `json:"json_path"`
}

// jsonPathFields has JSONPath's fields without its MarshalJSON method.
type jsonPathFields JSONPath

// MarshalJSON wraps the path in a "json_path" object so UnmarshalJSON can tell
// it apart from a nested expression.
func (p JSONPath) MarshalJSON() ([]byte, error) {
	return json.Marshal(jsonPathLeaf{JSONPath: (*jsonPathFields)(&p)})
}

// String renders the path in lucene dot syntax, e.g. labels.category.
func (p JSONPath) String() string {
	return string(p.Column) + "." + strings.Join(p.Path, ".")
}

// GoString is a debug print for the json path type
func (p JSONPath) GoString() string {
	return fmt.Sprintf("JSON_PATH(%s)", p)
}

// Validate reports whether the path can be rendered. The column must be
// non-empty, and each path segment non-empty valid UTF-8 without a double
// quote, backslash or control character.
func (p JSONPath) Validate() error {
	if len(p.Column) == 0 {
		return fmt.Errorf("json path column name is empty")
	}
	if len(p.Path) == 0 {
		return fmt.Errorf("json path on column %q is empty", p.Column)
	}
	for _, seg := range p.Path {
		switch {
		case seg == "":
			return fmt.Errorf("json path has an empty segment: %q", p.String())
		case !utf8.ValidString(seg):
			return fmt.Errorf("json path segment is not valid UTF-8: %q", seg)
		case strings.ContainsAny(seg, `"\`):
			return fmt.Errorf("json path segment contains a double quote or backslash: %q", seg)
		case strings.ContainsFunc(seg, unicode.IsControl):
			return fmt.Errorf("json path segment contains a control character: %q", seg)
		}
	}
	return nil
}

// Expr creates a general new expression. The other public functions are just helpers that call this
// function underneath.
func Expr(left any, op Operator, right ...any) *Expression {
	// A bare null literal in column position (e.g. `null:foo`) is a column
	// named "null", not a SQL NULL. Demote it to a literal column.
	if e, ok := left.(*Expression); ok && e.Op == Null && operatesOnColumn(op) {
		left = Lit(Column("null"))
	}

	if isStringlike(left) && operatesOnColumn(op) {
		left = wrapInColumn(left)
	}

	if isLiteral(left) && op != Literal && op != Wild && op != Regexp {
		left = literalToExpr(left)
	}

	e := ptr(empty())
	e.Left = left
	e.Op = op

	// support using a like operator with wildcards or regex
	if op == Equals && len(right) == 1 && shouldUseLikeOperator(right[0]) {
		e.Op = Like
		e.Right = right[0].(*Expression)
		return e
	}

	// support changing boost power
	if op == Boost {
		e.boostPower = 1.0
		if len(right) == 1 && isFloat(right[0]) {
			e.boostPower = right[0].(float64)
		}
		return e
	}

	// support changing fuzzy distance
	if op == Fuzzy {
		e.fuzzyDistance = 1
		if len(right) == 1 && isInt(right[0]) {
			e.fuzzyDistance = right[0].(int)
		}
		return e
	}

	// support passing a range with inclusivity
	if op == Range && len(right) == 3 && isBool(right[2]) {
		e.Right = &RangeBoundary{
			Min:       literalToExpr(right[0]),
			Max:       literalToExpr(right[1]),
			Inclusive: right[2].(bool),
		}
		return e
	}

	// support passing a slice to an IN operator
	if op == In && len(right) > 0 {
		e.Right = right[0].(*Expression)
		return e
	}

	if op == List {
		// super gross but this is how go handles any types that are slices
		slice, isSlice := left.([]any)[0].([]*Expression)
		if isSlice {
			e.Left = slice
			return e
		}

		l := left.([]any)
		vals := []*Expression{}
		for _, v := range l {
			vals = append(vals, v.(*Expression))
		}
		e.Left = vals
		return e
	}

	// if right is present and non nil then add it to the expression
	if len(right) >= 1 && right[0] != nil {
		if isLiteral(right[0]) {
			right[0] = literalToExpr(right[0])
		}

		e.Right = right[0]
	}

	return e
}

type jsonExpression struct {
	Left     json.RawMessage `json:"left"`
	Operator string          `json:"operator"`
	Right    json.RawMessage `json:"right,omitempty"`

	RangeBoundary *RangeBoundary `json:"boundaries,omitempty"`
	FuzzyDistance *int           `json:"distance,omitempty"`
	BoostPower    *float64       `json:"power,omitempty"`
}

// MarshalJSON is a custom JSON serialization for the Expression
func (e Expression) MarshalJSON() (out []byte, err error) {
	// if we are in a leaf node just marshal the value
	if e.Op == Literal && e.numberText != "" {
		return json.Marshal(numberLeaf{Number: numberFields{Value: e.Left, Text: e.numberText}})
	}
	if s, ok := e.Left.(string); ok {
		if key, tagged := termKeys[e.Op]; tagged && literalToExpr(s).Op != e.Op {
			return json.Marshal(map[string]string{key: s})
		}
	}
	if e.Op == Literal || e.Op == Wild || e.Op == Regexp || e.Op == Null {
		return json.Marshal(e.Left)
	}

	leftRaw, err := json.Marshal(e.Left)
	if err != nil {
		return out, err
	}

	c := jsonExpression{
		Left:     leftRaw,
		Operator: toString[e.Op],
	}

	// this is dumb but we need it so our "null" is not event given. Otherwise the json serialization
	// will persist a null value.
	if e.Right != nil {
		rightRaw, err := json.Marshal(e.Right)
		if err != nil {
			return out, err
		}
		c.Right = rightRaw
	}

	if e.boostPower != 1.0 {
		c.BoostPower = &e.boostPower
	}

	if e.fuzzyDistance != 1 {
		c.FuzzyDistance = &e.fuzzyDistance
	}

	return json.Marshal(c)
}

// UnmarshalJSON is a custom JSON deserialization for the Expression
func (e *Expression) UnmarshalJSON(data []byte) (err error) {
	// initalize our default values, e cannot be nil here.
	*e = empty()
	// if this does not look like an object it must be a literal
	if !isJSONObject(json.RawMessage(data)) {
		Expr, err := unmarshalLiteral(json.RawMessage(data))
		// this is required because apparently you can't swap pointers to your receiver mid method
		*e = *Expr
		return err
	}

	var keys map[string]json.RawMessage
	if err := json.Unmarshal(data, &keys); err != nil {
		return err
	}
	if raw, ok := keys["json_path"]; ok {
		p, err := unmarshalJSONPath(raw, len(keys))
		if err != nil {
			return err
		}
		*e = *Lit(p)
		return nil
	}
	if raw, ok := keys["number"]; ok {
		n, err := unmarshalNumber(raw, len(keys))
		if err != nil {
			return err
		}
		*e = *n
		return nil
	}
	if t, ok, err := unmarshalTerm(keys); ok || err != nil {
		if err != nil {
			return err
		}
		*e = *t
		return nil
	}

	// unmarshal the current layer in the json first, then worry about
	// the left and right hand subobjects
	var c jsonExpression
	err = json.Unmarshal(data, &c)
	if err != nil {
		return err
	}

	// check if it is an array so we can parse it into literals
	if isArray(json.RawMessage(c.Left)) {
		var l []json.RawMessage
		err = json.Unmarshal(c.Left, &l)
		if err != nil {
			return err
		}

		exprs := []*Expression{}
		for _, v := range l {
			parsedExp, err := unmarshalLiteral(v)
			if err != nil {
				return err
			}
			exprs = append(exprs, parsedExp)
		}
		e.Left = exprs
	} else {
		e.Left = ptr(empty())
		err = json.Unmarshal(c.Left, e.Left)
		if err != nil {
			return err
		}
	}

	e.Op = fromString[c.Operator]

	// if the left hand side is a string then it must be a column
	if isStringlike(e.Left) && operatesOnColumn(e.Op) {
		e.Left = wrapInColumn(e.Left)
	}

	if len(c.Right) > 0 && looksLikeRangeBoundary(c.Right) {
		var boundary RangeBoundary
		err = json.Unmarshal(c.Right, &boundary)
		if err != nil {
			return err
		}
		if boundary.Min, err = unmarshalBound(boundary.Min); err != nil {
			return err
		}
		if boundary.Max, err = unmarshalBound(boundary.Max); err != nil {
			return err
		}
		e.Right = &boundary
	} else if len(c.Right) > 0 {
		e.Right = ptr(empty())
		err = json.Unmarshal(c.Right, e.Right)
		if err != nil {
			return err
		}
	}

	if e.Op == Fuzzy {
		e.fuzzyDistance = 1
		if c.FuzzyDistance != nil {
			e.fuzzyDistance = *c.FuzzyDistance
		}
	}

	if e.Op == Boost {
		e.boostPower = 1.0
		if c.BoostPower != nil {
			e.boostPower = *c.BoostPower
		}
	}

	return nil
}

func unmarshalJSONPath(raw json.RawMessage, keys int) (JSONPath, error) {
	if keys != 1 {
		return JSONPath{}, fmt.Errorf("json_path must be the only key of its object")
	}
	var p jsonPathFields
	if err := json.Unmarshal(raw, &p); err != nil {
		return JSONPath{}, fmt.Errorf("invalid json_path: %w", err)
	}
	if err := JSONPath(p).Validate(); err != nil {
		return JSONPath{}, fmt.Errorf("invalid json_path: %w", err)
	}
	return JSONPath(p), nil
}

// numberLeaf is the JSON form of a number literal that keeps the text it was
// written as, such as {"number":{"value":1,"text":"1.0"}}.
type numberLeaf struct {
	Number numberFields `json:"number"`
}

type numberFields struct {
	Value any    `json:"value"`
	Text  string `json:"text"`
}

// unmarshalNumber decodes a numberLeaf. Like the parser, it reads the number
// as an int when its text is an int, and as a float64 otherwise.
func unmarshalNumber(raw json.RawMessage, keys int) (*Expression, error) {
	if keys != 1 {
		return nil, fmt.Errorf("number must be the only key of its object")
	}
	var n struct {
		Value json.Number `json:"value"`
		Text  string      `json:"text"`
	}
	if err := json.Unmarshal(raw, &n); err != nil {
		return nil, fmt.Errorf("invalid number: %w", err)
	}
	if n.Text == "" {
		return nil, fmt.Errorf("invalid number: text must not be empty")
	}
	if !numberSpelling.MatchString(n.Text) {
		return nil, fmt.Errorf("invalid number: text %q is not a decimal number", n.Text)
	}
	if _, err := strconv.Atoi(n.Text); err == nil {
		i, err := strconv.Atoi(n.Value.String())
		if err != nil {
			return nil, fmt.Errorf("invalid number: value %q is not an int", n.Value)
		}
		return checkedNumberLit(i, n.Text)
	}
	f, err := n.Value.Float64()
	if err != nil {
		return nil, fmt.Errorf("invalid number: %w", err)
	}
	return checkedNumberLit(f, n.Text)
}

func checkedNumberLit(v any, text string) (*Expression, error) {
	if !spellsNumber(text, v) {
		return nil, fmt.Errorf("invalid number: text %q does not equal value %v", text, v)
	}
	return NumberLit(v, text), nil
}

// termKeys gives the JSON key of each kind of string term that has an object
// form. A term is written as a plain JSON string when that string decodes
// back to the same kind of term, and as {"<key>": "<value>"} otherwise: the
// literal fo* is {"literal":"fo*"}, because the string "fo*" decodes as a
// wildcard.
var termKeys = map[Operator]string{
	Literal: "literal",
}

// unmarshalTerm decodes the object form of a string term. ok is false when
// keys holds none of the term keys.
func unmarshalTerm(keys map[string]json.RawMessage) (e *Expression, ok bool, err error) {
	for op, key := range termKeys {
		raw, found := keys[key]
		if !found {
			continue
		}
		if len(keys) != 1 {
			return nil, true, fmt.Errorf("%s must be the only key of its object", key)
		}
		var s string
		if err := json.Unmarshal(raw, &s); err != nil {
			return nil, true, fmt.Errorf("invalid %s: %w", key, err)
		}
		return Expr(s, op), true, nil
	}
	return nil, false, nil
}

// unmarshalBound turns a range bound decoded into an any back into a literal.
// Any other object stays a map, which Validate rejects.
func unmarshalBound(in any) (*Expression, error) {
	m, isObject := in.(map[string]any)
	if !isObject || !isLeafObject(m) {
		return literalToExpr(toIntIfNecessary(in)), nil
	}
	raw, err := json.Marshal(m)
	if err != nil {
		return nil, err
	}
	return unmarshalLiteral(raw)
}

// isLeafObject reports whether m is the object form of a single term: a
// number that keeps its text, or a string term.
func isLeafObject(m map[string]any) bool {
	if _, ok := m["number"]; ok {
		return true
	}
	for _, key := range termKeys {
		if _, ok := m[key]; ok {
			return true
		}
	}
	return false
}

func unmarshalLiteral(in json.RawMessage) (e *Expression, err error) {
	e = ptr(empty())

	// a number that keeps the text it was written as, or a string term
	if isJSONObject(in) {
		var keys map[string]json.RawMessage
		if err := json.Unmarshal(in, &keys); err != nil {
			return e, err
		}
		if raw, ok := keys["number"]; ok {
			return unmarshalNumber(raw, len(keys))
		}
		if t, ok, err := unmarshalTerm(keys); ok || err != nil {
			return t, err
		}
	}

	// JSON `null` -> NULL expression.
	if bytes.Equal(bytes.TrimSpace(in), []byte("null")) {
		return NULL(), nil
	}

	// check if it is an int first because all ints can be parsed as floats
	i, err := strconv.Atoi(string(in))
	if err == nil {
		return Lit(i), nil
	}

	// check if it is a float
	f, err := strconv.ParseFloat(string(in), 64)
	if err == nil {
		return Lit(f), nil
	}

	// we know it is some sort of string so decode it
	var s string
	err = json.Unmarshal(in, &s)
	if err != nil {
		return e, err
	}

	return literalToExpr(s), nil
}

func isArray(in json.RawMessage) bool {
	trimmed := bytes.TrimSpace(in)
	if len(trimmed) == 0 {
		return false
	}

	return trimmed[0] == '[' && trimmed[len(trimmed)-1] == ']'
}

// looksLikeRangeBoundary checks whether the marshalled json has the keys for a range boundary.
// This is a hack but we need to know whether to unmarshal an expression or a range boundary.
func looksLikeRangeBoundary(in json.RawMessage) bool {
	// strip all the whitespace out of the input
	s := strings.Join(strings.Fields(string(in)), "")

	return strings.Contains(s, "\"min\":") &&
		strings.Contains(s, "\"max\":") &&
		!strings.Contains(s, "\"left\":")
}

func literalToExpr(in any) *Expression {
	if IsExpr(in) {
		return in.(*Expression)
	}

	s, isStr := in.(string)
	if !isStr {
		return Lit(in)
	}

	if s == "" {
		return Lit(s)
	}

	// if it has leading and trailing /'s then it probably is a regex.
	// Note this needs to be checked before the wildcard check as a regex
	// can contain * and ?.
	// TODO this should probably check for escaping
	if len(s) >= 2 && s[0] == '/' && s[len(s)-1] == '/' {
		return REGEXP(s)
	}

	// A string with a * or ? is a wildcard pattern, even when they are all
	// escaped: the backslashes are part of the pattern, so fo\* matches the
	// text fo*.
	if strings.ContainsAny(s, "*?") {
		return WILD(s)
	}

	return Lit(s)
}

func isJSONObject(in json.RawMessage) bool {
	trimmed := bytes.TrimSpace(in)
	if len(trimmed) == 0 {
		return false
	}

	return trimmed[0] == '{' && trimmed[len(trimmed)-1] == '}'
}

// isStringLike checks if the input is a string or is a literal wrapping a string
func isStringlike(in any) bool {
	_, isStr := in.(string)
	e, isExpr := in.(*Expression)
	if isExpr {
		_, isStrLiteralExpr := e.Left.(string)
		return isStrLiteralExpr
	}

	return isStr
}

// operatesOnColumn checks if an operator can be applied to a column (the left side of the operator).
// Example: equal can be applied onto a column (e.g. myColumn = 'foo') but Boost (^) cannot.
func operatesOnColumn(op Operator) bool {
	return op == Equals ||
		op == Range ||
		op == Greater ||
		op == Less ||
		op == GreaterEq ||
		op == LessEq ||
		op == In ||
		op == Like
}

// wrapInColumn converts a string to a column and enforces column
// invariants (e.g. if the column name contains a space then it must be quoted)
func wrapInColumn(in any) (out *Expression) {
	s, isStr := in.(string)
	if isStr {
		return Lit(Column(s))
	}

	e, isExpr := in.(*Expression)
	if isExpr {
		s, isStr = e.Left.(string)
		if isStr {
			return Lit(Column(s))
		}
	}
	return e
}

// apparently the json unmarshal only parses float64 values so we check if the float64
// is actually a whole number. If it is then make it an int
func toIntIfNecessary(in any) (out any) {
	f, isFloat := in.(float64)
	if !isFloat {
		return in
	}

	if f == float64(int(f)) {
		return int(f)
	}

	return f
}

func empty() Expression {
	return Expression{
		fuzzyDistance: 1,
		boostPower:    1.0,
	}
}

func ptr[T any](in T) *T {
	return &in
}

func shouldUseLikeOperator(in any) bool {
	expr, isExpr := in.(*Expression)
	if !isExpr {
		return false
	}
	return expr.Op == Wild || expr.Op == Regexp
}
