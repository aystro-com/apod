package main

import (
	"os"

	"github.com/aystro/apod/internal/cli"
	"github.com/aystro/apod/internal/engine"
)

var version = "dev"

func main() {
	engine.Version = version
	if err := cli.Execute(); err != nil {
		os.Exit(1)
	}
}
