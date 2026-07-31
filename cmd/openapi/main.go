package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/chaserensberger/wingman/server"
)

func main() {
	output := flag.String("output", "", "write the OpenAPI document to this file")
	flag.Parse()

	document, err := server.OpenAPIDocument()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	document = append(document, '\n')
	if *output == "" {
		_, err = os.Stdout.Write(document)
	} else {
		err = os.WriteFile(*output, document, 0o644)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
