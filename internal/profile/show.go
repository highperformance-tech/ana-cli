package profile

import (
	"context"
	"fmt"
	"text/tabwriter"

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
	fs := newFlagSet("profile show")
	if err := parseFlags(fs, args); err != nil {
		return err
	}

	cfg, err := c.deps.LoadCfg()
	if err != nil {
		return fmt.Errorf("profile show: %w", err)
	}

	var name string
	rest := fs.Args()
	if len(rest) > 0 && rest[0] != "" {
		name = rest[0]
	} else {
		name = cfg.Active
	}
	p, ok := cfg.Profiles[name]
	if !ok {
		return fmt.Errorf("profile show: %w: %q", config.ErrUnknownProfile, name)
	}

	if cli.GlobalFrom(ctx).JSON {
		return writeJSON(stdio.Stdout, showPayload{
			Name:     name,
			Active:   name == cfg.Active,
			Endpoint: p.Endpoint,
			OrgName:  p.OrgName,
			HasToken: p.Token != "",
		})
	}

	tw := tabwriter.NewWriter(stdio.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintf(tw, "name\t%s\n", name)
	fmt.Fprintf(tw, "active\t%t\n", name == cfg.Active)
	fmt.Fprintf(tw, "endpoint\t%s\n", p.Endpoint)
	fmt.Fprintf(tw, "orgName\t%s\n", p.OrgName)
	fmt.Fprintf(tw, "token\t%s\n", redactToken(p.Token))
	return tw.Flush()
}

// redactToken returns a user-facing display for a token. Empty tokens print
// "(unset)" so operators can see the slot needs an `ana auth login`; any
// other value shows a fixed mask plus the last four characters to make it
// possible to disambiguate two tokens at a glance without leaking them.
func redactToken(tok string) string {
	if tok == "" {
		return "(unset)"
	}
	if len(tok) < 4 {
		return "********** (last 4: " + tok + ")"
	}
	return "********** (last 4: " + tok[len(tok)-4:] + ")"
}
