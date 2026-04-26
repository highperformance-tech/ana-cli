package profile

import (
	"context"
	"flag"
	"fmt"

	"github.com/highperformance-tech/ana-cli/internal/cli"
	"github.com/highperformance-tech/ana-cli/internal/config"
)

// addCmd inserts-or-replaces a named profile. We don't split add/edit: the
// caller almost always knows which slot they want, and silently overwriting
// is friendlier than forcing them to remove+add on re-login.
//
// `--endpoint` here is a LOCAL flag declared via Flagger — it shadows the
// root-level `--endpoint` global in the resolver's merged FlagSet so the
// value the user supplied lands on the new profile, not on the transport
// override the rest of the invocation would otherwise inherit.
type addCmd struct {
	deps Deps

	endpoint   string
	org        string
	tokenStdin bool
}

func (c *addCmd) Help() string {
	return "add   Create or overwrite a named profile.\n" +
		"Usage: ana profile add <name> [--endpoint URL] [--org NAME] [--token-stdin]\n" +
		"Reads the token from stdin (one line by default, or the full stream with --token-stdin)."
}

func (c *addCmd) Flags(fs *flag.FlagSet) {
	fs.StringVar(&c.endpoint, "endpoint", "", "API endpoint URL (defaults to https://app.textql.com)")
	fs.StringVar(&c.org, "org", "", "human-readable org label")
	fs.BoolVar(&c.tokenStdin, "token-stdin", false, "read entire stdin as the token (trimmed)")
}

func (c *addCmd) Run(ctx context.Context, args []string, stdio cli.IO) error {
	if len(args) == 0 || args[0] == "" {
		return cli.UsageErrf("profile add: name is required")
	}
	name := args[0]

	token, err := cli.ReadToken(stdio.Stdin, c.tokenStdin)
	if err != nil {
		return fmt.Errorf("profile add: %w", err)
	}

	cfg, err := c.deps.LoadCfg()
	if err != nil {
		return fmt.Errorf("profile add: load config: %w", err)
	}

	ep := c.endpoint
	if ep == "" {
		ep = config.DefaultEndpoint
	}
	cfg.Upsert(name, config.Profile{
		Endpoint: ep,
		Token:    cli.Token(token),
		OrgName:  c.org,
	})
	if err := c.deps.SaveCfg(cfg); err != nil {
		return fmt.Errorf("profile add: save config: %w", err)
	}

	path, err := c.deps.ConfigPath()
	if err != nil {
		fmt.Fprintf(stdio.Stdout, "saved profile %s\n", name)
		return fmt.Errorf("profile add: config path: %w", err)
	}
	fmt.Fprintf(stdio.Stdout, "saved profile %s to %s\n", name, path)
	return nil
}
