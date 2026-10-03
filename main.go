package main

import (
	"fmt"
	"os"

	"github.com/huchangjun118/quantum-circuit-sim-go/internal/cli"
)

func main() {
	if err := cli.Run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}
