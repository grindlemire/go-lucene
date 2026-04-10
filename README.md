# go-lucene

[![Go Reference](https://pkg.go.dev/badge/github.com/grindlemire/go-lucene.svg)](https://pkg.go.dev/github.com/grindlemire/go-lucene)

A zero-dependency Lucene query parser for Go that converts Lucene syntax into SQL queries.

## Features

- Full Lucene syntax support (compatible with [Apache Lucene 9.4.2](https://lucene.apache.org/core/9_4_2/queryparser/org/apache/lucene/queryparser/classic/package-summary.html#package.description))
- SQL injection safe with parameterized queries
- Zero dependencies
- Extensible with custom SQL drivers
- PostgreSQL and SQLite support out of the box

## Installation

```bash
go get github.com/grindlemire/go-lucene
```

## Basic Usage

```go
query := `name:"John Doe" AND age:[25 TO 35]`
filter, err := lucene.ToPostgres(query)
// Result: (("name" = 'John Doe') AND ("age" >= 25 AND "age" <= 35))
```

```go
filter, err := lucene.ToSQLite(query)
// Result: (("name" = 'John Doe') AND ("age" >= 25 AND "age" <= 35))
```

## API Methods

### Direct SQL Generation

```go
filter, err := lucene.ToPostgres(query)
```

### Parameterized Queries (Recommended)

```go
filter, params, err := lucene.ToParameterizedPostgres(query)
db.Query(sql, params...)
```

### SQLite

```go
filter, err := lucene.ToSQLite(query)

filter, params, err := lucene.ToParameterizedSQLite(query)
db.Query(filter, params...)
```

**SQLite notes:**

- Wildcards render as `GLOB` (case-sensitive, Unix glob syntax). Lucene's `*` and `?` map directly to GLOB's `*` and `?`.
- GLOB has no escape mechanism. If you need to match a literal `*` or `?`, use the regex form (`field:/.../`) instead.
- Regular expressions (`field:/pattern/`) render as `REGEXP`. SQLite does not provide a `regexp()` function by default, so you must register one on your connection. With `modernc.org/sqlite` that looks like:

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

  With `mattn/go-sqlite3`, build with the `sqlite_regex` tag. Without registration, regex queries error at query time.

- A standalone wildcard `field:*` renders as `"field" IS NOT NULL`, which matches any row where the field has a non-null value, regardless of storage class.
- GLOB does not support alternation. A wildcard pattern like `field:*(a|b)*` will match the **literal characters** `(a|b)`, not "a or b" as it would with Postgres's `SIMILAR TO`. Use the regex form `field:/.*(a|b).*/` for alternation in SQLite.
- Parameter placeholders are `?`, not `$1, $2, ...` as with Postgres.

### Default Fields

```go
filter, err := lucene.ToPostgres("red OR green", lucene.WithDefaultField("color"))
// Result: ("color" = 'red') OR ("color" = 'green')
```

## Lucene to SQL Operator Mapping

| Lucene Query | SQL Output | Description |
|--------------|------------|-------------|
| `field:value` | `"field" = 'value'` | Exact match |
| `field:"phrase with spaces"` | `"field" = 'phrase with spaces'` | Quoted phrase |
| `field1:value1 AND field2:value2` | `("field1" = 'value1') AND ("field2" = 'value2')` | Boolean AND |
| `field1:value1 OR field2:value2` | `("field1" = 'value1') OR ("field2" = 'value2')` | Boolean OR |
| `NOT field:value` | `NOT("field" = 'value')` | Boolean NOT |
| `+field:value` | `"field" = 'value'` | Required (equivalent to no operator) |
| `-field:value` | `NOT("field" = 'value')` | Prohibited (equivalent to NOT) |
| `field:[min TO max]` | `"field" >= min AND "field" <= max` | Inclusive range |
| `field:{min TO max}` | `"field" > min AND "field" < max` | Exclusive range |
| `field:[min TO *]` | `"field" >= min` | Open-ended range (min to infinity) |
| `field:[* TO max]` | `"field" <= max` | Open-ended range (negative infinity to max) |
| `field:*` | `"field" SIMILAR TO '%'` | Wildcard match (matches anything) |
| `field:pattern*` | `"field" SIMILAR TO 'pattern%'` | Wildcard suffix |
| `field:pattern?` | `"field" SIMILAR TO 'pattern_'` | Single character wildcard |
| `field:/regex/` | `"field" ~ 'regex'` | Regular expression match |
| `(field1:value1 OR field2:value2) AND field3:value3` | `(("field1" = 'value1') OR ("field2" = 'value2')) AND ("field3" = 'value3')` | Grouping |

### SQLite differences

For the SQLite driver, these operators render differently than the Postgres defaults shown above:

| Lucene Query | SQLite Output |
|---|---|
| `field:*` | `"field" IS NOT NULL` |
| `field:pattern*` | `"field" GLOB 'pattern*'` |
| `field:pattern?` | `"field" GLOB 'pattern?'` |
| `field:/regex/` | `"field" REGEXP 'regex'` (requires registered `regexp()` function) |
| parameter placeholders | `?` (not `$1, $2, ...`) |

## Examples

### Complex Query

```go
query := `name:"John Doe" AND age:[25 TO 35] AND NOT status:inactive`
// SQL: (("name" = 'John Doe') AND ("age" >= 25 AND "age" <= 35)) AND (NOT("status" = 'inactive'))
```

### Parameterized Output

```go
filter, params, err := lucene.ToParameterizedPostgres(`color:red AND type:"gala"`)
// SQL: ("color" = $1) AND ("type" = $2)
// Params: ["red", "gala"]
```

### Wildcard Queries

```go
filter, err := lucene.ToPostgres(`name:John* AND email:*@example.com`)
// SQL: ("name" SIMILAR TO 'John%') AND ("email" SIMILAR TO '%@example.com')
```

### Regular Expression Queries

```go
filter, err := lucene.ToPostgres(`url:/example\.com\/.*/`)
// SQL: "url" ~ 'example\.com\/.*'
```

## Custom SQL Drivers

Extend the library for different SQL dialects by creating custom drivers:

```go
import (
    "github.com/grindlemire/go-lucene/pkg/driver"
    "github.com/grindlemire/go-lucene/pkg/lucene/expr"
)

type MySQLDriver struct {
    driver.Base
}

func NewMySQLDriver() MySQLDriver {
    fns := map[expr.Operator]driver.RenderFN{
        expr.Equals: func(left, right string) (string, error) {
            return fmt.Sprintf("`%s` = %s", left, right), nil
        },
    }

    // Use shared functions for other operators
    for op, sharedFN := range driver.Shared {
        if _, exists := fns[op]; !exists {
            fns[op] = sharedFN
        }
    }

    return MySQLDriver{Base: driver.Base{RenderFNs: fns}}
}

// Usage
mysqlDriver := NewMySQLDriver()
expr, _ := lucene.Parse(`color:red`)
filter, _ := mysqlDriver.Render(expr)
// Result: `color` = 'red'
```

**Note on dialect behavior:** A custom driver that leaves `driver.Base.Dialect` unset inherits Postgres-flavored rendering for Like, Range, standalone wildcards, pattern escaping, and bool literals. That means `field:pat*` renders as `SIMILAR TO 'pat%'`, `field:/regex/` renders with `~`, `field:*` renders as `SIMILAR TO '%'`, and bool literals render as `true`/`false`. If your target database needs different semantics, supply your own `driver.Dialect` implementation on the embedded `Base`. The built-in `SQLiteDriver` is a reference example: it sets a `Dialect` that emits `GLOB`, `REGEXP`, `IS NOT NULL`, and `1`/`0` respectively.
