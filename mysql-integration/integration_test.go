// Package integration runs go-lucene's MySQL driver against a real MySQL
// instance (spun up via testcontainers-go) to verify that the rendered SQL
// produces the expected result sets, not just the expected strings.
//
// This sub-module lives outside the main go-lucene module so the main
// module stays zero-dep. It is wired into the workspace via go.work.
//
// The MYSQL_IMAGE environment variable selects which MySQL version to run
// against. Defaults to "mysql:8.0". Set to "mysql:5.7" (Henry Spencer POSIX
// regex) to exercise the older regex engine — some Perl-style escapes
// (\d, \w, \s) don't work on 5.7.
//
// If Docker is not available the tests skip rather than failing.
package integration

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	_ "github.com/go-sql-driver/mysql"
	lucene "github.com/grindlemire/go-lucene"
	mysqlcontainer "github.com/testcontainers/testcontainers-go/modules/mysql"
)

func mysqlImage() string {
	if img := os.Getenv("MYSQL_IMAGE"); img != "" {
		return img
	}
	return "mysql:8.0"
}

func is57() bool {
	return strings.HasPrefix(mysqlImage(), "mysql:5.7")
}

// openTestDB spins up a MySQL container, seeds a fixture table, and returns
// an *sql.DB connected to it. Skips the test if Docker is unavailable.
func openTestDB(t *testing.T) *sql.DB {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	t.Cleanup(cancel)

	container, err := mysqlcontainer.Run(ctx, mysqlImage(),
		mysqlcontainer.WithDatabase("testdb"),
		mysqlcontainer.WithUsername("root"),
		mysqlcontainer.WithPassword("rootpw"),
	)
	if err != nil {
		// testcontainers returns a wrapped error when the container runtime
		// is unreachable. Cover Docker and Podman variants + generic
		// connection-refused cases so CI/dev machines without a container
		// runtime skip gracefully instead of failing.
		msg := err.Error()
		switch {
		case strings.Contains(msg, "Cannot connect to the Docker daemon"),
			strings.Contains(msg, "docker: not found"),
			strings.Contains(msg, "rootless Docker not found"),
			strings.Contains(msg, "Cannot connect to Podman"),
			strings.Contains(msg, "podman: not found"),
			strings.Contains(msg, "connect: connection refused"),
			strings.Contains(msg, "no such file or directory"):
			t.Skipf("container runtime not available, skipping integration test: %v", err)
		}
		t.Fatalf("start MySQL container: %v", err)
	}
	t.Cleanup(func() {
		termCtx, termCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer termCancel()
		if err := container.Terminate(termCtx); err != nil {
			t.Logf("terminate container: %v", err)
		}
	})

	dsn, err := container.ConnectionString(ctx, "parseTime=true", "multiStatements=true")
	if err != nil {
		t.Fatalf("connection string: %v", err)
	}

	db, err := sql.Open("mysql", dsn)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	if err := db.PingContext(ctx); err != nil {
		t.Fatalf("ping: %v", err)
	}

	// The column name `rank` is a reserved word in MySQL 8.0 (it wasn't in
	// 5.7). Including it in the schema exercises the always-backtick policy.
	const schema = `
		CREATE TABLE items (
			id     INT PRIMARY KEY,
			name   VARCHAR(64),
			category VARCHAR(64),
			price  DECIMAL(10,2),
			status VARCHAR(32),
			` + "`rank`" + ` INT
		);
		INSERT INTO items (id, name, category, price, status, ` + "`rank`" + `) VALUES
			(1, 'apple',    'fruit',  1.50, 'active',   10),
			(2, 'banana',   'fruit',  0.50, 'active',   20),
			(3, 'carrot',   'veggie', 0.75, 'inactive', 30),
			(4, 'durian',   'fruit',  5.00, 'active',   40),
			(5, 'eggplant', 'veggie', 1.25, NULL,       50),
			(6, '100% off', 'promo',  0.00, 'active',   60),
			(7, 'foo_bar',  'promo',  0.00, 'active',   70);
	`
	if _, err := db.ExecContext(ctx, schema); err != nil {
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

type integrationCase struct {
	name    string
	lucene  string
	wantIDs []int
	skip57  bool // skip on MySQL 5.7 (usually ICU-only regex features)
}

var renderedCases = []integrationCase{
	// Fixture rows:
	//   1 apple     fruit  1.50 active    10
	//   2 banana    fruit  0.50 active    20
	//   3 carrot    veggie 0.75 inactive  30
	//   4 durian    fruit  5.00 active    40
	//   5 eggplant  veggie 1.25 NULL      50
	//   6 '100% off' promo  0.00 active   60
	//   7 foo_bar   promo  0.00 active    70
	{name: "equality", lucene: `category:fruit`, wantIDs: []int{1, 2, 4}},
	{name: "and", lucene: `category:fruit AND status:active`, wantIDs: []int{1, 2, 4}},
	{name: "or", lucene: `category:fruit OR category:veggie`, wantIDs: []int{1, 2, 3, 4, 5}},
	{name: "not", lucene: `NOT category:fruit`, wantIDs: []int{3, 5, 6, 7}},
	{name: "inclusive_range", lucene: `price:[1 TO 2]`, wantIDs: []int{1, 5}},
	{name: "exclusive_range", lucene: `price:{0.5 TO 5.0}`, wantIDs: []int{1, 3, 5}},
	{name: "open_ended_lte", lucene: `price:[* TO 1]`, wantIDs: []int{2, 3, 6, 7}},
	{name: "open_ended_gte", lucene: `price:[2 TO *]`, wantIDs: []int{4}},
	{name: "string_range", lucene: `name:{a TO d}`, wantIDs: []int{1, 2, 3, 6, 7}},
	{name: "in_list", lucene: `name:(apple OR banana OR carrot)`, wantIDs: []int{1, 2, 3}},
	{name: "like_prefix", lucene: `name:a*`, wantIDs: []int{1}},
	{name: "like_single_char", lucene: `name:?pple`, wantIDs: []int{1}},
	// The `#` escape means the literal `%` stays literal. Only row 6
	// ("100% off") matches "100%*" which translates to "100#%%".
	{name: "like_literal_percent", lucene: `name:100%*`, wantIDs: []int{6}},
	// Similarly, literal `_` must stay literal. Only row 7 matches
	// "foo_bar" → "foo#_bar%".
	{name: "like_literal_underscore", lucene: `name:foo_bar*`, wantIDs: []int{7}},
	// Standalone * must render IS NOT NULL — the NULL-status row is excluded.
	{name: "standalone_star_is_not_null", lucene: `status:*`, wantIDs: []int{1, 2, 3, 4, 6, 7}},
	// Regex literal: explicit /regex/ syntax. POSIX bracket class works on
	// both 5.7 and 8.0.
	{name: "regexp_literal", lucene: `name:/^[ab].*/`, wantIDs: []int{1, 2}},
	// Wildcard pattern containing a character class `[ab]` forces the
	// LIKE→REGEXP fallback (pkg/driver/mysql.go:luceneWildcardToRegex).
	// Exercises the anchored ^(...)$ wrapper on both 8.0 (ICU) and 5.7
	// (Henry Spencer POSIX) — the capturing group form works on both
	// engines; non-capturing `(?:...)` would not be guaranteed on 5.7.
	// Lucene requires escaping `[` and `]` in a value position.
	{name: "like_fallback_char_class", lucene: `name:\[ab\]*`, wantIDs: []int{1, 2}},
	// Reserved-word column name: exercises always-backtick policy.
	// `rank` is a reserved word in MySQL 8.0.
	{name: "reserved_word_column", lucene: `rank:>=50`, wantIDs: []int{5, 6, 7}},
}

var parameterizedCases = []integrationCase{
	{name: "equality", lucene: `category:fruit`, wantIDs: []int{1, 2, 4}},
	{name: "range", lucene: `price:[1 TO 2]`, wantIDs: []int{1, 5}},
	{name: "like_prefix", lucene: `name:a*`, wantIDs: []int{1}},
	{name: "like_literal_percent", lucene: `name:100%*`, wantIDs: []int{6}},
	{name: "standalone_star", lucene: `status:*`, wantIDs: []int{1, 2, 3, 4, 6, 7}},
	{name: "regexp_literal", lucene: `name:/^[ab].*/`, wantIDs: []int{1, 2}},
	{name: "reserved_word_column", lucene: `rank:>=50`, wantIDs: []int{5, 6, 7}},
}

func TestMySQLIntegrationRendered(t *testing.T) {
	db := openTestDB(t)

	for _, tc := range renderedCases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.skip57 && is57() {
				t.Skipf("skipping on %s", mysqlImage())
			}
			where, err := lucene.ToMySQL(tc.lucene)
			if err != nil {
				t.Fatalf("ToMySQL(%q): %v", tc.lucene, err)
			}
			got := queryIDs(t, db, where)
			if !equalInts(got, tc.wantIDs) {
				t.Fatalf("lucene=%q where=%q\n  want ids %v\n  got  ids %v",
					tc.lucene, where, tc.wantIDs, got)
			}
		})
	}
}

func TestMySQLIntegrationParameterized(t *testing.T) {
	db := openTestDB(t)

	for _, tc := range parameterizedCases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.skip57 && is57() {
				t.Skipf("skipping on %s", mysqlImage())
			}
			where, params, err := lucene.ToParameterizedMySQL(tc.lucene)
			if err != nil {
				t.Fatalf("ToParameterizedMySQL(%q): %v", tc.lucene, err)
			}
			got := queryIDs(t, db, where, params...)
			if !equalInts(got, tc.wantIDs) {
				t.Fatalf("lucene=%q where=%q params=%v\n  want ids %v\n  got  ids %v",
					tc.lucene, where, params, tc.wantIDs, got)
			}
		})
	}
}

// Guard against accidental removal of the ? placeholder contract: the
// parameterized MySQL renderer must emit ?, not $N.
func TestMySQLParameterPlaceholderIsQuestionMark(t *testing.T) {
	where, params, err := lucene.ToParameterizedMySQL(`a:b`)
	if err != nil {
		t.Fatalf("ToParameterizedMySQL: %v", err)
	}
	const wantStr = "`a` = ?"
	if where != wantStr {
		t.Fatalf("want %q, got %q", wantStr, where)
	}
	if len(params) != 1 || params[0] != "b" {
		t.Fatalf("want params [\"b\"], got %v", params)
	}
	if strings.Contains(where, fmt.Sprintf("$%d", 1)) {
		t.Fatalf("unexpected $N placeholder in MySQL output: %q", where)
	}
}
