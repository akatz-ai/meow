package main

import (
	"fmt"
	"os"
)

var version = "dev"

func main() {
	if len(os.Args) > 1 && os.Args[1] == "--version" {
		fmt.Printf("meow %s\n", version)
		return
	}

	fmt.Println("meow: workflow orchestration for AI agents")
	fmt.Println("Run 'meow --help' for usage.")
}
