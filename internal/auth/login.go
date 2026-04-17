package auth

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/textql/ana-cli/internal/cli"
)

// loginCmd persists a token to the config file. The token may come from
// stdin (single line or full stream, controlled by --token-stdin). Endpoint
// precedence: --endpoint global > already-loaded config > DefaultEndpoint.
type loginCmd struct{ deps Deps }

// Help is fixed text; flag names and behavior live here rather than in
// auth.go so the command file is self-contained.
func (c *loginCmd) Help() string {
	return "login   Save an API token to the config file.\n" +
		"Usage: ana auth login [--token-stdin]\n" +
		"Reads the token from stdin (one line by default, or the full stream with --token-stdin)."
}

// Run reads a token from stdio.Stdin, merges endpoint precedence, and saves
// via deps.SaveCfg. On success it prints `saved to <path>` to stdout.
func (c *loginCmd) Run(ctx context.Context, args []string, stdio cli.IO) error {
	fs := newFlagSet("auth login")
	tokenStdin := fs.Bool("token-stdin", false, "read entire stdin as the token (trimmed)")
	if err := parseFlags(fs, args); err != nil {
		return err
	}

	token, err := readToken(stdio.Stdin, *tokenStdin)
	if err != nil {
		return fmt.Errorf("auth login: %w", err)
	}
	if token == "" {
		return usageErrf("auth login: token is required")
	}

	loaded, err := c.deps.LoadCfg()
	if err != nil {
		return fmt.Errorf("auth login: load config: %w", err)
	}

	global := cli.GlobalFrom(ctx)
	endpoint := pickEndpoint(global.Endpoint, loaded.Endpoint)

	cfg := Config{Endpoint: endpoint, Token: token}
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

// readToken consumes stdin and returns a trimmed token. With tokenStdin=true
// the whole stream is consumed; otherwise a single newline-terminated line is
// read. Whitespace is trimmed in both modes so common pipe quirks (trailing
// newline from `echo`) don't poison the saved value.
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
	// Line-oriented path: bufio.Scanner handles the common interactive and
	// piped-single-line cases identically. We ignore io.EOF without a line.
	scan := bufio.NewScanner(r)
	// Boost the buffer so unusually long tokens (JWTs etc.) fit in one line.
	scan.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	if scan.Scan() {
		return strings.TrimSpace(scan.Text()), nil
	}
	if err := scan.Err(); err != nil {
		return "", fmt.Errorf("read stdin: %w", err)
	}
	return "", nil
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
