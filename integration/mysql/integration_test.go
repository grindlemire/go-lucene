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

// sweepLeakedContainers finds any container tagged with this test run's
// testcontainers session ID and force-removes it. No-op if the runtime CLI
// isn't on PATH or if no containers match.
func sweepLeakedContainers() {
	sid := testcontainers.SessionID()
	if sid == "" {
		return
	}
	cli := "docker"
	if _, err := exec.LookPath(cli); err != nil {
		cli = "podman"
		if _, err := exec.LookPath(cli); err != nil {
			return
		}
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
		return
	}
	if sock, err := podmanSocket(); err == nil {
		os.Setenv("DOCKER_HOST", "unix://"+sock)
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
	requireIntegration(t)
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
	requireIntegration(t)
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

// TestMySQLIntegrationFallbackPath exercises the LIKE -> REGEXP fallback
// (pkg/driver/mysql.go:luceneWildcardToRegex) against a real MySQL. We
// bypass the Lucene parser here because no clean Lucene syntax produces
// a value containing the fallback-triggering metacharacters (`|`, `[]`,
// `()`, `{}`, `+`) — they're reserved, and escaped forms keep the
// backslash in the parsed value. Constructing the expression directly
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
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
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
	if strings.Contains(where, fmt.Sprintf("$%d", 1)) {
		t.Fatalf("unexpected $N placeholder in MySQL output: %q", where)
	}
}
