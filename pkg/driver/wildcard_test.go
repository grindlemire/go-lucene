package driver

import (
	"testing"

	"github.com/grindlemire/go-lucene/pkg/lucene/expr"
)

// TestWildcardPatternEscapes checks how each dialect writes a Lucene wildcard
// pattern. * and ? are wildcards and a backslash makes the next character
// literal. Every literal character must match only itself, so each dialect
// escapes the characters its pattern syntax gives a meaning to: % _ * ? and
// the parentheses, brackets, braces, | and + of SIMILAR TO; % _ and the
// escape character # of MySQL LIKE; * ? and [ of GLOB.
func TestWildcardPatternEscapes(t *testing.T) {
	type want struct{ postgres, mysql, sqlite string }
	tcs := map[string]struct {
		pattern string
		want    want
	}{
		"escaped_star":       {`\*x*`, want{`\*x%`, `*x%`, `[*]x*`}},
		"escaped_question":   {`x\?*`, want{`x\?%`, `x?%`, `x[?]*`}},
		"star_after_escape":  {`fo\*bar*`, want{`fo\*bar%`, `fo*bar%`, `fo[*]bar*`}},
		"percent":            {`100%*`, want{`100\%%`, `100#%%`, `100%*`}},
		"escaped_percent":    {`100\%*`, want{`100\%%`, `100#%%`, `100%*`}},
		"underscore":         {`a_*`, want{`a\_%`, `a#_%`, `a_*`}},
		"escaped_underscore": {`a\_*`, want{`a\_%`, `a#_%`, `a_*`}},
		"bracket":            {`a\[*`, want{`a\[%`, `a[%`, `a[[]*`}},
		"close_bracket":      {`a\]*`, want{`a\]%`, `a]%`, `a]*`}},
		"backslash":          {`a\\*`, want{`a\\%`, `a\%`, `a\*`}},
		"hash":               {`a#*`, want{`a#%`, `a##%`, `a#*`}},
		"escaped_hash":       {`a\#*`, want{`a#%`, `a##%`, `a#*`}},
		"space":              {`a\ b*`, want{`a b%`, `a b%`, `a b*`}},
		"colon":              {`a\:b?`, want{`a:b_`, `a:b_`, `a:b?`}},
		"letter":             {`\ab*`, want{`ab%`, `ab%`, `ab*`}},
		"dot":                {`a.b*`, want{`a.b%`, `a.b%`, `a.b*`}},
		"parens":             {`a\(b\)*`, want{`a\(b\)%`, `a(b)%`, `a(b)*`}},
		"pipe":               {`a\|b*`, want{`a\|b%`, `a|b%`, `a|b*`}},
		"plus":               {`a\+b*`, want{`a\+b%`, `a+b%`, `a+b*`}},
		"braces":             {`a\{1\}*`, want{`a\{1\}%`, `a{1}%`, `a{1}*`}},
		"quote":              {`it\'s*`, want{`it's%`, `it's%`, `it's*`}},
		"no_wildcard":        {`fo\*`, want{`fo\*`, `fo*`, `fo[*]`}},
		"trailing_escape":    {`a*\`, want{`a%\\`, `a%\`, `a*\`}},
		"multibyte":          {`\é*`, want{`é%`, `é%`, `é*`}},
	}
	for name, tc := range tcs {
		for dname, d := range allDrivers {
			pattern := map[string]string{"postgres": tc.want.postgres, "mysql": tc.want.mysql, "sqlite": tc.want.sqlite}[dname]
			op := map[string]string{"postgres": `"a" SIMILAR TO `, "mysql": "`a` LIKE ", "sqlite": `"a" GLOB `}[dname]
			suffix := map[string]string{"mysql": " ESCAPE '#'"}[dname]
			dialect := map[string]Dialect{"postgres": postgresDialect{}, "mysql": mysqlDialect{}, "sqlite": sqliteDialect{}}[dname]
			e := expr.LIKE("a", expr.WILD(tc.pattern))
			t.Run(name+"/"+dname, func(t *testing.T) {
				got, err := d.Render(e)
				if err != nil {
					t.Fatalf("Render: unexpected error: %v", err)
				}
				if want := op + dialect.EscapeStringLiteral(pattern) + suffix; got != want {
					t.Fatalf(errTemplate, "Render sql does not match", want, got)
				}
				got, params, err := d.RenderParam(e)
				if err != nil {
					t.Fatalf("RenderParam: unexpected error: %v", err)
				}
				wantSQL := op + "?" + suffix
				if dname == "postgres" {
					wantSQL = op + "$1"
				}
				if got != wantSQL {
					t.Fatalf(errTemplate, "RenderParam sql does not match", wantSQL, got)
				}
				if len(params) != 1 || params[0] != pattern {
					t.Fatalf("want params [%q], got %#v", pattern, params)
				}
			})
		}
	}
}

// TestWildcardEscapesInRenderedSQL spells out the full SQL for the cases
// where a backslash meets each dialect's string-literal escaping.
func TestWildcardEscapesInRenderedSQL(t *testing.T) {
	tcs := map[string]struct {
		input *expr.Expression
		want  map[string]string
	}{
		"escaped_star": {
			input: expr.LIKE("a", expr.WILD(`\*x*`)),
			want: map[string]string{
				"postgres": `"a" SIMILAR TO '\*x%'`,
				"mysql":    "`a` LIKE '*x%' ESCAPE '#'",
				"sqlite":   `"a" GLOB '[*]x*'`,
			},
		},
		"escaped_backslash": {
			input: expr.LIKE("a", expr.WILD(`a\\*`)),
			want: map[string]string{
				"postgres": `"a" SIMILAR TO 'a\\%'`,
				"mysql":    "`a` LIKE 'a\\\\%' ESCAPE '#'",
				"sqlite":   `"a" GLOB 'a\*'`,
			},
		},
	}
	for name, tc := range tcs {
		for dname, d := range allDrivers {
			t.Run(name+"/"+dname, func(t *testing.T) {
				got, err := d.Render(tc.input)
				if err != nil {
					t.Fatalf("Render: unexpected error: %v", err)
				}
				if got != tc.want[dname] {
					t.Fatalf(errTemplate, "Render sql does not match", tc.want[dname], got)
				}
			})
		}
	}
}

// TestEscapeWildcardMatchesItself checks that text passed through
// expr.EscapeWildcard reaches each dialect with every character literal: the
// pattern has no wildcard or operator left, and a * appended to it is the
// only wildcard.
func TestEscapeWildcardMatchesItself(t *testing.T) {
	text := `a\b*c?d%e_f#g[h]i(j)k|l{m}n+o^p-q.r$s'u"v`
	want := map[string]string{
		"postgres": `a\\b\*c\?d\%e\_f#g\[h\]i\(j\)k\|l\{m\}n\+o^p-q.r$s'u"v`,
		"mysql":    `a\b*c?d#%e#_f##g[h]i(j)k|l{m}n+o^p-q.r$s'u"v`,
		"sqlite":   `a\b[*]c[?]d%e_f#g[[]h]i(j)k|l{m}n+o^p-q.r$s'u"v`,
	}
	wildcard := map[string]string{"postgres": "%", "mysql": "%", "sqlite": "*"}
	for dname, d := range allDrivers {
		t.Run(dname, func(t *testing.T) {
			_, params, err := d.RenderParam(expr.LIKE("a", expr.WILD(expr.EscapeWildcard(text))))
			if err != nil {
				t.Fatalf("RenderParam: unexpected error: %v", err)
			}
			if len(params) != 1 || params[0] != want[dname] {
				t.Fatalf("want params [%q], got %#v", want[dname], params)
			}
			_, params, err = d.RenderParam(expr.LIKE("a", expr.EscapeWildcard(text)+"*"))
			if err != nil {
				t.Fatalf("RenderParam: unexpected error: %v", err)
			}
			if len(params) != 1 || params[0] != want[dname]+wildcard[dname] {
				t.Fatalf("want params [%q], got %#v", want[dname]+wildcard[dname], params)
			}
		})
	}
}

// TestWildcardBracketExpression checks a bracket expression in a pattern
// built in code. Inside the brackets every character is a member of the set,
// so * and ? are not translated, and an escaped ^, -, ], [ or \ keeps its
// backslash where the dialect reads one. x[\^a]* used to give Postgres
// x[^a]%, which negates the set, and x[a\-c]* became the range a-c.
// GLOB has no escape inside brackets, so SQLite lists only the cases it can
// write.
func TestWildcardBracketExpression(t *testing.T) {
	tcs := map[string]map[string]string{
		`x[\^a]*`: {
			"postgres": `x[\^a]%`,
			"mysql":    `^(x[\^a].*)$`,
		},
		`x[a\-c]*`: {
			"postgres": `x[a\-c]%`,
			"mysql":    `^(x[a\-c].*)$`,
		},
		`x[a\]]*`: {
			"postgres": `x[a\]]%`,
			"mysql":    `^(x[a\]].*)$`,
		},
		`x[a\[]*`: {
			"postgres": `x[a\[]%`,
			"mysql":    `^(x[a\[].*)$`,
		},
		`x[a\\]*`: {
			"postgres": `x[a\\]%`,
			"mysql":    `^(x[a\\].*)$`,
			"sqlite":   `x[a\]*`,
		},
		`x[*?%_.]*`: {
			"postgres": `x[*?%_.]%`,
			"mysql":    `^(x[*?%_.].*)$`,
			"sqlite":   `x[*?%_.]*`,
		},
		`x[\*\?]?`: {
			"postgres": `x[*?]_`,
			"mysql":    `^(x[*?].)$`,
			"sqlite":   `x[*?]?`,
		},
		// A ] right after the [ is a member, as in POSIX.
		`x[]a]*`: {
			"postgres": `x[]a]%`,
			"mysql":    `^(x[]a].*)$`,
			"sqlite":   `x[]a]*`,
		},
		// Without its ], a [ starts no bracket expression.
		`x[a*`: {
			"postgres": `x[a%`,
			"mysql":    `^(x[a.*)$`,
			"sqlite":   `x[a*`,
		},
	}
	for pattern, want := range tcs {
		for dname, d := range allDrivers {
			w, ok := want[dname]
			if !ok {
				continue
			}
			t.Run(pattern+"/"+dname, func(t *testing.T) {
				_, params, err := d.RenderParam(expr.LIKE("a", expr.WILD(pattern)))
				if err != nil {
					t.Fatalf("RenderParam: unexpected error: %v", err)
				}
				if len(params) != 1 || params[0] != w {
					t.Fatalf("want params [%q], got %#v", w, params)
				}
			})
		}
	}
}

// TestMySQLRegexFallbackKeepsEscapesLiteral checks the MySQL REGEXP fallback,
// which a pattern built in code takes when it has an unescaped SIMILAR TO
// operator such as | or (. An escaped character in the same pattern is
// still literal, so it is escaped for the regex.
func TestMySQLRegexFallbackKeepsEscapesLiteral(t *testing.T) {
	tcs := map[string]string{
		`(a|b)\**`:  `^((a|b)\*.*)$`,
		`(a|b)\?`:   `^((a|b)\?)$`,
		`(a)\.x`:    `^((a)\.x)$`,
		`(a)\\x`:    `^((a)\\x)$`,
		`(a)\|x`:    `^((a)\|x)$`,
		`(a)\[x\]?`: `^((a)\[x\].)$`,
		`(a)\bx`:    `^((a)bx)$`,
		`(a)#%_`:    `^((a)#%_)$`,
	}
	d := NewMySQLDriver()
	for pattern, want := range tcs {
		t.Run(pattern, func(t *testing.T) {
			_, params, err := d.RenderParam(expr.LIKE("a", expr.WILD(pattern)))
			if err != nil {
				t.Fatalf("RenderParam: unexpected error: %v", err)
			}
			if len(params) != 1 || params[0] != want {
				t.Fatalf("want params [%q], got %#v", want, params)
			}
		})
	}
}

// TestWildcardRangeBoundIsUnescaped checks that a wildcard used as a range
// bound is compared as the text it spells, without its escapes. Lucene
// reads a range bound as plain text, so * and ? there are not wildcards.
func TestWildcardRangeBoundIsUnescaped(t *testing.T) {
	e := expr.Rang("a", expr.WILD(`f\o*`), expr.WILD(`z\\?`), true)
	for dname, d := range allDrivers {
		t.Run(dname, func(t *testing.T) {
			_, params, err := d.RenderParam(e)
			if err != nil {
				t.Fatalf("RenderParam: unexpected error: %v", err)
			}
			if len(params) != 2 || params[0] != "fo*" || params[1] != `z\?` {
				t.Fatalf("want params [\"fo*\" %q], got %#v", `z\?`, params)
			}
		})
	}
}
