package main

import (
	"fmt"
	"os"

	"github.com/chaserensberger/wingman/models/catalog"
)

func main() {
	snapshot, err := catalog.CompileDir("data")
	if err != nil {
		fmt.Fprintf(os.Stderr, "compile catalog: %v\n", err)
		os.Exit(1)
	}
	if err := catalog.WriteSnapshot("wingmodels_snapshot.json", snapshot); err != nil {
		fmt.Fprintf(os.Stderr, "write snapshot: %v\n", err)
		os.Exit(1)
	}
}
