package lucene

import "github.com/grindlemire/go-lucene/pkg/driver"

var (
	postgres = driver.NewPostgresDriver()
	sqlite   = driver.NewSQLiteDriver()
	mysql    = driver.NewMySQLDriver()
)

// ToPostgres is a wrapper that will render the lucene expression string as a postgres sql filter string.
func ToPostgres(in string, opts ...Opt) (string, error) {
	e, err := Parse(in, opts...)
	if err != nil {
		return "", err
	}

	return postgres.Render(e)
}

// ToParameterizedPostgres is a wrapper that will render the lucene expression string as a postgres sql filter string with parameters.
// The returned string will contain placeholders for the parameters that can be passed directly to a Query statement.
func ToParameterizedPostgres(in string, opts ...Opt) (s string, params []any, err error) {
	e, err := Parse(in, opts...)
	if err != nil {
		return "", nil, err
	}

	return postgres.RenderParam(e)
}

// ToSQLite is a wrapper that will render the lucene expression string as a SQLite sql filter string.
func ToSQLite(in string, opts ...Opt) (string, error) {
	e, err := Parse(in, opts...)
	if err != nil {
		return "", err
	}

	return sqlite.Render(e)
}

// ToParameterizedSQLite is a wrapper that will render the lucene expression string as a SQLite sql filter
// string with parameters. The returned string will contain ? placeholders that can be passed directly to a
// Query statement.
func ToParameterizedSQLite(in string, opts ...Opt) (s string, params []any, err error) {
	e, err := Parse(in, opts...)
	if err != nil {
		return "", nil, err
	}

	return sqlite.RenderParam(e)
}

// ToMySQL is a wrapper that will render the lucene expression string as a MySQL sql filter string.
func ToMySQL(in string, opts ...Opt) (string, error) {
	e, err := Parse(in, opts...)
	if err != nil {
		return "", err
	}

	return mysql.Render(e)
}

// ToParameterizedMySQL is a wrapper that will render the lucene expression string as a MySQL sql filter
// string with parameters. The returned string will contain ? placeholders that can be passed directly to a
// Query statement.
func ToParameterizedMySQL(in string, opts ...Opt) (s string, params []any, err error) {
	e, err := Parse(in, opts...)
	if err != nil {
		return "", nil, err
	}

	return mysql.RenderParam(e)
}

// ToMariaDB renders a lucene expression as a MariaDB sql filter string.
// It is a thin alias over ToMySQL: everything this package emits (backtick
// quoting, `LIKE ... ESCAPE '#'`, `REGEXP`, `BETWEEN`, bool literals,
// `?` placeholders) is identical on MySQL and MariaDB. The `^(...)$` regex
// fallback avoids `(?:...)` so it works on every regex engine either
// database has shipped (MySQL 5.7/MariaDB 10.4 Henry Spencer, MySQL 8.0 ICU,
// MariaDB 10.5+ PCRE2).
func ToMariaDB(in string, opts ...Opt) (string, error) {
	return ToMySQL(in, opts...)
}

// ToParameterizedMariaDB renders a lucene expression as a MariaDB sql filter
// string with ? placeholders. Alias over ToParameterizedMySQL; see ToMariaDB
// for the compatibility rationale.
func ToParameterizedMariaDB(in string, opts ...Opt) (s string, params []any, err error) {
	return ToParameterizedMySQL(in, opts...)
}
