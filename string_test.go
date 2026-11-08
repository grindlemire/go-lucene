package lucene

import (
	"reflect"
	"testing"
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

