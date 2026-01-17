package main

import (
	"fmt"
	"os"

	"github.com/akatz-ai/meow/cmd/meow/cmd"
)

func main() {
	if err := cmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
