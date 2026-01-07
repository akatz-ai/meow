package main

import (
	"os"

	"github.com/meow-stack/meow-machine/cmd/meow/cmd"
)

func main() {
	if err := cmd.Execute(); err != nil {
		os.Exit(1)
	}
}
