# go-lucene

[![Go Reference](https://pkg.go.dev/badge/github.com/grindlemire/go-lucene.svg)](https://pkg.go.dev/github.com/grindlemire/go-lucene)

Parse [Lucene](https://lucene.apache.org/core/9_4_2/queryparser/org/apache/lucene/queryparser/classic/package-summary.html#package.description) queries and turn them into SQL. No dependencies. PostgreSQL, SQLite, MySQL, and MariaDB work out of the box, and you can plug in your own dialect for anything else.

```go
query := `name:"John Doe" AND age:[25 TO 35]`
sql, params, err := lucene.ToParameterizedPostgres(query)
// sql:    (("name" = $1) AND ("age" >= $2 AND "age" <= $3))
// params: ["John Doe", 25, 35]
```

## Contents

- [Install](#install)
- [Usage](#usage)
- [Operator reference](#operator-reference)
- [SQLite](#sqlite)
- [MySQL](#mysql)
- [Custom drivers](#custom-drivers)

## Install

```bash
go get github.com/grindlemire/go-lucene
```

## Usage

### Parameterized queries (recommended)

Use the parameterized form for anything that touches user input. It returns a SQL string with placeholders and a separate slice of values that your driver will bind safely.

```go
sql, params, err := lucene.ToParameterizedPostgres(`color:red AND type:"gala"`)
// sql:    ("color" = $1) AND ("type" = $2)
// params: ["red", "gala"]

rows, err := db.Query(sql, params...)
```

SQLite has an equivalent that uses `?` placeholders:

```go
sql, params, err := lucene.ToParameterizedSQLite(`color:red AND type:"gala"`)
// sql:    ("color" = ?) AND ("type" = ?)
// params: ["red", "gala"]
```

MySQL also uses `?` placeholders and backtick-quoted identifiers:

```go
sql, params, err := lucene.ToParameterizedMySQL(`color:red AND type:"gala"`)
// sql:    (`color` = ?) AND (`type` = ?)
// params: ["red", "gala"]
```

### Inline values

If you don't need parameter binding (for example, when generating SQL for inspection), `ToPostgres`, `ToSQLite`, and `ToMySQL` embed values directly into the string:

```go
sql, err := lucene.ToPostgres(`name:"John Doe" AND age:[25 TO 35]`)
// (("name" = 'John Doe') AND ("age" >= 25 AND "age" <= 35))
```

### Default field

When a term has no field prefix, the parser can fall back to one you supply:

```go
sql, err := lucene.ToPostgres(`red OR green`, lucene.WithDefaultField("color"))
// ("color" = 'red') OR ("color" = 'green')
```

## Operator reference

Output below is Postgres. See [SQLite](#sqlite) and [MySQL](#mysql) for where those drivers differ.

| Lucene | SQL | Notes |
|---|---|---|
| `field:value` | `"field" = 'value'` | Exact match |
| `field:"phrase with spaces"` | `"field" = 'phrase with spaces'` | Quoted phrase |
| `a:1 AND b:2` | `("a" = 1) AND ("b" = 2)` | Boolean AND |
| `a:1 OR b:2` | `("a" = 1) OR ("b" = 2)` | Boolean OR |
| `NOT field:value` | `NOT("field" = 'value')` | Negation |
| `+field:value` | `"field" = 'value'` | Required term (same as no prefix) |
| `-field:value` | `NOT("field" = 'value')` | Prohibited term |
| `field:[min TO max]` | `"field" >= min AND "field" <= max` | Inclusive range |
| `field:{min TO max}` | `"field" > min AND "field" < max` | Exclusive range |
| `field:[min TO *]` | `"field" >= min` | Open upper bound |
| `field:[* TO max]` | `"field" <= max` | Open lower bound |
| `field:*` | `"field" SIMILAR TO '%'` | Match anything non-null |
| `field:pat*` | `"field" SIMILAR TO 'pat%'` | Wildcard suffix |
| `field:pat?` | `"field" SIMILAR TO 'pat_'` | Single-character wildcard |
| `field:/regex/` | `"field" ~ 'regex'` | Regular expression |
| `(a:1 OR b:2) AND c:3` | `(("a" = 1) OR ("b" = 2)) AND ("c" = 3)` | Grouping |

## SQLite

SQLite lacks direct equivalents for a few Postgres operators, so the SQLite driver renders them differently:

| Lucene | Postgres | SQLite |
|---|---|---|
| `field:*` | `"field" SIMILAR TO '%'` | `"field" IS NOT NULL` |
| `field:pat*` | `"field" SIMILAR TO 'pat%'` | `"field" GLOB 'pat*'` |
| `field:pat?` | `"field" SIMILAR TO 'pat_'` | `"field" GLOB 'pat?'` |
| `field:/regex/` | `"field" ~ 'regex'` | `"field" REGEXP 'regex'` |
| parameters | `$1, $2, ...` | `?` |

### Things to watch for

**GLOB is case-sensitive** and uses Unix glob syntax. Lucene's `*` and `?` map cleanly onto GLOB's `*` and `?`.

**GLOB has no escape character.** To match a literal `*` or `?`, use the regex form `field:/.../`.

**GLOB has no alternation.** A pattern like `field:*(a|b)*` matches the literal characters `(a|b)`, not "a or b". Use `field:/.*(a|b).*/` if you need alternation in SQLite.

**A bare `field:*` becomes `IS NOT NULL`**, matching any row where the field has a value regardless of storage class.

### Registering REGEXP

SQLite ships without a `regexp()` function, so regex queries will error at query time unless you register one on your connection.

With `modernc.org/sqlite`:

```go
import (
    "database/sql/driver"
    "regexp"

    "modernc.org/sqlite"
)

func init() {
    sqlite.MustRegisterDeterministicScalarFunction(
        "regexp",
        2,
        func(ctx *sqlite.FunctionContext, args []driver.Value) (driver.Value, error) {
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
                return false, nil
            }
            return matched, nil
        },
    )
}
```

With `mattn/go-sqlite3`, build with the `sqlite_regex` tag.

## MySQL

MySQL uses backticks for identifiers and doesn't have `SIMILAR TO`, so the MySQL driver routes wildcards through `LIKE ... ESCAPE '#'` and falls back to `REGEXP` when a pattern uses SIMILAR-TO-only constructs (alternation, character classes, grouping):

| Lucene | Postgres | MySQL |
|---|---|---|
| `field:value` | `"field" = 'value'` | `` `field` = 'value' `` |
| `field:*` | `"field" SIMILAR TO '%'` | `` `field` IS NOT NULL `` |
| `field:pat*` | `"field" SIMILAR TO 'pat%'` | `` `field` LIKE 'pat%' ESCAPE '#' `` |
| `field:pat?` | `"field" SIMILAR TO 'pat_'` | `` `field` LIKE 'pat_' ESCAPE '#' `` |
| `field:100%*` (literal `%`) | `"field" SIMILAR TO '100\%%'` | `` `field` LIKE '100#%%' ESCAPE '#' `` |
| `field:*(a\|b)*` (passed via `expr.LIKE`, see note) | `"field" SIMILAR TO '%(a\|b)%'` | `` `field` REGEXP '^(.*(a\|b).*)$' `` |
| `field:/regex/` | `"field" ~ 'regex'` | `` `field` REGEXP 'regex' `` |
| bool literal `true` | `true` | `TRUE` |
| parameters | `$1, $2, ...` | `?` |

### Things to watch for

**Identifiers always quote with backticks.** Column names containing a backtick are rejected at render time. The always-quote policy handles MySQL 8.0's expanded reserved-word list (`RANK`, `LEAD`, `WINDOW`, `ROWS`, etc.) automatically.

**`LIKE` vs `REGEXP` is decided by pattern content.** Simple Lucene wildcards (`*`, `?`) stay on the index-friendly `LIKE` path. Patterns containing `|`, `()`, `[]`, `{}`, or `+` fall back to `REGEXP` with an anchored `^(...)$` translation so the match semantics line up with Postgres `SIMILAR TO`. A plain capturing group is used rather than `(?:...)` because the non-capturing form is a Perl extension that POSIX ERE (MySQL 5.7 / MariaDB 10.0-10.4) does not guarantee.

**The `ESCAPE '#'` clause is intentional.** Using `#` instead of the default backslash keeps the rendered SQL portable across `sql_mode` settings. Under `NO_BACKSLASH_ESCAPES`, a `\` escape clause would be reinterpreted and break the LIKE pattern.

**Non-parameterized output is best-effort under `NO_BACKSLASH_ESCAPES`.** `ToMySQL` doubles backslashes in string literals (correct under the default `sql_mode`, portable under `NO_BACKSLASH_ESCAPES` in a harmless-but-literal way). If your server runs with `NO_BACKSLASH_ESCAPES`, use `ToParameterizedMySQL` instead: parameters travel over the wire protocol and bypass string-literal parsing entirely.

**Case sensitivity follows the column collation.** `LIKE` and `REGEXP` honor the operand collation: a `_ci` collation matches case-insensitively, `_bin` matches case-sensitively. The driver can't fix this without parsing column metadata; if you need explicit casing, attach `BINARY` or `COLLATE` in your query.

**Regex engine varies by database and version.** MySQL 5.7 and MariaDB 10.0-10.4 use Henry Spencer POSIX regex. MySQL 8.0+ uses ICU. MariaDB 10.5+ uses PCRE2. Perl-style escapes (`\d`, `\w`, `\s`) work on ICU and PCRE2 but not on Henry Spencer. For portability across all four, use POSIX bracket classes (`[[:digit:]]`, `[[:space:]]`, `[[:alnum:]_]`).

**Booleans render as `TRUE`/`FALSE` in SQL and pass through as Go `bool` as a parameter.** Both forms evaluate to `1`/`0` in MySQL. Against a `TINYINT(1)` column storing values other than 0 or 1, `col = TRUE` won't match those rows because `TRUE` is exactly `1`. That's a MySQL data-modeling quirk, not a driver bug.

### MariaDB

MariaDB uses the same driver. `lucene.ToMariaDB` and `lucene.ToParameterizedMariaDB` are aliases over the MySQL renderers.

```go
sql, params, err := lucene.ToParameterizedMariaDB(`color:red AND type:"gala"`)
// sql:    (`color` = ?) AND (`type` = ?)
// params: ["red", "gala"]
```

Every construct the driver emits (backtick quoting, `LIKE ... ESCAPE`, `REGEXP`, `BETWEEN`, `?` placeholders, bool literals) is identical on both databases, and the regex fallback avoids Perl extensions so it runs on every regex engine either database has shipped. No new dialect is needed; the MySQL test suite covers MariaDB by swapping `MYSQL_IMAGE=mariadb:10.x`.

## Custom drivers

To target a database other than Postgres, SQLite, or MySQL, embed `driver.Base` and supply a `Dialect` that matches your database's semantics. The dialect covers the operators that actually vary between databases (wildcards, regex, standalone `*`, bool literals, string-literal escaping, identifier quoting); the simple operators (`AND`, `OR`, `=`, comparisons, `IN`, `NOT`) are handled by `driver.Base` through the shared `RenderFNs` map.

The `Dialect` interface has seven methods:

```go
type Dialect interface {
    RenderLike(left, right string, isRegex bool) (string, error)
    RenderStandaloneWild(left string) (string, error)
    PrepareLikePattern(pattern string) (transformed string, useRegex bool)
    EscapeStringLiteral(s string) string
    SerializeBool(b bool) string
    BoolParam(b bool) any
    QuoteColumn(name string) (string, error)
}
```

Here's a sketch of a SQL Server dialect. SQL Server uses `[...]` for identifiers, `LIKE` with `%`/`_` for wildcards, no built-in regex:

```go
import (
    "fmt"
    "strings"

    "github.com/grindlemire/go-lucene/pkg/driver"
    "github.com/grindlemire/go-lucene/pkg/lucene/expr"
)

type SQLServerDriver struct {
    driver.Base
}

func NewSQLServerDriver() SQLServerDriver {
    fns := map[expr.Operator]driver.RenderFN{}
    for op, sharedFN := range driver.Shared {
        fns[op] = sharedFN
    }
    return SQLServerDriver{
        Base: driver.Base{RenderFNs: fns, Dialect: sqlServerDialect{}},
    }
}

type sqlServerDialect struct{}

func (sqlServerDialect) RenderLike(left, right string, isRegex bool) (string, error) {
    if isRegex {
        return "", fmt.Errorf("SQL Server has no built-in regex operator")
    }
    return fmt.Sprintf("%s LIKE %s", left, right), nil
}

func (sqlServerDialect) RenderStandaloneWild(left string) (string, error) {
    return fmt.Sprintf("%s IS NOT NULL", left), nil
}

func (sqlServerDialect) PrepareLikePattern(pattern string) (string, bool) {
    // SQL Server LIKE uses % and _ like SIMILAR TO; escape literals with [].
    pattern = strings.ReplaceAll(pattern, "%", "[%]")
    pattern = strings.ReplaceAll(pattern, "_", "[_]")
    pattern = strings.ReplaceAll(pattern, "*", "%")
    pattern = strings.ReplaceAll(pattern, "?", "_")
    return pattern, false
}

func (sqlServerDialect) EscapeStringLiteral(s string) string {
    return "'" + strings.ReplaceAll(s, "'", "''") + "'"
}

func (sqlServerDialect) SerializeBool(b bool) string {
    if b {
        return "1"
    }
    return "0"
}

func (sqlServerDialect) BoolParam(b bool) any {
    if b {
        return 1
    }
    return 0
}

func (sqlServerDialect) QuoteColumn(name string) (string, error) {
    if strings.ContainsRune(name, ']') {
        return "", fmt.Errorf("column name contains a right bracket: %q", name)
    }
    return "[" + name + "]", nil
}
```

The three built-in drivers are the best reference implementations: [`pkg/driver/postgresql.go`](pkg/driver/postgresql.go), [`pkg/driver/sqlite.go`](pkg/driver/sqlite.go), and [`pkg/driver/mysql.go`](pkg/driver/mysql.go).

### Dialect defaults

A driver that leaves `driver.Base.Dialect` unset inherits Postgres-flavored rendering: `SIMILAR TO` for wildcards, `~` for regex, and `true`/`false` for bool literals. Set a `Dialect` on the embedded `Base` whenever your target database diverges from that.

### Migrating from pre-Dialect drivers

`expr.Like`, `expr.Range`, and `expr.Regexp` are no longer dispatched through `RenderFNs`. If a custom driver used to override any of those three through the map, move the logic into a `driver.Dialect` implementation (`RenderLike`, `RenderStandaloneWild`, `PrepareLikePattern`) and set it on `Base.Dialect`. Map entries for those operators are silently ignored.

The `Dialect` interface evolved across releases: `EscapeLikePattern` was replaced by `PrepareLikePattern` (which additionally returns a `useRegex` flag so dialects can route alternation/grouping patterns to a regex path), and `EscapeStringLiteral` was added so dialects can control string-literal quoting (MySQL needs this to double backslashes under default `sql_mode`).
