package main

import (
	"fmt"
	"os"

	"github.com/grindlemire/go-lucene"
)

func main() {
	input := "  a = b"
	e, err := lucene.Parse(input)
	if err != nil {
		fmt.Printf("Error parsing: %s\n", err)
		os.Exit(1)
	}

	fmt.Printf("Successfully parsed expression: %s\n", e)
}
