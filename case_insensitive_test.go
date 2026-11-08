package lucene

import (
	"reflect"
	"testing"
)

// ciWant is the expected output of one dialect: the inline SQL, the
// parameterized SQL, and its params.
type ciWant struct {
	str      string
	paramStr string
	params   []any
}

// TestCaseInsensitiveFields covers issue #57: WithCaseInsensitiveFields makes
// string equality, IN lists and wildcards on the listed fields ignore case, in
// every dialect, and leaves everything else as it renders today.
func TestCaseInsensitiveFields(t *testing.T) {
	type render func(string, ...Opt) (string, error)
	type renderParam func(string, ...Opt) (string, []any, error)

	dialects := map[string]struct {
		render      render
		renderParam renderParam
	}{
		"postgres": {ToPostgres, ToParameterizedPostgres},
		"mysql":    {ToMySQL, ToParameterizedMySQL},
		"mariadb":  {ToMariaDB, ToParameterizedMariaDB},
		"sqlite":   {ToSQLite, ToParameterizedSQLite},
	}

	listed := []Opt{WithCaseInsensitiveFields("name")}

	tcs := map[string]struct {
		input string
		opts  []Opt
		want  map[string]ciWant
	}{
		// Wildcards.
		"prefix_wildcard": {
			input: "name:Foo*",
			opts:  listed,
			want: map[string]ciWant{
				"postgres": {`LOWER("name") SIMILAR TO LOWER('Foo%')`, `LOWER("name") SIMILAR TO LOWER($1)`, []any{"Foo%"}},
				"mysql":    {"LOWER(`name`) LIKE LOWER('Foo%') ESCAPE '#'", "LOWER(`name`) LIKE LOWER(?) ESCAPE '#'", []any{"Foo%"}},
				"sqlite":   {`LOWER("name") GLOB LOWER('Foo*')`, `LOWER("name") GLOB LOWER(?)`, []any{"Foo*"}},
			},
		},
		"single_char_wildcard": {
			input: "name:F?o",
			opts:  listed,
			want: map[string]ciWant{
				"postgres": {`LOWER("name") SIMILAR TO LOWER('F_o')`, `LOWER("name") SIMILAR TO LOWER($1)`, []any{"F_o"}},
				"mysql":    {"LOWER(`name`) LIKE LOWER('F_o') ESCAPE '#'", "LOWER(`name`) LIKE LOWER(?) ESCAPE '#'", []any{"F_o"}},
				"sqlite":   {`LOWER("name") GLOB LOWER('F?o')`, `LOWER("name") GLOB LOWER(?)`, []any{"F?o"}},
			},
		},
		"wildcard_keeps_pattern_escapes": {
			input: "name:100%_*",
			opts:  listed,
			want: map[string]ciWant{
				"postgres": {`LOWER("name") SIMILAR TO LOWER('100\%\_%')`, `LOWER("name") SIMILAR TO LOWER($1)`, []any{`100\%\_%`}},
				"mysql":    {"LOWER(`name`) LIKE LOWER('100#%#_%') ESCAPE '#'", "LOWER(`name`) LIKE LOWER(?) ESCAPE '#'", []any{"100#%#_%"}},
				"sqlite":   {`LOWER("name") GLOB LOWER('100%_*')`, `LOWER("name") GLOB LOWER(?)`, []any{"100%_*"}},
			},
		},
		"negated_wildcard": {
			input: "-name:Foo*",
			opts:  listed,
			want: map[string]ciWant{
				"postgres": {`NOT(LOWER("name") SIMILAR TO LOWER('Foo%'))`, `NOT(LOWER("name") SIMILAR TO LOWER($1))`, []any{"Foo%"}},
				"mysql":    {"NOT(LOWER(`name`) LIKE LOWER('Foo%') ESCAPE '#')", "NOT(LOWER(`name`) LIKE LOWER(?) ESCAPE '#')", []any{"Foo%"}},
				"sqlite":   {`NOT(LOWER("name") GLOB LOWER('Foo*'))`, `NOT(LOWER("name") GLOB LOWER(?))`, []any{"Foo*"}},
			},
		},

		// Exact matches.
		"equality": {
			input: "name:Foo",
			opts:  listed,
			want: map[string]ciWant{
				"postgres": {`LOWER("name") = LOWER('Foo')`, `LOWER("name") = LOWER($1)`, []any{"Foo"}},
				"mysql":    {"LOWER(`name`) = LOWER('Foo')", "LOWER(`name`) = LOWER(?)", []any{"Foo"}},
				"sqlite":   {`LOWER("name") = LOWER('Foo')`, `LOWER("name") = LOWER(?)`, []any{"Foo"}},
			},
		},
		"phrase": {
			input: `name:"Foo Bar"`,
			opts:  listed,
			want: map[string]ciWant{
				"postgres": {`LOWER("name") = LOWER('Foo Bar')`, `LOWER("name") = LOWER($1)`, []any{"Foo Bar"}},
				"mysql":    {"LOWER(`name`) = LOWER('Foo Bar')", "LOWER(`name`) = LOWER(?)", []any{"Foo Bar"}},
				"sqlite":   {`LOWER("name") = LOWER('Foo Bar')`, `LOWER("name") = LOWER(?)`, []any{"Foo Bar"}},
			},
		},
		"quote_in_value": {
			input: `name:"O'Brien"`,
			opts:  listed,
			want: map[string]ciWant{
				"postgres": {`LOWER("name") = LOWER('O''Brien')`, `LOWER("name") = LOWER($1)`, []any{"O'Brien"}},
				"mysql":    {"LOWER(`name`) = LOWER('O''Brien')", "LOWER(`name`) = LOWER(?)", []any{"O'Brien"}},
				"sqlite":   {`LOWER("name") = LOWER('O''Brien')`, `LOWER("name") = LOWER(?)`, []any{"O'Brien"}},
			},
		},
		// A quoted number is a string, so it folds.
		"quoted_number": {
			input: `name:"5"`,
			opts:  listed,
			want: map[string]ciWant{
				"postgres": {`LOWER("name") = LOWER('5')`, `LOWER("name") = LOWER($1)`, []any{"5"}},
				"mysql":    {"LOWER(`name`) = LOWER('5')", "LOWER(`name`) = LOWER(?)", []any{"5"}},
				"sqlite":   {`LOWER("name") = LOWER('5')`, `LOWER("name") = LOWER(?)`, []any{"5"}},
			},
		},
		"negated_equality": {
			input: "NOT name:Foo",
			opts:  listed,
			want: map[string]ciWant{
				"postgres": {`NOT(LOWER("name") = LOWER('Foo'))`, `NOT(LOWER("name") = LOWER($1))`, []any{"Foo"}},
				"mysql":    {"NOT(LOWER(`name`) = LOWER('Foo'))", "NOT(LOWER(`name`) = LOWER(?))", []any{"Foo"}},
				"sqlite":   {`NOT(LOWER("name") = LOWER('Foo'))`, `NOT(LOWER("name") = LOWER(?))`, []any{"Foo"}},
			},
		},

		// IN lists.
		"in_list": {
			input: "name:(Foo OR Bar)",
			opts:  listed,
			want: map[string]ciWant{
				"postgres": {`LOWER("name") IN (LOWER('Foo'), LOWER('Bar'))`, `LOWER("name") IN (LOWER($1), LOWER($2))`, []any{"Foo", "Bar"}},
				"mysql":    {"LOWER(`name`) IN (LOWER('Foo'), LOWER('Bar'))", "LOWER(`name`) IN (LOWER(?), LOWER(?))", []any{"Foo", "Bar"}},
				"sqlite":   {`LOWER("name") IN (LOWER('Foo'), LOWER('Bar'))`, `LOWER("name") IN (LOWER(?), LOWER(?))`, []any{"Foo", "Bar"}},
			},
		},
		"in_list_with_null": {
			input: "name:(Foo OR Bar OR null)",
			opts:  listed,
			want: map[string]ciWant{
				"postgres": {`(LOWER("name") IN (LOWER('Foo'), LOWER('Bar')) OR "name" IS NULL)`, `(LOWER("name") IN (LOWER($1), LOWER($2)) OR "name" IS NULL)`, []any{"Foo", "Bar"}},
				"mysql":    {"(LOWER(`name`) IN (LOWER('Foo'), LOWER('Bar')) OR `name` IS NULL)", "(LOWER(`name`) IN (LOWER(?), LOWER(?)) OR `name` IS NULL)", []any{"Foo", "Bar"}},
				"sqlite":   {`(LOWER("name") IN (LOWER('Foo'), LOWER('Bar')) OR "name" IS NULL)`, `(LOWER("name") IN (LOWER(?), LOWER(?)) OR "name" IS NULL)`, []any{"Foo", "Bar"}},
			},
		},
		"single_term_with_null": {
			input: "name:(Foo OR null)",
			opts:  listed,
			want: map[string]ciWant{
				"postgres": {`(LOWER("name") = LOWER('Foo') OR "name" IS NULL)`, `(LOWER("name") = LOWER($1) OR "name" IS NULL)`, []any{"Foo"}},
				"mysql":    {"(LOWER(`name`) = LOWER('Foo') OR `name` IS NULL)", "(LOWER(`name`) = LOWER(?) OR `name` IS NULL)", []any{"Foo"}},
				"sqlite":   {`(LOWER("name") = LOWER('Foo') OR "name" IS NULL)`, `(LOWER("name") = LOWER(?) OR "name" IS NULL)`, []any{"Foo"}},
			},
		},
		// A mixed list splits: strings compare folded, numbers compare as
		// before, so LOWER() never sees a number.
		"mixed_in_list": {
			input: "name:(Foo OR 5)",
			opts:  listed,
			want: map[string]ciWant{
				"postgres": {`(LOWER("name") = LOWER('Foo') OR "name" = 5)`, `(LOWER("name") = LOWER($1) OR "name" = $2)`, []any{"Foo", 5}},
				"mysql":    {"(LOWER(`name`) = LOWER('Foo') OR `name` = 5)", "(LOWER(`name`) = LOWER(?) OR `name` = ?)", []any{"Foo", 5}},
				"sqlite":   {`(LOWER("name") = LOWER('Foo') OR "name" = 5)`, `(LOWER("name") = LOWER(?) OR "name" = ?)`, []any{"Foo", 5}},
			},
		},
		"mixed_in_list_number_first": {
			input: "name:(5 OR Foo)",
			opts:  listed,
			want: map[string]ciWant{
				"postgres": {`(LOWER("name") = LOWER('Foo') OR "name" = 5)`, `(LOWER("name") = LOWER($1) OR "name" = $2)`, []any{"Foo", 5}},
				"mysql":    {"(LOWER(`name`) = LOWER('Foo') OR `name` = 5)", "(LOWER(`name`) = LOWER(?) OR `name` = ?)", []any{"Foo", 5}},
				"sqlite":   {`(LOWER("name") = LOWER('Foo') OR "name" = 5)`, `(LOWER("name") = LOWER(?) OR "name" = ?)`, []any{"Foo", 5}},
			},
		},
		"mixed_in_list_with_null": {
			input: "name:(Foo OR 5 OR Bar OR 6.5 OR null)",
			opts:  listed,
			want: map[string]ciWant{
				"postgres": {
					`(LOWER("name") IN (LOWER('Foo'), LOWER('Bar')) OR "name" IN (5, 6.5) OR "name" IS NULL)`,
					`(LOWER("name") IN (LOWER($1), LOWER($2)) OR "name" IN ($3, $4) OR "name" IS NULL)`,
					[]any{"Foo", "Bar", 5, 6.5},
				},
				"mysql": {
					"(LOWER(`name`) IN (LOWER('Foo'), LOWER('Bar')) OR `name` IN (5, 6.5) OR `name` IS NULL)",
					"(LOWER(`name`) IN (LOWER(?), LOWER(?)) OR `name` IN (?, ?) OR `name` IS NULL)",
					[]any{"Foo", "Bar", 5, 6.5},
				},
				"sqlite": {
					`(LOWER("name") IN (LOWER('Foo'), LOWER('Bar')) OR "name" IN (5, 6.5) OR "name" IS NULL)`,
					`(LOWER("name") IN (LOWER(?), LOWER(?)) OR "name" IN (?, ?) OR "name" IS NULL)`,
					[]any{"Foo", "Bar", 5, 6.5},
				},
			},
		},
		"numbers_only_in_list_unchanged": {
			input: "name:(5 OR 6)",
			opts:  listed,
			want: map[string]ciWant{
				"postgres": {`"name" IN (5, 6)`, `"name" IN ($1, $2)`, []any{5, 6}},
				"mysql":    {"`name` IN (5, 6)", "`name` IN (?, ?)", []any{5, 6}},
				"sqlite":   {`"name" IN (5, 6)`, `"name" IN (?, ?)`, []any{5, 6}},
			},
		},

		// Numbers are not folded.
		"integer_unchanged": {
			input: "name:5",
			opts:  listed,
			want: map[string]ciWant{
				"postgres": {`"name" = 5`, `"name" = $1`, []any{5}},
				"mysql":    {"`name` = 5", "`name` = ?", []any{5}},
				"sqlite":   {`"name" = 5`, `"name" = ?`, []any{5}},
			},
		},
		"float_unchanged": {
			input: "name:5.5",
			opts:  listed,
			want: map[string]ciWant{
				"postgres": {`"name" = 5.5`, `"name" = $1`, []any{5.5}},
				"mysql":    {"`name` = 5.5", "`name` = ?", []any{5.5}},
				"sqlite":   {`"name" = 5.5`, `"name" = ?`, []any{5.5}},
			},
		},

		// Only listed fields fold.
		"listed_and_unlisted": {
			input: "name:Foo AND other:Bar*",
			opts:  listed,
			want: map[string]ciWant{
				"postgres": {`(LOWER("name") = LOWER('Foo')) AND ("other" SIMILAR TO 'Bar%')`, `(LOWER("name") = LOWER($1)) AND ("other" SIMILAR TO $2)`, []any{"Foo", "Bar%"}},
				"mysql":    {"(LOWER(`name`) = LOWER('Foo')) AND (`other` LIKE 'Bar%' ESCAPE '#')", "(LOWER(`name`) = LOWER(?)) AND (`other` LIKE ? ESCAPE '#')", []any{"Foo", "Bar%"}},
				"sqlite":   {`(LOWER("name") = LOWER('Foo')) AND ("other" GLOB 'Bar*')`, `(LOWER("name") = LOWER(?)) AND ("other" GLOB ?)`, []any{"Foo", "Bar*"}},
			},
		},
		"unlisted_in_list_unchanged": {
			input: "other:(Foo OR Bar)",
			opts:  listed,
			want: map[string]ciWant{
				"postgres": {`"other" IN ('Foo', 'Bar')`, `"other" IN ($1, $2)`, []any{"Foo", "Bar"}},
				"mysql":    {"`other` IN ('Foo', 'Bar')", "`other` IN (?, ?)", []any{"Foo", "Bar"}},
				"sqlite":   {`"other" IN ('Foo', 'Bar')`, `"other" IN (?, ?)`, []any{"Foo", "Bar"}},
			},
		},
		// Field names match exactly.
		"field_name_is_case_sensitive": {
			input: "Name:Foo",
			opts:  listed,
			want: map[string]ciWant{
				"postgres": {`"Name" = 'Foo'`, `"Name" = $1`, []any{"Foo"}},
				"mysql":    {"`Name` = 'Foo'", "`Name` = ?", []any{"Foo"}},
				"sqlite":   {`"Name" = 'Foo'`, `"Name" = ?`, []any{"Foo"}},
			},
		},
		"option_accumulates": {
			input: "name:Foo AND title:Bar",
			opts:  []Opt{WithCaseInsensitiveFields("name"), WithCaseInsensitiveFields("title")},
			want: map[string]ciWant{
				"postgres": {`(LOWER("name") = LOWER('Foo')) AND (LOWER("title") = LOWER('Bar'))`, `(LOWER("name") = LOWER($1)) AND (LOWER("title") = LOWER($2))`, []any{"Foo", "Bar"}},
				"mysql":    {"(LOWER(`name`) = LOWER('Foo')) AND (LOWER(`title`) = LOWER('Bar'))", "(LOWER(`name`) = LOWER(?)) AND (LOWER(`title`) = LOWER(?))", []any{"Foo", "Bar"}},
				"sqlite":   {`(LOWER("name") = LOWER('Foo')) AND (LOWER("title") = LOWER('Bar'))`, `(LOWER("name") = LOWER(?)) AND (LOWER("title") = LOWER(?))`, []any{"Foo", "Bar"}},
			},
		},

		// Left alone on a listed field.
		"regex_unchanged": {
			input: "name:/Fo+/",
			opts:  listed,
			want: map[string]ciWant{
				"postgres": {`"name" ~ 'Fo+'`, `"name" ~ $1`, []any{"Fo+"}},
				"mysql":    {"`name` REGEXP 'Fo+'", "`name` REGEXP ?", []any{"Fo+"}},
				"sqlite":   {`"name" REGEXP 'Fo+'`, `"name" REGEXP ?`, []any{"Fo+"}},
			},
		},
		"range_unchanged": {
			input: "name:[A TO C]",
			opts:  listed,
			want: map[string]ciWant{
				"postgres": {`"name" BETWEEN 'A' AND 'C'`, `"name" BETWEEN $1 AND $2`, []any{"A", "C"}},
				"mysql":    {"`name` BETWEEN 'A' AND 'C'", "`name` BETWEEN ? AND ?", []any{"A", "C"}},
				"sqlite":   {`"name" BETWEEN 'A' AND 'C'`, `"name" BETWEEN ? AND ?`, []any{"A", "C"}},
			},
		},
		"comparison_unchanged": {
			input: "name:>Foo",
			opts:  listed,
			want: map[string]ciWant{
				"postgres": {`"name" > 'Foo'`, `"name" > $1`, []any{"Foo"}},
				"mysql":    {"`name` > 'Foo'", "`name` > ?", []any{"Foo"}},
				"sqlite":   {`"name" > 'Foo'`, `"name" > ?`, []any{"Foo"}},
			},
		},
		"standalone_wild_unchanged": {
			input: "name:*",
			opts:  listed,
			want: map[string]ciWant{
				"postgres": {`"name" SIMILAR TO '%'`, `"name" SIMILAR TO '%'`, nil},
				"mysql":    {"`name` IS NOT NULL", "`name` IS NOT NULL", nil},
				"sqlite":   {`"name" IS NOT NULL`, `"name" IS NOT NULL`, nil},
			},
		},
		"null_unchanged": {
			input: "name:null",
			opts:  listed,
			want: map[string]ciWant{
				"postgres": {`"name" IS NULL`, `"name" IS NULL`, nil},
				"mysql":    {"`name` IS NULL", "`name` IS NULL", nil},
				"sqlite":   {`"name" IS NULL`, `"name" IS NULL`, nil},
			},
		},
		"not_null_unchanged": {
			input: "NOT name:null",
			opts:  listed,
			want: map[string]ciWant{
				"postgres": {`"name" IS NOT NULL`, `"name" IS NOT NULL`, nil},
				"mysql":    {"`name` IS NOT NULL", "`name` IS NOT NULL", nil},
				"sqlite":   {`"name" IS NOT NULL`, `"name" IS NOT NULL`, nil},
			},
		},

		// Default field.
		"listed_default_field_folds_bare_terms": {
			input: `Foo OR "Bar Baz"`,
			opts:  []Opt{WithDefaultField("name"), WithCaseInsensitiveFields("name")},
			want: map[string]ciWant{
				"postgres": {`(LOWER("name") = LOWER('Foo')) OR (LOWER("name") = LOWER('Bar Baz'))`, `(LOWER("name") = LOWER($1)) OR (LOWER("name") = LOWER($2))`, []any{"Foo", "Bar Baz"}},
				"mysql":    {"(LOWER(`name`) = LOWER('Foo')) OR (LOWER(`name`) = LOWER('Bar Baz'))", "(LOWER(`name`) = LOWER(?)) OR (LOWER(`name`) = LOWER(?))", []any{"Foo", "Bar Baz"}},
				"sqlite":   {`(LOWER("name") = LOWER('Foo')) OR (LOWER("name") = LOWER('Bar Baz'))`, `(LOWER("name") = LOWER(?)) OR (LOWER("name") = LOWER(?))`, []any{"Foo", "Bar Baz"}},
			},
		},
		"listed_default_field_single_term": {
			input: "Foo",
			opts:  []Opt{WithDefaultField("name"), WithCaseInsensitiveFields("name")},
			want: map[string]ciWant{
				"postgres": {`LOWER("name") = LOWER('Foo')`, `LOWER("name") = LOWER($1)`, []any{"Foo"}},
				"mysql":    {"LOWER(`name`) = LOWER('Foo')", "LOWER(`name`) = LOWER(?)", []any{"Foo"}},
				"sqlite":   {`LOWER("name") = LOWER('Foo')`, `LOWER("name") = LOWER(?)`, []any{"Foo"}},
			},
		},
		"unlisted_default_field_unchanged": {
			input: "Foo",
			opts:  []Opt{WithDefaultField("other"), WithCaseInsensitiveFields("name")},
			want: map[string]ciWant{
				"postgres": {`"other" = 'Foo'`, `"other" = $1`, []any{"Foo"}},
				"mysql":    {"`other` = 'Foo'", "`other` = ?", []any{"Foo"}},
				"sqlite":   {`"other" = 'Foo'`, `"other" = ?`, []any{"Foo"}},
			},
		},

		// Without the option nothing changes.
		"default_wildcard_unchanged": {
			input: "name:Foo*",
			want: map[string]ciWant{
				"postgres": {`"name" SIMILAR TO 'Foo%'`, `"name" SIMILAR TO $1`, []any{"Foo%"}},
				"mysql":    {"`name` LIKE 'Foo%' ESCAPE '#'", "`name` LIKE ? ESCAPE '#'", []any{"Foo%"}},
				"sqlite":   {`"name" GLOB 'Foo*'`, `"name" GLOB ?`, []any{"Foo*"}},
			},
		},
		"default_equality_unchanged": {
			input: "name:Foo",
			want: map[string]ciWant{
				"postgres": {`"name" = 'Foo'`, `"name" = $1`, []any{"Foo"}},
				"mysql":    {"`name` = 'Foo'", "`name` = ?", []any{"Foo"}},
				"sqlite":   {`"name" = 'Foo'`, `"name" = ?`, []any{"Foo"}},
			},
		},
		"default_in_list_unchanged": {
			input: "name:(Foo OR Bar OR null)",
			want: map[string]ciWant{
				"postgres": {`("name" IN ('Foo', 'Bar') OR "name" IS NULL)`, `("name" IN ($1, $2) OR "name" IS NULL)`, []any{"Foo", "Bar"}},
				"mysql":    {"(`name` IN ('Foo', 'Bar') OR `name` IS NULL)", "(`name` IN (?, ?) OR `name` IS NULL)", []any{"Foo", "Bar"}},
				"sqlite":   {`("name" IN ('Foo', 'Bar') OR "name" IS NULL)`, `("name" IN (?, ?) OR "name" IS NULL)`, []any{"Foo", "Bar"}},
			},
		},
	}

	for name, tc := range tcs {
		for dname, d := range dialects {
			wantKey := dname
			if dname == "mariadb" {
				wantKey = "mysql"
			}
			w, ok := tc.want[wantKey]
			if !ok {
				t.Fatalf("%s: no expectation for %s", name, wantKey)
			}
			t.Run(name+"/"+dname, func(t *testing.T) {
				got, err := d.render(tc.input, tc.opts...)
				if err != nil {
					t.Fatalf("render: %v", err)
				}
				if got != w.str {
					t.Errorf("render\n got: %s\nwant: %s", got, w.str)
				}

				gotStr, gotParams, err := d.renderParam(tc.input, tc.opts...)
				if err != nil {
					t.Fatalf("renderParam: %v", err)
				}
				if gotStr != w.paramStr {
					t.Errorf("renderParam\n got: %s\nwant: %s", gotStr, w.paramStr)
				}
				if len(gotParams) != 0 || len(w.params) != 0 {
					if !reflect.DeepEqual(gotParams, w.params) {
						t.Errorf("params\n got: %#v\nwant: %#v", gotParams, w.params)
					}
				}
			})
		}
	}
}

// TestCaseInsensitiveFieldsLeaveTheASTAlone pins where the option acts: at
// render time, in the ToX functions. Parse returns the same expression with or
// without it, so its String() and JSON forms do not change.
func TestCaseInsensitiveFieldsLeaveTheASTAlone(t *testing.T) {
	for _, input := range []string{"name:Foo", "name:Foo*", "name:(Foo OR Bar)"} {
		want, err := Parse(input)
		if err != nil {
			t.Fatalf("Parse(%q): %v", input, err)
		}
		got, err := Parse(input, WithCaseInsensitiveFields("name"))
		if err != nil {
			t.Fatalf("Parse(%q) with option: %v", input, err)
		}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("Parse(%q) with option\n got: %#v\nwant: %#v", input, got, want)
		}
	}
}
