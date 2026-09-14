package main

import (
	"fmt"
	"os"

	"github.com/Sinnaminty/tmig/internal/cli"
)

func main() {
	if err := cli.Run(os.Args[1:], os.Stdout, os.Stderr); err != nil {
		fmt.Fprintln(os.Stderr, "tmig:", err)
		os.Exit(1)
	}
}
