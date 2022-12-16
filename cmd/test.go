package main

import (
	"fmt"
	"os"

	"github.com/davecgh/go-spew/spew"
	"github.com/grindlemire/go-lucene"
)

func main() {
	input := "a:b AND c:d OR -e:f OR h:[* TO 100] AND NOT i:j"
	e, err := lucene.Parse(input)
	if err != nil {
		fmt.Printf("Error parsing: %s\n", err)
		os.Exit(1)
	}

	spew.Printf("Struct: %#v\n", e)
	fmt.Printf("String: %s\n", e)
}
