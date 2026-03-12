package main

import (
	"os"

	"github.com/samuelbailey123/ditto/internal/cli"
)

func main() {
	if err := cli.Execute(); err != nil {
		os.Exit(1)
	}
}
