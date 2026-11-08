package lucene

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/grindlemire/go-lucene/pkg/driver"
	"github.com/grindlemire/go-lucene/pkg/lucene/expr"
)

type stringCase struct {
	input string
	want  string
}

// assertStringRoundTrip checks that String() renders the parsed input as
// want, and that want parses back to the same expression.
func assertStringRoundTrip(t *testing.T, tc stringCase, opts ...Opt) {
	t.Helper()
	first, err := Parse(tc.input, opts...)
	if err != nil {
		t.Fatalf("expected no error parsing %q, got: %v", tc.input, err)
	}

	rendered := first.String()
	if rendered != tc.want {
		t.Errorf("String():\n got: %s\nwant: %s", rendered, tc.want)
	}

	second, err := Parse(rendered, opts...)
	if err != nil {
		t.Fatalf("expected no error re-parsing rendered form %q, got: %v", rendered, err)
	}

	if !reflect.DeepEqual(first, second) {
		t.Fatalf(errTemplate, "round trip through String() changed the expression", first, second)
	}
}

func runStringCases(t *testing.T, tcs map[string]stringCase, opts ...Opt) {
	t.Helper()
	for name, tc := range tcs {
		t.Run(name, func(t *testing.T) {
			assertStringRoundTrip(t, tc, opts...)
		})
	}
}

// TestStringOperators checks that comparisons, wildcards, regexes and IN
// lists print as Lucene syntax rather than as SQL-like text (issue #70).
func TestStringOperators(t *testing.T) {
	runStringCases(t, map[string]stringCase{
		"greater":          {`a:>5`, `a:>5`},
		"less":             {`a:<5`, `a:<5`},
		"greater_eq":       {`a:>=5`, `a:>=5`},
		"less_eq":          {`a:<=5`, `a:<=5`},
		"greater_negative": {`a:>-5`, `a:>-5`},
		"less_float":       {`a:<1.5`, `a:<1.5`},
		"greater_word":     {`a:>foo`, `a:>foo`},
		"greater_phrase":   {`a:>"foo bar"`, `a:>"foo bar"`},
		"less_eq_null":     {`a:<=null`, `a:<=null`},
		"numeric_field":    {`0:>1`, `0:>1`},
		"must_not_greater": {`-a:>5`, `-a:>5`},
		"compare_and":      {`a:<22 AND b:>=33`, `a:<22 AND b:>=33`},

		"wildcard":          {`a:fo*`, `a:fo*`},
		"wildcard_single":   {`a:f?o`, `a:f?o`},
		"wildcard_star":     {`a:*`, `a:*`},
		"wildcard_escaped":  {`a:\*x*`, `a:\*x*`},
		"wildcard_negative": {`a:-1*`, `a:-1*`},
		"wildcard_boost":    {`a:fo*^2`, `a:fo*^2.0`},
		"wildcard_fuzzy":    {`a:fo*~2`, `a:fo*~2`},
		"must_not_wildcard": {`-a:fo*`, `-a:fo*`},

		"regex":               {`a:/re.*/`, `a:/re.*/`},
		"regex_escaped_slash": {`a:/a\/b/`, `a:/a\/b/`},
		"not_regex":           {`NOT a:/re.*/`, `NOT(a:/re.*/)`},

		"in_strings":     {`a:(b OR c)`, `a:(b OR c)`},
		"in_three":       {`a:(b OR c OR d)`, `a:(b OR c OR d)`},
		"in_flattened":   {`a:((b OR c) OR d)`, `a:(b OR c OR d)`},
		"in_ints":        {`a:(1 OR 2)`, `a:(1 OR 2)`},
		"in_negative":    {`a:(-1 OR 2)`, `a:(-1 OR 2)`},
		"in_floats":      {`a:(1.5 OR 2.5)`, `a:(1.5 OR 2.5)`},
		"in_phrases":     {`a:("foo bar" OR "x y")`, `a:("foo bar" OR "x y")`},
		"in_phrase_word": {`a:("foo bar" OR baz)`, `a:("foo bar" OR baz)`},
		"in_null":        {`a:(b OR null)`, `a:(b OR null)`},
		"in_only_null":   {`a:(null OR null)`, `a:(null OR null)`},
		"in_mixed":       {`a:(Foo OR 5 OR "x y" OR 6.5 OR null)`, `a:(Foo OR 5 OR "x y" OR 6.5 OR null)`},
		"in_numeric_fld": {`0:(1 OR 2)`, `0:(1 OR 2)`},
		"must_not_in":    {`-a:(b OR c)`, `-a:(b OR c)`},
		"in_and":         {`k1:v1 k2:(v2 OR v3)`, `k1:v1 AND k2:(v2 OR v3)`},
		"in_boost":       {`a:(b OR c)^2`, `a:(b OR c)^2.0`},

		"mixed": {
			`(+k1:v1 AND -k2:v2) OR k3:"foo bar"^2 OR k4:a*~10`,
			`+k1:v1 AND -k2:v2 OR k3:"foo bar"^2.0 OR k4:a*~10`,
		},
	})
}

// TestStringOperatorsDefaultField checks the same output when a default field
// is set: the rendered text names the field, so it parses back the same way.
func TestStringOperatorsDefaultField(t *testing.T) {
	runStringCases(t, map[string]stringCase{
		"bare_term":   {`x AND a:>5`, `default:x AND a:>5`},
		"bare_phrase": {`"x y" OR a:fo*`, `default:"x y" OR a:fo*`},
	}, WithDefaultField("default"))
}

// TestStringTerms checks that each value prints so it lexes back as the same
// term. A value that would not is printed as a phrase, with " and \ escaped.
// Wildcards and regexes print as written.
func TestStringTerms(t *testing.T) {
	runStringCases(t, map[string]stringCase{
		"wildcard_space":   {`a:fo\ o*`, `a:fo\ o*`},
		"regex_space":      {`a:/foo bar/`, `a:/foo bar/`},
		"regex_quote":      {`a:/b "[c]/`, `a:/b "[c]/`},
		"regex_bare_space": {`/b [c]/`, `/b [c]/`},
		"in_quoted_num":    {`a:("5" OR x)`, `a:("5" OR x)`},
		"in_quoted_phrase": {`a:("a\"b" OR "")`, `a:("a\"b" OR "")`},

		"quoted_int":        {`a:"5"`, `a:"5"`},
		"quoted_negative":   {`a:"-5"`, `a:"-5"`},
		"quoted_float":      {`a:"1.5"`, `a:"1.5"`},
		"quoted_exponent":   {`a:"1e3"`, `a:"1e3"`},
		"quoted_null":       {`a:"null"`, `a:"null"`},
		"quoted_null_upper": {`a:"NULL"`, `a:"NULL"`},
		"quoted_and":        {`a:"AND"`, `a:"AND"`},
		"quoted_or":         {`a:"or"`, `a:"or"`},
		"quoted_not":        {`a:"Not"`, `a:"Not"`},
		"quoted_to":         {`a:"to"`, `a:"to"`},
		"quoted_star":       {`a:"fo*"`, `a:"fo*"`},
		"quoted_question":   {`a:"f?o"`, `a:"f?o"`},
		"quoted_empty":      {`a:""`, `a:""`},
		"bare_empty":        {`""`, `""`},
		"bare_and":          {`"AND"`, `"AND"`},
		"quoted_colon":      {`a:"a:b"`, `a:"a:b"`},
		"escaped_colon":     {`x:a\:b`, `x:"a:b"`},
		"escaped_parens":    {`a:\(1\+1\)\:2`, `a:"(1+1):2"`},
		"quoted_parens":     {`a:"(x)"`, `a:"(x)"`},
		"quoted_leading":    {`a:"-x"`, `a:"-x"`},
		"quoted_plus":       {`a:"+x"`, `a:"+x"`},
		"plus_in_term":      {`a:"c++"`, `a:c++`},
		"bare_plus_in_term": {`a:1+2 AND b+c:d`, `a:1+2 AND b+c:d`},
		// The parser stores a + in a wildcard escaped, so it prints escaped.
		"plus_in_wildcard":       {`a:c++*`, `a:c\+\+*`},
		"plus_before_question":   {`a:a+b?`, `a:a\+b?`},
		"plus_between_wildcards": {`a:*+*`, `a:*\+*`},
		"quoted_bang":            {`a:"x!"`, `a:"x!"`},
		"windows_path":           {`a:"C:\temp"`, `a:"C:\\temp"`},
		"backslash":              {`a:"foo\\bar"`, `a:"foo\\bar"`},
		"trailing_slash":         {`a:"foo\\"`, `a:"foo\\"`},
		"single_quoted":          {`a:'it\'s'`, `a:"it's"`},
		"escaped_quotes":         {`a:"say \"hi\""`, `a:"say \"hi\""`},
		"phrase":                 {`a:"x y"`, `a:"x y"`},
		"tab":                    {"a:\"x\ty\"", "a:\"x\ty\""},
		"inf":                    {`a:inf`, `a:"inf"`},

		"word":         {`a:foo`, `a:foo`},
		"unicode":      {`a:héllo`, `a:héllo`},
		"email":        {`a:foo@bar.com`, `a:foo@bar.com`},
		"hash":         {`a:#tag`, `a:#tag`},
		"percent":      {`a:50%`, `a:50%`},
		"dotted":       {`a:a.b`, `a:a.b`},
		"dashed":       {`a:a-b`, `a:a-b`},
		"underscore":   {`a:_x`, `a:_x`},
		"digit_prefix": {`a:5a`, `a:5a`},

		"range_quoted":  {`a:["5" TO "TO"]`, `a:["5" TO "TO"]`},
		"range_colon":   {`a:{"a:b" TO c}`, `a:{"a:b" TO c}`},
		"range_escaped": {`a:[a\:b TO c]`, `a:["a:b" TO c]`},
	})
}

// TestStringQuotesFields checks that a field name that would not lex back as
// one term is printed as a phrase.
func TestStringQuotesFields(t *testing.T) {
	runStringCases(t, map[string]stringCase{
		"space":         {`"foo bar":x`, `"foo bar":x`},
		"escaped_space": {`foo\ bar:x`, `"foo bar":x`},
		"keyword":       {`"AND":x`, `"AND":x`},
		"colon":         {`"a:b":c`, `"a:b":c`},
		"quote":         {`"a\"b":c`, `"a\"b":c`},
		"empty":         {`"":x`, `"":x`},
		"range":         {`"a b":[1 TO 2]`, `"a b":[1 TO 2]`},
		"compare":       {`"a b":>5`, `"a b":>5`},
		"in":            {`"a b":(x OR y)`, `"a b":(x OR y)`},
		"wildcard":      {`"a b":fo*`, `"a b":fo*`},

		// A field that looks like a number or null is still a column, so it
		// needs no quotes.
		"number":     {`5:x`, `5:x`},
		"float":      {`1.50:x`, `1.50:x`},
		"null":       {`null:x`, `null:x`},
		"null_upper": {`"NULL":x`, `"NULL":x`},
		"null_mixed": {`"Null":x`, `"Null":x`},
		"quoted":     {`"5":x`, `5:x`},
	})
}

// TestParseBoostAndFuzzyParams pins how the parser reads a boost power or
// fuzzy distance written as a quoted or float literal. The reducers used to
// read it through String(), so a change to how literals print must not
// change what these parse to. A quoted power or distance is text, not a
// number, so it is a separate term.
func TestParseBoostAndFuzzyParams(t *testing.T) {
	tcs := map[string]struct {
		input string
		want  *expr.Expression
	}{
		"fuzzy_quoted":   {`a~"2"`, expr.AND(expr.FUZZY(expr.Lit("a"), 1), expr.Lit("2"))},
		"fuzzy_float":    {`a~2.0`, expr.FUZZY(expr.Lit("a"), 2)},
		"boost_quoted":   {`a^"2"`, expr.AND(expr.BOOST(expr.Lit("a"), 1.0), expr.Lit("2"))},
		"boost_quoted_f": {`a^"0.5"`, expr.AND(expr.BOOST(expr.Lit("a"), 1.0), expr.Lit("0.5"))},
		"boost_float":    {`a^2.5`, expr.BOOST(expr.Lit("a"), 2.5)},
		// A negated group is not a distance. This used to parse as a fuzzy
		// distance of -2, read through MUST_NOT(2).String().
		"fuzzy_negated_group": {`a~-(2)`, expr.AND(expr.FUZZY(expr.Lit("a"), 1), expr.MUSTNOT(expr.Lit(2)))},
		"boost_negated_group": {`a^-(2)`, expr.AND(expr.BOOST(expr.Lit("a"), 1.0), expr.MUSTNOT(expr.Lit(2)))},
	}

	for name, tc := range tcs {
		t.Run(name, func(t *testing.T) {
			got, err := Parse(tc.input)
			if err != nil {
				t.Fatalf("unexpected parse error: %v", err)
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf(errTemplate, "parsed expression does not match", tc.want, got)
			}
		})
	}
}

// TestStringKeepsNumberType checks that a float prints with a decimal point or
// an exponent, so it does not parse back as an int.
func TestStringKeepsNumberType(t *testing.T) {
	runStringCases(t, map[string]stringCase{
		"whole_float":    {`a:5.0`, `a:5.0`},
		"exponent":       {`a:1e3`, `a:1e3`},
		"large_exponent": {`a:1e21`, `a:1e21`},
		"small_exponent": {`a:1.5e-7`, `a:1.5e-7`},
		"huge":           {`a:1e300`, `a:1e300`},
		"small":          {`a:0.0001`, `a:0.0001`},
		"wide_int":       {`a:100000000.0`, `a:100000000.0`},
		"trailing_zero":  {`a:-2.50`, `a:-2.50`},
		"range":          {`a:[0.5 TO 5.0]`, `a:[0.5 TO 5.0]`},
		"bare":           {`8.0`, `8.0`},
		"in":             {`a:(1.0 OR 2)`, `a:(1.0 OR 2)`},
		"int":            {`a:5`, `a:5`},
		"trailing_dot":   {`a:0.`, `a:0.`},
		"big_whole":      {`a:20000000022222200000`, `a:20000000022222200000`},
		"small_digits":   {`a:0.00001`, `a:0.00001`},
		"must_not_dot":   {`- .5`, `-(.5)`},
	})
	runStringCases(t, map[string]stringCase{
		"default_field": {`0.`, `default:0.`},
	}, WithDefaultField("default"))
}

// TestStringFieldGroup checks how a field group prints once its field has been
// applied. A term inside the group with its own field keeps it, as in Lucene,
// so the group's field drops out of a:(b:c), and each other term prints with
// the group's field.
func TestStringFieldGroup(t *testing.T) {
	runStringCases(t, map[string]stringCase{
		"field":          {`a:(b:c)`, `b:c`},
		"range":          {`a:(b:[1 TO 5])`, `b:[1 TO 5]`},
		"compare":        {`a:(b:>5)`, `b:>5`},
		"wildcard":       {`a:(b:fo*)`, `b:fo*`},
		"in":             {`a:(b:(c OR d))`, `b:(c OR d)`},
		"must_not":       {`-a:(b:c)`, `-b:c`},
		"boost":          {`a:(b:c)^2`, `b:c^2.0`},
		"mixed":          {`a:(b:c OR d)`, `b:c OR a:d`},
		"and":            {`a:(x AND -y)`, `a:x AND -a:y`},
		"must_not_plain": {`-a:b`, `-a:b`},
	})
}

// TestStringNegatedNumber checks that a negated number keeps its parentheses.
// -5 parses as the number -5, not as MUST_NOT(5).
func TestStringNegatedNumber(t *testing.T) {
	runStringCases(t, map[string]stringCase{
		"int":          {`-(5)`, `-(5)`},
		"float":        {`-(1.5)`, `-(1.5)`},
		"wildcard":     {`-(1*)`, `-(1*)`},
		"digit_word":   {`-(5a)`, `-(5a)`},
		"negative":     {`-(-5)`, `--5`},
		"literal":      {`-5`, `-5`},
		"must":         {`+(5)`, `+5`},
		"must_not_and": {`a AND -(5)`, `a AND -(5)`},
	})
}

// TestStringBoostAndFuzzyParams checks that String() keeps the boost power and
// fuzzy distance exactly.
func TestStringBoostAndFuzzyParams(t *testing.T) {
	runStringCases(t, map[string]stringCase{
		"boost_default":    {`a:b^`, `a:b^`},
		"boost_whole":      {`a:b^2`, `a:b^2.0`},
		"boost_fraction":   {`a:b^1.25`, `a:b^1.25`},
		"boost_below_one":  {`a:b^0.5`, `a:b^0.5`},
		"fuzzy_default":    {`a:b~`, `a:b~`},
		"fuzzy_two":        {`a:b~2`, `a:b~2`},
		"fuzzy_zero":       {`a:b~0`, `a:b~0`},
		"fuzzy_wild_boost": {`(a:b*~0)^0.5`, `(a:b*~0)^0.5`},
	})
}

// TestStringEscapedWildcardChars checks that String() keeps an escaped * or ?
// literal. A term with only escaped ones prints as a phrase, and a wildcard
// keeps its escapes.
func TestStringEscapedWildcardChars(t *testing.T) {
	runStringCases(t, map[string]stringCase{
		"only_escaped_star":  {`a:fo\*`, `a:"fo*"`},
		"only_escaped_quest": {`a:fo\?`, `a:"fo?"`},
		"escaped_star_first": {`a:\*x*`, `a:\*x*`},
		"escaped_then_wild":  {`a:fo\*bar*`, `a:fo\*bar*`},
		"escaped_backslash":  {`a:fo\\*`, `a:fo\\*`},
		"escaped_space":      {`a:foo\ bar*`, `a:foo\ bar*`},
		"in_group":           {`a:(fo\* OR b?)`, `a:"fo*" OR a:b?`},
		"range_bound":        {`a:[fo\* TO z]`, `a:["fo*" TO z]`},
		"escaped_field":      {`fo\*:x`, `"fo*":x`},
		"backslash_term":     {`a:foo\\bar`, `a:"foo\\bar"`},
		"exponent_plus":      {`a:1e+5*`, `a:1e\+5*`},
	})
}

// TestStringBuiltWildcard checks String() on wildcards and regexes built in
// code, which hold only their value. String() escapes each character the
// lexer would split off or read differently, and keeps the * and ? that are
// wildcards, so the text parses back to a term that renders the same SQL.
// expr.LIKE("a", "foo bar*") used to print a:foo bar*, which parses as
// a:foo AND bar*.
func TestStringBuiltWildcard(t *testing.T) {
	tcs := map[string]struct {
		input *expr.Expression
		want  string
	}{
		"space":              {expr.LIKE("a", "foo bar*"), `a:foo\ bar*`},
		"colon":              {expr.LIKE("a", "x:y*"), `a:x\:y*`},
		"comma":              {expr.LIKE("a", "x,y*"), `a:x\,y*`},
		"tab":                {expr.LIKE("a", "x\ty*"), "a:x\\\ty*"},
		"quotes":             {expr.LIKE("a", `it's "x"*`), `a:it\'s\ \"x\"*`},
		"slashes":            {expr.LIKE("a", expr.WILD("/x*/")), `a:\/x*\/`},
		"escaped_star":       {expr.LIKE("a", `fo\*bar*`), `a:fo\*bar*`},
		"escaped_backslash":  {expr.LIKE("a", `fo\\*`), `a:fo\\*`},
		"escaped_letter":     {expr.LIKE("a", `f\oo*`), `a:f\oo*`},
		"trailing_backslash": {expr.LIKE("a", `a*\`), `a:a*\\`},
		"leading_minus":      {expr.LIKE("a", "-x*"), `a:\-x*`},
		"negative_number":    {expr.LIKE("a", "-5*"), `a:-5*`},
		"negative_fraction":  {expr.LIKE("a", "-.5*"), `a:-.5*`},
		"inner_minus":        {expr.LIKE("a", "x-y*"), `a:x-y*`},
		"leading_dot":        {expr.LIKE("a", ".x*"), `a:.x*`},
		"leading_bang":       {expr.LIKE("a", "!x*"), `a:\!x*`},
		"literal_chars":      {expr.LIKE("a", "50%_@#$*"), `a:50%_@#$*`},
		"keyword":            {expr.LIKE("a", "AND*"), `a:AND*`},
		"unicode_letter":     {expr.LIKE("a", "é*"), `a:é*`},
		"unicode_symbol":     {expr.LIKE("a", "€*"), `a:\€*`},
		"range_bound":        {expr.Rang("a", expr.WILD("f o*"), "z", true), `a:[f\ o* TO z]`},
		"in_must_not":        {expr.MUSTNOT(expr.LIKE("a", "x y*")), `-a:x\ y*`},
		// A LIKE renders a literal on its right as a pattern, so String()
		// prints it as one. It used to print a:"fo*", an exact match.
		"like_literal":        {expr.LIKE("a", expr.Lit("fo*")), `a:fo*`},
		"like_literal_space":  {expr.LIKE("a", expr.Lit("x y?")), `a:x\ y?`},
		"like_literal_escape": {expr.LIKE("a", expr.Lit(`fo\*bar*`)), `a:fo\*bar*`},
		"like_literal_json":   {mustDecode(t, `{"left":"a","operator":"LIKE","right":{"literal":"fo*"}}`), `a:fo*`},
	}
	for name, tc := range tcs {
		t.Run(name, func(t *testing.T) {
			got := tc.input.String()
			if got != tc.want {
				t.Fatalf("String():\n got: %s\nwant: %s", got, tc.want)
			}
			parsed, err := Parse(got)
			if err != nil {
				t.Fatalf("String() of %#v is %q, which does not parse: %v", tc.input, got, err)
			}
			assertSameSQL(t, tc.input, parsed)
		})
	}
}

// TestStringBuiltWildcardWithoutWildcard checks a wildcard built in code with
// no unescaped * or ?. No wildcard term can hold it, so String() prints the
// text it matches as a plain term.
func TestStringBuiltWildcardWithoutWildcard(t *testing.T) {
	runBuilt(t, map[string]builtCase{
		"word":         {expr.LIKE("a", expr.WILD("foo")), `a:foo`, expr.Eq("a", "foo")},
		"space":        {expr.LIKE("a", expr.WILD("foo bar")), `a:"foo bar"`, expr.Eq("a", "foo bar")},
		"keyword":      {expr.LIKE("a", expr.WILD("AND")), `a:"AND"`, expr.Eq("a", "AND")},
		"number":       {expr.LIKE("a", expr.WILD("5")), `a:"5"`, expr.Eq("a", "5")},
		"only_escaped": {expr.LIKE("a", expr.WILD(`fo\*`)), `a:"fo*"`, expr.Eq("a", expr.Lit("fo*"))},
		"empty":        {expr.LIKE("a", expr.WILD("")), `a:""`, expr.Eq("a", "")},
		// A LIKE reads the backslash in a literal on its right as an escape.
		"like_literal": {expr.LIKE("a", expr.Lit(`a\b`)), `a:ab`, expr.Eq("a", "ab")},
	})
}

// TestStringBuiltWildcardWithoutField checks a wildcard with no field, which
// the parser reads as a bare term.
func TestStringBuiltWildcardWithoutField(t *testing.T) {
	runBuilt(t, map[string]builtCase{
		"space": {expr.WILD("foo bar*"), `foo\ bar*`, expr.WILD(`foo\ bar*`)},
		"or":    {expr.OR(expr.WILD("x y*"), expr.Lit("z")), `x\ y* OR z`, expr.OR(expr.WILD(`x\ y*`), expr.Lit("z"))},
	})
}

// TestStringBuiltPatternOperators checks a pattern built in code that uses
// SIMILAR TO operators such as | and (). Lucene has no syntax for them, so
// String() escapes them and they parse back as literal characters.
func TestStringBuiltPatternOperators(t *testing.T) {
	runBuilt(t, map[string]builtCase{
		"alternation": {expr.LIKE("a", "*(x|y)*"), `a:*\(x\|y\)*`, expr.LIKE("a", expr.WILD(`*\(x\|y\)*`))},
		"class":       {expr.LIKE("a", "[xy]*"), `a:\[xy\]*`, expr.LIKE("a", expr.WILD(`\[xy\]*`))},
		"plus":        {expr.LIKE("a", "x+*"), `a:x\+*`, expr.LIKE("a", expr.WILD(`x\+*`))},
	})
}

// TestStringBuiltRegexp checks String() on regexes built in code. A regex
// without its slashes gets them, and a slash inside one is escaped, so the
// text parses back to a regex.
func TestStringBuiltRegexp(t *testing.T) {
	runBuilt(t, map[string]builtCase{
		"no_slashes":        {expr.LIKE("a", expr.REGEXP("fo+")), `a:/fo+/`, expr.LIKE("a", expr.REGEXP("/fo+/"))},
		"inner_slash":       {expr.LIKE("a", expr.REGEXP("/x/y/")), `a:/x\/y/`, expr.LIKE("a", expr.REGEXP(`/x\/y/`))},
		"escaped_slash":     {expr.LIKE("a", expr.REGEXP(`/x\/y/`)), `a:/x\/y/`, expr.LIKE("a", expr.REGEXP(`/x\/y/`))},
		"trailing_escape":   {expr.LIKE("a", expr.REGEXP(`/x\/`)), `a:/x\\/`, expr.LIKE("a", expr.REGEXP(`/x\\/`))},
		"only_slash":        {expr.LIKE("a", expr.REGEXP("/")), `a:/\//`, expr.LIKE("a", expr.REGEXP(`/\//`))},
		"space":             {expr.LIKE("a", expr.REGEXP("/x y/")), `a:/x y/`, expr.LIKE("a", expr.REGEXP("/x y/"))},
		"no_field":          {expr.REGEXP("x/y"), `/x\/y/`, expr.REGEXP(`/x\/y/`)},
		"escaped_backslash": {expr.LIKE("a", expr.REGEXP(`/x\\/`)), `a:/x\\/`, expr.LIKE("a", expr.REGEXP(`/x\\/`))},
	})
}

func mustDecode(t *testing.T, raw string) *expr.Expression {
	t.Helper()
	var e expr.Expression
	if err := json.Unmarshal([]byte(raw), &e); err != nil {
		t.Fatalf("unmarshal %s: %v", raw, err)
	}
	return &e
}

type builtCase struct {
	input *expr.Expression
	want  string
	// parsed is what want parses to.
	parsed *expr.Expression
}

func runBuilt(t *testing.T, tcs map[string]builtCase) {
	t.Helper()
	for name, tc := range tcs {
		t.Run(name, func(t *testing.T) {
			got := tc.input.String()
			if got != tc.want {
				t.Fatalf("String():\n got: %s\nwant: %s", got, tc.want)
			}
			parsed, err := Parse(got)
			if err != nil {
				t.Fatalf("String() of %#v is %q, which does not parse: %v", tc.input, got, err)
			}
			if !reflect.DeepEqual(parsed, tc.parsed) {
				t.Fatalf(errTemplate, "String() parses to a different expression", tc.parsed, parsed)
			}
		})
	}
}

// assertSameSQL checks that two expressions render the same SQL and
// parameters on every driver.
func assertSameSQL(t *testing.T, want, got *expr.Expression) {
	t.Helper()
	drivers := map[string]interface {
		Render(*expr.Expression) (string, error)
		RenderParam(*expr.Expression) (string, []any, error)
	}{
		"postgres": driver.NewPostgresDriver(),
		"mysql":    driver.NewMySQLDriver(),
		"sqlite":   driver.NewSQLiteDriver(),
	}
	for name, d := range drivers {
		wantSQL, err := d.Render(want)
		if err != nil {
			t.Fatalf("%s: render %#v: %v", name, want, err)
		}
		gotSQL, err := d.Render(got)
		if err != nil {
			t.Fatalf("%s: render %#v: %v", name, got, err)
		}
		if gotSQL != wantSQL {
			t.Fatalf("%s: the parsed String() renders different SQL:\n got: %s\nwant: %s", name, gotSQL, wantSQL)
		}
		_, wantParams, err := d.RenderParam(want)
		if err != nil {
			t.Fatalf("%s: render param %#v: %v", name, want, err)
		}
		_, gotParams, err := d.RenderParam(got)
		if err != nil {
			t.Fatalf("%s: render param %#v: %v", name, got, err)
		}
		if !reflect.DeepEqual(gotParams, wantParams) {
			t.Fatalf("%s: the parsed String() renders different params:\n got: %#v\nwant: %#v", name, gotParams, wantParams)
		}
	}
}
