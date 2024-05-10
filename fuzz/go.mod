module github.com/grindlemire/go-lucene/fuzz

go 1.22

require (
	github.com/grindlemire/go-lucene v0.0.14
	github.com/pganalyze/pg_query_go/v4 v4.2.3
)

require (
	github.com/golang/protobuf v1.4.2 // indirect
	google.golang.org/protobuf v1.23.0 // indirect
)

// Always just use the current version of go-lucene
replace github.com/grindlemire/go-lucene => ../
