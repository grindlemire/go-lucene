# go-lucene

A lucene parser written in go with no dependencies.

With this package you can quickly integrate lucene style searching inside your app and generate sql filters for a particular query. There are no external dependencies and the grammar fully supports [Apache Lucene 9.4.2](https://lucene.apache.org/core/9_4_2/queryparser/org/apache/lucene/queryparser/classic/package-summary.html#package.description).

Out of the box go-lucene support postgres compliant sql generation but it can be extended to support different flavors of sql (or no sql) as well.

# Usage

```go
// suppose you want a query for a red apple that is not a honey crisp and is younger than 5 months old
myQuery := `color:red AND (NOT type:"honey crisp" OR age_in_months:[5 TO *])`
expression, err := lucene.Parse(myQuery)
if err != nil {
    // handle error
}

filter, err := driver.NewPostgresDriver().Render(expression)
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
    color = red
    AND (
      NOT(type = "honey crisp")
      OR age_in_months >= 5
    )
LIMIT 10;
`
```
