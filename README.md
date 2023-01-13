# go-lucene

A depedency free pure go implementation of a lucene syntax parser that can be used to generate sql filters.

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
