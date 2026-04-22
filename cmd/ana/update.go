package main

import (
	"context"
	"errors"
	"fmt"

	"github.com/highperformance-tech/ana-cli/internal/cli"
	"github.com/highperformance-tech/ana-cli/internal/update"
)

// updateCmd implements `ana update`: fetches the matching release archive,
// verifies its sha256, and atomically replaces the running binary. Mirrors
// versionCmd's leaf shape — deps are pulled in via the package-level default
// so cmd/ana keeps its "pure wiring" posture.
type updateCmd struct {
	deps update.Deps
}

func (updateCmd) Help() string {
	return "Download and install the latest ana release."
}

func (c updateCmd) Run(ctx context.Context, args []string, stdio cli.IO) error {
	if len(args) > 0 && cli.IsHelpArg(args[0]) {
		fmt.Fprintln(stdio.Stdout, updateCmd{}.Help())
		return cli.ErrHelp
	}
	jsonOut := cli.GlobalFrom(ctx).JSON
	if err := update.SelfUpdate(ctx, c.deps, version, stdio.Stdout, jsonOut); err != nil {
		fmt.Fprintln(stdio.Stderr, err)
		return errors.Join(err, cli.ErrReported)
	}
	return nil
}
