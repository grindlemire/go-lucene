# go-lucene

[![Go Reference](https://pkg.go.dev/badge/github.com/grindlemire/go-lucene.svg)](https://pkg.go.dev/github.com/grindlemire/go-lucene)

A zero-dependency Lucene query parser for Go that converts Lucene syntax into SQL queries.

## Features

- Full Lucene syntax support (compatible with [Apache Lucene 9.4.2](https://lucene.apache.org/core/9_4_2/queryparser/org/apache/lucene/queryparser/classic/package-summary.html#package.description))
- SQL injection safe with parameterized queries
- Zero dependencies
- Extensible with custom SQL drivers
- PostgreSQL support out of the box

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
| `field:{min TO max}` | `"field" BETWEEN 'min' AND 'max'` (strings) or `"field" > min AND "field" < max` (numbers) | Exclusive range |
| `field:[min TO *]` | `"field" >= min` | Open-ended range (min to infinity) |
| `field:[* TO max]` | `"field" <= max` | Open-ended range (negative infinity to max) |
| `field:*` | `"field" SIMILAR TO '%'` | Wildcard match (matches anything) |
| `field:pattern*` | `"field" SIMILAR TO 'pattern%'` | Wildcard suffix |
| `field:pattern?` | `"field" SIMILAR TO 'pattern_'` | Single character wildcard |
| `field:/regex/` | `"field" ~ '/regex/'` | Regular expression match |
| `(field1:value1 OR field2:value2) AND field3:value3` | `(("field1" = 'value1') OR ("field2" = 'value2')) AND ("field3" = 'value3')` | Grouping |

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
