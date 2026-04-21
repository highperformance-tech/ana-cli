package connector

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/highperformance-tech/ana-cli/internal/cli"
)

// snowflakeKeypairCmd is the leaf for
// `ana connector create snowflake keypair`. Auth mode shares
// authStrategy=service_role with password; the server discriminates by which
// credential field is populated (`privateKey` vs `password`).
//
// Snowflake binds RSA public keys to a user, so `--user` is still required
// even though the credential is the private key. The passphrase pair is
// optional (PKCS#8 keys may be unencrypted); when either passphrase flag is
// set, its value populates `privateKeyPassphrase` on the wire.
type snowflakeKeypairCmd struct {
	deps Deps

	// Ancestor-flag targets.
	name      *string
	locator   *string
	database  *string
	warehouse *string
	schema    *string
	role      *string

	// Leaf-specific flag targets.
	user                string
	privateKeyFile      string
	privateKeyPass      string
	privateKeyPassStdin bool
}

func (c *snowflakeKeypairCmd) Help() string {
	return "keypair    Key-pair Snowflake auth (authStrategy=service_role; private key in PEM).\n" +
		"Usage: ana connector create snowflake keypair --name <n> --locator <acct> --database <db> --user <u> --private-key-file <path> [--private-key-passphrase <p>|--private-key-passphrase-stdin] [--warehouse <w>] [--schema <s>] [--role <r>]"
}

func (c *snowflakeKeypairCmd) Flags(fs *flag.FlagSet) {
	fs.StringVar(&c.user, "user", "", "Snowflake username bound to the key (required)")
	fs.StringVar(&c.privateKeyFile, "private-key-file", "", "path to PEM-encoded PKCS#8 private key file (required)")
	fs.StringVar(&c.privateKeyPass, "private-key-passphrase", "", "passphrase for encrypted PKCS#8 key (optional)")
	fs.BoolVar(&c.privateKeyPassStdin, "private-key-passphrase-stdin", false, "read passphrase from the first stdin line (optional)")
}

func (c *snowflakeKeypairCmd) Run(ctx context.Context, args []string, stdio cli.IO) error {
	fs := cli.NewFlagSet("connector create snowflake keypair")
	c.Flags(fs)
	cli.ApplyAncestorFlags(ctx, fs)
	if err := cli.ParseFlags(fs, args); err != nil {
		return err
	}
	if err := cli.RequireFlags(fs, "connector create snowflake keypair",
		"name", "locator", "database", "user", "private-key-file"); err != nil {
		return err
	}
	for _, p := range []struct {
		name, val string
	}{
		{"name", *c.name}, {"locator", *c.locator}, {"database", *c.database},
		{"user", c.user}, {"private-key-file", c.privateKeyFile},
	} {
		if p.val == "" {
			return cli.UsageErrf("connector create snowflake keypair: --%s must not be empty", p.name)
		}
	}
	keyBytes, err := os.ReadFile(c.privateKeyFile)
	if err != nil {
		return fmt.Errorf("connector create snowflake keypair: read --private-key-file: %w", err)
	}
	if len(keyBytes) == 0 {
		return cli.UsageErrf("connector create snowflake keypair: --private-key-file %q is empty", c.privateKeyFile)
	}
	passphrase, err := resolveOptionalPassphrase(c.privateKeyPass, c.privateKeyPassStdin, stdio.Stdin)
	if err != nil {
		return fmt.Errorf("connector create snowflake keypair: %w", err)
	}

	req := createReq{Config: configEnvelope{
		ConnectorType: "SNOWFLAKE",
		Name:          *c.name,
		AuthStrategy:  "service_role",
		Snowflake: &snowflakeSpec{
			Locator:              *c.locator,
			Database:             *c.database,
			Warehouse:            *c.warehouse,
			Schema:               *c.schema,
			Role:                 *c.role,
			Username:             c.user,
			PrivateKey:           string(keyBytes),
			PrivateKeyPassphrase: passphrase,
		},
	}}
	var raw map[string]any
	if err := c.deps.Unary(ctx, servicePath+"/CreateConnector", req, &raw); err != nil {
		return fmt.Errorf("connector create snowflake keypair: %w", err)
	}
	var typed createResp
	if err := cli.RenderOutput(stdio.Stdout, raw, cli.GlobalFrom(ctx).JSON, &typed, func(w io.Writer, t *createResp) error {
		_, err := fmt.Fprintf(w, "connectorId: %d\nname: %s\nconnectorType: %s\n",
			t.ConnectorID, t.Name, t.ConnectorType)
		return err
	}); err != nil {
		return fmt.Errorf("connector create snowflake keypair: %w", err)
	}
	return nil
}

// resolveOptionalPassphrase returns "" when neither flag is set (passphrase
// is legitimately optional for unencrypted PKCS#8 keys), reads one line from
// r when --…-stdin is set (empty stdin is a usage error — the flag was set
// but the secret didn't arrive), and otherwise returns the --… value
// verbatim. Whitespace semantics match cli.ReadPassword: only the trailing
// line terminator is stripped.
func resolveOptionalPassphrase(passFlag string, stdinFlag bool, r io.Reader) (string, error) {
	if stdinFlag {
		pass, err := cli.ReadPassword(r)
		if err != nil {
			return "", fmt.Errorf("read passphrase: %w", err)
		}
		if pass == "" {
			return "", cli.UsageErrf("--private-key-passphrase-stdin set but stdin was empty")
		}
		return pass, nil
	}
	return passFlag, nil
}
