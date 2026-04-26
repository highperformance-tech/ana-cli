package profile

import (
	"context"
	"fmt"

	"github.com/highperformance-tech/ana-cli/internal/cli"
)

// removeCmd deletes a named profile. config.Config.Remove handles the active
// pointer bookkeeping; we only need to translate its bool return into the
// user-visible "not found" error and choose the right success message.
type removeCmd struct{ deps Deps }

func (c *removeCmd) Help() string {
	return "remove   Delete a named profile.\n" +
		"Usage: ana profile remove <name>"
}

func (c *removeCmd) Run(ctx context.Context, args []string, stdio cli.IO) error {
	if len(args) == 0 || args[0] == "" {
		return cli.UsageErrf("profile remove: name is required")
	}
	name := args[0]

	cfg, err := c.deps.LoadCfg()
	if err != nil {
		return fmt.Errorf("profile remove: %w", err)
	}
	if !cfg.Remove(name) {
		return fmt.Errorf("profile remove: profile %q not found", name)
	}
	if err := c.deps.SaveCfg(cfg); err != nil {
		return fmt.Errorf("profile remove: save config: %w", err)
	}
	if cfg.Active == "" {
		fmt.Fprintf(stdio.Stdout, "removed profile %s; no profiles remain\n", name)
		return nil
	}
	fmt.Fprintf(stdio.Stdout, "removed profile %s; active profile is now %s\n", name, cfg.Active)
	return nil
}
