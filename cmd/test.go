package main

import (
	"fmt"
	"os"

	"github.com/grindlemire/go-lucene"
)

func main() {
	e, err := lucene.Parse(os.Args[1])
	if err != nil {
		fmt.Printf("Error parsing: %s\n", err)
		os.Exit(1)
	}

	fmt.Printf("Parsed input: %s\n", e)
}
