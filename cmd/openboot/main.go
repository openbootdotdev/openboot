package main

import (
	"os"

	"github.com/openbootdotdev/openboot/internal/cli"
)

func main() {
	if err := cli.Execute(); err != nil {
		os.Exit(1)
	}
}
