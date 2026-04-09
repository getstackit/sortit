package main

import (
	"os"

	"sortit/internal/cli"
)

var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

func main() {
	os.Exit(run())
}

func run() int {
	rootCmd := cli.NewRootCmd(version, commit, date)
	if err := rootCmd.Execute(); err != nil {
		return 1
	}
	return 0
}
