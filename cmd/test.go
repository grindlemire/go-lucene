package main

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/grindlemire/go-lucene"
	"github.com/grindlemire/go-lucene/pkg/lucene/expr"
)

func main() {
	e, err := lucene.Parse(os.Args[1])
	if err != nil {
		fmt.Printf("Error parsing: %s\n", err)
		os.Exit(1)
	}

	fmt.Printf("Parsed input: %s\n", e)

	s, err := json.MarshalIndent(e, "", "  ")
	if err != nil {
		fmt.Printf("Error marshalling to json: %s\n", err)
		os.Exit(1)
	}

	fmt.Printf("\n%s\n", s)

	var e1 expr.Equals
	err = json.Unmarshal(s, &e1)
	if err != nil {
		fmt.Printf("Error unmarshalling to json: %s\n", err)
		os.Exit(1)
	}

	fmt.Printf("Reparsed input: %v\n", e1)
}
