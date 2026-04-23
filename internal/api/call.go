package api

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"strings"

	"github.com/highperformance-tech/ana-cli/internal/cli"
)

// callCmd implements the `ana api <path>` leaf. Flags:
//
//	--method      HTTP verb (default POST; any non-empty string accepted).
//	--data        literal JSON request body.
//	--data-stdin  read the request body from stdin (mutually exclusive with --data).
//	--raw         emit the response body verbatim (skip json.Indent).
//
// The global --json flag is a no-op here — the default output IS pretty JSON;
// --raw is the opposite. Documented in Help().
type callCmd struct {
	deps Deps

	method    string
	data      string
	dataStdin bool
	raw       bool
}

func (c *callCmd) Help() string {
	return "api   Authenticated HTTP passthrough. JSON pretty-printed by default; --raw for verbatim bytes.\n" +
		"Usage: ana api <path> [--method M] [--data JSON | --data-stdin] [--raw]\n" +
		"\n" +
		"Paths:\n" +
		"  <service>/<Method>   Connect-RPC short form; prefixed with /rpc/public/\n" +
		"                       e.g. textql.rpc.public.auth.PublicAuthService/GetOrganization\n" +
		"  /rpc/public/<...>    Connect-RPC full path; sent as-is\n" +
		"  /v1/<...>            Documented REST API path (docs.textql.com/api-reference)\n" +
		"\n" +
		"Note: the global --json flag is ignored here; output is JSON by default."
}

// Flags declares this leaf's flags. Implementing cli.Flagger lets dispatchChild
// render a `Flags:` block under --help so the four knobs are discoverable.
func (c *callCmd) Flags(fs *flag.FlagSet) {
	fs.StringVar(&c.method, "method", "POST", "HTTP method (default POST)")
	fs.StringVar(&c.data, "data", "", "literal JSON request body")
	fs.BoolVar(&c.dataStdin, "data-stdin", false, "read the request body from stdin")
	fs.BoolVar(&c.raw, "raw", false, "pass the response body through verbatim (skip pretty-print)")
}

func (c *callCmd) Run(ctx context.Context, args []string, stdio cli.IO) error {
	fs := cli.NewFlagSet("api")
	c.Flags(fs)
	cli.ApplyAncestorFlags(ctx, fs)
	if err := cli.ParseFlags(fs, args); err != nil {
		return err
	}

	if fs.NArg() == 0 {
		return cli.UsageErrf("api: <path> positional argument required")
	}
	if fs.NArg() > 1 {
		return cli.UsageErrf("api: unexpected positional arguments: %v", fs.Args()[1:])
	}
	// Trim once and reuse — otherwise a whitespace-padded arg like
	// `" /v1/things "` passes the blank check but gets forwarded to the
	// transport verbatim, which joinURL would then stitch into a malformed URL.
	path := strings.TrimSpace(fs.Arg(0))
	if path == "" {
		return cli.UsageErrf("api: <path> positional argument required")
	}

	if c.method == "" {
		return cli.UsageErrf("api: --method must not be empty")
	}
	dataSet := cli.FlagWasSet(fs, "data")
	if dataSet && c.dataStdin {
		return cli.UsageErrf("api: --data and --data-stdin are mutually exclusive")
	}

	// Path dispatch: leading slash → verbatim; otherwise Connect-RPC short form.
	resolvedPath := path
	if !strings.HasPrefix(path, "/") {
		resolvedPath = connectRPCPrefix + path
	}

	body, err := resolveBody(c, dataSet, stdio.Stdin)
	if err != nil {
		return err
	}

	status, respBody, err := c.deps.DoRaw(ctx, c.method, resolvedPath, body)
	if err != nil {
		return fmt.Errorf("api: %w", err)
	}

	if status < 200 || status >= 300 {
		return c.emitError(stdio, status, respBody)
	}
	return c.emitSuccess(stdio, respBody)
}

// resolveBody picks the outbound body bytes. Precedence (after the
// --data/--data-stdin mutual-exclusion check in the caller):
//
//   - --data-stdin: io.ReadAll so the bytes round-trip exactly (ReadToken
//     would trim whitespace, which matters for binary-ish JSON payloads).
//   - --data set (even to ""): use the literal bytes.
//   - neither: nil for GET/HEAD (no body), `{}` otherwise so Connect-RPC's
//     required-body contract is still satisfied.
func resolveBody(c *callCmd, dataSet bool, stdin io.Reader) ([]byte, error) {
	switch {
	case c.dataStdin:
		b, err := io.ReadAll(stdin)
		if err != nil {
			return nil, fmt.Errorf("api: read stdin: %w", err)
		}
		return b, nil
	case dataSet:
		return []byte(c.data), nil
	}
	if strings.EqualFold(c.method, "GET") || strings.EqualFold(c.method, "HEAD") {
		return nil, nil
	}
	return []byte("{}"), nil
}

// emitError writes the server's error body to stderr (so stdout stays empty
// for `| jq`) and returns a summary error. Main's fallback printer adds the
// `api: HTTP <status>` line on its own stderr write — body + status together.
// A trailing newline is appended if the body didn't already end with one so
// the status line doesn't get glued to the last byte of the response.
func (c *callCmd) emitError(stdio cli.IO, status int, body []byte) error {
	if len(body) > 0 {
		if _, werr := stdio.Stderr.Write(body); werr != nil {
			return fmt.Errorf("api: %w", werr)
		}
		if !bytes.HasSuffix(body, []byte("\n")) {
			if _, werr := fmt.Fprintln(stdio.Stderr); werr != nil {
				return fmt.Errorf("api: %w", werr)
			}
		}
	}
	return fmt.Errorf("api: HTTP %d", status)
}

// emitSuccess writes the 2xx body. With --raw (or when the body is empty)
// the bytes are passed through verbatim; otherwise json.Indent pretty-prints.
// A body that isn't valid JSON (e.g. 204 empty, or some future text endpoint)
// falls through to the raw path so we don't fail an otherwise-successful call.
func (c *callCmd) emitSuccess(stdio cli.IO, body []byte) error {
	if c.raw || len(body) == 0 {
		if _, werr := stdio.Stdout.Write(body); werr != nil {
			return fmt.Errorf("api: %w", werr)
		}
		return nil
	}
	var pretty bytes.Buffer
	if err := json.Indent(&pretty, body, "", "  "); err != nil {
		if _, werr := stdio.Stdout.Write(body); werr != nil {
			return fmt.Errorf("api: %w", werr)
		}
		return nil
	}
	if _, werr := stdio.Stdout.Write(pretty.Bytes()); werr != nil {
		return fmt.Errorf("api: %w", werr)
	}
	if _, werr := stdio.Stdout.Write([]byte("\n")); werr != nil {
		return fmt.Errorf("api: %w", werr)
	}
	return nil
}
