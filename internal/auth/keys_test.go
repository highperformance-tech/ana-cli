package auth

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/highperformance-tech/ana-cli/internal/cli"
	"github.com/highperformance-tech/ana-cli/internal/testcli"
)

// --- keys list ---

func TestKeysListTable(t *testing.T) {
	t.Parallel()
	f := &fakeDeps{
		unaryFn: func(_ context.Context, _ string, _, resp any) error {
			out := resp.(*map[string]any)
			*out = map[string]any{
				"apiKeys": []any{
					map[string]any{"id": "k1", "name": "first", "lastUsedAt": "2026-04-01"},
					map[string]any{"id": "k2", "name": "second"},
				},
			}
			return nil
		},
	}
	cmd := &keysListCmd{deps: f.deps()}
	stdio, out, _ := testcli.NewIO(strings.NewReader(""))
	if err := cmd.Run(context.Background(), nil, stdio); err != nil {
		t.Fatalf("err=%v", err)
	}
	s := out.String()
	if !strings.Contains(s, "ID") || !strings.Contains(s, "NAME") || !strings.Contains(s, "LAST USED") {
		t.Errorf("missing headers: %q", s)
	}
	if !strings.Contains(s, "k1") || !strings.Contains(s, "k2") || !strings.Contains(s, "first") {
		t.Errorf("missing rows: %q", s)
	}
	// Row-specific dash assertion: a bare `strings.Contains(s, "-")` would
	// pass trivially because the k1 row carries a real date (`2026-04-01`)
	// containing hyphens. Instead, locate the k2 row — its lastUsedAt is
	// unset, so that row's trailing LAST USED cell must render as "-".
	foundK2Dash := false
	for _, line := range strings.Split(strings.TrimSpace(s), "\n") {
		if strings.Contains(line, "k2") && strings.HasSuffix(strings.TrimSpace(line), "-") {
			foundK2Dash = true
			break
		}
	}
	if !foundK2Dash {
		t.Errorf("expected k2 row to end with '-' LAST USED placeholder: %q", s)
	}
}

func TestKeysListJSON(t *testing.T) {
	t.Parallel()
	f := &fakeDeps{
		unaryFn: func(_ context.Context, _ string, _, resp any) error {
			out := resp.(*map[string]any)
			*out = map[string]any{"apiKeys": []any{}}
			return nil
		},
	}
	cmd := &keysListCmd{deps: f.deps()}
	ctx := cli.WithGlobal(context.Background(), cli.Global{JSON: true})
	stdio, out, _ := testcli.NewIO(strings.NewReader(""))
	if err := cmd.Run(ctx, nil, stdio); err != nil {
		t.Fatalf("err=%v", err)
	}
	if !strings.Contains(out.String(), "\"apiKeys\"") {
		t.Errorf("stdout=%q", out.String())
	}
}

func TestKeysListUnaryErr(t *testing.T) {
	t.Parallel()
	f := &fakeDeps{unaryFn: func(_ context.Context, _ string, _, _ any) error { return errors.New("boom") }}
	cmd := &keysListCmd{deps: f.deps()}
	stdio, _, _ := testcli.NewIO(strings.NewReader(""))
	err := cmd.Run(context.Background(), nil, stdio)
	if err == nil || !strings.Contains(err.Error(), "boom") {
		t.Errorf("err=%v", err)
	}
}

func TestKeysListBadFlag(t *testing.T) {
	t.Parallel()
	f := &fakeDeps{}
	stdio, _, _ := testcli.NewIO(strings.NewReader(""))
	err := New(f.deps()).Run(context.Background(), []string{"keys", "list", "--nope"}, stdio)
	if !errors.Is(err, cli.ErrUsage) {
		t.Errorf("err=%v", err)
	}
}

// TestKeysListRejectsExtraPositionals pins the no-positional contract for the
// list verb: trailing tokens must yield ErrUsage before the RPC fires.
func TestKeysListRejectsExtraPositionals(t *testing.T) {
	t.Parallel()
	f := &fakeDeps{}
	stdio, _, _ := testcli.NewIO(nil)
	err := New(f.deps()).Run(context.Background(), []string{"keys", "list", "unexpected"}, stdio)
	if !errors.Is(err, cli.ErrUsage) {
		t.Errorf("err=%v want ErrUsage", err)
	}
}

// TestKeysListRemarshalErr exercises the remarshal error path: the fixture is
// syntactically valid JSON but the `apiKeys` field is a string rather than
// the expected array, so Unmarshal into listApiKeysResp.APIKeys fails with a
// schema/type mismatch (not a parse error).
func TestKeysListRemarshalErr(t *testing.T) {
	t.Parallel()
	f := &fakeDeps{
		unaryFn: func(_ context.Context, _ string, _, resp any) error {
			out := resp.(*map[string]any)
			*out = map[string]any{"apiKeys": "not-an-array"}
			return nil
		},
	}
	cmd := &keysListCmd{deps: f.deps()}
	stdio, _, _ := testcli.NewIO(strings.NewReader(""))
	err := cmd.Run(context.Background(), nil, stdio)
	if err == nil || !strings.Contains(err.Error(), "decode response") {
		t.Errorf("err=%v", err)
	}
}

// --- keys create ---

func TestKeysCreateHappy(t *testing.T) {
	t.Parallel()
	f := &fakeDeps{
		unaryFn: func(_ context.Context, _ string, _, resp any) error {
			out := resp.(*createApiKeyResp)
			out.APIKey.ID = "k1"
			out.APIKey.Name = "n"
			out.APIKeyHash = "plaintext-token"
			return nil
		},
	}
	stdio, out, errb := testcli.NewIO(strings.NewReader(""))
	err := New(f.deps()).Run(context.Background(), []string{"keys", "create", "--name", "n", "--service-account", "sa-1"}, stdio)
	if err != nil {
		t.Fatalf("err=%v", err)
	}
	if !strings.Contains(out.String(), "plaintext-token") {
		t.Errorf("stdout=%q", out.String())
	}
	// The plaintext must be printed to stdout exactly once, with nothing
	// before it (i.e. first line).
	if lines := strings.Count(strings.TrimSpace(out.String()), "\n"); lines != 0 {
		t.Errorf("stdout should have exactly one line, got: %q", out.String())
	}
	if !strings.Contains(errb.String(), "# store this token") {
		t.Errorf("stderr missing note: %q", errb.String())
	}
	// The wire-level request must include camelCase serviceAccountId.
	if !strings.Contains(string(f.lastRawReq), `"serviceAccountId":"sa-1"`) {
		t.Errorf("request=%s", string(f.lastRawReq))
	}
	if !strings.Contains(string(f.lastRawReq), `"name":"n"`) {
		t.Errorf("request=%s", string(f.lastRawReq))
	}
}

func TestKeysCreateOmitsEmptyServiceAccount(t *testing.T) {
	t.Parallel()
	f := &fakeDeps{
		unaryFn: func(_ context.Context, _ string, _, resp any) error {
			out := resp.(*createApiKeyResp)
			out.APIKeyHash = "tok"
			return nil
		},
	}
	stdio, _, _ := testcli.NewIO(strings.NewReader(""))
	if err := New(f.deps()).Run(context.Background(), []string{"keys", "create", "--name", "n"}, stdio); err != nil {
		t.Fatalf("err=%v", err)
	}
	if strings.Contains(string(f.lastRawReq), "serviceAccountId") {
		t.Errorf("serviceAccountId should be omitted: %s", string(f.lastRawReq))
	}
}

func TestKeysCreateMissingName(t *testing.T) {
	t.Parallel()
	f := &fakeDeps{}
	cmd := &keysCreateCmd{deps: f.deps()}
	stdio, _, _ := testcli.NewIO(strings.NewReader(""))
	err := cmd.Run(context.Background(), nil, stdio)
	if !errors.Is(err, cli.ErrUsage) {
		t.Errorf("err=%v want ErrUsage", err)
	}
}

// TestKeysCreateRejectsExtraPositionals pins the no-positional contract: any
// trailing token after the verb path must yield ErrUsage before RequireFlags
// or any RPC fires.
func TestKeysCreateRejectsExtraPositionals(t *testing.T) {
	t.Parallel()
	f := &fakeDeps{}
	stdio, _, _ := testcli.NewIO(strings.NewReader(""))
	err := New(f.deps()).Run(context.Background(), []string{"keys", "create", "--name", "n", "extra"}, stdio)
	if !errors.Is(err, cli.ErrUsage) {
		t.Errorf("err=%v want ErrUsage", err)
	}
}

// TestKeysCreateRejectsEmptyName covers the explicit empty-name guard that
// fires AFTER RequireFlags (which only checks "was the flag set"): supplying
// `--name ""` must still yield ErrUsage.
func TestKeysCreateRejectsEmptyName(t *testing.T) {
	t.Parallel()
	f := &fakeDeps{}
	stdio, _, _ := testcli.NewIO(strings.NewReader(""))
	err := New(f.deps()).Run(context.Background(), []string{"keys", "create", "--name", ""}, stdio)
	if !errors.Is(err, cli.ErrUsage) {
		t.Errorf("err=%v want ErrUsage", err)
	}
}

func TestKeysCreateUnaryErr(t *testing.T) {
	t.Parallel()
	f := &fakeDeps{unaryFn: func(_ context.Context, _ string, _, _ any) error { return errors.New("boom") }}
	stdio, _, _ := testcli.NewIO(strings.NewReader(""))
	err := New(f.deps()).Run(context.Background(), []string{"keys", "create", "--name", "n"}, stdio)
	if err == nil || !strings.Contains(err.Error(), "boom") {
		t.Errorf("err=%v", err)
	}
}

func TestKeysCreateBadFlag(t *testing.T) {
	t.Parallel()
	f := &fakeDeps{}
	stdio, _, _ := testcli.NewIO(strings.NewReader(""))
	err := New(f.deps()).Run(context.Background(), []string{"keys", "create", "--nope"}, stdio)
	if !errors.Is(err, cli.ErrUsage) {
		t.Errorf("err=%v", err)
	}
}

// --- keys rotate ---

func TestKeysRotateHappy(t *testing.T) {
	t.Parallel()
	f := &fakeDeps{
		unaryFn: func(_ context.Context, _ string, _, resp any) error {
			out := resp.(*createApiKeyResp)
			out.APIKeyHash = "new-plaintext"
			return nil
		},
	}
	cmd := &keysRotateCmd{deps: f.deps()}
	stdio, out, errb := testcli.NewIO(strings.NewReader(""))
	if err := cmd.Run(context.Background(), []string{"k-id"}, stdio); err != nil {
		t.Fatalf("err=%v", err)
	}
	if !strings.Contains(out.String(), "new-plaintext") {
		t.Errorf("stdout=%q", out.String())
	}
	if !strings.Contains(errb.String(), "# store this token") {
		t.Errorf("stderr=%q", errb.String())
	}
	if !strings.Contains(string(f.lastRawReq), `"apiKeyId":"k-id"`) {
		t.Errorf("req=%s", string(f.lastRawReq))
	}
}

func TestKeysRotateMissingPositional(t *testing.T) {
	t.Parallel()
	f := &fakeDeps{}
	cmd := &keysRotateCmd{deps: f.deps()}
	stdio, _, _ := testcli.NewIO(strings.NewReader(""))
	err := cmd.Run(context.Background(), nil, stdio)
	if !errors.Is(err, cli.ErrUsage) {
		t.Errorf("err=%v", err)
	}
}

func TestKeysRotateUnaryErr(t *testing.T) {
	t.Parallel()
	f := &fakeDeps{unaryFn: func(_ context.Context, _ string, _, _ any) error { return errors.New("boom") }}
	cmd := &keysRotateCmd{deps: f.deps()}
	stdio, _, _ := testcli.NewIO(strings.NewReader(""))
	err := cmd.Run(context.Background(), []string{"id"}, stdio)
	if err == nil || !strings.Contains(err.Error(), "boom") {
		t.Errorf("err=%v", err)
	}
}

func TestKeysRotateBadFlag(t *testing.T) {
	t.Parallel()
	f := &fakeDeps{}
	stdio, _, _ := testcli.NewIO(strings.NewReader(""))
	err := New(f.deps()).Run(context.Background(), []string{"keys", "rotate", "--nope"}, stdio)
	if !errors.Is(err, cli.ErrUsage) {
		t.Errorf("err=%v", err)
	}
}

// Extra trailing positionals must be rejected: the migration to
// cli.RequireStringID introduced a regression where args[1:] were silently
// dropped, so this test pins the strict-arity contract.
func TestKeysRotateExtraPositional(t *testing.T) {
	t.Parallel()
	called := 0
	f := &fakeDeps{unaryFn: func(_ context.Context, _ string, _, _ any) error { called++; return nil }}
	cmd := &keysRotateCmd{deps: f.deps()}
	stdio, _, _ := testcli.NewIO(strings.NewReader(""))
	err := cmd.Run(context.Background(), []string{"id", "extra"}, stdio)
	if !errors.Is(err, cli.ErrUsage) {
		t.Errorf("err=%v want ErrUsage", err)
	}
	if called != 0 {
		t.Errorf("Unary must not be invoked on arity failure, called=%d", called)
	}
}

// --- keys revoke ---

func TestKeysRevokeHappy(t *testing.T) {
	t.Parallel()
	f := &fakeDeps{}
	cmd := &keysRevokeCmd{deps: f.deps()}
	stdio, out, _ := testcli.NewIO(strings.NewReader(""))
	if err := cmd.Run(context.Background(), []string{"k-id"}, stdio); err != nil {
		t.Fatalf("err=%v", err)
	}
	if !strings.Contains(out.String(), "revoked k-id") {
		t.Errorf("stdout=%q", out.String())
	}
	if !strings.Contains(string(f.lastRawReq), `"apiKeyId":"k-id"`) {
		t.Errorf("req=%s", string(f.lastRawReq))
	}
}

func TestKeysRevokeMissingPositional(t *testing.T) {
	t.Parallel()
	f := &fakeDeps{}
	cmd := &keysRevokeCmd{deps: f.deps()}
	stdio, _, _ := testcli.NewIO(strings.NewReader(""))
	err := cmd.Run(context.Background(), nil, stdio)
	if !errors.Is(err, cli.ErrUsage) {
		t.Errorf("err=%v", err)
	}
}

func TestKeysRevokeUnaryErr(t *testing.T) {
	t.Parallel()
	f := &fakeDeps{unaryFn: func(_ context.Context, _ string, _, _ any) error { return errors.New("boom") }}
	cmd := &keysRevokeCmd{deps: f.deps()}
	stdio, _, _ := testcli.NewIO(strings.NewReader(""))
	err := cmd.Run(context.Background(), []string{"id"}, stdio)
	if err == nil || !strings.Contains(err.Error(), "boom") {
		t.Errorf("err=%v", err)
	}
}

func TestKeysRevokeBadFlag(t *testing.T) {
	t.Parallel()
	f := &fakeDeps{}
	stdio, _, _ := testcli.NewIO(strings.NewReader(""))
	err := New(f.deps()).Run(context.Background(), []string{"keys", "revoke", "--nope"}, stdio)
	if !errors.Is(err, cli.ErrUsage) {
		t.Errorf("err=%v", err)
	}
}

func TestKeysRevokeExtraPositional(t *testing.T) {
	t.Parallel()
	called := 0
	f := &fakeDeps{unaryFn: func(_ context.Context, _ string, _, _ any) error { called++; return nil }}
	cmd := &keysRevokeCmd{deps: f.deps()}
	stdio, _, _ := testcli.NewIO(strings.NewReader(""))
	err := cmd.Run(context.Background(), []string{"id", "extra"}, stdio)
	if !errors.Is(err, cli.ErrUsage) {
		t.Errorf("err=%v want ErrUsage", err)
	}
	if called != 0 {
		t.Errorf("Unary must not be invoked on arity failure, called=%d", called)
	}
}
