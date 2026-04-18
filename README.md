# go-lucene

[![Go Reference](https://pkg.go.dev/badge/github.com/grindlemire/go-lucene.svg)](https://pkg.go.dev/github.com/grindlemire/go-lucene)

Parse [Lucene](https://lucene.apache.org/core/9_4_2/queryparser/org/apache/lucene/queryparser/classic/package-summary.html#package.description) queries and turn them into SQL. No dependencies. PostgreSQL and SQLite work out of the box, and you can plug in your own dialect for anything else.

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

### Inline values

If you don't need parameter binding (for example, when generating SQL for inspection), `ToPostgres` and `ToSQLite` embed values directly into the string:

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

Output below is Postgres. See [SQLite](#sqlite) for where SQLite differs.

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

## Custom drivers

To target a database other than Postgres or SQLite, embed `driver.Base` and supply a `Dialect` that matches your database's semantics. The dialect covers the operators that actually vary between databases (wildcards, regex, standalone `*`, bool literals, identifier quoting); the simple operators (`AND`, `OR`, `=`, comparisons, `IN`, `NOT`) are handled by `driver.Base` through the shared `RenderFNs` map.

Here's a MySQL driver. MySQL uses backticks for identifiers, `LIKE` (not `SIMILAR TO`) for wildcards, and `REGEXP` for regex:

```go
import (
    "fmt"
    "strings"

    "github.com/grindlemire/go-lucene/pkg/driver"
)

type MySQLDriver struct {
    driver.Base
}

func NewMySQLDriver() MySQLDriver {
    fns := map[expr.Operator]driver.RenderFN{}
    for op, sharedFN := range driver.Shared {
        fns[op] = sharedFN
    }
    return MySQLDriver{
        Base: driver.Base{
            RenderFNs: fns,
            Dialect:   mysqlDialect{},
        },
    }
}

type mysqlDialect struct{}

func (mysqlDialect) RenderLike(left, right string, isRegex bool) (string, error) {
    if isRegex {
        return fmt.Sprintf("%s REGEXP %s", left, right), nil
    }
    return fmt.Sprintf("%s LIKE %s", left, right), nil
}

func (mysqlDialect) RenderStandaloneWild(left string) (string, error) {
    return fmt.Sprintf("%s IS NOT NULL", left), nil
}

// Lucene * and ? map onto SQL LIKE's % and _.
func (mysqlDialect) EscapeLikePattern(pattern string) string {
    r := strings.NewReplacer("*", "%", "?", "_")
    return r.Replace(pattern)
}

func (mysqlDialect) SerializeBool(b bool) string {
    if b {
        return "1"
    }
    return "0"
}

func (mysqlDialect) BoolParam(b bool) any {
    if b {
        return 1
    }
    return 0
}

func (mysqlDialect) QuoteColumn(name string) (string, error) {
    if strings.ContainsRune(name, '`') {
        return "", fmt.Errorf("column name contains a backtick: %q", name)
    }
    return "`" + name + "`", nil
}
```

Usage:

```go
d := NewMySQLDriver()
e, _ := lucene.Parse(`name:John* AND active:true`)
sql, _ := d.Render(e)
// (`name` LIKE 'John%') AND (`active` = 1)
```

The built-in `SQLiteDriver` ([pkg/driver/sqlite.go](pkg/driver/sqlite.go)) is another reference implementation worth reading.

### Dialect defaults

A driver that leaves `driver.Base.Dialect` unset inherits Postgres-flavored rendering: `SIMILAR TO` for wildcards, `~` for regex, and `true`/`false` for bool literals. Set a `Dialect` on the embedded `Base` whenever your target database diverges from that.

### Migrating from pre-Dialect drivers

`expr.Like`, `expr.Range`, and `expr.Regexp` are no longer dispatched through `RenderFNs`. If a custom driver used to override any of those three through the map, move the logic into a `driver.Dialect` implementation (`RenderLike`, `RenderStandaloneWild`, `EscapeLikePattern`) and set it on `Base.Dialect`. Map entries for those operators are silently ignored.
