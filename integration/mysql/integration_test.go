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
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"testing"
	"time"

	_ "github.com/go-sql-driver/mysql"
	lucene "github.com/grindlemire/go-lucene"
	"github.com/grindlemire/go-lucene/pkg/driver"
	"github.com/grindlemire/go-lucene/pkg/lucene/expr"
	"github.com/testcontainers/testcontainers-go"
	mysqlcontainer "github.com/testcontainers/testcontainers-go/modules/mysql"
	"github.com/testcontainers/testcontainers-go/wait"
)

// TestMain configures the container runtime endpoint before any test runs,
// so `go test ./...` just works without the caller having to export
// DOCKER_HOST or start a container machine by hand. The logic is:
//
//  1. If DOCKER_HOST is already set, respect it.
//  2. Probe common Docker socket paths (Linux, Docker Desktop on macOS).
//     If one is reachable, point DOCKER_HOST at it.
//  3. Otherwise probe Podman. If a machine is configured but stopped,
//     start it, then point DOCKER_HOST at its socket.
//  4. If none of the above work, do nothing — openTestDB will skip
//     individual tests cleanly with its existing runtime-unavailable check.
//
// TESTCONTAINERS_RYUK_DISABLED is set unconditionally. Ryuk is the
// testcontainers cleanup sidecar; it breaks under rootless Podman and is
// redundant here because the tests terminate their containers via
// t.Cleanup. Disabling it is a no-op under Docker.
func TestMain(m *testing.M) {
	// Skip expensive runtime probing (and potentially starting a Podman
	// machine) unless the caller actually asked for integration tests.
	if isIntegrationRun() {
		configureContainerRuntime()
	}
	code := m.Run()
	if isIntegrationRun() {
		// Belt-and-suspenders: mysqlcontainer.Run discards its container
		// reference when the wait strategy fails (modules/mysql/mysql.go:88-91
		// in v0.33.0), so our per-test t.Cleanup never sees it. Ryuk is
		// disabled, so nothing cleans these up. Sweep by session label.
		sweepLeakedContainers()
	}
	os.Exit(code)
}

// chosenRuntimeCLI records the runtime configureContainerRuntime pointed at
// (for example "docker" or "podman") so sweepLeakedContainers queries the
// same one. Without this, a machine with both CLIs on PATH could hit the
// wrong daemon — e.g., docker CLI present but DOCKER_HOST set to Podman —
// and the sweep would silently find nothing.
var chosenRuntimeCLI string

// sweepLeakedContainers finds any container tagged with this test run's
// testcontainers session ID and force-removes it. No-op if no runtime CLI
// is reachable or if no containers match.
func sweepLeakedContainers() {
	sid := testcontainers.SessionID()
	if sid == "" {
		return
	}
	cli := chosenRuntimeCLI
	if cli != "" {
		if _, err := exec.LookPath(cli); err != nil {
			cli = ""
		}
	}
	if cli == "" {
		for _, c := range []string{"docker", "podman"} {
			if _, err := exec.LookPath(c); err == nil {
				cli = c
				break
			}
		}
	}
	if cli == "" {
		return
	}
	out, err := exec.Command(cli, "ps", "-aq",
		"--filter", "label=org.testcontainers.session-id="+sid).Output()
	if err != nil {
		return
	}
	ids := strings.Fields(string(out))
	if len(ids) == 0 {
		return
	}
	fmt.Fprintf(os.Stderr, "sweeping %d leaked container(s) from session %s\n", len(ids), sid)
	_ = exec.Command(cli, append([]string{"rm", "-f"}, ids...)...).Run()
}

// isIntegrationRun reports whether the INTEGRATION env var is set to a
// truthy value. Individual tests gate on this too so they skip cleanly
// when run as part of `go test ./...` across the workspace.
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

func configureContainerRuntime() {
	if os.Getenv("TESTCONTAINERS_RYUK_DISABLED") == "" {
		os.Setenv("TESTCONTAINERS_RYUK_DISABLED", "true")
	}
	// mysql:5.7 has no arm64 image on Docker Hub, so on Apple Silicon
	// (or any non-amd64 host) we have to force amd64 and rely on the
	// runtime's emulation layer (Rosetta under Podman Desktop, QEMU
	// elsewhere). Harmless on amd64 hosts.
	if is57() && runtime.GOARCH != "amd64" && os.Getenv("DOCKER_DEFAULT_PLATFORM") == "" {
		os.Setenv("DOCKER_DEFAULT_PLATFORM", "linux/amd64")
	}
	if os.Getenv("DOCKER_HOST") != "" {
		return
	}
	if sock, ok := findDockerSocket(); ok {
		os.Setenv("DOCKER_HOST", "unix://"+sock)
		chosenRuntimeCLI = "docker"
		return
	}
	if sock, err := podmanSocket(); err == nil {
		os.Setenv("DOCKER_HOST", "unix://"+sock)
		chosenRuntimeCLI = "podman"
	}
}

// findDockerSocket probes the usual locations in order. /var/run/docker.sock
// covers Linux and newer Docker Desktop versions (which symlink from
// ~/.docker/run/docker.sock). The $HOME paths cover older Docker Desktop
// installs that don't create the /var/run symlink for rootless/non-admin
// users.
func findDockerSocket() (string, bool) {
	candidates := []string{"/var/run/docker.sock"}
	if home, err := os.UserHomeDir(); err == nil {
		candidates = append(candidates,
			home+"/.docker/run/docker.sock",
			home+"/.docker/desktop/docker.sock",
		)
	}
	for _, p := range candidates {
		if _, err := os.Stat(p); err == nil {
			return p, true
		}
	}
	return "", false
}

func podmanSocket() (string, error) {
	if _, err := exec.LookPath("podman"); err != nil {
		return "", err
	}
	sockPath := func() (string, error) {
		out, err := exec.Command("podman", "machine", "inspect",
			"--format", "{{.ConnectionInfo.PodmanSocket.Path}}").Output()
		if err != nil {
			return "", err
		}
		for _, line := range strings.Split(string(out), "\n") {
			if s := strings.TrimSpace(line); s != "" {
				return s, nil
			}
		}
		return "", fmt.Errorf("no podman machine configured")
	}
	p, err := sockPath()
	if err != nil {
		return "", err
	}
	if _, err := os.Stat(p); err == nil {
		return p, nil
	}
	// Socket missing means the machine is stopped. Start it and re-check.
	// This adds ~30s to the first run but removes a manual step.
	fmt.Fprintln(os.Stderr, "podman machine not running; starting (this takes ~30s)...")
	if err := exec.Command("podman", "machine", "start").Run(); err != nil {
		return "", err
	}
	p, err = sockPath()
	if err != nil {
		return "", err
	}
	if _, err := os.Stat(p); err != nil {
		return "", err
	}
	return p, nil
}

func mysqlImage() string {
	if img := os.Getenv("MYSQL_IMAGE"); img != "" {
		return img
	}
	return "mysql:8.0"
}

func is57() bool {
	return strings.HasPrefix(mysqlImage(), "mysql:5.7")
}

func isMariaDB() bool {
	return strings.HasPrefix(mysqlImage(), "mariadb:")
}

// withPlatform forces the container's image platform. Used for mysql:5.7 on
// non-amd64 hosts because the official image has no arm64 manifest.
// DOCKER_DEFAULT_PLATFORM alone isn't enough — testcontainers-go builds the
// container-create request without reading that env var, so the platform
// has to be set explicitly on ContainerRequest.
func withPlatform(platform string) testcontainers.CustomizeRequestOption {
	return func(req *testcontainers.GenericContainerRequest) error {
		req.ImagePlatform = platform
		return nil
	}
}

// openTestDB spins up a MySQL container, seeds a fixture table, and returns
// an *sql.DB connected to it. Skips the test if Docker is unavailable.
func openTestDB(t *testing.T) *sql.DB {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	t.Cleanup(cancel)

	opts := []testcontainers.ContainerCustomizer{
		mysqlcontainer.WithDatabase("testdb"),
		mysqlcontainer.WithUsername("root"),
		mysqlcontainer.WithPassword("rootpw"),
	}
	if is57() && runtime.GOARCH != "amd64" {
		opts = append(opts, withPlatform("linux/amd64"))
	}
	if isMariaDB() {
		// testcontainers-go's default wait matches the MySQL banner
		// ("MySQL Community Server"), which MariaDB never prints. Match
		// "ready for connections" instead — MariaDB emits it twice
		// during startup (first for the temporary init server, then for
		// the real server), so occurrence=2 waits for the actual server.
		// Log-match avoids the EOF-spam that ForSQL produces while
		// polling a not-yet-listening socket.
		opts = append(opts, testcontainers.WithWaitStrategy(
			wait.ForLog("ready for connections").
				WithOccurrence(2).
				WithStartupTimeout(2*time.Minute),
		))
	}
	container, err := mysqlcontainer.Run(ctx, mysqlImage(), opts...)
	// Register termination *before* checking the error. If Run fails during
	// the readiness hook (e.g., mysqld segfaults under emulation), the
	// container may have been created and started before the failure — and
	// Ryuk is disabled — so we have to clean it up explicitly or it leaks.
	if container != nil {
		t.Cleanup(func() {
			termCtx, termCancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer termCancel()
			if err := container.Terminate(termCtx); err != nil {
				t.Logf("terminate container: %v", err)
			}
		})
	}
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
	{name: "string_range", lucene: `name:{a TO d}`, wantIDs: []int{1, 2, 3}},
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
	// Note: the LIKE -> REGEXP fallback path (alternation, character
	// classes, grouping) is exercised by a separate test below that
	// bypasses the Lucene parser. The parser can't cleanly produce a
	// value containing an unescaped Lucene metacharacter like `[`
	// or `|`, so going through `ToMySQL` isn't a faithful test of
	// the fallback path.
	// Reserved-word column name: exercises always-backtick policy.
	// `rank` is a reserved word in MySQL 8.0.
	{name: "reserved_word_column", lucene: `rank:>=50`, wantIDs: []int{5, 6, 7}},
	// A field group applies its field to every term inside it.
	{name: "field_group_must_not", lucene: `name:(*an* -banana)`, wantIDs: []int{4, 5}},
	{name: "field_group_ranges", lucene: `price:([1 TO 2] OR [5 TO *])`, wantIDs: []int{1, 4, 5}},
}

var parameterizedCases = []integrationCase{
	{name: "equality", lucene: `category:fruit`, wantIDs: []int{1, 2, 4}},
	{name: "range", lucene: `price:[1 TO 2]`, wantIDs: []int{1, 5}},
	{name: "like_prefix", lucene: `name:a*`, wantIDs: []int{1}},
	{name: "like_literal_percent", lucene: `name:100%*`, wantIDs: []int{6}},
	{name: "standalone_star", lucene: `status:*`, wantIDs: []int{1, 2, 3, 4, 6, 7}},
	{name: "regexp_literal", lucene: `name:/^[ab].*/`, wantIDs: []int{1, 2}},
	{name: "reserved_word_column", lucene: `rank:>=50`, wantIDs: []int{5, 6, 7}},
	// A field group applies its field to every term inside it.
	{name: "field_group_must_not", lucene: `name:(*an* -banana)`, wantIDs: []int{4, 5}},
	{name: "field_group_ranges", lucene: `price:([1 TO 2] OR [5 TO *])`, wantIDs: []int{1, 4, 5}},
}

func TestMySQLIntegrationRendered(t *testing.T) {
	requireIntegration(t)
	db := openTestDB(t)

	for _, tc := range renderedCases {
		t.Run(tc.name, func(t *testing.T) {
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
	requireIntegration(t)
	db := openTestDB(t)

	for _, tc := range parameterizedCases {
		t.Run(tc.name, func(t *testing.T) {
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

// TestMySQLIntegrationHandBuiltFieldGroup runs EQUALS on a group, built in
// code or read from JSON, which the driver spreads over every term the way
// the parser reads a field group. `name` = ('apple' OR 'banana') used to
// match no rows.
func TestMySQLIntegrationHandBuiltFieldGroup(t *testing.T) {
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
		{"or_null", expr.Eq("status", expr.OR("active", expr.NULL())), []int{1, 2, 4, 5, 6, 7}},
		{"ranges", expr.Eq("price", expr.OR(priceRange(expr.Lit(1), expr.Lit(2)), priceRange(expr.Lit(5), expr.WILD("*")))), []int{1, 4, 5}},
		{"from_json", &fromJSON, []int{1, 2}},
		{"ranges_from_json", roundTrip(t, expr.Eq("price", expr.OR(priceRange(expr.Lit(1), expr.Lit(2)), priceRange(expr.Lit(5), expr.WILD("*"))))), []int{1, 4, 5}},
		// A value with a field of its own keeps it, as name:(category:fruit) does.
		{"own_field", expr.Eq("name", expr.Eq("category", "fruit")), []int{1, 2, 4}},
	}

	d := driver.NewMySQLDriver()
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

// TestMySQLIntegrationFallbackPath exercises the LIKE -> REGEXP fallback
// (pkg/driver/mysql.go:luceneWildcardToRegex) against a real MySQL. We
// bypass the Lucene parser here because no clean Lucene syntax produces
// a value containing the fallback-triggering metacharacters (`|`, `[]`,
// `()`, `{}`, `+`) — they're reserved, and an escaped one is a literal
// character, which stays on the LIKE path. Constructing the expression directly
// via pkg/driver lets us verify the anchored ^(...)$ form actually runs
// on both 8.0 (ICU) and 5.7 (Henry Spencer POSIX) regex engines.
func TestMySQLIntegrationFallbackPath(t *testing.T) {
	requireIntegration(t)
	db := openTestDB(t)

	drv := driver.NewMySQLDriver()
	cases := []struct {
		name    string
		pattern string
		wantIDs []int
	}{
		{name: "char_class", pattern: "[ab]*", wantIDs: []int{1, 2}},
		{name: "grouping", pattern: "(app)*", wantIDs: []int{1}},
		{name: "alternation", pattern: "*(ppl|nan)*", wantIDs: []int{1, 2}},
		// `.` and `$` are literal in a Lucene wildcard. If MySQL's string
		// parser drops the backslash the fallback puts in front of them,
		// `ppl.` matches "apple" and `e$` anchors on it, and row 1 leaks in.
		{name: "literal_dot", pattern: "*(ppl.|nan)*", wantIDs: []int{2}},
		{name: "literal_dollar", pattern: "*(e$|nan)*", wantIDs: []int{2}},
	}
	for _, tc := range cases {
		t.Run(tc.name+"/rendered", func(t *testing.T) {
			e := expr.LIKE("name", tc.pattern)
			where, err := drv.Render(e)
			if err != nil {
				t.Fatalf("render: %v", err)
			}
			got := queryIDs(t, db, where)
			if !equalInts(got, tc.wantIDs) {
				t.Fatalf("pattern=%q where=%s\n  want ids %v\n  got  ids %v",
					tc.pattern, where, tc.wantIDs, got)
			}
		})
		t.Run(tc.name+"/parameterized", func(t *testing.T) {
			e := expr.LIKE("name", tc.pattern)
			where, params, err := drv.RenderParam(e)
			if err != nil {
				t.Fatalf("render: %v", err)
			}
			got := queryIDs(t, db, where, params...)
			if !equalInts(got, tc.wantIDs) {
				t.Fatalf("pattern=%q where=%s params=%v\n  want ids %v\n  got  ids %v",
					tc.pattern, where, params, tc.wantIDs, got)
			}
		})
	}
}

// TestMySQLIntegrationEscapedWildcards matches rows whose names hold the
// characters a wildcard pattern gives a meaning to. An escaped character
// must match only itself, on the LIKE path and on the REGEXP fallback.
func TestMySQLIntegrationEscapedWildcards(t *testing.T) {
	requireIntegration(t)
	db := openTestDB(t)

	// MySQL reads a backslash in a string literal as an escape, so 'a\\b'
	// is the three characters a\b.
	const rows = `
		INSERT INTO items (id, name) VALUES
			(11, 'a*b'), (12, 'axb'), (13, 'a?b'), (14, 'a%b'), (15, 'a_b'),
			(16, 'a[b'), (17, 'a\\b'), (18, 'a#b'), (19, 'a]b'), (20, 'a.b'),
			(21, 'a b'), (22, 'a(b'), (23, 'a|b'), (24, 'a+b'), (25, 'a{b'),
			(26, 'ab'), (27, 'a^b'), (28, 'a-b');
	`
	if _, err := db.Exec(rows); err != nil {
		t.Fatalf("seed: %v", err)
	}

	cases := []integrationCase{
		{name: "escaped_star_term", lucene: `name:a\*b`, wantIDs: []int{11}},
		{name: "escaped_star", lucene: `name:a\**`, wantIDs: []int{11}},
		{name: "escaped_star_inside", lucene: `name:*\*b`, wantIDs: []int{11}},
		{name: "escaped_question_term", lucene: `name:a\?b`, wantIDs: []int{13}},
		{name: "escaped_question", lucene: `name:a\?*`, wantIDs: []int{13}},
		{name: "percent", lucene: `name:a%*`, wantIDs: []int{14}},
		{name: "underscore", lucene: `name:a_*`, wantIDs: []int{15}},
		{name: "escaped_bracket", lucene: `name:a\[*`, wantIDs: []int{16}},
		{name: "escaped_close_bracket", lucene: `name:a\]*`, wantIDs: []int{19}},
		{name: "escaped_backslash", lucene: `name:a\\*`, wantIDs: []int{17}},
		{name: "escaped_backslash_term", lucene: `name:a\\b`, wantIDs: []int{17}},
		{name: "hash", lucene: `name:a#*`, wantIDs: []int{18}},
		{name: "dot", lucene: `name:a.*`, wantIDs: []int{20}},
		{name: "escaped_space", lucene: `name:a\ *`, wantIDs: []int{21}},
		{name: "escaped_paren", lucene: `name:a\(*`, wantIDs: []int{22}},
		{name: "escaped_pipe", lucene: `name:a\|*`, wantIDs: []int{23}},
		{name: "escaped_plus", lucene: `name:a\+*`, wantIDs: []int{24}},
		{name: "escaped_brace", lucene: `name:a\{*`, wantIDs: []int{25}},
		{name: "single_char", lucene: `name:a?b`, wantIDs: []int{11, 12, 13, 14, 15, 16, 17, 18, 19, 20, 21, 22, 23, 24, 25, 27, 28}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			where, err := lucene.ToMySQL(tc.lucene)
			if err != nil {
				t.Fatalf("ToMySQL(%q): %v", tc.lucene, err)
			}
			if got := queryIDs(t, db, where); !equalInts(got, tc.wantIDs) {
				t.Fatalf("lucene=%q where=%q\n  want ids %v\n  got  ids %v",
					tc.lucene, where, tc.wantIDs, got)
			}

			where, params, err := lucene.ToParameterizedMySQL(tc.lucene)
			if err != nil {
				t.Fatalf("ToParameterizedMySQL(%q): %v", tc.lucene, err)
			}
			if got := queryIDs(t, db, where, params...); !equalInts(got, tc.wantIDs) {
				t.Fatalf("lucene=%q where=%q params=%v\n  want ids %v\n  got  ids %v",
					tc.lucene, where, params, tc.wantIDs, got)
			}
		})
	}

	// expr.EscapeWildcard turns any text into a pattern that matches only
	// that text, and only text that starts with it once a * follows.
	names := map[int]string{}
	nameRows, err := db.Query("SELECT id, name FROM items WHERE id >= 11")
	if err != nil {
		t.Fatalf("query names: %v", err)
	}
	for nameRows.Next() {
		var id int
		var name string
		if err := nameRows.Scan(&id, &name); err != nil {
			t.Fatalf("scan: %v", err)
		}
		names[id] = name
	}
	if err := nameRows.Close(); err != nil {
		t.Fatalf("rows: %v", err)
	}
	for id, name := range names {
		t.Run(fmt.Sprintf("escape_wildcard_%d", id), func(t *testing.T) {
			for _, tc := range []struct {
				pattern string
				want    []int
			}{
				{expr.EscapeWildcard(name), []int{id}},
				{expr.EscapeWildcard(name) + "*", []int{id}},
			} {
				e := expr.LIKE("name", tc.pattern)
				where, params, err := driver.NewMySQLDriver().RenderParam(e)
				if err != nil {
					t.Fatalf("render param: %v", err)
				}
				if got := queryIDs(t, db, where, params...); !equalInts(got, tc.want) {
					t.Fatalf("name=%q where=%s params=%v\n  want ids %v\n  got  ids %v", name, where, params, tc.want, got)
				}
				where, err = driver.NewMySQLDriver().Render(e)
				if err != nil {
					t.Fatalf("render: %v", err)
				}
				if got := queryIDs(t, db, where); !equalInts(got, tc.want) {
					t.Fatalf("name=%q where=%s\n  want ids %v\n  got  ids %v", name, where, tc.want, got)
				}
			}
		})
	}

	// The | below sends these patterns down the REGEXP fallback. The
	// escaped characters next to it must still match only themselves.
	drv := driver.NewMySQLDriver()
	fallback := []struct {
		name    string
		pattern string
		wantIDs []int
	}{
		{name: "fallback_escaped_star", pattern: `(a\*|zz)b`, wantIDs: []int{11}},
		{name: "fallback_escaped_question", pattern: `(a\?|zz)b`, wantIDs: []int{13}},
		{name: "fallback_escaped_backslash", pattern: `(a\\|zz)b`, wantIDs: []int{17}},
		{name: "fallback_escaped_pipe", pattern: `(a\||zz)b`, wantIDs: []int{23}},
		{name: "fallback_escaped_bracket", pattern: `(a\[|zz)b`, wantIDs: []int{16}},
		{name: "fallback_dot", pattern: `(a.|zz)b`, wantIDs: []int{20}},
		// Inside brackets an escaped ^, -, ] or [ is a member of the set.
		// MySQL 5.7 reads the backslash as a member too, so these run on
		// 8.0 and MariaDB 10.5+ only.
		{name: "bracket_escaped_caret", pattern: `a[\^x]b`, wantIDs: []int{12, 27}},
		{name: "bracket_escaped_dash", pattern: `a[x\-z]b`, wantIDs: []int{12, 28}},
		{name: "bracket_escaped_close", pattern: `a[\]x]b`, wantIDs: []int{12, 19}},
		{name: "bracket_escaped_open", pattern: `a[\[x]b`, wantIDs: []int{12, 16}},
		{name: "bracket_escaped_backslash", pattern: `a[\\x]b`, wantIDs: []int{12, 17}},
		{name: "bracket_wildcard_chars", pattern: `a[*?]b`, wantIDs: []int{11, 13}},
	}
	for _, tc := range fallback {
		t.Run(tc.name, func(t *testing.T) {
			if is57() && strings.HasPrefix(tc.name, "bracket_escaped") {
				t.Skip("MySQL 5.7 reads a backslash inside brackets as a member")
			}
			e := expr.LIKE("name", tc.pattern)
			where, err := drv.Render(e)
			if err != nil {
				t.Fatalf("render: %v", err)
			}
			if got := queryIDs(t, db, where); !equalInts(got, tc.wantIDs) {
				t.Fatalf("pattern=%q where=%s\n  want ids %v\n  got  ids %v",
					tc.pattern, where, tc.wantIDs, got)
			}
			where, params, err := drv.RenderParam(e)
			if err != nil {
				t.Fatalf("render param: %v", err)
			}
			if got := queryIDs(t, db, where, params...); !equalInts(got, tc.wantIDs) {
				t.Fatalf("pattern=%q where=%s params=%v\n  want ids %v\n  got  ids %v",
					tc.pattern, where, params, tc.wantIDs, got)
			}
		})
	}
}

// Guard against accidental removal of the ? placeholder contract: the
// parameterized MySQL renderer must emit ?, not $N.
func TestMySQLParameterPlaceholderIsQuestionMark(t *testing.T) {
	requireIntegration(t)
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
}

// TestMySQLIntegrationCaseInsensitiveFields runs WithCaseInsensitiveFields
// against a utf8mb4_bin column, where plain comparisons are case-sensitive,
// so every match below comes from LOWER().
func TestMySQLIntegrationCaseInsensitiveFields(t *testing.T) {
	requireIntegration(t)
	db := openTestDB(t)

	const labels = `
		ALTER TABLE items
			ADD COLUMN label VARCHAR(64) CHARACTER SET utf8mb4 COLLATE utf8mb4_bin,
			ADD COLUMN bin VARBINARY(64);
		UPDATE items SET label = 'FooBar' WHERE id = 1;
		UPDATE items SET label = 'Foo' WHERE id = 2;
		UPDATE items SET label = 'foo' WHERE id = 3;
		UPDATE items SET label = 'BAR' WHERE id = 4;
		UPDATE items SET label = 'Éclair' WHERE id = 6;
		UPDATE items SET label = 'other' WHERE id = 7;
		UPDATE items SET bin = label;
	`
	if _, err := db.Exec(labels); err != nil {
		t.Fatalf("seed labels: %v", err)
	}

	t.Run("column_is_case_sensitive_without_option", func(t *testing.T) {
		for q, want := range map[string][]int{`label:FOO`: nil, `label:foo*`: {3}} {
			where, err := lucene.ToMySQL(q)
			if err != nil {
				t.Fatalf("ToMySQL(%q): %v", q, err)
			}
			if got := queryIDs(t, db, where); !equalInts(got, want) {
				t.Fatalf("lucene=%q where=%q\n  want ids %v\n  got  ids %v", q, where, want, got)
			}
		}
	})

	// Row 5 has label = NULL.
	cases := []integrationCase{
		{name: "wildcard", lucene: `label:foo*`, wantIDs: []int{1, 2, 3}},
		{name: "wildcard_upper", lucene: `label:FOO*`, wantIDs: []int{1, 2, 3}},
		{name: "single_char", lucene: `label:F?O`, wantIDs: []int{2, 3}},
		{name: "equality", lucene: `label:foo`, wantIDs: []int{2, 3}},
		{name: "equality_upper", lucene: `label:FOO`, wantIDs: []int{2, 3}},
		{name: "phrase", lucene: `label:"FOOBAR"`, wantIDs: []int{1}},
		{name: "in_list", lucene: `label:(foo OR bar)`, wantIDs: []int{2, 3, 4}},
		{name: "in_list_with_null", lucene: `label:(foo OR null)`, wantIDs: []int{2, 3, 5}},
		{name: "mixed_in_list", lucene: `label:(foo OR 5)`, wantIDs: []int{2, 3}},
		{name: "negated_wildcard", lucene: `-label:foo*`, wantIDs: []int{4, 6, 7}},
		{name: "regex_keeps_case", lucene: `label:/^Foo/`, wantIDs: []int{1, 2}},
		{name: "standalone_star", lucene: `label:*`, wantIDs: []int{1, 2, 3, 4, 6, 7}},
		{name: "null", lucene: `label:null`, wantIDs: []int{5}},
		// MySQL's LOWER() folds non-ASCII letters too.
		{name: "non_ascii", lucene: `label:ÉCLAIR`, wantIDs: []int{6}},
		// rank is listed, but a number is compared as a number.
		{name: "number_not_folded", lucene: "rank:50", wantIDs: []int{5}},
		// LOWER() does nothing to a binary string.
		{name: "binary_column_keeps_case", lucene: `bin:foo`, wantIDs: []int{3}},
	}

	opt := lucene.WithCaseInsensitiveFields("label", "rank", "bin")
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			where, err := lucene.ToMySQL(tc.lucene, opt)
			if err != nil {
				t.Fatalf("ToMySQL(%q): %v", tc.lucene, err)
			}
			if got := queryIDs(t, db, where); !equalInts(got, tc.wantIDs) {
				t.Fatalf("lucene=%q where=%q\n  want ids %v\n  got  ids %v",
					tc.lucene, where, tc.wantIDs, got)
			}

			where, params, err := lucene.ToParameterizedMySQL(tc.lucene, opt)
			if err != nil {
				t.Fatalf("ToParameterizedMySQL(%q): %v", tc.lucene, err)
			}
			if got := queryIDs(t, db, where, params...); !equalInts(got, tc.wantIDs) {
				t.Fatalf("lucene=%q where=%q params=%v\n  want ids %v\n  got  ids %v",
					tc.lucene, where, params, tc.wantIDs, got)
			}
		})
	}

	t.Run("listed_default_field", func(t *testing.T) {
		where, params, err := lucene.ToParameterizedMySQL(`FOO`, opt, lucene.WithDefaultField("label"))
		if err != nil {
			t.Fatalf("ToParameterizedMySQL: %v", err)
		}
		if got, want := queryIDs(t, db, where, params...), []int{2, 3}; !equalInts(got, want) {
			t.Fatalf("where=%q params=%v\n  want ids %v\n  got  ids %v", where, params, want, got)
		}
	})

	// The LIKE -> REGEXP fallback folds too. As in
	// TestMySQLIntegrationFallbackPath, the parser cannot produce these
	// patterns, so the expressions are built directly.
	drv := driver.NewMySQLDriver()
	drv.CaseInsensitiveFields = []string{"label"}
	fallback := []struct {
		name    string
		pattern string
		wantIDs []int
	}{
		{name: "fallback_alternation", pattern: "(FOO|bar)", wantIDs: []int{2, 3, 4}},
		{name: "fallback_grouping", pattern: "(foo)*", wantIDs: []int{1, 2, 3}},
	}
	for _, tc := range fallback {
		t.Run(tc.name, func(t *testing.T) {
			e := expr.LIKE("label", tc.pattern)
			where, err := drv.Render(e)
			if err != nil {
				t.Fatalf("render: %v", err)
			}
			if got := queryIDs(t, db, where); !equalInts(got, tc.wantIDs) {
				t.Fatalf("pattern=%q where=%s\n  want ids %v\n  got  ids %v",
					tc.pattern, where, tc.wantIDs, got)
			}
			where, params, err := drv.RenderParam(e)
			if err != nil {
				t.Fatalf("render param: %v", err)
			}
			if got := queryIDs(t, db, where, params...); !equalInts(got, tc.wantIDs) {
				t.Fatalf("pattern=%q where=%s params=%v\n  want ids %v\n  got  ids %v",
					tc.pattern, where, params, tc.wantIDs, got)
			}
		})
	}
}

func TestMySQLIntegrationJSONPath(t *testing.T) {
	requireIntegration(t)
	db := openTestDB(t)

	const labels = `
		ALTER TABLE items ADD COLUMN labels JSON;
		UPDATE items SET labels = '{"color": "red", "stock": {"count": 10}, "it''s": "yes", "n": 10, "v": "2"}' WHERE id = 1;
		UPDATE items SET labels = '{"color": "yellow", "stock": {"count": 3}, "n": "abc", "v": 2}' WHERE id = 2;
		UPDATE items SET labels = '{"color": "orange", "stock": {"count": 25}, "n": null, "v": 2.0}' WHERE id = 3;
		UPDATE items SET labels = '{"color": "green", "n": true, "v": "abc"}' WHERE id = 4;
	`
	if _, err := db.Exec(labels); err != nil {
		t.Fatalf("seed labels: %v", err)
	}

	// Rows 5-7 have labels = NULL; row 4 has no stock key.
	cases := []integrationCase{
		{name: "equality", lucene: `labels.color:red`, wantIDs: []int{1}},
		{name: "nested_greater", lucene: `labels.stock.count:>5`, wantIDs: []int{1, 3}},
		{name: "nested_range", lucene: `labels.stock.count:[3 TO 10]`, wantIDs: []int{1, 2}},
		{name: "nested_in_numbers", lucene: `labels.stock.count:(3 OR 25)`, wantIDs: []int{2, 3}},
		{name: "in_list", lucene: `labels.color:(red OR yellow)`, wantIDs: []int{1, 2}},
		{name: "like_prefix", lucene: `labels.color:ye*`, wantIDs: []int{2}},
		{name: "null", lucene: `labels.stock.count:null`, wantIDs: []int{4, 5, 6, 7}},
		{name: "not_null", lucene: `NOT labels.stock.count:null`, wantIDs: []int{1, 2, 3}},
		{name: "in_list_with_null", lucene: `labels.color:(red OR null)`, wantIDs: []int{1, 5, 6, 7}},
		{name: "standalone_star", lucene: `labels.color:*`, wantIDs: []int{1, 2, 3, 4}},
		{name: "quote_in_key", lucene: `labels.it\'s:yes`, wantIDs: []int{1}},
		{name: "injection_in_key", lucene: `labels.x\'\ OR\ \'a\'\ \=\ \'a:z`, wantIDs: nil},
		{name: "with_plain_column", lucene: `category:fruit AND labels.color:yellow`, wantIDs: []int{2}},
		// n is 10, "abc", JSON null and true in rows 1-4.
		{name: "numeric_skips_text", lucene: `labels.n:>5`, wantIDs: []int{1}},
		{name: "numeric_zero_skips_text", lucene: `labels.n:0`, wantIDs: nil},
		{name: "numeric_less_skips_text", lucene: `labels.n:<100`, wantIDs: []int{1}},
		{name: "json_null", lucene: `labels.n:null`, wantIDs: []int{3, 5, 6, 7}},
		{name: "not_json_null", lucene: `NOT labels.n:null`, wantIDs: []int{1, 2, 4}},
		{name: "numeric_in_with_null", lucene: `labels.n:(10 OR null)`, wantIDs: []int{1, 3, 5, 6, 7}},
		{name: "text_value", lucene: `labels.n:abc`, wantIDs: []int{2}},
		{name: "not_numeric_keeps_non_numbers", lucene: `-labels.n:>5`, wantIDs: []int{2, 4}},
		{name: "not_equal_number", lucene: `-labels.n:7`, wantIDs: []int{1, 2, 4}},
		// v is "2", 2, 2.0 and "abc" in rows 1-4.
		{name: "number_matches_text_form", lucene: `labels.v:2`, wantIDs: []int{1, 2, 3}},
		{name: "not_number_or_text_form", lucene: `-labels.v:2`, wantIDs: []int{4}},
		{name: "in_numbers_match_text_form", lucene: `labels.v:(2 OR 7)`, wantIDs: []int{1, 2, 3}},
		{name: "greater_skips_text_form", lucene: `labels.v:>1`, wantIDs: []int{2, 3}},
		{name: "mixed_in_compares_text", lucene: `labels.v:(2 OR abc)`, wantIDs: []int{1, 2, 4}},
		{name: "mixed_range_compares_text", lucene: `labels.v:{2 TO abc}`, wantIDs: []int{3}},
		{name: "quoted_number_matches_number", lucene: `labels.stock.count:"10"`, wantIDs: []int{1}},
		{name: "bool_text", lucene: `labels.n:true`, wantIDs: []int{4}},
		{name: "written_decimal_keeps_text", lucene: `labels.v:2.0`, wantIDs: []int{2, 3}},
		{name: "leading_zero_keeps_text", lucene: `labels.v:02`, wantIDs: []int{2, 3}},
		{name: "not_plain_text_skips_json_null", lucene: `-labels.n:abc`, wantIDs: []int{1, 4}},
		{name: "plain_text_in_list", lucene: `labels.color:(red OR green)`, wantIDs: []int{1, 4}},
	}

	opt := lucene.WithJSONFields("labels")
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			where, err := lucene.ToMySQL(tc.lucene, opt)
			if err != nil {
				t.Fatalf("ToMySQL(%q): %v", tc.lucene, err)
			}
			if got := queryIDs(t, db, where); !equalInts(got, tc.wantIDs) {
				t.Fatalf("lucene=%q where=%q\n  want ids %v\n  got  ids %v",
					tc.lucene, where, tc.wantIDs, got)
			}

			where, params, err := lucene.ToParameterizedMySQL(tc.lucene, opt)
			if err != nil {
				t.Fatalf("ToParameterizedMySQL(%q): %v", tc.lucene, err)
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
			where, err := lucene.ToMySQL(tc.lucene, folded...)
			if err != nil {
				t.Fatalf("ToMySQL(%q): %v", tc.lucene, err)
			}
			if got := queryIDs(t, db, where); !equalInts(got, tc.wantIDs) {
				t.Fatalf("lucene=%q where=%q\n  want ids %v\n  got  ids %v", tc.lucene, where, tc.wantIDs, got)
			}
			where, params, err := lucene.ToParameterizedMySQL(tc.lucene, folded...)
			if err != nil {
				t.Fatalf("ToParameterizedMySQL(%q): %v", tc.lucene, err)
			}
			if got := queryIDs(t, db, where, params...); !equalInts(got, tc.wantIDs) {
				t.Fatalf("lucene=%q where=%q params=%v\n  want ids %v\n  got  ids %v", tc.lucene, where, params, tc.wantIDs, got)
			}
		})
	}

	// A hand-built bool compares as the text true or false.
	n := expr.JSONPath{Column: "labels", Path: []string{"n"}}
	d := driver.NewMySQLDriver()
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
