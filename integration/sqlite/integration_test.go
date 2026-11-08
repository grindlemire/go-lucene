// Package integration runs go-lucene's SQLite driver against a real
// in-memory SQLite database (modernc.org/sqlite, pure Go) to verify that
// the rendered SQL produces the expected result sets, not just the
// expected strings.
//
// This sub-module lives outside the main go-lucene module so the main
// module stays zero-dep. It is wired into the workspace via go.work.
package integration

import (
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"strings"
	"testing"

	lucene "github.com/grindlemire/go-lucene"
	lucenedriver "github.com/grindlemire/go-lucene/pkg/driver"
	"github.com/grindlemire/go-lucene/pkg/lucene/expr"

	// Register the modernc.org/sqlite driver under the name "sqlite".
	_ "modernc.org/sqlite"

	sqlitelib "modernc.org/sqlite"
)

// isIntegrationRun reports whether the INTEGRATION env var is set to a
// truthy value.
func isIntegrationRun() bool {
	switch os.Getenv("INTEGRATION") {
	case "1", "true", "TRUE", "yes", "YES":
		return true
	}
	return false
}

func requireIntegration(t *testing.T) {
	t.Helper()
	if !isIntegrationRun() {
		t.Skip("set INTEGRATION=1 to run integration tests")
	}
}

func init() {
	// Register a SQL function named "regexp" so SQLite's REGEXP operator
	// resolves at runtime. SQLite looks up a user function literally named
	// "regexp" (lowercase) when it sees `column REGEXP pattern`.
	//
	// modernc.org/sqlite registers user functions globally on the driver,
	// so this runs once and applies to every connection.
	sqlitelib.MustRegisterDeterministicScalarFunction(
		"regexp",
		2,
		func(ctx *sqlitelib.FunctionContext, args []driver.Value) (driver.Value, error) {
			pattern, ok := args[0].(string)
			if !ok {
				return false, nil
			}
			value, ok := args[1].(string)
			if !ok {
				return false, nil
			}
			matched, err := regexp.MatchString(pattern, value)
			if err != nil {
				// Treat invalid regex as a non-match rather than a SQL error.
				return false, nil
			}
			return matched, nil
		},
	)
}

func openTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	const schema = `
        CREATE TABLE items (
            id INTEGER PRIMARY KEY,
            name TEXT,
            category TEXT,
            price REAL,
            status TEXT
        );
        INSERT INTO items (id, name, category, price, status) VALUES
            (1, 'apple',    'fruit',  1.50, 'active'),
            (2, 'banana',   'fruit',  0.50, 'active'),
            (3, 'carrot',   'veggie', 0.75, 'inactive'),
            (4, 'durian',   'fruit',  5.00, 'active'),
            (5, 'eggplant', 'veggie', 1.25, NULL);
    `
	if _, err := db.Exec(schema); err != nil {
		t.Fatalf("seed: %v", err)
	}
	return db
}

func queryIDs(t *testing.T, db *sql.DB, where string, args ...any) []int {
	t.Helper()
	rows, err := db.Query("SELECT id FROM items WHERE "+where+" ORDER BY id", args...)
	if err != nil {
		t.Fatalf("query %q: %v", where, err)
	}
	defer rows.Close()
	var out []int
	for rows.Next() {
		var id int
		if err := rows.Scan(&id); err != nil {
			t.Fatalf("scan: %v", err)
		}
		out = append(out, id)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("rows: %v", err)
	}
	return out
}

func equalInts(a, b []int) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestSQLiteIntegrationRendered(t *testing.T) {
	requireIntegration(t)
	db := openTestDB(t)

	cases := []struct {
		name    string
		lucene  string
		wantIDs []int
	}{
		// Fixture rows:
		//   1 apple    fruit  1.50 active
		//   2 banana   fruit  0.50 active
		//   3 carrot   veggie 0.75 inactive
		//   4 durian   fruit  5.00 active
		//   5 eggplant veggie 1.25 NULL
		{"equality", `category:fruit`, []int{1, 2, 4}},
		{"and", `category:fruit AND status:active`, []int{1, 2, 4}},
		{"or", `category:fruit OR category:veggie`, []int{1, 2, 3, 4, 5}},
		{"not", `NOT category:fruit`, []int{3, 5}},
		// inclusive: 1.50 and 1.25 are in [1, 2]
		{"inclusive_range", `price:[1 TO 2]`, []int{1, 5}},
		// exclusive: prices in (0.5, 5.0) -> 0.75, 1.25, 1.50
		{"exclusive_range", `price:{0.5 TO 5.0}`, []int{1, 3, 5}},
		// inclusive lte: prices <= 1 -> 0.50, 0.75
		{"open_ended_lte", `price:[* TO 1]`, []int{2, 3}},
		// inclusive gte: prices >= 2 -> 5.00
		{"open_ended_gte", `price:[2 TO *]`, []int{4}},
		// Exclusive string range: `"name" > 'a' AND "name" < 'd'`.
		// "apple", "banana", "carrot" are in (a, d); "durian" >= 'd', "eggplant" > 'd'.
		{"string_range", `name:{a TO d}`, []int{1, 2, 3}},
		{"in_list", `name:(apple OR banana OR carrot)`, []int{1, 2, 3}},
		{"glob_prefix", `name:a*`, []int{1}},
		{"glob_single_char", `name:?pple`, []int{1}},
		// Critical: standalone-* must render IS NOT NULL, so the row with
		// status = NULL is excluded.
		{"standalone_star_is_not_null", `status:*`, []int{1, 2, 3, 4}},
		{"regexp", `name:/^[ab].*/`, []int{1, 2}},
		// Row 5 has status = NULL.
		{"field_null_returns_null_rows", `status:null`, []int{5}},
		{"not_field_null_returns_non_null_rows", `NOT status:null`, []int{1, 2, 3, 4}},
		{"minus_field_null_returns_non_null_rows", `-status:null`, []int{1, 2, 3, 4}},
		{"or_with_null_returns_both", `status:(active OR null)`, []int{1, 2, 4, 5}},
		{"in_list_with_null", `status:(active OR inactive OR null)`, []int{1, 2, 3, 4, 5}},
		// A field group applies its field to every term inside it.
		{"field_group_must_not", `name:(*an* -banana)`, []int{4, 5}},
		{"field_group_ranges", `price:([1 TO 2] OR [5 TO *])`, []int{1, 4, 5}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			where, err := lucene.ToSQLite(tc.lucene)
			if err != nil {
				t.Fatalf("ToSQLite(%q): %v", tc.lucene, err)
			}
			got := queryIDs(t, db, where)
			if !equalInts(got, tc.wantIDs) {
				t.Fatalf("lucene=%q where=%q\n  want ids %v\n  got  ids %v",
					tc.lucene, where, tc.wantIDs, got)
			}
		})
	}
}

func TestSQLiteIntegrationParameterized(t *testing.T) {
	requireIntegration(t)
	db := openTestDB(t)

	cases := []struct {
		name    string
		lucene  string
		wantIDs []int
	}{
		{"equality", `category:fruit`, []int{1, 2, 4}},
		{"range", `price:[1 TO 2]`, []int{1, 5}},
		{"glob", `name:a*`, []int{1}},
		{"standalone_star", `status:*`, []int{1, 2, 3, 4}},
		{"regexp", `name:/^[ab].*/`, []int{1, 2}},
		{"field_null", `status:null`, []int{5}},
		{"not_field_null", `NOT status:null`, []int{1, 2, 3, 4}},
		{"or_with_null", `status:(active OR null)`, []int{1, 2, 4, 5}},
		{"field_group_must_not", `name:(*an* -banana)`, []int{4, 5}},
		{"field_group_ranges", `price:([1 TO 2] OR [5 TO *])`, []int{1, 4, 5}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			where, params, err := lucene.ToParameterizedSQLite(tc.lucene)
			if err != nil {
				t.Fatalf("ToParameterizedSQLite(%q): %v", tc.lucene, err)
			}
			got := queryIDs(t, db, where, params...)
			if !equalInts(got, tc.wantIDs) {
				t.Fatalf("lucene=%q where=%q params=%v\n  want ids %v\n  got  ids %v",
					tc.lucene, where, params, tc.wantIDs, got)
			}
		})
	}
}

// roundTrip returns e after a trip through JSON.
func roundTrip(t *testing.T, e *expr.Expression) *expr.Expression {
	t.Helper()
	raw, err := json.Marshal(e)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var out expr.Expression
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("unmarshal %s: %v", raw, err)
	}
	return &out
}

// TestSQLiteIntegrationHandBuiltFieldGroup runs EQUALS on a group, built in
// code or read from JSON, which the driver spreads over every term the way
// the parser reads a field group. name = ('apple' OR 'banana') used to match
// no rows.
func TestSQLiteIntegrationHandBuiltFieldGroup(t *testing.T) {
	requireIntegration(t)
	db := openTestDB(t)

	var fromJSON expr.Expression
	if err := json.Unmarshal([]byte(`{"left":"name","operator":"EQUALS","right":{"left":"apple","operator":"OR","right":"banana"}}`), &fromJSON); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	priceRange := func(min, max any) *expr.Expression {
		return &expr.Expression{Op: expr.Range, Right: &expr.RangeBoundary{Min: min, Max: max, Inclusive: true}}
	}

	cases := []struct {
		name    string
		input   *expr.Expression
		wantIDs []int
	}{
		{"must_not", expr.Eq("name", expr.AND(expr.WILD("*an*"), expr.MUSTNOT(expr.Lit("banana")))), []int{4, 5}},
		{"or_null", expr.Eq("status", expr.OR("active", expr.NULL())), []int{1, 2, 4, 5}},
		{"ranges", expr.Eq("price", expr.OR(priceRange(expr.Lit(1), expr.Lit(2)), priceRange(expr.Lit(5), expr.WILD("*")))), []int{1, 4, 5}},
		{"from_json", &fromJSON, []int{1, 2}},
		{"ranges_from_json", roundTrip(t, expr.Eq("price", expr.OR(priceRange(expr.Lit(1), expr.Lit(2)), priceRange(expr.Lit(5), expr.WILD("*"))))), []int{1, 4, 5}},
		// A value with a field of its own keeps it, as name:(category:fruit) does.
		{"own_field", expr.Eq("name", expr.Eq("category", "fruit")), []int{1, 2, 4}},
	}

	d := lucenedriver.NewSQLiteDriver()
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			where, err := d.Render(tc.input)
			if err != nil {
				t.Fatalf("Render: %v", err)
			}
			if got := queryIDs(t, db, where); !equalInts(got, tc.wantIDs) {
				t.Fatalf("where=%q\n  want ids %v\n  got  ids %v", where, tc.wantIDs, got)
			}

			where, params, err := d.RenderParam(tc.input)
			if err != nil {
				t.Fatalf("RenderParam: %v", err)
			}
			if got := queryIDs(t, db, where, params...); !equalInts(got, tc.wantIDs) {
				t.Fatalf("where=%q params=%v\n  want ids %v\n  got  ids %v", where, params, tc.wantIDs, got)
			}
		})
	}
}

func TestSQLiteIntegrationCaseInsensitiveFields(t *testing.T) {
	requireIntegration(t)
	db := openTestDB(t)

	const labels = `
        ALTER TABLE items ADD COLUMN label TEXT;
        UPDATE items SET label = 'FooBar' WHERE id = 1;
        UPDATE items SET label = 'Foo' WHERE id = 2;
        UPDATE items SET label = 'foo' WHERE id = 3;
        UPDATE items SET label = 'BAR' WHERE id = 4;
        INSERT INTO items (id, name, category, price, status, label) VALUES
            (6, 'fig', 'fruit', 2.00, 'active', 'Éclair');
    `
	if _, err := db.Exec(labels); err != nil {
		t.Fatalf("seed labels: %v", err)
	}

	// Row 5 has label = NULL.
	cases := []struct {
		name    string
		lucene  string
		wantIDs []int
	}{
		{"wildcard", `label:foo*`, []int{1, 2, 3}},
		{"wildcard_upper", `label:FOO*`, []int{1, 2, 3}},
		{"single_char", `label:F?O`, []int{2, 3}},
		{"equality", `label:foo`, []int{2, 3}},
		{"equality_upper", `label:FOO`, []int{2, 3}},
		{"phrase", `label:"FOOBAR"`, []int{1}},
		{"in_list", `label:(foo OR bar)`, []int{2, 3, 4}},
		{"in_list_with_null", `label:(foo OR null)`, []int{2, 3, 5}},
		{"mixed_in_list", `label:(foo OR 5)`, []int{2, 3}},
		{"negated_wildcard", `-label:foo*`, []int{4, 6}},
		{"regex_keeps_case", `label:/^Foo/`, []int{1, 2}},
		{"standalone_star", `label:*`, []int{1, 2, 3, 4, 6}},
		{"null", `label:null`, []int{5}},
		{"unlisted_keeps_case", `name:APPLE`, nil},
		// SQLite's lower() folds ASCII letters only.
		{"non_ascii_keeps_case", `label:éclair`, nil},
		// price is listed too, but a number is compared as a number.
		{"number_not_folded", `price:1.5`, []int{1}},
	}

	opt := lucene.WithCaseInsensitiveFields("label", "price")
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			where, err := lucene.ToSQLite(tc.lucene, opt)
			if err != nil {
				t.Fatalf("ToSQLite(%q): %v", tc.lucene, err)
			}
			if got := queryIDs(t, db, where); !equalInts(got, tc.wantIDs) {
				t.Fatalf("lucene=%q where=%q\n  want ids %v\n  got  ids %v",
					tc.lucene, where, tc.wantIDs, got)
			}

			where, params, err := lucene.ToParameterizedSQLite(tc.lucene, opt)
			if err != nil {
				t.Fatalf("ToParameterizedSQLite(%q): %v", tc.lucene, err)
			}
			if got := queryIDs(t, db, where, params...); !equalInts(got, tc.wantIDs) {
				t.Fatalf("lucene=%q where=%q params=%v\n  want ids %v\n  got  ids %v",
					tc.lucene, where, params, tc.wantIDs, got)
			}
		})
	}

	t.Run("listed_default_field", func(t *testing.T) {
		where, params, err := lucene.ToParameterizedSQLite(`FOO`, opt, lucene.WithDefaultField("label"))
		if err != nil {
			t.Fatalf("ToParameterizedSQLite: %v", err)
		}
		if got, want := queryIDs(t, db, where, params...), []int{2, 3}; !equalInts(got, want) {
			t.Fatalf("where=%q params=%v\n  want ids %v\n  got  ids %v", where, params, want, got)
		}
	})
}

// escapedWildcardCases match rows whose names hold the characters a wildcard
// pattern gives a meaning to. An escaped character must match only itself.
var escapedWildcardCases = []struct {
	name    string
	lucene  string
	wantIDs []int
}{
	{"escaped_star_term", `name:a\*b`, []int{11}},
	{"escaped_star", `name:a\**`, []int{11}},
	{"escaped_star_inside", `name:*\*b`, []int{11}},
	{"escaped_question_term", `name:a\?b`, []int{13}},
	{"escaped_question", `name:a\?*`, []int{13}},
	{"percent", `name:a%*`, []int{14}},
	{"underscore", `name:a_*`, []int{15}},
	{"escaped_bracket", `name:a\[*`, []int{16}},
	{"escaped_close_bracket", `name:a\]*`, []int{19}},
	{"escaped_backslash", `name:a\\*`, []int{17}},
	{"escaped_backslash_term", `name:a\\b`, []int{17}},
	{"hash", `name:a#*`, []int{18}},
	{"dot", `name:a.*`, []int{20}},
	{"escaped_space", `name:a\ *`, []int{21}},
	{"escaped_paren", `name:a\(*`, []int{22}},
	{"escaped_pipe", `name:a\|*`, []int{23}},
	{"escaped_plus", `name:a\+*`, []int{24}},
	{"escaped_brace", `name:a\{*`, []int{25}},
	{"single_char", `name:a?b`, []int{11, 12, 13, 14, 15, 16, 17, 18, 19, 20, 21, 22, 23, 24, 25, 27, 28}},
}

func TestSQLiteIntegrationEscapedWildcards(t *testing.T) {
	requireIntegration(t)
	db := openTestDB(t)

	const rows = `
        INSERT INTO items (id, name) VALUES
            (11, 'a*b'), (12, 'axb'), (13, 'a?b'), (14, 'a%b'), (15, 'a_b'),
            (16, 'a[b'), (17, 'a\b'), (18, 'a#b'), (19, 'a]b'), (20, 'a.b'),
            (21, 'a b'), (22, 'a(b'), (23, 'a|b'), (24, 'a+b'), (25, 'a{b'),
            (26, 'ab'), (27, 'a^b'), (28, 'a-b');
    `
	if _, err := db.Exec(rows); err != nil {
		t.Fatalf("seed: %v", err)
	}

	for _, tc := range escapedWildcardCases {
		t.Run(tc.name, func(t *testing.T) {
			where, err := lucene.ToSQLite(tc.lucene)
			if err != nil {
				t.Fatalf("ToSQLite(%q): %v", tc.lucene, err)
			}
			if got := queryIDs(t, db, where); !equalInts(got, tc.wantIDs) {
				t.Fatalf("lucene=%q where=%q\n  want ids %v\n  got  ids %v",
					tc.lucene, where, tc.wantIDs, got)
			}

			where, params, err := lucene.ToParameterizedSQLite(tc.lucene)
			if err != nil {
				t.Fatalf("ToParameterizedSQLite(%q): %v", tc.lucene, err)
			}
			if got := queryIDs(t, db, where, params...); !equalInts(got, tc.wantIDs) {
				t.Fatalf("lucene=%q where=%q params=%v\n  want ids %v\n  got  ids %v",
					tc.lucene, where, params, tc.wantIDs, got)
			}
		})
	}

	// expr.EscapeWildcard turns any text into a pattern that matches only
	// that text, and only text that starts with it once a * follows.
	for id, name := range seededNames(t, db) {
		t.Run(fmt.Sprintf("escape_wildcard_%d", id), func(t *testing.T) {
			for _, tc := range []struct {
				pattern string
				want    []int
			}{
				{expr.EscapeWildcard(name), []int{id}},
				{expr.EscapeWildcard(name) + "*", []int{id}},
			} {
				e := expr.LIKE("name", tc.pattern)
				where, params, err := lucenedriver.NewSQLiteDriver().RenderParam(e)
				if err != nil {
					t.Fatalf("render param: %v", err)
				}
				if got := queryIDs(t, db, where, params...); !equalInts(got, tc.want) {
					t.Fatalf("name=%q where=%s params=%v\n  want ids %v\n  got  ids %v", name, where, params, tc.want, got)
				}
				where, err = lucenedriver.NewSQLiteDriver().Render(e)
				if err != nil {
					t.Fatalf("render: %v", err)
				}
				if got := queryIDs(t, db, where); !equalInts(got, tc.want) {
					t.Fatalf("name=%q where=%s\n  want ids %v\n  got  ids %v", name, where, tc.want, got)
				}
			}
		})
	}

	// A pattern built in code can hold a GLOB bracket expression. Inside it
	// * ? [ and \ are members of the set, escaped or not.
	drv := lucenedriver.NewSQLiteDriver()
	brackets := []struct {
		name    string
		pattern string
		wantIDs []int
	}{
		{"bracket_wildcard_chars", `a[*?]b`, []int{11, 13}},
		{"bracket_escaped_wildcard_chars", `a[\*\?]b`, []int{11, 13}},
		{"bracket_escaped_open", `a[\[x]b`, []int{12, 16}},
		{"bracket_escaped_backslash", `a[\\x]b`, []int{12, 17}},
	}
	for _, tc := range brackets {
		t.Run(tc.name, func(t *testing.T) {
			e := expr.LIKE("name", tc.pattern)
			where, err := drv.Render(e)
			if err != nil {
				t.Fatalf("render: %v", err)
			}
			if got := queryIDs(t, db, where); !equalInts(got, tc.wantIDs) {
				t.Fatalf("pattern=%q where=%s\n  want ids %v\n  got  ids %v", tc.pattern, where, tc.wantIDs, got)
			}
			where, params, err := drv.RenderParam(e)
			if err != nil {
				t.Fatalf("render param: %v", err)
			}
			if got := queryIDs(t, db, where, params...); !equalInts(got, tc.wantIDs) {
				t.Fatalf("pattern=%q where=%s params=%v\n  want ids %v\n  got  ids %v", tc.pattern, where, params, tc.wantIDs, got)
			}
		})
	}
}

// seededNames returns the names of the rows TestSQLiteIntegrationEscapedWildcards
// adds, by id.
func seededNames(t *testing.T, db *sql.DB) map[int]string {
	t.Helper()
	rows, err := db.Query("SELECT id, name FROM items WHERE id >= 11")
	if err != nil {
		t.Fatalf("query names: %v", err)
	}
	defer rows.Close()
	names := map[int]string{}
	for rows.Next() {
		var id int
		var name string
		if err := rows.Scan(&id, &name); err != nil {
			t.Fatalf("scan: %v", err)
		}
		names[id] = name
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("rows: %v", err)
	}
	return names
}

func TestSQLiteIntegrationJSONPath(t *testing.T) {
	requireIntegration(t)
	db := openTestDB(t)

	const labels = `
        ALTER TABLE items ADD COLUMN labels TEXT;
        UPDATE items SET labels = '{"color": "red", "stock": {"count": 10}, "it''s": "yes", "n": 10, "v": "2"}' WHERE id = 1;
        UPDATE items SET labels = '{"color": "yellow", "stock": {"count": 3}, "n": "abc", "v": 2}' WHERE id = 2;
        UPDATE items SET labels = '{"color": "orange", "stock": {"count": 25}, "n": null, "v": 2.0}' WHERE id = 3;
        UPDATE items SET labels = '{"color": "green", "n": true, "v": "abc"}' WHERE id = 4;
    `
	if _, err := db.Exec(labels); err != nil {
		t.Fatalf("seed labels: %v", err)
	}

	// Row 5 has labels = NULL; row 4 has no stock key.
	cases := []struct {
		name    string
		lucene  string
		wantIDs []int
	}{
		{"equality", `labels.color:red`, []int{1}},
		{"nested_greater", `labels.stock.count:>5`, []int{1, 3}},
		{"nested_range", `labels.stock.count:[3 TO 10]`, []int{1, 2}},
		{"nested_in_numbers", `labels.stock.count:(3 OR 25)`, []int{2, 3}},
		{"in_list", `labels.color:(red OR yellow)`, []int{1, 2}},
		{"glob", `labels.color:ye*`, []int{2}},
		{"null", `labels.stock.count:null`, []int{4, 5}},
		{"not_null", `NOT labels.stock.count:null`, []int{1, 2, 3}},
		{"in_list_with_null", `labels.color:(red OR null)`, []int{1, 5}},
		{"standalone_star", `labels.color:*`, []int{1, 2, 3, 4}},
		{"quote_in_key", `labels.it\'s:yes`, []int{1}},
		{"injection_in_key", `labels.x\'\ OR\ \'a\'\ \=\ \'a:z`, nil},
		{"with_plain_column", `category:fruit AND labels.color:yellow`, []int{2}},
		// n is 10, "abc", JSON null and true in rows 1-4.
		{"numeric_skips_text", `labels.n:>5`, []int{1}},
		{"numeric_zero_skips_text", `labels.n:0`, nil},
		{"numeric_less_skips_text", `labels.n:<100`, []int{1}},
		{"json_null", `labels.n:null`, []int{3, 5}},
		{"not_json_null", `NOT labels.n:null`, []int{1, 2, 4}},
		{"numeric_in_with_null", `labels.n:(10 OR null)`, []int{1, 3, 5}},
		{"text_value", `labels.n:abc`, []int{2}},
		{"not_numeric_keeps_non_numbers", `-labels.n:>5`, []int{2, 4}},
		{"not_equal_number", `-labels.n:7`, []int{1, 2, 4}},
		// v is "2", 2, 2.0 and "abc" in rows 1-4.
		{"number_matches_text_form", `labels.v:2`, []int{1, 2, 3}},
		{"not_number_or_text_form", `-labels.v:2`, []int{4}},
		{"in_numbers_match_text_form", `labels.v:(2 OR 7)`, []int{1, 2, 3}},
		{"greater_skips_text_form", `labels.v:>1`, []int{2, 3}},
		{"mixed_in_compares_text", `labels.v:(2 OR abc)`, []int{1, 2, 4}},
		{"mixed_range_compares_text", `labels.v:{2 TO abc}`, []int{3}},
		{"quoted_number_matches_number", `labels.stock.count:"10"`, []int{1}},
		{"bool_text", `labels.n:true`, []int{4}},
		{"written_decimal_keeps_text", `labels.v:2.0`, []int{2, 3}},
		{"leading_zero_keeps_text", `labels.v:02`, []int{2, 3}},
		{"not_plain_text_skips_json_null", `-labels.n:abc`, []int{1, 4}},
		{"plain_text_in_list", `labels.color:(red OR green)`, []int{1, 4}},
	}

	opt := lucene.WithJSONFields("labels")
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			where, err := lucene.ToSQLite(tc.lucene, opt)
			if err != nil {
				t.Fatalf("ToSQLite(%q): %v", tc.lucene, err)
			}
			if got := queryIDs(t, db, where); !equalInts(got, tc.wantIDs) {
				t.Fatalf("lucene=%q where=%q\n  want ids %v\n  got  ids %v",
					tc.lucene, where, tc.wantIDs, got)
			}

			where, params, err := lucene.ToParameterizedSQLite(tc.lucene, opt)
			if err != nil {
				t.Fatalf("ToParameterizedSQLite(%q): %v", tc.lucene, err)
			}
			if got := queryIDs(t, db, where, params...); !equalInts(got, tc.wantIDs) {
				t.Fatalf("lucene=%q where=%q params=%v\n  want ids %v\n  got  ids %v",
					tc.lucene, where, params, tc.wantIDs, got)
			}
		})
	}
	// labels.color is listed by its dotted name, so it folds case.
	folded := []lucene.Opt{opt, lucene.WithCaseInsensitiveFields("labels.color")}
	for _, tc := range []struct {
		lucene  string
		wantIDs []int
	}{
		{`labels.color:RED`, []int{1}},
		{`labels.color:(RED OR Green)`, []int{1, 4}},
	} {
		t.Run("folded_"+tc.lucene, func(t *testing.T) {
			where, err := lucene.ToSQLite(tc.lucene, folded...)
			if err != nil {
				t.Fatalf("ToSQLite(%q): %v", tc.lucene, err)
			}
			if got := queryIDs(t, db, where); !equalInts(got, tc.wantIDs) {
				t.Fatalf("lucene=%q where=%q\n  want ids %v\n  got  ids %v", tc.lucene, where, tc.wantIDs, got)
			}
			where, params, err := lucene.ToParameterizedSQLite(tc.lucene, folded...)
			if err != nil {
				t.Fatalf("ToParameterizedSQLite(%q): %v", tc.lucene, err)
			}
			if got := queryIDs(t, db, where, params...); !equalInts(got, tc.wantIDs) {
				t.Fatalf("lucene=%q where=%q params=%v\n  want ids %v\n  got  ids %v", tc.lucene, where, params, tc.wantIDs, got)
			}
		})
	}

	// A hand-built bool compares as the text true or false.
	n := expr.JSONPath{Column: "labels", Path: []string{"n"}}
	d := lucenedriver.NewSQLiteDriver()
	for name, tc := range map[string]struct {
		e       *expr.Expression
		wantIDs []int
	}{
		"bool_true":  {expr.Eq(n, true), []int{4}},
		"bool_false": {expr.Eq(n, false), nil},
		"bool_in":    {expr.IN(n, expr.LIST(expr.Lit(true), expr.Lit(false))), []int{4}},
	} {
		t.Run(name, func(t *testing.T) {
			where, err := d.Render(tc.e)
			if err != nil {
				t.Fatalf("render: %v", err)
			}
			if got := queryIDs(t, db, where); !equalInts(got, tc.wantIDs) {
				t.Fatalf("where=%q\n  want ids %v\n  got  ids %v", where, tc.wantIDs, got)
			}
			where, params, err := d.RenderParam(tc.e)
			if err != nil {
				t.Fatalf("render param: %v", err)
			}
			if got := queryIDs(t, db, where, params...); !equalInts(got, tc.wantIDs) {
				t.Fatalf("where=%q params=%v\n  want ids %v\n  got  ids %v", where, params, tc.wantIDs, got)
			}
		})
	}
}

// TestSQLiteIntegrationJSONPathUsesIndex checks that equality with plain text
// compares the bare json_extract, so an index on that expression applies.
func TestSQLiteIntegrationJSONPathUsesIndex(t *testing.T) {
	requireIntegration(t)
	db := openTestDB(t)

	const schema = `
        ALTER TABLE items ADD COLUMN labels TEXT;
        CREATE INDEX items_labels_color ON items (json_extract(labels, '$.color'));
    `
	if _, err := db.Exec(schema); err != nil {
		t.Fatalf("schema: %v", err)
	}

	opt := lucene.WithJSONFields("labels")
	for _, q := range []string{`labels.color:red`, `labels.color:(red OR green)`} {
		where, err := lucene.ToSQLite(q, opt)
		if err != nil {
			t.Fatalf("ToSQLite(%q): %v", q, err)
		}
		rows, err := db.Query("EXPLAIN QUERY PLAN SELECT id FROM items WHERE " + where)
		if err != nil {
			t.Fatalf("explain %q: %v", where, err)
		}
		var plan []string
		for rows.Next() {
			var id, parent, notused int
			var detail string
			if err := rows.Scan(&id, &parent, &notused, &detail); err != nil {
				t.Fatal(err)
			}
			plan = append(plan, detail)
		}
		rows.Close()
		if joined := strings.Join(plan, "; "); !strings.HasPrefix(joined, "SEARCH") || !strings.Contains(joined, "INDEX items_labels_color") {
			t.Errorf("lucene=%q where=%q: want a search on the index, got plan %q", q, where, joined)
		}
	}
}
