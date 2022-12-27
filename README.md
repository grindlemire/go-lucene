# go-lucene

A search string parser for Go programs that will generate SQL for you and has no external dependencies.

Out of the box it supports [Apache Lucene](https://lucene.apache.org/core/9_4_2/queryparser/org/apache/lucene/queryparser/classic/package-summary.html#package.description) but it can be extended to other syntaxes as well. It uses a bottom up grammar parser using the [shift reduce](https://en.wikipedia.org/wiki/Shift-reduce_parser) method.

# Usage

```go
// suppose you want a query for a red apple that is not a honey crisp and is younger than 5 months old
myQuery := `color:red AND (NOT type:"honey crisp" OR age_in_months:[5 TO *])`
expression, err := search.Parse(myQuery)
if err != nil {
    // handle error
}

filter, err := search.NewSQLDriver().Render(expression)
if err != nil {
    // handle error
}

SQLTemplate := `
SELECT *
FROM apples
WHERE %s
LIMIT 10;
`
mySQLQuery := fmt.Sprintf(SQLTemplate, filter)

// mySQLQuery is:
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

This can also plug directly into an ORM without much issue.
