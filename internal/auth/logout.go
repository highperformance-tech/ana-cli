package auth

import (
	"context"
	"fmt"

	"github.com/highperformance-tech/ana-cli/internal/cli"
)

// logoutCmd clears the token from the persisted config while preserving the
// endpoint. Running logout on a fresh install is a no-op that still writes
// the file — acceptable because SaveCfg is idempotent.
type logoutCmd struct{ deps Deps }

// Help is fixed and self-contained.
func (c *logoutCmd) Help() string {
	return "logout   Clear the saved API token.\n" +
		"Usage: ana auth logout"
}

// Run loads the config, zeroes the token, and saves.
func (c *logoutCmd) Run(ctx context.Context, args []string, stdio cli.IO) error {
	fs := newFlagSet("auth logout")
	if err := parseFlags(fs, args); err != nil {
		return err
	}
	if fs.NArg() != 0 {
		return usageErrf("auth logout: unexpected arguments")
	}
	cfg, err := c.deps.LoadCfg()
	if err != nil {
		return fmt.Errorf("auth logout: load config: %w", err)
	}
	cfg.Token = ""
	if err := c.deps.SaveCfg(cfg); err != nil {
		return fmt.Errorf("auth logout: save config: %w", err)
	}
	fmt.Fprintln(stdio.Stdout, "logged out")
	return nil
}
