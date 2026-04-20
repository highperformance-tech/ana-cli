package auth

import (
	"context"
	"fmt"

	"github.com/highperformance-tech/ana-cli/internal/cli"
)

// loginCmd persists a token to the config file. The token may come from
// stdin (single line or full stream, controlled by --token-stdin). Endpoint
// precedence: --endpoint global > already-loaded config > DefaultEndpoint.
type loginCmd struct{ deps Deps }

// Help is fixed text; flag names and behavior live here rather than in
// auth.go so the command file is self-contained.
func (c *loginCmd) Help() string {
	return "login   Save an API token to the active config profile.\n" +
		"Usage: ana auth login [--token-stdin]\n" +
		"Reads the token from stdin (one line by default, or the full stream with --token-stdin).\n" +
		"\n" +
		"API keys are scoped to a single TextQL organization (they are minted by a\n" +
		"member, and each user has a separate member record per org). To work across\n" +
		"multiple orgs, mint one key per org in that org's /settings#dev page and save\n" +
		"each under its own profile via `ana profile add <name>`. Select a profile with\n" +
		"the global --profile flag or `ana profile use <name>`."
}

// Run reads a token from stdio.Stdin, merges endpoint precedence, and saves
// via deps.SaveCfg. On success it prints `saved to <path>` to stdout.
func (c *loginCmd) Run(ctx context.Context, args []string, stdio cli.IO) error {
	fs := cli.NewFlagSet("auth login")
	tokenStdin := fs.Bool("token-stdin", false, "read entire stdin as the token (trimmed)")
	if err := cli.ParseFlags(fs, args); err != nil {
		return err
	}

	token, err := cli.ReadToken(stdio.Stdin, *tokenStdin)
	if err != nil {
		return fmt.Errorf("auth login: %w", err)
	}
	if token == "" {
		return cli.UsageErrf("auth login: token is required")
	}

	loaded, err := c.deps.LoadCfg()
	if err != nil {
		return fmt.Errorf("auth login: load config: %w", err)
	}

	global := cli.GlobalFrom(ctx)
	endpoint := pickEndpoint(global.Endpoint, loaded.Endpoint)

	cfg := Config{Endpoint: endpoint, Token: cli.Token(token)}
	if err := c.deps.SaveCfg(cfg); err != nil {
		return fmt.Errorf("auth login: save config: %w", err)
	}

	path, err := c.deps.ConfigPath()
	if err != nil {
		// Save succeeded; emit a softer message so the user still gets
		// feedback. We still return the error for visibility.
		fmt.Fprintln(stdio.Stdout, "saved")
		return fmt.Errorf("auth login: config path: %w", err)
	}
	fmt.Fprintf(stdio.Stdout, "saved to %s\n", path)
	return nil
}

// pickEndpoint applies the precedence rule global > loaded > default.
func pickEndpoint(global, loaded string) string {
	switch {
	case global != "":
		return global
	case loaded != "":
		return loaded
	default:
		return DefaultEndpoint
	}
}
