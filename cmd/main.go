package main

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/grindlemire/go-lucene"
	"github.com/grindlemire/go-lucene/pkg/driver"
	"github.com/grindlemire/go-lucene/pkg/lucene/expr"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Printf("Please provide a lucene query\n")
		os.Exit(1)
	}

	e, err := lucene.Parse(os.Args[1])
	if err != nil {
		fmt.Printf("Error parsing: %s\n", err)
		os.Exit(1)
	}

	fmt.Printf("Parsed  input: %s\n", e)
	fmt.Printf("Verbose input: %#v\n", e)

	s, err := json.MarshalIndent(e, "", "  ")
	if err != nil {
		fmt.Printf("Error marshalling to json: %s\n", err)
		os.Exit(1)
	}

	fmt.Printf("\n%s\n", s)

	var e1 expr.Expression
	err = json.Unmarshal(s, &e1)
	if err != nil {
		fmt.Printf("Error unmarshalling from json: %s\n", err)
		os.Exit(1)
	}

	sq, err := driver.NewPostgresDriver().Render(e)
	if err != nil {
		fmt.Printf("Error rendering sql: %s\n", err)
		os.Exit(1)
	}

	fmt.Printf("Reparsed input: %v\n", e1)
	fmt.Printf("Verbose  input: %#v\n", e1)
	fmt.Printf("SQL     output: %s\n", sq)
}
