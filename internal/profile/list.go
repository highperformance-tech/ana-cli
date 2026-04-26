package profile

import (
	"context"
	"fmt"
	"slices"

	"github.com/highperformance-tech/ana-cli/internal/cli"
)

// listCmd prints every profile with active marker, endpoint, and org. Tokens
// are NEVER rendered (not even redacted masks) — the table view is a
// discoverability tool; use `ana profile show` for token-presence info.
type listCmd struct{ deps Deps }

func (c *listCmd) Help() string {
	return "list   List all configured profiles.\n" +
		"Usage: ana profile list [--json]"
}

// listEntry is the per-profile --json payload. hasToken lets tooling check
// for presence without the CLI ever exporting a raw token value.
type listEntry struct {
	Name     string `json:"name"`
	Endpoint string `json:"endpoint"`
	OrgName  string `json:"orgName,omitempty"`
	HasToken bool   `json:"hasToken"`
}

type listPayload struct {
	Profiles []listEntry `json:"profiles"`
	Active   string      `json:"active"`
}

func (c *listCmd) Run(ctx context.Context, args []string, stdio cli.IO) error {
	cfg, err := c.deps.LoadCfg()
	if err != nil {
		return fmt.Errorf("profile list: %w", err)
	}

	names := make([]string, 0, len(cfg.Profiles))
	for n := range cfg.Profiles {
		names = append(names, n)
	}
	slices.Sort(names)

	if cli.GlobalFrom(ctx).JSON {
		payload := listPayload{Profiles: make([]listEntry, 0, len(names)), Active: cfg.Active}
		for _, n := range names {
			p := cfg.Profiles[n]
			payload.Profiles = append(payload.Profiles, listEntry{
				Name:     n,
				Endpoint: p.Endpoint,
				OrgName:  p.OrgName,
				HasToken: p.Token != "",
			})
		}
		return cli.WriteJSON(stdio.Stdout, payload)
	}

	tw := cli.NewTableWriter(stdio.Stdout)
	fmt.Fprintln(tw, "NAME\tACTIVE\tENDPOINT\tORG")
	for _, n := range names {
		p := cfg.Profiles[n]
		active := ""
		if n == cfg.Active {
			active = "*"
		}
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n", n, active, p.Endpoint, p.OrgName)
	}
	return tw.Flush()
}
