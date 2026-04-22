package main

import (
	"context"
	"fmt"

	"github.com/highperformance-tech/ana-cli/internal/cli"
)

// Module-level vars populated by GoReleaser via -ldflags -X at build time.
// Defaults keep `go build`/`go run` from a source checkout identifiable
// without requiring the build toolchain to set them.
var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

// versionCmd implements the `ana version` verb. It is a leaf Command whose
// only job is to print the build metadata injected into the main package.
type versionCmd struct{}

func (versionCmd) Help() string {
	return "Print ana version, commit, and build date."
}

func (versionCmd) Run(_ context.Context, args []string, stdio cli.IO) error {
	if len(args) > 0 && cli.IsHelpArg(args[0]) {
		fmt.Fprintln(stdio.Stdout, versionCmd{}.Help())
		return cli.ErrHelp
	}
	fmt.Fprintf(stdio.Stdout, "ana version %s (%s) built %s\n", version, commit, date)
	return nil
}
