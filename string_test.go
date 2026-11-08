package lucene

import (
	"reflect"
	"testing"

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
		"greater_group":    {`a:>(x OR c)`, `a:>(x OR c)`},
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
		"quoted_bang":       {`a:"x!"`, `a:"x!"`},
		"windows_path":      {`a:"C:\temp"`, `a:"C:\\temp"`},
		"backslash":         {`a:"foo\\bar"`, `a:"foo\\bar"`},
		"trailing_slash":    {`a:"foo\\"`, `a:"foo\\"`},
		"single_quoted":     {`a:'it\'s'`, `a:"it's"`},
		"escaped_quotes":    {`a:"say \"hi\""`, `a:"say \"hi\""`},
		"phrase":            {`a:"x y"`, `a:"x y"`},
		"tab":               {"a:\"x\ty\"", "a:\"x\ty\""},
		"inf":               {`a:inf`, `a:"inf"`},

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
		"number": {`5:x`, `5:x`},
		"float":  {`1.50:x`, `1.50:x`},
		"null":   {`null:x`, `null:x`},
		"quoted": {`"5":x`, `5:x`},
	})
}

// TestParseBoostAndFuzzyParams pins how the parser reads a boost power or
// fuzzy distance written as a quoted or float literal. The reducers used to
// read it through String(), so a change to how literals print must not
// change what these parse to.
func TestParseBoostAndFuzzyParams(t *testing.T) {
	tcs := map[string]struct {
		input string
		want  *expr.Expression
	}{
		"fuzzy_quoted":   {`a~"2"`, expr.FUZZY(expr.Lit("a"), 2)},
		"fuzzy_float":    {`a~2.0`, expr.FUZZY(expr.Lit("a"), 2)},
		"boost_quoted":   {`a^"2"`, expr.BOOST(expr.Lit("a"), 2.0)},
		"boost_quoted_f": {`a^"0.5"`, expr.BOOST(expr.Lit("a"), 0.5)},
		"boost_float":    {`a^2.5`, expr.BOOST(expr.Lit("a"), 2.5)},
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
		"exponent":       {`a:1e3`, `a:1000.0`},
		"large_exponent": {`a:1e21`, `a:1e21`},
		"small_exponent": {`a:1.5e-7`, `a:1.5e-07`},
		"huge":           {`a:1e300`, `a:1e300`},
		"small":          {`a:0.0001`, `a:0.0001`},
		"wide_int":       {`a:100000000.0`, `a:100000000.0`},
		"trailing_zero":  {`a:-2.50`, `a:-2.5`},
		"range":          {`a:[0.5 TO 5.0]`, `a:[0.5 TO 5.0]`},
		"bare":           {`8.0`, `8.0`},
		"in":             {`a:(1.0 OR 2)`, `a:(1.0 OR 2)`},
		"int":            {`a:5`, `a:5`},
	})
}
