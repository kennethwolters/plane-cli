// The plane-cli command provides agent-oriented Plane.so project management workflows.
package main

import (
	"context"
	"os"

	"github.com/kennethwolters/plane-cli/internal/cli"
)

var (
	version = "dev"
	commit  = ""
	date    = ""
)

func main() {
	build := cli.BuildInfo{
		Version:   version,
		Commit:    commit,
		BuildDate: date,
	}
	os.Exit(cli.Run(context.Background(), os.Args[1:], os.Stdout, os.Stderr, build))
}
