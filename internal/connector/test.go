package connector

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/highperformance-tech/ana-cli/internal/cli"
)

// testCmd implements `ana connector test <id>` — TestConnector.
//
// CATALOG REALITY: the server's TestConnector endpoint validates a full config
// body (`{config: {connectorType, name, <dialect>: {...}}}`), not a bare id —
// it's a pre-create probe, not a test-existing op. To preserve the CLI's
// id-based UX we GET the connector first, rebuild a config from the returned
// `<dialect>Metadata` block, and POST that to TestConnector. Passwords/secrets
// are redacted in GetConnector responses, so the dial typically fails with an
// auth or connection error — which the server still returns as a 200 with the
// driver message in `error`. Both `OK` and `FAIL: <msg>` are valid CLI outputs.
type testCmd struct{ deps Deps }

func (c *testCmd) Help() string {
	return "test   Test an existing connector's connection.\n" +
		"Usage: ana connector test <id>"
}

type testReq struct {
	Config map[string]any `json:"config"`
}

type testResp struct {
	Error string `json:"error"`
}

func (c *testCmd) Run(ctx context.Context, args []string, stdio cli.IO) error {
	if len(args) != 1 {
		return cli.UsageErrf("connector test: <id> positional argument required")
	}
	id, err := cli.RequireIntID("connector test", args)
	if err != nil {
		return err
	}
	var getRaw map[string]any
	if err := c.deps.Unary(ctx, servicePath+"/GetConnector", getReq{ConnectorID: id}, &getRaw); err != nil {
		return fmt.Errorf("connector test: %w", err)
	}
	cfg, err := configFromGetConnector(getRaw)
	if err != nil {
		return fmt.Errorf("connector test: %w", err)
	}
	var raw map[string]any
	if err := c.deps.Unary(ctx, servicePath+"/TestConnector", testReq{Config: cfg}, &raw); err != nil {
		return fmt.Errorf("connector test: %w", err)
	}
	var typed testResp
	if err := cli.RenderOutput(stdio.Stdout, raw, cli.GlobalFrom(ctx).JSON, &typed, func(w io.Writer, t *testResp) error {
		if t.Error != "" {
			_, err := fmt.Fprintf(w, "FAIL: %s\n", t.Error)
			return err
		}
		_, err := fmt.Fprintln(w, "OK")
		return err
	}); err != nil {
		return fmt.Errorf("connector test: %w", err)
	}
	return nil
}

// configFromGetConnector rebuilds a TestConnector `config` body from a
// GetConnector response. It copies the top-level `name` + `connectorType`, and
// moves the `<dialect>Metadata` block into `<dialect>`. Secrets are absent
// from the metadata block; the server accepts the probe and returns a driver
// error for the missing credential.
func configFromGetConnector(raw map[string]any) (map[string]any, error) {
	conn, _ := raw["connector"].(map[string]any)
	if conn == nil {
		return nil, fmt.Errorf("GetConnector: missing connector object")
	}
	connectorType, _ := conn["connectorType"].(string)
	name, _ := conn["name"].(string)
	if connectorType == "" {
		return nil, fmt.Errorf("GetConnector: missing connectorType")
	}
	cfg := map[string]any{"connectorType": connectorType, "name": name}
	for k, v := range conn {
		if !strings.HasSuffix(k, "Metadata") {
			continue
		}
		if block, ok := v.(map[string]any); ok {
			dialectKey := strings.TrimSuffix(k, "Metadata")
			// Shallow-copy so we never mutate the caller's map.
			out := make(map[string]any, len(block)+1)
			for bk, bv := range block {
				out[bk] = bv
			}
			// GetConnector redacts secrets, so fill a placeholder for any
			// required secret field. The server returns the driver's auth
			// failure as a 200 `{error: ...}` which the CLI surfaces as FAIL:.
			// NOTE: only "password" is placeholdered today. Postgres uses it;
			// Snowflake's non-password auth modes (keypair, oauth-sso,
			// oauth-individual) use privateKey / oauthClientSecret and will
			// still surface a driver-side auth error via the same FAIL: path
			// — extend this slice when adding a dialect whose TestConnector
			// body requires a different secret field name.
			for _, secret := range []string{"password"} {
				if _, present := out[secret]; !present {
					out[secret] = "redacted"
				}
			}
			cfg[dialectKey] = out
		}
	}
	return cfg, nil
}
