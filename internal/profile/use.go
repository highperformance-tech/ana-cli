package profile

import (
	"context"
	"fmt"

	"github.com/highperformance-tech/ana-cli/internal/cli"
	"github.com/highperformance-tech/ana-cli/internal/config"
)

// useCmd flips Config.Active to a named profile. We reuse ErrUnknownProfile
// (same sentinel Resolve emits) so callers can errors.Is across both paths.
type useCmd struct{ deps Deps }

func (c *useCmd) Help() string {
	return "use   Switch the active profile.\n" +
		"Usage: ana profile use <name>"
}

func (c *useCmd) Run(ctx context.Context, args []string, stdio cli.IO) error {
	if len(args) == 0 || args[0] == "" {
		return cli.UsageErrf("profile use: name is required")
	}
	if len(args) > 1 {
		return cli.UsageErrf("profile use: unexpected positional arguments: %v", args[1:])
	}
	name := args[0]

	cfg, err := c.deps.LoadCfg()
	if err != nil {
		return fmt.Errorf("profile use: %w", err)
	}
	if _, ok := cfg.Profiles[name]; !ok {
		return fmt.Errorf("profile use: %w: %q", config.ErrUnknownProfile, name)
	}
	cfg.Active = name
	if err := c.deps.SaveCfg(cfg); err != nil {
		return fmt.Errorf("profile use: save config: %w", err)
	}
	fmt.Fprintf(stdio.Stdout, "active profile: %s\n", name)
	return nil
}
