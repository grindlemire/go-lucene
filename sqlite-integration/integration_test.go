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
	"regexp"
	"testing"

	lucene "github.com/grindlemire/go-lucene"

	// Register the modernc.org/sqlite driver under the name "sqlite".
	_ "modernc.org/sqlite"

	sqlitelib "modernc.org/sqlite"
)

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
		// String ranges render as BETWEEN regardless of inclusive/exclusive,
		// so this is `"name" BETWEEN 'a' AND 'd'`. "apple", "banana", "carrot"
		// all match; "durian" is lexicographically > 'd' and "eggplant" > 'd'.
		{"string_range", `name:{a TO d}`, []int{1, 2, 3}},
		{"in_list", `name:(apple OR banana OR carrot)`, []int{1, 2, 3}},
		{"glob_prefix", `name:a*`, []int{1}},
		{"glob_single_char", `name:?pple`, []int{1}},
		// Critical: standalone-* must render IS NOT NULL, so the row with
		// status = NULL is excluded.
		{"standalone_star_is_not_null", `status:*`, []int{1, 2, 3, 4}},
		{"regexp", `name:/^[ab].*/`, []int{1, 2}},
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
