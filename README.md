# go-lucene
[![Go Reference](https://pkg.go.dev/badge/github.com/grindlemire/go-lucene.svg)](https://pkg.go.dev/github.com/grindlemire/go-lucene)

A lucene parser written in go with no dependencies.

With this package you can quickly integrate lucene style searching inside your app and generate sql filters for a particular query. There are no external dependencies and the grammar fully supports [Apache Lucene 9.4.2](https://lucene.apache.org/core/9_4_2/queryparser/org/apache/lucene/queryparser/classic/package-summary.html#package.description).

Out of the box go-lucene support postgres compliant sql generation but it can be extended to support different flavors of sql (or no sql) as well.

# Usage
Use `lucene.ToPostgres` to generate a single string with all your filters in it.

Use `lucene.ToParmeterizedPostgres` to generate a parameterized string with args that you can pass to your DB query.

Use the `lucene.WithDefaultField` option to associate bare literals with a field.


## Pure String
```go
// suppose you want a query for red apples that are not honey crisp or granny smith and are older than 5 months old
myQuery := `color:red AND NOT (type:"honey crisp" OR type:"granny smith") AND age_in_months:[5 TO *]`
filter, err := lucene.ToPostgres(myQuery)
if err != nil {
    // handle error
}

SQLTemplate := `
SELECT *
FROM apples
WHERE %s
LIMIT 10;
`
sqlQuery := fmt.Sprintf(SQLTemplate, filter)

// sqlQuery is:
`

SELECT *
FROM apples
WHERE
    (
		("color" = 'red') AND 
		(
			NOT(
				("type" = 'honey crisp') OR 
				("type" = 'granny smith')
			)
		)
	) AND 
	("age_in_months" >= 5)
LIMIT 10;
`
```

## Parameterized Input
```go
// suppose you want a query for red apples that are not honey crisp or granny smith and are older than 5 months old
myQuery := `color:red AND NOT (type:"honey crisp" OR type:"granny smith") AND age_in_months:[5 TO *]`
filter, params, err := lucene.ToParameterizedPostgres(myQuery)
if err != nil {
    // handle error
}

SQLTemplate := `
SELECT *
FROM apples
WHERE %s
LIMIT 10;
`
sqlQuery := fmt.Sprintf(SQLTemplate, filter)

db.Query(sqlQuery, params)
```

## Default Fields
```go
myQuery := "red OR green"
filter, err := lucene.ToPostgres(
	myQuery, lucene.WithDefaultField("appleColor"))
if err != nil {
    // handle error
}

SQLTemplate := `
SELECT *
FROM apples
WHERE %s
LIMIT 10;
`
sqlQuery := fmt.Sprintf(SQLTemplate, filter)

// sqlQuery is:
`
SELECT *
FROM apples
WHERE
    ("appleColor" = 'red') OR ("appleColor" = 'green')
LIMIT 10;
`
```

## Extending with a custom driver

Just embed the `Base` driver in your custom driver and override the `RenderFN`'s with your own custom rendering functions.

```Go
import (
	"github.com/grindlemire/go-lucene"
	"github.com/grindlemire/go-lucene/pkg/driver"
	"github.com/grindlemire/go-lucene/pkg/lucene/expr"
)

type MyDriver struct {
	driver.Base
}
// Suppose we want a customer driver that is postgres but uses "==" rather than "=" for an equality check.
func NewMyDriver() MyDriver {
	// register your new custom render functions. Each render function
	// takes a left and optionally right rendered string and returns the rendered
	// output string for the entire expression.
	fns := map[expr.Operator]driver.RenderFN{
		expr.Equals: myEquals,
	}

	// iterate over the existing base render functions and swap out any that you want to
	for op, sharedFN := range driver.Shared {
		_, found := fns[op]
		if !found {
			fns[op] = sharedFN
		}
	}

	// return the new driver ready to use
	return MyDriver{
		driver.Base{
			RenderFNs: fns,
		},
	}
}

// Suppose we wanted to implement equals using a "==" operator instead of "="
func myEquals(left, right string) (string, error) {
	return left + " == " + right, nil
}

...
func main() {
	// create a new instance of the driver
	driver := NewMyDriver()

	// render an expression
	expr, _ := lucene.Parse(`color:red AND NOT (type:"honey crisp" OR type:"granny smith") AND age_in_months:[5 TO *]`)
	filter, _ := driver.Render(expr)
	
	SQLTemplate := `
	SELECT *
	FROM apples
	WHERE %s
	LIMIT 10;
	`
	sqlQuery := fmt.Sprintf(SQLTemplate, filter)

	// sqlQuery is:
	`

	SELECT *
	FROM apples
	WHERE
		(
			("color" == 'red') AND 
			(
				NOT(
					("type" == 'honey crisp') OR 
					("type" == 'granny smith')
				)
			)
		) AND 
		("age_in_months" >= 5)
	LIMIT 10;
	`
}
```
