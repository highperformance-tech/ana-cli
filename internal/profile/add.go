package profile

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/highperformance-tech/ana-cli/internal/cli"
	"github.com/highperformance-tech/ana-cli/internal/config"
)

// addCmd inserts-or-replaces a named profile. We don't split add/edit: the
// caller almost always knows which slot they want, and silently overwriting
// is friendlier than forcing them to remove+add on re-login.
type addCmd struct{ deps Deps }

func (c *addCmd) Help() string {
	return "add   Create or overwrite a named profile.\n" +
		"Usage: ana profile add <name> [--endpoint URL] [--org NAME] [--token-stdin]\n" +
		"Reads the token from stdin (one line by default, or the full stream with --token-stdin)."
}

func (c *addCmd) Run(ctx context.Context, args []string, stdio cli.IO) error {
	fs := cli.NewFlagSet("profile add")
	endpoint := fs.String("endpoint", "", "API endpoint URL (defaults to https://app.textql.com)")
	org := fs.String("org", "", "human-readable org label")
	tokenStdin := fs.Bool("token-stdin", false, "read entire stdin as the token (trimmed)")
	if err := cli.ParseFlags(fs, args); err != nil {
		return err
	}
	rest := fs.Args()
	if len(rest) == 0 || rest[0] == "" {
		return cli.UsageErrf("profile add: name is required")
	}
	name := rest[0]

	token, err := readToken(stdio.Stdin, *tokenStdin)
	if err != nil {
		return fmt.Errorf("profile add: %w", err)
	}

	cfg, err := c.deps.LoadCfg()
	if err != nil {
		return fmt.Errorf("profile add: load config: %w", err)
	}

	ep := *endpoint
	if ep == "" {
		ep = config.DefaultEndpoint
	}
	cfg.Upsert(name, config.Profile{
		Endpoint: ep,
		Token:    token,
		OrgName:  *org,
	})
	if err := c.deps.SaveCfg(cfg); err != nil {
		return fmt.Errorf("profile add: save config: %w", err)
	}

	path, err := c.deps.ConfigPath()
	if err != nil {
		// Save succeeded; still surface the path lookup error so users see it.
		fmt.Fprintf(stdio.Stdout, "saved profile %s\n", name)
		return fmt.Errorf("profile add: config path: %w", err)
	}
	fmt.Fprintf(stdio.Stdout, "saved profile %s to %s\n", name, path)
	return nil
}

// readToken matches internal/auth/login.go's token reader: one line by
// default, full stream with --token-stdin. Duplicated here (rather than
// shared via an imported helper) so internal/profile stays independent of
// internal/auth — same rule as the other per-verb helpers.
func readToken(r io.Reader, tokenStdin bool) (string, error) {
	if r == nil {
		return "", fmt.Errorf("stdin is nil")
	}
	if tokenStdin {
		b, err := io.ReadAll(r)
		if err != nil {
			return "", fmt.Errorf("read stdin: %w", err)
		}
		return strings.TrimSpace(string(b)), nil
	}
	scan := bufio.NewScanner(r)
	scan.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	if scan.Scan() {
		return strings.TrimSpace(scan.Text()), nil
	}
	if err := scan.Err(); err != nil {
		return "", fmt.Errorf("read stdin: %w", err)
	}
	return "", nil
}
