package auth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/highperformance-tech/ana-cli/internal/cli"
	"github.com/highperformance-tech/ana-cli/internal/testcli"
)

// memberPath and orgPath are the two RPCs whoami fans out. Duplicating them
// here (rather than importing from whoami.go) keeps tests decoupled from the
// production constant names while still asserting the exact wire paths.
const (
	memberPath = "/rpc/public/textql.rpc.public.auth.PublicAuthService/GetMember"
	orgPath    = "/rpc/public/textql.rpc.public.auth.PublicAuthService/GetOrganization"
)

// whoamiRouter builds a Unary fake that dispatches by path to per-endpoint
// handlers. Each handler receives the resp pointer and returns an error. A
// nil handler means "succeed with empty payload" so callers can leave out
// branches they don't care about.
func whoamiRouter(member, org func(resp any) error) func(context.Context, string, any, any) error {
	return func(_ context.Context, path string, _ any, resp any) error {
		switch path {
		case memberPath:
			if member != nil {
				return member(resp)
			}
		case orgPath:
			if org != nil {
				return org(resp)
			}
		default:
			return fmt.Errorf("unexpected path %s", path)
		}
		return nil
	}
}

// setMap writes v into a *map[string]any response pointer; abstracts the
// type-assertion boilerplate that would otherwise repeat in every fake.
func setMap(resp any, v map[string]any) {
	out := resp.(*map[string]any)
	*out = v
}

func TestWhoamiHappy(t *testing.T) {
	t.Parallel()
	f := &fakeDeps{
		loadFn: func() (Config, error) { return Config{Token: "t"}, nil },
		unaryFn: whoamiRouter(
			func(resp any) error {
				setMap(resp, map[string]any{"member": map[string]any{
					"emailAddress": "user@example.com",
					"orgId":        "f31322df",
					"role":         "member",
				}})
				return nil
			},
			func(resp any) error {
				setMap(resp, map[string]any{"organization": map[string]any{
					"orgId":            "f31322df",
					"organizationName": "Example Org",
				}})
				return nil
			},
		),
	}
	cmd := &whoamiCmd{deps: f.deps()}
	stdio, out, _ := testcli.NewIO(strings.NewReader(""))
	if err := cmd.Run(context.Background(), nil, stdio); err != nil {
		t.Fatalf("err=%v", err)
	}
	s := out.String()
	// All four columns must render on their own labelled line.
	for _, want := range []string{
		"email", "user@example.com",
		"organization", "Example Org",
		"orgId", "f31322df",
		"role", "member",
	} {
		if !strings.Contains(s, want) {
			t.Errorf("stdout missing %q: %s", want, s)
		}
	}
}

func TestWhoamiJSON(t *testing.T) {
	t.Parallel()
	f := &fakeDeps{
		loadFn: func() (Config, error) { return Config{Token: "t"}, nil },
		unaryFn: whoamiRouter(
			func(resp any) error {
				setMap(resp, map[string]any{"member": map[string]any{"emailAddress": "x@y", "role": "admin"}})
				return nil
			},
			func(resp any) error {
				setMap(resp, map[string]any{"organization": map[string]any{"organizationName": "Acme", "orgId": "o1"}})
				return nil
			},
		),
	}
	cmd := &whoamiCmd{deps: f.deps()}
	ctx := cli.WithGlobal(context.Background(), cli.Global{JSON: true})
	stdio, out, _ := testcli.NewIO(strings.NewReader(""))
	if err := cmd.Run(ctx, nil, stdio); err != nil {
		t.Fatalf("err=%v", err)
	}
	// Round-trip the wrapper to assert both raw maps survived.
	var got map[string]any
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatalf("output is not JSON: %v, %s", err, out.String())
	}
	m, ok := got["member"].(map[string]any)
	if !ok || m["member"] == nil {
		t.Errorf("wrapper missing member: %v", got)
	}
	o, ok := got["organization"].(map[string]any)
	if !ok || o["organization"] == nil {
		t.Errorf("wrapper missing organization: %v", got)
	}
}

func TestWhoamiNoToken(t *testing.T) {
	t.Parallel()
	called := 0
	f := &fakeDeps{
		loadFn: func() (Config, error) { return Config{}, nil },
		unaryFn: func(_ context.Context, _ string, _, _ any) error {
			called++
			return nil
		},
	}
	cmd := &whoamiCmd{deps: f.deps()}
	stdio, _, _ := testcli.NewIO(strings.NewReader(""))
	err := cmd.Run(context.Background(), nil, stdio)
	if !errors.Is(err, ErrNotLoggedIn) {
		t.Errorf("err=%v want ErrNotLoggedIn", err)
	}
	if called != 0 {
		t.Errorf("Unary called %d times before token check", called)
	}
}

func TestWhoamiLoadErr(t *testing.T) {
	t.Parallel()
	f := &fakeDeps{loadFn: func() (Config, error) { return Config{}, errors.New("load boom") }}
	cmd := &whoamiCmd{deps: f.deps()}
	stdio, _, _ := testcli.NewIO(strings.NewReader(""))
	err := cmd.Run(context.Background(), nil, stdio)
	if err == nil || !strings.Contains(err.Error(), "load boom") {
		t.Errorf("err=%v", err)
	}
}

func TestWhoamiMemberErr(t *testing.T) {
	t.Parallel()
	f := &fakeDeps{
		loadFn: func() (Config, error) { return Config{Token: "t"}, nil },
		unaryFn: whoamiRouter(
			func(_ any) error { return errors.New("member boom") },
			func(resp any) error {
				setMap(resp, map[string]any{"organization": map[string]any{"organizationName": "x"}})
				return nil
			},
		),
	}
	cmd := &whoamiCmd{deps: f.deps()}
	stdio, _, _ := testcli.NewIO(strings.NewReader(""))
	err := cmd.Run(context.Background(), nil, stdio)
	if err == nil {
		t.Fatalf("expected error")
	}
	if !strings.Contains(err.Error(), "member boom") {
		t.Errorf("err=%v", err)
	}
	if !strings.Contains(err.Error(), "auth whoami:") {
		t.Errorf("err not wrapped: %v", err)
	}
}

func TestWhoamiOrgErr(t *testing.T) {
	t.Parallel()
	f := &fakeDeps{
		loadFn: func() (Config, error) { return Config{Token: "t"}, nil },
		unaryFn: whoamiRouter(
			func(resp any) error {
				setMap(resp, map[string]any{"member": map[string]any{"emailAddress": "x"}})
				return nil
			},
			func(_ any) error { return errors.New("org boom") },
		),
	}
	cmd := &whoamiCmd{deps: f.deps()}
	stdio, _, _ := testcli.NewIO(strings.NewReader(""))
	err := cmd.Run(context.Background(), nil, stdio)
	if err == nil {
		t.Fatalf("expected error")
	}
	if !strings.Contains(err.Error(), "org boom") {
		t.Errorf("err=%v", err)
	}
	if !strings.Contains(err.Error(), "auth whoami:") {
		t.Errorf("err not wrapped: %v", err)
	}
}

func TestWhoamiBothErr(t *testing.T) {
	t.Parallel()
	// Both goroutines fail — errors.Join yields a deterministic surface that
	// contains both messages regardless of scheduler ordering.
	f := &fakeDeps{
		loadFn: func() (Config, error) { return Config{Token: "t"}, nil },
		unaryFn: whoamiRouter(
			func(_ any) error { return errors.New("member boom") },
			func(_ any) error { return errors.New("org boom") },
		),
	}
	cmd := &whoamiCmd{deps: f.deps()}
	stdio, _, _ := testcli.NewIO(strings.NewReader(""))
	err := cmd.Run(context.Background(), nil, stdio)
	if err == nil {
		t.Fatalf("expected error")
	}
	if !strings.Contains(err.Error(), "member boom") {
		t.Errorf("missing member error: %v", err)
	}
	if !strings.Contains(err.Error(), "org boom") {
		t.Errorf("missing org error: %v", err)
	}
	if !strings.Contains(err.Error(), "auth whoami:") {
		t.Errorf("err not wrapped: %v", err)
	}
}

// TestWhoamiCancelsSiblingOnFirstErr pins the cancel-on-first-error contract:
// when one RPC fails fast, the shared fan-out ctx must be cancelled so the
// sibling RPC observes context.Canceled instead of running to completion and
// burning its full request budget. The "slow" RPC blocks on ctx.Done so its
// observed ctx error can be asserted after Run returns.
func TestWhoamiCancelsSiblingOnFirstErr(t *testing.T) {
	t.Parallel()
	observed := make(chan error, 1)
	f := &fakeDeps{
		loadFn: func() (Config, error) { return Config{Token: "t"}, nil },
		unaryFn: func(ctx context.Context, path string, _ any, _ any) error {
			switch path {
			case memberPath:
				return errors.New("member boom")
			case orgPath:
				<-ctx.Done()
				observed <- ctx.Err()
				return ctx.Err()
			}
			return fmt.Errorf("unexpected path %s", path)
		},
	}
	cmd := &whoamiCmd{deps: f.deps()}
	stdio, _, _ := testcli.NewIO(strings.NewReader(""))
	err := cmd.Run(context.Background(), nil, stdio)
	if err == nil {
		t.Fatalf("expected error")
	}
	// Run's wg.Wait guarantees the slow side returned before we got here, so
	// `observed` already has its value buffered — this receive is non-blocking
	// in practice but we keep the channel form for the happens-before edge.
	slowCtxErr := <-observed
	if !errors.Is(slowCtxErr, context.Canceled) {
		t.Errorf("sibling RPC ctx err = %v, want context.Canceled", slowCtxErr)
	}
	if !strings.Contains(err.Error(), "member boom") {
		t.Errorf("caller missing member error: %v", err)
	}
}

// stubAuthErr implements authSignaler. Used to verify auth-error translation
// flows up through whoami (but any command's Unary would behave identically).
type stubAuthErr struct{}

func (stubAuthErr) Error() string     { return "remote says no" }
func (stubAuthErr) IsAuthError() bool { return true }

func TestWhoamiAuthErrTranslated(t *testing.T) {
	t.Parallel()
	f := &fakeDeps{
		loadFn:  func() (Config, error) { return Config{Token: "t"}, nil },
		unaryFn: func(_ context.Context, _ string, _, _ any) error { return stubAuthErr{} },
	}
	cmd := &whoamiCmd{deps: f.deps()}
	stdio, _, _ := testcli.NewIO(strings.NewReader(""))
	err := cmd.Run(context.Background(), nil, stdio)
	// cli.ExitCode uses errors.As with an IsAuthError()-bearing interface.
	var signaler interface{ IsAuthError() bool }
	if !errors.As(err, &signaler) || !signaler.IsAuthError() {
		t.Errorf("expected translated auth error, got %v", err)
	}
}

func TestWhoamiAuthErrViaString(t *testing.T) {
	t.Parallel()
	// Server returned a plain error with "unauthenticated" in the message —
	// translateErr should still flag it.
	f := &fakeDeps{
		loadFn:  func() (Config, error) { return Config{Token: "t"}, nil },
		unaryFn: func(_ context.Context, _ string, _, _ any) error { return errors.New("Unauthenticated request") },
	}
	cmd := &whoamiCmd{deps: f.deps()}
	stdio, _, _ := testcli.NewIO(strings.NewReader(""))
	err := cmd.Run(context.Background(), nil, stdio)
	var signaler interface{ IsAuthError() bool }
	if !errors.As(err, &signaler) || !signaler.IsAuthError() {
		t.Errorf("expected translated auth error, got %v", err)
	}
}

func TestWhoamiMissingEmail(t *testing.T) {
	t.Parallel()
	f := &fakeDeps{
		loadFn: func() (Config, error) { return Config{Token: "t"}, nil },
		unaryFn: whoamiRouter(
			func(resp any) error {
				setMap(resp, map[string]any{"member": map[string]any{}})
				return nil
			},
			func(resp any) error {
				setMap(resp, map[string]any{"organization": map[string]any{"organizationName": "x"}})
				return nil
			},
		),
	}
	cmd := &whoamiCmd{deps: f.deps()}
	stdio, _, _ := testcli.NewIO(strings.NewReader(""))
	err := cmd.Run(context.Background(), nil, stdio)
	if err == nil || !strings.Contains(err.Error(), "emailAddress") {
		t.Errorf("err=%v", err)
	}
}

func TestWhoamiMissingOrgName(t *testing.T) {
	t.Parallel()
	// Org missing organizationName must not error: org is secondary to the
	// "who am I" claim; the line simply renders with an empty value.
	f := &fakeDeps{
		loadFn: func() (Config, error) { return Config{Token: "t"}, nil },
		unaryFn: whoamiRouter(
			func(resp any) error {
				setMap(resp, map[string]any{"member": map[string]any{"emailAddress": "x@y", "role": "member"}})
				return nil
			},
			func(resp any) error {
				setMap(resp, map[string]any{"organization": map[string]any{}})
				return nil
			},
		),
	}
	cmd := &whoamiCmd{deps: f.deps()}
	stdio, out, _ := testcli.NewIO(strings.NewReader(""))
	if err := cmd.Run(context.Background(), nil, stdio); err != nil {
		t.Fatalf("unexpected err=%v", err)
	}
	if !strings.Contains(out.String(), "organization") {
		t.Errorf("missing organization line: %q", out.String())
	}
	if !strings.Contains(out.String(), "x@y") {
		t.Errorf("missing email: %q", out.String())
	}
}

func TestWhoamiBadFlag(t *testing.T) {
	t.Parallel()
	f := &fakeDeps{}
	cmd := &whoamiCmd{deps: f.deps()}
	stdio, _, _ := testcli.NewIO(strings.NewReader(""))
	err := cmd.Run(context.Background(), []string{"--nope"}, stdio)
	if !errors.Is(err, cli.ErrUsage) {
		t.Errorf("err=%v", err)
	}
}

func TestWhoamiJSONEncodeError(t *testing.T) {
	t.Parallel()
	f := &fakeDeps{
		loadFn: func() (Config, error) { return Config{Token: "t"}, nil },
		unaryFn: whoamiRouter(
			func(resp any) error {
				setMap(resp, map[string]any{"member": map[string]any{"emailAddress": "x"}})
				return nil
			},
			func(resp any) error {
				setMap(resp, map[string]any{"organization": map[string]any{"organizationName": "x"}})
				return nil
			},
		),
	}
	cmd := &whoamiCmd{deps: f.deps()}
	ctx := cli.WithGlobal(context.Background(), cli.Global{JSON: true})
	stdio := testcli.FailingIO()
	err := cmd.Run(ctx, nil, stdio)
	if err == nil || !strings.Contains(err.Error(), "w boom") {
		t.Errorf("err=%v", err)
	}
}

// TestWhoamiMemberRemarshalErr / TestWhoamiOrgRemarshalErr: the remarshal path
// can fail if the server returns a shape we can't decode into the typed
// struct. Force that by returning `member` as a non-object.
func TestWhoamiMemberRemarshalErr(t *testing.T) {
	t.Parallel()
	f := &fakeDeps{
		loadFn: func() (Config, error) { return Config{Token: "t"}, nil },
		unaryFn: whoamiRouter(
			func(resp any) error {
				setMap(resp, map[string]any{"member": "not-an-object"})
				return nil
			},
			func(resp any) error {
				setMap(resp, map[string]any{"organization": map[string]any{"organizationName": "x"}})
				return nil
			},
		),
	}
	cmd := &whoamiCmd{deps: f.deps()}
	stdio, _, _ := testcli.NewIO(strings.NewReader(""))
	err := cmd.Run(context.Background(), nil, stdio)
	if err == nil || !strings.Contains(err.Error(), "decode response") {
		t.Errorf("err=%v", err)
	}
}

func TestWhoamiOrgRemarshalErr(t *testing.T) {
	t.Parallel()
	f := &fakeDeps{
		loadFn: func() (Config, error) { return Config{Token: "t"}, nil },
		unaryFn: whoamiRouter(
			func(resp any) error {
				setMap(resp, map[string]any{"member": map[string]any{"emailAddress": "x"}})
				return nil
			},
			func(resp any) error {
				setMap(resp, map[string]any{"organization": "not-an-object"})
				return nil
			},
		),
	}
	cmd := &whoamiCmd{deps: f.deps()}
	stdio, _, _ := testcli.NewIO(strings.NewReader(""))
	err := cmd.Run(context.Background(), nil, stdio)
	if err == nil || !strings.Contains(err.Error(), "decode response") {
		t.Errorf("err=%v", err)
	}
}
