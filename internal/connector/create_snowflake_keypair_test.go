package connector

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/highperformance-tech/ana-cli/internal/cli"
	"github.com/highperformance-tech/ana-cli/internal/testcli"
)

// samplePEM is a structurally plausible PKCS#8 PEM block used only so wire-
// shape assertions can check the literal string round-trips through the
// request body. No cryptography happens client-side; the server stores it
// opaquely.
const samplePEM = "-----BEGIN PRIVATE KEY-----\nMIIEvQIBADANBgkqhkiG9w0BAQEFAASCBKcwggSjAgEAAoIBAQC\n-----END PRIVATE KEY-----\n"

func writeKeyFile(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "key.p8")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("write key: %v", err)
	}
	return path
}

func snowflakeKeypairArgs(keyPath string) []string {
	return []string{
		"snowflake", "keypair",
		"--name", "sf1",
		"--locator", "abc12345.us-east-1",
		"--database", "D",
		"--user", "U",
		"--private-key-file", keyPath,
	}
}

func runSnowflakeKeypair(t *testing.T, deps Deps, args []string, stdin string) (*bytes.Buffer, error) {
	t.Helper()
	g := newCreateGroup(deps)
	stdio, out, _ := testcli.NewIO(strings.NewReader(stdin))
	return out, g.Run(context.Background(), args, stdio)
}

func TestCreateSnowflakeKeypairHappy(t *testing.T) {
	t.Parallel()
	keyPath := writeKeyFile(t, samplePEM)
	f := &fakeDeps{
		unaryFn: func(_ context.Context, _ string, _, resp any) error {
			out := resp.(*map[string]any)
			*out = map[string]any{"connectorId": 77.0, "name": "sf1", "connectorType": "SNOWFLAKE"}
			return nil
		},
	}
	out, err := runSnowflakeKeypair(t, f.deps(), snowflakeKeypairArgs(keyPath), "")
	if err != nil {
		t.Fatalf("err=%v", err)
	}
	s := out.String()
	if !strings.Contains(s, "connectorId: 77") || !strings.Contains(s, "name: sf1") {
		t.Errorf("stdout=%q", s)
	}
	req := string(f.lastRawReq)
	for _, want := range []string{
		`"connectorType":"SNOWFLAKE"`, `"name":"sf1"`, `"authStrategy":"service_role"`,
		`"locator":"abc12345.us-east-1"`, `"username":"U"`,
		`"privateKey":"-----BEGIN PRIVATE KEY-----`,
	} {
		if !strings.Contains(req, want) {
			t.Errorf("req missing %s in %s", want, req)
		}
	}
	// Password-mode field must be absent; passphrase absent when not set.
	for _, unwanted := range []string{`"password":`, `"privateKeyPassphrase":`, `"oauthClientId":`} {
		if strings.Contains(req, unwanted) {
			t.Errorf("req unexpectedly contains %s in %s", unwanted, req)
		}
	}
}

func TestCreateSnowflakeKeypairWithPassphraseFlag(t *testing.T) {
	t.Parallel()
	keyPath := writeKeyFile(t, samplePEM)
	f := &fakeDeps{
		unaryFn: func(_ context.Context, _ string, _, resp any) error {
			out := resp.(*map[string]any)
			*out = map[string]any{"connectorId": 1.0, "name": "sf1", "connectorType": "SNOWFLAKE"}
			return nil
		},
	}
	args := append(snowflakeKeypairArgs(keyPath),
		"--private-key-passphrase", "secret-phrase",
	)
	_, err := runSnowflakeKeypair(t, f.deps(), args, "")
	if err != nil {
		t.Fatalf("err=%v", err)
	}
	if !strings.Contains(string(f.lastRawReq), `"privateKeyPassphrase":"secret-phrase"`) {
		t.Errorf("req=%s", string(f.lastRawReq))
	}
}

func TestCreateSnowflakeKeypairWithPassphraseStdin(t *testing.T) {
	t.Parallel()
	keyPath := writeKeyFile(t, samplePEM)
	f := &fakeDeps{}
	args := append(snowflakeKeypairArgs(keyPath), "--private-key-passphrase-stdin")
	_, err := runSnowflakeKeypair(t, f.deps(), args, "stdin-phrase\n")
	if err != nil {
		t.Fatalf("err=%v", err)
	}
	if !strings.Contains(string(f.lastRawReq), `"privateKeyPassphrase":"stdin-phrase"`) {
		t.Errorf("req=%s", string(f.lastRawReq))
	}
}

func TestCreateSnowflakeKeypairPassphraseStdinEmpty(t *testing.T) {
	t.Parallel()
	keyPath := writeKeyFile(t, samplePEM)
	args := append(snowflakeKeypairArgs(keyPath), "--private-key-passphrase-stdin")
	_, err := runSnowflakeKeypair(t, (&fakeDeps{}).deps(), args, "")
	if !errors.Is(err, cli.ErrUsage) {
		t.Errorf("err=%v", err)
	}
}

func TestCreateSnowflakeKeypairPassphraseStdinReadErr(t *testing.T) {
	t.Parallel()
	keyPath := writeKeyFile(t, samplePEM)
	args := append(snowflakeKeypairArgs(keyPath), "--private-key-passphrase-stdin")
	g := newCreateGroup((&fakeDeps{}).deps())
	stdio, _, _ := testcli.NewIO(errReader{err: errors.New("read fail")})
	err := g.Run(context.Background(), args, stdio)
	if err == nil || !strings.Contains(err.Error(), "read fail") {
		t.Errorf("err=%v", err)
	}
}

func TestCreateSnowflakeKeypairOptionalFields(t *testing.T) {
	t.Parallel()
	keyPath := writeKeyFile(t, samplePEM)
	f := &fakeDeps{
		unaryFn: func(_ context.Context, _ string, _, resp any) error {
			out := resp.(*map[string]any)
			*out = map[string]any{"connectorId": 1.0, "name": "sf1", "connectorType": "SNOWFLAKE"}
			return nil
		},
	}
	args := append(snowflakeKeypairArgs(keyPath),
		"--warehouse", "W",
		"--schema", "S",
		"--role", "R",
	)
	_, err := runSnowflakeKeypair(t, f.deps(), args, "")
	if err != nil {
		t.Fatalf("err=%v", err)
	}
	req := string(f.lastRawReq)
	for _, want := range []string{`"warehouse":"W"`, `"schema":"S"`, `"role":"R"`} {
		if !strings.Contains(req, want) {
			t.Errorf("req missing %s in %s", want, req)
		}
	}
}

func TestCreateSnowflakeKeypairMissingKeyFile(t *testing.T) {
	t.Parallel()
	args := []string{
		"snowflake", "keypair",
		"--name", "n",
		"--locator", "acct",
		"--database", "D",
		"--user", "U",
	}
	_, err := runSnowflakeKeypair(t, (&fakeDeps{}).deps(), args, "")
	if !errors.Is(err, cli.ErrUsage) {
		t.Errorf("err=%v", err)
	}
}

func TestCreateSnowflakeKeypairMissingFlags(t *testing.T) {
	t.Parallel()
	_, err := runSnowflakeKeypair(t, (&fakeDeps{}).deps(), []string{"snowflake", "keypair"}, "")
	if !errors.Is(err, cli.ErrUsage) {
		t.Errorf("err=%v", err)
	}
}

func TestCreateSnowflakeKeypairEmptyString(t *testing.T) {
	t.Parallel()
	keyPath := writeKeyFile(t, samplePEM)
	for _, flag := range []string{"name", "locator", "database", "user"} {
		t.Run(flag, func(t *testing.T) {
			t.Parallel()
			args := append(snowflakeKeypairArgs(keyPath), "--"+flag, "")
			_, err := runSnowflakeKeypair(t, (&fakeDeps{}).deps(), args, "")
			if !errors.Is(err, cli.ErrUsage) || !strings.Contains(err.Error(), "--"+flag) {
				t.Errorf("err=%v", err)
			}
		})
	}
}

func TestCreateSnowflakeKeypairKeyFileMissing(t *testing.T) {
	t.Parallel()
	args := []string{
		"snowflake", "keypair",
		"--name", "n",
		"--locator", "acct",
		"--database", "D",
		"--user", "U",
		"--private-key-file", "/no/such/file.p8",
	}
	_, err := runSnowflakeKeypair(t, (&fakeDeps{}).deps(), args, "")
	if err == nil || !strings.Contains(err.Error(), "read --private-key-file") {
		t.Errorf("err=%v", err)
	}
}

func TestCreateSnowflakeKeypairKeyFileEmpty(t *testing.T) {
	t.Parallel()
	keyPath := writeKeyFile(t, "")
	_, err := runSnowflakeKeypair(t, (&fakeDeps{}).deps(), snowflakeKeypairArgs(keyPath), "")
	if !errors.Is(err, cli.ErrUsage) {
		t.Errorf("err=%v", err)
	}
}

func TestCreateSnowflakeKeypairJSONBypass(t *testing.T) {
	t.Parallel()
	keyPath := writeKeyFile(t, samplePEM)
	f := &fakeDeps{
		unaryFn: func(_ context.Context, _ string, _, resp any) error {
			out := resp.(*map[string]any)
			*out = map[string]any{"connectorId": 1.0, "name": "n", "connectorType": "SNOWFLAKE"}
			return nil
		},
	}
	ctx := cli.WithGlobal(context.Background(), cli.Global{JSON: true})
	g := newCreateGroup(f.deps())
	stdio, out, _ := testcli.NewIO(strings.NewReader(""))
	if err := g.Run(ctx, snowflakeKeypairArgs(keyPath), stdio); err != nil {
		t.Fatalf("err=%v", err)
	}
	if !strings.Contains(out.String(), "\"connectorId\"") {
		t.Errorf("stdout=%q", out.String())
	}
}

func TestCreateSnowflakeKeypairRenderWriteErr(t *testing.T) {
	t.Parallel()
	keyPath := writeKeyFile(t, samplePEM)
	f := &fakeDeps{
		unaryFn: func(_ context.Context, _ string, _, resp any) error {
			out := resp.(*map[string]any)
			*out = map[string]any{"connectorId": 1.0, "name": "sf1", "connectorType": "SNOWFLAKE"}
			return nil
		},
	}
	g := newCreateGroup(f.deps())
	err := g.Run(context.Background(), snowflakeKeypairArgs(keyPath), testcli.FailingIO())
	if err == nil || !strings.Contains(err.Error(), "boom") {
		t.Errorf("err=%v want boom", err)
	}
}

// TestCreateSnowflakeKeypairRejectsExtraPositionals pins the no-positional
// contract for the deeply-nested leaf: trailing tokens after the verb path
// must yield ErrUsage before RequireFlags or any RPC fires.
func TestCreateSnowflakeKeypairRejectsExtraPositionals(t *testing.T) {
	t.Parallel()
	_, err := runSnowflakeKeypair(t, (&fakeDeps{}).deps(), []string{"snowflake", "keypair", "extra"}, "")
	if !errors.Is(err, cli.ErrUsage) {
		t.Errorf("err=%v want ErrUsage", err)
	}
}

func TestCreateSnowflakeKeypairBadFlag(t *testing.T) {
	t.Parallel()
	_, err := runSnowflakeKeypair(t, (&fakeDeps{}).deps(), []string{"snowflake", "keypair", "--nope"}, "")
	if !errors.Is(err, cli.ErrUsage) {
		t.Errorf("err=%v", err)
	}
}

func TestCreateSnowflakeKeypairUnaryErr(t *testing.T) {
	t.Parallel()
	keyPath := writeKeyFile(t, samplePEM)
	f := &fakeDeps{unaryFn: func(_ context.Context, _ string, _, _ any) error { return errors.New("boom") }}
	_, err := runSnowflakeKeypair(t, f.deps(), snowflakeKeypairArgs(keyPath), "")
	if err == nil || !strings.Contains(err.Error(), "boom") {
		t.Errorf("err=%v", err)
	}
}

func TestCreateSnowflakeKeypairRemarshalErr(t *testing.T) {
	t.Parallel()
	keyPath := writeKeyFile(t, samplePEM)
	f := &fakeDeps{
		unaryFn: func(_ context.Context, _ string, _, resp any) error {
			out := resp.(*map[string]any)
			*out = map[string]any{"connectorId": "not-an-int"}
			return nil
		},
	}
	_, err := runSnowflakeKeypair(t, f.deps(), snowflakeKeypairArgs(keyPath), "")
	if err == nil || !strings.Contains(err.Error(), "decode response") {
		t.Errorf("err=%v", err)
	}
}

// TestResolveOptionalPassphraseUnset locks in that the passphrase is
// legitimately optional — PKCS#8 keys may be unencrypted and the UI omits
// the field entirely in that case.
func TestResolveOptionalPassphraseUnset(t *testing.T) {
	t.Parallel()
	got, err := resolveOptionalPassphrase("", false, nil)
	if err != nil {
		t.Fatalf("err=%v", err)
	}
	if got != "" {
		t.Errorf("got=%q want empty", got)
	}
}
