package main

import (
	"os"

	"github.com/awood45/grimoire-cli/internal/cli"
)

func main() {
	cmd := cli.NewRootCommand()
	if err := cmd.Execute(); err != nil {
		os.Exit(1)
	}
}
