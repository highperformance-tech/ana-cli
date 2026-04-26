package profile

import (
	"context"
	"fmt"

	"github.com/highperformance-tech/ana-cli/internal/cli"
	"github.com/highperformance-tech/ana-cli/internal/config"
)

// showCmd prints one profile's details. Tokens are always redacted — even
// under --json — because this CLI must never emit a raw token except into
// the config file it owns. If you need the token, read the file yourself.
type showCmd struct{ deps Deps }

func (c *showCmd) Help() string {
	return "show   Print a single profile's details (active by default).\n" +
		"Usage: ana profile show [<name>]"
}

// showPayload is the --json shape. Endpoint/OrgName come straight from the
// stored profile; the token is represented only by hasToken so consumers can
// detect presence without ever seeing the value.
type showPayload struct {
	Name     string `json:"name"`
	Active   bool   `json:"active"`
	Endpoint string `json:"endpoint"`
	OrgName  string `json:"orgName,omitempty"`
	HasToken bool   `json:"hasToken"`
}

func (c *showCmd) Run(ctx context.Context, args []string, stdio cli.IO) error {
	cfg, err := c.deps.LoadCfg()
	if err != nil {
		return fmt.Errorf("profile show: %w", err)
	}

	var name string
	if len(args) > 0 && args[0] != "" {
		name = args[0]
	} else {
		name = cfg.Active
	}
	p, ok := cfg.Profiles[name]
	if !ok {
		return fmt.Errorf("profile show: %w: %q", config.ErrUnknownProfile, name)
	}

	if cli.GlobalFrom(ctx).JSON {
		return cli.WriteJSON(stdio.Stdout, showPayload{
			Name:     name,
			Active:   name == cfg.Active,
			Endpoint: p.Endpoint,
			OrgName:  p.OrgName,
			HasToken: p.Token != "",
		})
	}

	tw := cli.NewTableWriter(stdio.Stdout)
	fmt.Fprintf(tw, "name\t%s\n", name)
	fmt.Fprintf(tw, "active\t%t\n", name == cfg.Active)
	fmt.Fprintf(tw, "endpoint\t%s\n", p.Endpoint)
	fmt.Fprintf(tw, "orgName\t%s\n", p.OrgName)
	fmt.Fprintf(tw, "token\t%s\n", p.Token)
	return tw.Flush()
}
