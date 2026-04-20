package main

import (
	"os"

	"github.com/aystro/apod/internal/cli"
)

func main() {
	if err := cli.Execute(); err != nil {
		os.Exit(1)
	}
}
