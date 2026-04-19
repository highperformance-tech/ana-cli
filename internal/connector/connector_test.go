package connector

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/highperformance-tech/ana-cli/internal/cli"
)

// --- fakes and helpers ---

// fakeDeps captures each Unary invocation's path + encoded request bytes so
// tests can assert on both the endpoint and the wire-level JSON shape
// (camelCase field names, omitted-when-empty behavior, etc.).
type fakeDeps struct {
	unaryFn    func(ctx context.Context, path string, req, resp any) error
	lastPath   string
	lastRawReq []byte
}

func (f *fakeDeps) deps() Deps {
	return Deps{
		Unary: func(ctx context.Context, path string, req, resp any) error {
			f.lastPath = path
			if b, err := json.Marshal(req); err == nil {
				f.lastRawReq = b
			}
			if f.unaryFn != nil {
				return f.unaryFn(ctx, path, req, resp)
			}
			return nil
		},
	}
}

// newIO returns a cli.IO with in-memory streams.
func newIO(stdin io.Reader) (cli.IO, *bytes.Buffer, *bytes.Buffer) {
	var out, errb bytes.Buffer
	return cli.IO{
		Stdin:  stdin,
		Stdout: &out,
		Stderr: &errb,
		Env:    func(string) string { return "" },
		Now:    func() time.Time { return time.Unix(0, 0) },
	}, &out, &errb
}

// errReader trips the password-stdin / scanner error path.
type errReader struct{ err error }

func (e errReader) Read([]byte) (int, error) { return 0, e.err }

// --- New / Group surface ---

func TestNewReturnsGroupWithExpectedChildren(t *testing.T) {
	g := New(Deps{})
	if g == nil || g.Children == nil {
		t.Fatalf("New returned empty group")
	}
	expected := []string{"list", "get", "create", "update", "delete", "test", "tables", "examples"}
	for _, name := range expected {
		if _, ok := g.Children[name]; !ok {
			t.Errorf("missing child %q", name)
		}
	}
	if g.Summary == "" {
		t.Errorf("Summary should be non-empty")
	}
}

// --- Help() coverage ---

func TestHelpStringsNonEmpty(t *testing.T) {
	cases := map[string]cli.Command{
		"list":     &listCmd{},
		"get":      &getCmd{},
		"create":   &createCmd{},
		"update":   &updateCmd{},
		"delete":   &deleteCmd{},
		"test":     &testCmd{},
		"tables":   &tablesCmd{},
		"examples": &examplesCmd{},
	}
	for n, c := range cases {
		h := c.Help()
		if h == "" {
			t.Errorf("%s: empty help", n)
		}
		if !strings.Contains(strings.ToLower(h), "usage") {
			t.Errorf("%s: help missing usage: %q", n, h)
		}
	}
}

// --- list ---

func TestListTable(t *testing.T) {
	f := &fakeDeps{
		unaryFn: func(_ context.Context, _ string, _, resp any) error {
			out := resp.(*map[string]any)
			*out = map[string]any{
				"connectors": []any{
					map[string]any{"id": 1.0, "name": "alpha", "connectorType": "POSTGRES"},
					map[string]any{"id": 2.0, "name": "beta", "connectorType": "BIGQUERY"},
				},
			}
			return nil
		},
	}
	cmd := &listCmd{deps: f.deps()}
	stdio, out, _ := newIO(strings.NewReader(""))
	if err := cmd.Run(context.Background(), nil, stdio); err != nil {
		t.Fatalf("err=%v", err)
	}
	s := out.String()
	if !strings.Contains(s, "ID") || !strings.Contains(s, "NAME") || !strings.Contains(s, "TYPE") {
		t.Errorf("missing headers: %q", s)
	}
	if !strings.Contains(s, "alpha") || !strings.Contains(s, "beta") {
		t.Errorf("missing rows: %q", s)
	}
	if f.lastPath != servicePath+"/GetConnectors" {
		t.Errorf("path=%s", f.lastPath)
	}
	// Request is empty object.
	if string(f.lastRawReq) != "{}" {
		t.Errorf("req=%s", string(f.lastRawReq))
	}
}

func TestListJSON(t *testing.T) {
	f := &fakeDeps{
		unaryFn: func(_ context.Context, _ string, _, resp any) error {
			out := resp.(*map[string]any)
			*out = map[string]any{"connectors": []any{}}
			return nil
		},
	}
	cmd := &listCmd{deps: f.deps()}
	ctx := cli.WithGlobal(context.Background(), cli.Global{JSON: true})
	stdio, out, _ := newIO(strings.NewReader(""))
	if err := cmd.Run(ctx, nil, stdio); err != nil {
		t.Fatalf("err=%v", err)
	}
	if !strings.Contains(out.String(), "\"connectors\"") {
		t.Errorf("stdout=%q", out.String())
	}
}

func TestListUnaryErr(t *testing.T) {
	f := &fakeDeps{unaryFn: func(_ context.Context, _ string, _, _ any) error { return errors.New("boom") }}
	cmd := &listCmd{deps: f.deps()}
	stdio, _, _ := newIO(strings.NewReader(""))
	err := cmd.Run(context.Background(), nil, stdio)
	if err == nil || !strings.Contains(err.Error(), "boom") {
		t.Errorf("err=%v", err)
	}
	// And wrapped context.
	if err == nil || !strings.Contains(err.Error(), "connector list") {
		t.Errorf("err=%v", err)
	}
}

func TestListBadFlag(t *testing.T) {
	cmd := &listCmd{deps: (&fakeDeps{}).deps()}
	stdio, _, _ := newIO(strings.NewReader(""))
	err := cmd.Run(context.Background(), []string{"--nope"}, stdio)
	if !errors.Is(err, cli.ErrUsage) {
		t.Errorf("err=%v", err)
	}
}

func TestListRemarshalErr(t *testing.T) {
	f := &fakeDeps{
		unaryFn: func(_ context.Context, _ string, _, resp any) error {
			out := resp.(*map[string]any)
			*out = map[string]any{"connectors": "not-an-array"}
			return nil
		},
	}
	cmd := &listCmd{deps: f.deps()}
	stdio, _, _ := newIO(strings.NewReader(""))
	err := cmd.Run(context.Background(), nil, stdio)
	if err == nil || !strings.Contains(err.Error(), "decode response") {
		t.Errorf("err=%v", err)
	}
}

// --- get ---

func TestGetTwoCol(t *testing.T) {
	f := &fakeDeps{
		unaryFn: func(_ context.Context, _ string, _, resp any) error {
			out := resp.(*map[string]any)
			*out = map[string]any{
				"connector": map[string]any{
					"id":            float64(42),
					"name":          "pg",
					"connectorType": "POSTGRES",
					"postgresMetadata": map[string]any{
						"host": "127.0.0.1",
						"port": float64(5432),
					},
				},
			}
			return nil
		},
	}
	cmd := &getCmd{deps: f.deps()}
	stdio, out, _ := newIO(strings.NewReader(""))
	if err := cmd.Run(context.Background(), []string{"42"}, stdio); err != nil {
		t.Fatalf("err=%v", err)
	}
	s := out.String()
	if !strings.Contains(s, "name:") || !strings.Contains(s, "pg") {
		t.Errorf("stdout=%q", s)
	}
	if !strings.Contains(s, "postgresMetadata:") || !strings.Contains(s, "host:") {
		t.Errorf("nested block missing: %q", s)
	}
	if !strings.Contains(string(f.lastRawReq), `"connectorId":42`) {
		t.Errorf("req=%s", string(f.lastRawReq))
	}
	if f.lastPath != servicePath+"/GetConnector" {
		t.Errorf("path=%s", f.lastPath)
	}
}

func TestGetJSON(t *testing.T) {
	f := &fakeDeps{
		unaryFn: func(_ context.Context, _ string, _, resp any) error {
			out := resp.(*map[string]any)
			*out = map[string]any{"connector": map[string]any{"id": 1.0}}
			return nil
		},
	}
	cmd := &getCmd{deps: f.deps()}
	ctx := cli.WithGlobal(context.Background(), cli.Global{JSON: true})
	stdio, out, _ := newIO(strings.NewReader(""))
	if err := cmd.Run(ctx, []string{"1"}, stdio); err != nil {
		t.Fatalf("err=%v", err)
	}
	if !strings.Contains(out.String(), "\"connector\"") {
		t.Errorf("stdout=%q", out.String())
	}
}

func TestGetFallbackNoConnectorKey(t *testing.T) {
	// When `connector` key is absent the command falls back to raw JSON.
	f := &fakeDeps{
		unaryFn: func(_ context.Context, _ string, _, resp any) error {
			out := resp.(*map[string]any)
			*out = map[string]any{"other": 1.0}
			return nil
		},
	}
	cmd := &getCmd{deps: f.deps()}
	stdio, out, _ := newIO(strings.NewReader(""))
	if err := cmd.Run(context.Background(), []string{"1"}, stdio); err != nil {
		t.Fatalf("err=%v", err)
	}
	if !strings.Contains(out.String(), "\"other\"") {
		t.Errorf("stdout=%q", out.String())
	}
}

func TestGetMissingPositional(t *testing.T) {
	cmd := &getCmd{deps: (&fakeDeps{}).deps()}
	stdio, _, _ := newIO(strings.NewReader(""))
	err := cmd.Run(context.Background(), nil, stdio)
	if !errors.Is(err, cli.ErrUsage) {
		t.Errorf("err=%v", err)
	}
}

func TestGetNonIntPositional(t *testing.T) {
	cmd := &getCmd{deps: (&fakeDeps{}).deps()}
	stdio, _, _ := newIO(strings.NewReader(""))
	err := cmd.Run(context.Background(), []string{"abc"}, stdio)
	if !errors.Is(err, cli.ErrUsage) {
		t.Errorf("err=%v", err)
	}
}

func TestGetUnaryErr(t *testing.T) {
	f := &fakeDeps{unaryFn: func(_ context.Context, _ string, _, _ any) error { return errors.New("boom") }}
	cmd := &getCmd{deps: f.deps()}
	stdio, _, _ := newIO(strings.NewReader(""))
	err := cmd.Run(context.Background(), []string{"1"}, stdio)
	if err == nil || !strings.Contains(err.Error(), "boom") {
		t.Errorf("err=%v", err)
	}
}

func TestGetBadFlag(t *testing.T) {
	cmd := &getCmd{deps: (&fakeDeps{}).deps()}
	stdio, _, _ := newIO(strings.NewReader(""))
	err := cmd.Run(context.Background(), []string{"--nope"}, stdio)
	if !errors.Is(err, cli.ErrUsage) {
		t.Errorf("err=%v", err)
	}
}

// --- create ---

// requiredFlags is the minimal happy-path flag set for create.
func requiredFlags() []string {
	return []string{
		"--type", "postgres",
		"--name", "pg1",
		"--host", "h",
		"--port", "5432",
		"--user", "u",
		"--database", "d",
		"--password", "p",
	}
}

func TestCreateHappy(t *testing.T) {
	f := &fakeDeps{
		unaryFn: func(_ context.Context, _ string, _, resp any) error {
			out := resp.(*map[string]any)
			*out = map[string]any{"connectorId": 99.0, "name": "pg1", "connectorType": "POSTGRES"}
			return nil
		},
	}
	cmd := &createCmd{deps: f.deps()}
	stdio, out, _ := newIO(strings.NewReader(""))
	if err := cmd.Run(context.Background(), requiredFlags(), stdio); err != nil {
		t.Fatalf("err=%v", err)
	}
	s := out.String()
	if !strings.Contains(s, "connectorId: 99") || !strings.Contains(s, "name: pg1") {
		t.Errorf("stdout=%q", s)
	}
	// Verify camelCase wire fields.
	req := string(f.lastRawReq)
	for _, want := range []string{
		`"connectorType":"POSTGRES"`, `"name":"pg1"`,
		`"postgres":`, `"host":"h"`, `"port":5432`, `"user":"u"`,
		`"password":"p"`, `"database":"d"`,
	} {
		if !strings.Contains(req, want) {
			t.Errorf("req missing %s in %s", want, req)
		}
	}
	if f.lastPath != servicePath+"/CreateConnector" {
		t.Errorf("path=%s", f.lastPath)
	}
}

func TestCreateJSONBypass(t *testing.T) {
	f := &fakeDeps{
		unaryFn: func(_ context.Context, _ string, _, resp any) error {
			out := resp.(*map[string]any)
			*out = map[string]any{"connectorId": 1.0, "name": "n", "connectorType": "POSTGRES"}
			return nil
		},
	}
	cmd := &createCmd{deps: f.deps()}
	ctx := cli.WithGlobal(context.Background(), cli.Global{JSON: true})
	stdio, out, _ := newIO(strings.NewReader(""))
	if err := cmd.Run(ctx, requiredFlags(), stdio); err != nil {
		t.Fatalf("err=%v", err)
	}
	// JSON dump path — ensure table-style "connectorId:" formatting isn't used;
	// raw JSON would have `"connectorId":` (with quotes).
	if !strings.Contains(out.String(), "\"connectorId\"") {
		t.Errorf("stdout=%q", out.String())
	}
}

func TestCreatePasswordStdin(t *testing.T) {
	f := &fakeDeps{}
	cmd := &createCmd{deps: f.deps()}
	args := []string{
		"--type", "postgres", "--name", "n", "--host", "h",
		"--port", "5432", "--user", "u", "--database", "d",
		"--password-stdin",
	}
	stdio, _, _ := newIO(strings.NewReader("secret-line\n"))
	if err := cmd.Run(context.Background(), args, stdio); err != nil {
		t.Fatalf("err=%v", err)
	}
	if !strings.Contains(string(f.lastRawReq), `"password":"secret-line"`) {
		t.Errorf("req=%s", string(f.lastRawReq))
	}
}

func TestCreatePasswordStdinEmpty(t *testing.T) {
	cmd := &createCmd{deps: (&fakeDeps{}).deps()}
	args := []string{
		"--type", "postgres", "--name", "n", "--host", "h",
		"--port", "5432", "--user", "u", "--database", "d",
		"--password-stdin",
	}
	stdio, _, _ := newIO(strings.NewReader(""))
	err := cmd.Run(context.Background(), args, stdio)
	if !errors.Is(err, cli.ErrUsage) {
		t.Errorf("err=%v", err)
	}
}

func TestCreatePasswordStdinReadErr(t *testing.T) {
	cmd := &createCmd{deps: (&fakeDeps{}).deps()}
	args := []string{
		"--type", "postgres", "--name", "n", "--host", "h",
		"--port", "5432", "--user", "u", "--database", "d",
		"--password-stdin",
	}
	stdio, _, _ := newIO(errReader{err: errors.New("read fail")})
	err := cmd.Run(context.Background(), args, stdio)
	if err == nil || !strings.Contains(err.Error(), "read fail") {
		t.Errorf("err=%v", err)
	}
}

func TestCreatePasswordStdinNilReader(t *testing.T) {
	// resolvePassword directly, to exercise the nil-reader branch.
	_, err := resolvePassword("", true, nil)
	if err == nil {
		t.Errorf("want error on nil reader")
	}
}

func TestCreateMissingPassword(t *testing.T) {
	cmd := &createCmd{deps: (&fakeDeps{}).deps()}
	// Missing both --password and --password-stdin.
	args := []string{
		"--type", "postgres", "--name", "n", "--host", "h",
		"--port", "5432", "--user", "u", "--database", "d",
	}
	stdio, _, _ := newIO(strings.NewReader(""))
	err := cmd.Run(context.Background(), args, stdio)
	if !errors.Is(err, cli.ErrUsage) {
		t.Errorf("err=%v", err)
	}
}

func TestCreateWrongType(t *testing.T) {
	cmd := &createCmd{deps: (&fakeDeps{}).deps()}
	args := []string{"--type", "mysql", "--name", "n"}
	stdio, _, _ := newIO(strings.NewReader(""))
	err := cmd.Run(context.Background(), args, stdio)
	if !errors.Is(err, cli.ErrUsage) {
		t.Errorf("err=%v", err)
	}
}

func TestCreateMissingFlags(t *testing.T) {
	cmd := &createCmd{deps: (&fakeDeps{}).deps()}
	// type is ok but everything else missing.
	args := []string{"--type", "postgres"}
	stdio, _, _ := newIO(strings.NewReader(""))
	err := cmd.Run(context.Background(), args, stdio)
	if !errors.Is(err, cli.ErrUsage) {
		t.Errorf("err=%v", err)
	}
}

func TestCreateBadFlag(t *testing.T) {
	cmd := &createCmd{deps: (&fakeDeps{}).deps()}
	stdio, _, _ := newIO(strings.NewReader(""))
	err := cmd.Run(context.Background(), []string{"--nope"}, stdio)
	if !errors.Is(err, cli.ErrUsage) {
		t.Errorf("err=%v", err)
	}
}

func TestCreateUnaryErr(t *testing.T) {
	f := &fakeDeps{unaryFn: func(_ context.Context, _ string, _, _ any) error { return errors.New("boom") }}
	cmd := &createCmd{deps: f.deps()}
	stdio, _, _ := newIO(strings.NewReader(""))
	err := cmd.Run(context.Background(), requiredFlags(), stdio)
	if err == nil || !strings.Contains(err.Error(), "boom") {
		t.Errorf("err=%v", err)
	}
}

func TestCreateRemarshalErr(t *testing.T) {
	f := &fakeDeps{
		unaryFn: func(_ context.Context, _ string, _, resp any) error {
			out := resp.(*map[string]any)
			*out = map[string]any{"connectorId": "not-an-int"}
			return nil
		},
	}
	cmd := &createCmd{deps: f.deps()}
	stdio, _, _ := newIO(strings.NewReader(""))
	err := cmd.Run(context.Background(), requiredFlags(), stdio)
	if err == nil || !strings.Contains(err.Error(), "decode response") {
		t.Errorf("err=%v", err)
	}
}

// sortStrings helper is exercised indirectly by create tests' flag-missing
// cases; cover directly for sanity.
func TestSortStrings(t *testing.T) {
	in := []string{"b", "a", "c"}
	sortStrings(in)
	if in[0] != "a" || in[1] != "b" || in[2] != "c" {
		t.Errorf("%v", in)
	}
}

// --- update ---

func TestUpdateHappyPartial(t *testing.T) {
	f := &fakeDeps{
		unaryFn: func(_ context.Context, path string, _, resp any) error {
			// Pre-fetch inherits connectorType + full postgres block; server
			// requires both on every UpdateConnector call (captured 400).
			if strings.HasSuffix(path, "/GetConnector") {
				out := resp.(*getConnectorResp)
				out.Connector.ConnectorType = "POSTGRES"
				out.Connector.Name = "old-name"
				out.Connector.PostgresMetadata = postgresSpec{
					Host: "oldhost", Port: 5432, User: "olduser",
					Database: "olddb", SSLMode: false,
				}
				return nil
			}
			out := resp.(*map[string]any)
			*out = map[string]any{"connector": map[string]any{"id": 1.0, "name": "renamed"}}
			return nil
		},
	}
	cmd := &updateCmd{deps: f.deps()}
	args := []string{"--name", "renamed", "1"}
	stdio, out, _ := newIO(strings.NewReader(""))
	if err := cmd.Run(context.Background(), args, stdio); err != nil {
		t.Fatalf("err=%v", err)
	}
	if !strings.Contains(out.String(), "name:") {
		t.Errorf("stdout=%q", out.String())
	}
	// Wire body (final UpdateConnector call): connectorId top-level, renamed
	// config.name, inherited connectorType + full postgres block.
	req := string(f.lastRawReq)
	if !strings.Contains(req, `"connectorId":1`) {
		t.Errorf("connectorId must be top-level: %s", req)
	}
	if !strings.Contains(req, `"name":"renamed"`) {
		t.Errorf("config.name missing: %s", req)
	}
	if !strings.Contains(req, `"connectorType":"POSTGRES"`) {
		t.Errorf("connectorType must be inherited from GetConnector: %s", req)
	}
	// Server rejects updates without the full postgres block even when only
	// --name changes — baseline values must be forwarded as-is.
	if !strings.Contains(req, `"host":"oldhost"`) || !strings.Contains(req, `"port":5432`) {
		t.Errorf("postgres baseline must be inherited: %s", req)
	}
	// Password isn't returned by GetConnector — must not leak into the body.
	if strings.Contains(req, `"password":`) {
		t.Errorf("password must be omitted when user didn't touch it: %s", req)
	}
}

// Regression: positional <id> placed BEFORE flags must not drop the trailing
// flags (stdlib fs.Parse stops at the first non-flag token). We fixed this by
// routing every verb through cli.ParseFlags, but 100% branch coverage on that
// helper did not catch the verb-level regression we hit in prod — so each verb
// that takes positional+flag gets an explicit ordering test.
func TestUpdateRegressionPositionalBeforeFlags(t *testing.T) {
	f := &fakeDeps{
		unaryFn: func(_ context.Context, path string, _, resp any) error {
			if strings.HasSuffix(path, "/GetConnector") {
				out := resp.(*getConnectorResp)
				out.Connector.ConnectorType = "POSTGRES"
				out.Connector.Name = "old"
				return nil
			}
			out := resp.(*map[string]any)
			*out = map[string]any{"connector": map[string]any{"id": 9.0}}
			return nil
		},
	}
	cmd := &updateCmd{deps: f.deps()}
	stdio, _, _ := newIO(strings.NewReader(""))
	// Positional FIRST (the ordering that broke profile add in prod).
	args := []string{"9", "--name", "renamed", "--host", "h2"}
	if err := cmd.Run(context.Background(), args, stdio); err != nil {
		t.Fatalf("err=%v", err)
	}
	req := string(f.lastRawReq)
	if !strings.Contains(req, `"connectorId":9`) {
		t.Errorf("id lost: %s", req)
	}
	if !strings.Contains(req, `"name":"renamed"`) {
		t.Errorf("--name dropped when placed after positional: %s", req)
	}
	if !strings.Contains(req, `"host":"h2"`) {
		t.Errorf("--host dropped when placed after positional: %s", req)
	}
}

func TestUpdateDialectFields(t *testing.T) {
	f := &fakeDeps{
		unaryFn: func(_ context.Context, path string, _, resp any) error {
			if strings.HasSuffix(path, "/GetConnector") {
				out := resp.(*getConnectorResp)
				out.Connector.ConnectorType = "POSTGRES"
				return nil
			}
			out := resp.(*map[string]any)
			*out = map[string]any{"connector": map[string]any{"id": 1.0}}
			return nil
		},
	}
	cmd := &updateCmd{deps: f.deps()}
	args := []string{"--host", "newhost", "--port", "6543", "--ssl=true", "7"}
	stdio, _, _ := newIO(strings.NewReader(""))
	if err := cmd.Run(context.Background(), args, stdio); err != nil {
		t.Fatalf("err=%v", err)
	}
	req := string(f.lastRawReq)
	if !strings.Contains(req, `"connectorId":7`) {
		t.Errorf("req=%s", req)
	}
	if !strings.Contains(req, `"connectorType":"POSTGRES"`) {
		t.Errorf("dialect touched → connectorType required: %s", req)
	}
	if !strings.Contains(req, `"host":"newhost"`) || !strings.Contains(req, `"port":6543`) {
		t.Errorf("req=%s", req)
	}
	if !strings.Contains(req, `"sslMode":true`) {
		t.Errorf("sslMode missing: %s", req)
	}
}

func TestUpdatePasswordStdin(t *testing.T) {
	f := &fakeDeps{}
	cmd := &updateCmd{deps: f.deps()}
	args := []string{"--password-stdin", "42"}
	stdio, _, _ := newIO(strings.NewReader("new-pw\n"))
	if err := cmd.Run(context.Background(), args, stdio); err != nil {
		t.Fatalf("err=%v", err)
	}
	if !strings.Contains(string(f.lastRawReq), `"password":"new-pw"`) {
		t.Errorf("req=%s", string(f.lastRawReq))
	}
}

func TestUpdatePasswordFlag(t *testing.T) {
	f := &fakeDeps{}
	cmd := &updateCmd{deps: f.deps()}
	args := []string{"--password", "inline-pw", "42"}
	stdio, _, _ := newIO(strings.NewReader(""))
	if err := cmd.Run(context.Background(), args, stdio); err != nil {
		t.Fatalf("err=%v", err)
	}
	if !strings.Contains(string(f.lastRawReq), `"password":"inline-pw"`) {
		t.Errorf("req=%s", string(f.lastRawReq))
	}
}

func TestUpdatePasswordStdinReadErr(t *testing.T) {
	cmd := &updateCmd{deps: (&fakeDeps{}).deps()}
	stdio, _, _ := newIO(errReader{err: errors.New("read fail")})
	err := cmd.Run(context.Background(), []string{"--password-stdin", "1"}, stdio)
	if err == nil || !strings.Contains(err.Error(), "read fail") {
		t.Errorf("err=%v", err)
	}
}

func TestUpdateTypeAloneSetsConnectorType(t *testing.T) {
	f := &fakeDeps{}
	cmd := &updateCmd{deps: f.deps()}
	stdio, _, _ := newIO(strings.NewReader(""))
	if err := cmd.Run(context.Background(), []string{"--type", "postgres", "1"}, stdio); err != nil {
		t.Fatalf("err=%v", err)
	}
	if !strings.Contains(string(f.lastRawReq), `"connectorType":"POSTGRES"`) {
		t.Errorf("req=%s", string(f.lastRawReq))
	}
}

func TestUpdateUserAndDatabaseOnly(t *testing.T) {
	// Exercises the --user / --database flag-visited branches that the dialect
	// tests above didn't touch.
	f := &fakeDeps{}
	cmd := &updateCmd{deps: f.deps()}
	stdio, _, _ := newIO(strings.NewReader(""))
	args := []string{"--user", "newu", "--database", "newdb", "1"}
	if err := cmd.Run(context.Background(), args, stdio); err != nil {
		t.Fatalf("err=%v", err)
	}
	req := string(f.lastRawReq)
	if !strings.Contains(req, `"user":"newu"`) || !strings.Contains(req, `"database":"newdb"`) {
		t.Errorf("req=%s", req)
	}
}

func TestUpdateWrongType(t *testing.T) {
	cmd := &updateCmd{deps: (&fakeDeps{}).deps()}
	stdio, _, _ := newIO(strings.NewReader(""))
	err := cmd.Run(context.Background(), []string{"--type", "mysql", "1"}, stdio)
	if !errors.Is(err, cli.ErrUsage) {
		t.Errorf("err=%v", err)
	}
}

func TestUpdateMissingPositional(t *testing.T) {
	cmd := &updateCmd{deps: (&fakeDeps{}).deps()}
	stdio, _, _ := newIO(strings.NewReader(""))
	err := cmd.Run(context.Background(), []string{"--name", "n"}, stdio)
	if !errors.Is(err, cli.ErrUsage) {
		t.Errorf("err=%v", err)
	}
}

func TestUpdateNonIntPositional(t *testing.T) {
	cmd := &updateCmd{deps: (&fakeDeps{}).deps()}
	stdio, _, _ := newIO(strings.NewReader(""))
	err := cmd.Run(context.Background(), []string{"--name", "n", "abc"}, stdio)
	if !errors.Is(err, cli.ErrUsage) {
		t.Errorf("err=%v", err)
	}
}

func TestUpdateNoFieldsProvided(t *testing.T) {
	cmd := &updateCmd{deps: (&fakeDeps{}).deps()}
	stdio, _, _ := newIO(strings.NewReader(""))
	err := cmd.Run(context.Background(), []string{"1"}, stdio)
	if !errors.Is(err, cli.ErrUsage) {
		t.Errorf("err=%v", err)
	}
}

func TestUpdateBadFlag(t *testing.T) {
	cmd := &updateCmd{deps: (&fakeDeps{}).deps()}
	stdio, _, _ := newIO(strings.NewReader(""))
	err := cmd.Run(context.Background(), []string{"--nope", "1"}, stdio)
	if !errors.Is(err, cli.ErrUsage) {
		t.Errorf("err=%v", err)
	}
}

func TestUpdateUnaryErr(t *testing.T) {
	// GetConnector pre-fetch fails — the "fetch current" branch.
	f := &fakeDeps{unaryFn: func(_ context.Context, _ string, _, _ any) error { return errors.New("boom") }}
	cmd := &updateCmd{deps: f.deps()}
	stdio, _, _ := newIO(strings.NewReader(""))
	err := cmd.Run(context.Background(), []string{"--name", "n", "1"}, stdio)
	if err == nil || !strings.Contains(err.Error(), "fetch current") {
		t.Errorf("err=%v", err)
	}
}

func TestUpdateCallErr(t *testing.T) {
	// GetConnector succeeds; UpdateConnector itself errors — separate branch.
	f := &fakeDeps{
		unaryFn: func(_ context.Context, path string, _, resp any) error {
			if strings.HasSuffix(path, "/GetConnector") {
				out := resp.(*getConnectorResp)
				out.Connector.ConnectorType = "POSTGRES"
				return nil
			}
			return errors.New("update-boom")
		},
	}
	cmd := &updateCmd{deps: f.deps()}
	stdio, _, _ := newIO(strings.NewReader(""))
	err := cmd.Run(context.Background(), []string{"--name", "n", "1"}, stdio)
	if err == nil || !strings.Contains(err.Error(), "update-boom") {
		t.Errorf("err=%v", err)
	}
}

func TestUpdateJSONBypass(t *testing.T) {
	f := &fakeDeps{
		unaryFn: func(_ context.Context, path string, _, resp any) error {
			if strings.HasSuffix(path, "/GetConnector") {
				out := resp.(*getConnectorResp)
				out.Connector.ConnectorType = "POSTGRES"
				return nil
			}
			out := resp.(*map[string]any)
			*out = map[string]any{"connector": map[string]any{"id": 1.0}}
			return nil
		},
	}
	cmd := &updateCmd{deps: f.deps()}
	ctx := cli.WithGlobal(context.Background(), cli.Global{JSON: true})
	stdio, out, _ := newIO(strings.NewReader(""))
	if err := cmd.Run(ctx, []string{"--name", "n", "1"}, stdio); err != nil {
		t.Fatalf("err=%v", err)
	}
	if !strings.Contains(out.String(), "\"connector\"") {
		t.Errorf("stdout=%q", out.String())
	}
}

func TestUpdateNoConnectorKey(t *testing.T) {
	// Response without "connector" — falls back to raw JSON dump.
	f := &fakeDeps{
		unaryFn: func(_ context.Context, path string, _, resp any) error {
			if strings.HasSuffix(path, "/GetConnector") {
				out := resp.(*getConnectorResp)
				out.Connector.ConnectorType = "POSTGRES"
				return nil
			}
			out := resp.(*map[string]any)
			*out = map[string]any{"weird": 1.0}
			return nil
		},
	}
	cmd := &updateCmd{deps: f.deps()}
	stdio, out, _ := newIO(strings.NewReader(""))
	if err := cmd.Run(context.Background(), []string{"--name", "n", "1"}, stdio); err != nil {
		t.Fatalf("err=%v", err)
	}
	if !strings.Contains(out.String(), "\"weird\"") {
		t.Errorf("stdout=%q", out.String())
	}
}

// --- delete ---

func TestDeleteHappy(t *testing.T) {
	f := &fakeDeps{}
	cmd := &deleteCmd{deps: f.deps()}
	stdio, out, _ := newIO(strings.NewReader(""))
	if err := cmd.Run(context.Background(), []string{"7"}, stdio); err != nil {
		t.Fatalf("err=%v", err)
	}
	if !strings.Contains(out.String(), "deleted 7") {
		t.Errorf("stdout=%q", out.String())
	}
	if !strings.Contains(string(f.lastRawReq), `"connectorId":7`) {
		t.Errorf("req=%s", string(f.lastRawReq))
	}
	if f.lastPath != servicePath+"/DeleteConnector" {
		t.Errorf("path=%s", f.lastPath)
	}
}

func TestDeleteMissingPositional(t *testing.T) {
	cmd := &deleteCmd{deps: (&fakeDeps{}).deps()}
	stdio, _, _ := newIO(strings.NewReader(""))
	err := cmd.Run(context.Background(), nil, stdio)
	if !errors.Is(err, cli.ErrUsage) {
		t.Errorf("err=%v", err)
	}
}

func TestDeleteNonInt(t *testing.T) {
	cmd := &deleteCmd{deps: (&fakeDeps{}).deps()}
	stdio, _, _ := newIO(strings.NewReader(""))
	err := cmd.Run(context.Background(), []string{"abc"}, stdio)
	if !errors.Is(err, cli.ErrUsage) {
		t.Errorf("err=%v", err)
	}
}

func TestDeleteUnaryErr(t *testing.T) {
	f := &fakeDeps{unaryFn: func(_ context.Context, _ string, _, _ any) error { return errors.New("boom") }}
	cmd := &deleteCmd{deps: f.deps()}
	stdio, _, _ := newIO(strings.NewReader(""))
	err := cmd.Run(context.Background(), []string{"1"}, stdio)
	if err == nil || !strings.Contains(err.Error(), "boom") {
		t.Errorf("err=%v", err)
	}
}

func TestDeleteBadFlag(t *testing.T) {
	cmd := &deleteCmd{deps: (&fakeDeps{}).deps()}
	stdio, _, _ := newIO(strings.NewReader(""))
	err := cmd.Run(context.Background(), []string{"--nope"}, stdio)
	if !errors.Is(err, cli.ErrUsage) {
		t.Errorf("err=%v", err)
	}
}

// --- test ---

func TestTestOK(t *testing.T) {
	f := &fakeDeps{
		unaryFn: func(_ context.Context, _ string, _, resp any) error {
			out := resp.(*map[string]any)
			*out = map[string]any{"error": ""}
			return nil
		},
	}
	cmd := &testCmd{deps: f.deps()}
	stdio, out, _ := newIO(strings.NewReader(""))
	if err := cmd.Run(context.Background(), []string{"1"}, stdio); err != nil {
		t.Fatalf("err=%v", err)
	}
	if strings.TrimSpace(out.String()) != "OK" {
		t.Errorf("stdout=%q", out.String())
	}
	if f.lastPath != servicePath+"/TestConnector" {
		t.Errorf("path=%s", f.lastPath)
	}
}

func TestTestFail(t *testing.T) {
	f := &fakeDeps{
		unaryFn: func(_ context.Context, _ string, _, resp any) error {
			out := resp.(*map[string]any)
			*out = map[string]any{"error": "connection refused"}
			return nil
		},
	}
	cmd := &testCmd{deps: f.deps()}
	stdio, out, _ := newIO(strings.NewReader(""))
	if err := cmd.Run(context.Background(), []string{"1"}, stdio); err != nil {
		t.Fatalf("err=%v", err)
	}
	if !strings.Contains(out.String(), "FAIL") || !strings.Contains(out.String(), "connection refused") {
		t.Errorf("stdout=%q", out.String())
	}
}

func TestTestJSON(t *testing.T) {
	f := &fakeDeps{
		unaryFn: func(_ context.Context, _ string, _, resp any) error {
			out := resp.(*map[string]any)
			*out = map[string]any{"error": ""}
			return nil
		},
	}
	cmd := &testCmd{deps: f.deps()}
	ctx := cli.WithGlobal(context.Background(), cli.Global{JSON: true})
	stdio, out, _ := newIO(strings.NewReader(""))
	if err := cmd.Run(ctx, []string{"1"}, stdio); err != nil {
		t.Fatalf("err=%v", err)
	}
	if !strings.Contains(out.String(), "\"error\"") {
		t.Errorf("stdout=%q", out.String())
	}
}

func TestTestMissingPositional(t *testing.T) {
	cmd := &testCmd{deps: (&fakeDeps{}).deps()}
	stdio, _, _ := newIO(strings.NewReader(""))
	err := cmd.Run(context.Background(), nil, stdio)
	if !errors.Is(err, cli.ErrUsage) {
		t.Errorf("err=%v", err)
	}
}

func TestTestNonInt(t *testing.T) {
	cmd := &testCmd{deps: (&fakeDeps{}).deps()}
	stdio, _, _ := newIO(strings.NewReader(""))
	err := cmd.Run(context.Background(), []string{"abc"}, stdio)
	if !errors.Is(err, cli.ErrUsage) {
		t.Errorf("err=%v", err)
	}
}

func TestTestUnaryErr(t *testing.T) {
	f := &fakeDeps{unaryFn: func(_ context.Context, _ string, _, _ any) error { return errors.New("boom") }}
	cmd := &testCmd{deps: f.deps()}
	stdio, _, _ := newIO(strings.NewReader(""))
	err := cmd.Run(context.Background(), []string{"1"}, stdio)
	if err == nil || !strings.Contains(err.Error(), "boom") {
		t.Errorf("err=%v", err)
	}
}

func TestTestBadFlag(t *testing.T) {
	cmd := &testCmd{deps: (&fakeDeps{}).deps()}
	stdio, _, _ := newIO(strings.NewReader(""))
	err := cmd.Run(context.Background(), []string{"--nope"}, stdio)
	if !errors.Is(err, cli.ErrUsage) {
		t.Errorf("err=%v", err)
	}
}

func TestTestRemarshalErr(t *testing.T) {
	f := &fakeDeps{
		unaryFn: func(_ context.Context, _ string, _, resp any) error {
			out := resp.(*map[string]any)
			*out = map[string]any{"error": 123.0}
			return nil
		},
	}
	cmd := &testCmd{deps: f.deps()}
	stdio, _, _ := newIO(strings.NewReader(""))
	err := cmd.Run(context.Background(), []string{"1"}, stdio)
	if err == nil || !strings.Contains(err.Error(), "decode response") {
		t.Errorf("err=%v", err)
	}
}

// --- tables ---

func TestTablesHappy(t *testing.T) {
	f := &fakeDeps{
		unaryFn: func(_ context.Context, _ string, _, resp any) error {
			out := resp.(*map[string]any)
			*out = map[string]any{
				"tables": []any{
					map[string]any{"tableSchema": "demo", "tableName": "trips"},
					map[string]any{"tableSchema": "demo", "tableName": "stations"},
				},
			}
			return nil
		},
	}
	cmd := &tablesCmd{deps: f.deps()}
	stdio, out, _ := newIO(strings.NewReader(""))
	if err := cmd.Run(context.Background(), []string{"70"}, stdio); err != nil {
		t.Fatalf("err=%v", err)
	}
	s := out.String()
	if !strings.Contains(s, "SCHEMA") || !strings.Contains(s, "NAME") {
		t.Errorf("headers missing: %q", s)
	}
	if !strings.Contains(s, "trips") || !strings.Contains(s, "stations") {
		t.Errorf("rows missing: %q", s)
	}
	if f.lastPath != servicePath+"/ListConnectorTables" {
		t.Errorf("path=%s", f.lastPath)
	}
	if !strings.Contains(string(f.lastRawReq), `"connectorId":70`) {
		t.Errorf("req=%s", string(f.lastRawReq))
	}
}

func TestTablesJSON(t *testing.T) {
	f := &fakeDeps{
		unaryFn: func(_ context.Context, _ string, _, resp any) error {
			out := resp.(*map[string]any)
			*out = map[string]any{"tables": []any{}}
			return nil
		},
	}
	cmd := &tablesCmd{deps: f.deps()}
	ctx := cli.WithGlobal(context.Background(), cli.Global{JSON: true})
	stdio, out, _ := newIO(strings.NewReader(""))
	if err := cmd.Run(ctx, []string{"1"}, stdio); err != nil {
		t.Fatalf("err=%v", err)
	}
	if !strings.Contains(out.String(), "\"tables\"") {
		t.Errorf("stdout=%q", out.String())
	}
}

func TestTablesMissingPositional(t *testing.T) {
	cmd := &tablesCmd{deps: (&fakeDeps{}).deps()}
	stdio, _, _ := newIO(strings.NewReader(""))
	err := cmd.Run(context.Background(), nil, stdio)
	if !errors.Is(err, cli.ErrUsage) {
		t.Errorf("err=%v", err)
	}
}

func TestTablesNonInt(t *testing.T) {
	cmd := &tablesCmd{deps: (&fakeDeps{}).deps()}
	stdio, _, _ := newIO(strings.NewReader(""))
	err := cmd.Run(context.Background(), []string{"abc"}, stdio)
	if !errors.Is(err, cli.ErrUsage) {
		t.Errorf("err=%v", err)
	}
}

func TestTablesBadFlag(t *testing.T) {
	cmd := &tablesCmd{deps: (&fakeDeps{}).deps()}
	stdio, _, _ := newIO(strings.NewReader(""))
	err := cmd.Run(context.Background(), []string{"--nope"}, stdio)
	if !errors.Is(err, cli.ErrUsage) {
		t.Errorf("err=%v", err)
	}
}

func TestTablesUnaryErr(t *testing.T) {
	f := &fakeDeps{unaryFn: func(_ context.Context, _ string, _, _ any) error { return errors.New("boom") }}
	cmd := &tablesCmd{deps: f.deps()}
	stdio, _, _ := newIO(strings.NewReader(""))
	err := cmd.Run(context.Background(), []string{"1"}, stdio)
	if err == nil || !strings.Contains(err.Error(), "boom") {
		t.Errorf("err=%v", err)
	}
}

func TestTablesRemarshalErr(t *testing.T) {
	f := &fakeDeps{
		unaryFn: func(_ context.Context, _ string, _, resp any) error {
			out := resp.(*map[string]any)
			*out = map[string]any{"tables": "nope"}
			return nil
		},
	}
	cmd := &tablesCmd{deps: f.deps()}
	stdio, _, _ := newIO(strings.NewReader(""))
	err := cmd.Run(context.Background(), []string{"1"}, stdio)
	if err == nil || !strings.Contains(err.Error(), "decode response") {
		t.Errorf("err=%v", err)
	}
}

// --- examples ---

func TestExamplesHappy(t *testing.T) {
	f := &fakeDeps{
		unaryFn: func(_ context.Context, _ string, _, resp any) error {
			out := resp.(*map[string]any)
			*out = map[string]any{
				"examples": []any{
					map[string]any{"label": "Explore", "message": "hi", "category": "Exploration"},
				},
			}
			return nil
		},
	}
	cmd := &examplesCmd{deps: f.deps()}
	stdio, out, _ := newIO(strings.NewReader(""))
	if err := cmd.Run(context.Background(), []string{"608"}, stdio); err != nil {
		t.Fatalf("err=%v", err)
	}
	if !strings.Contains(out.String(), "[Exploration] Explore: hi") {
		t.Errorf("stdout=%q", out.String())
	}
	// Body wraps id in connectorContexts array.
	if !strings.Contains(string(f.lastRawReq), `"connectorContexts":[{"connectorId":608}]`) {
		t.Errorf("req=%s", string(f.lastRawReq))
	}
	if f.lastPath != servicePath+"/GetExampleQueries" {
		t.Errorf("path=%s", f.lastPath)
	}
}

func TestExamplesJSON(t *testing.T) {
	f := &fakeDeps{
		unaryFn: func(_ context.Context, _ string, _, resp any) error {
			out := resp.(*map[string]any)
			*out = map[string]any{"examples": []any{}}
			return nil
		},
	}
	cmd := &examplesCmd{deps: f.deps()}
	ctx := cli.WithGlobal(context.Background(), cli.Global{JSON: true})
	stdio, out, _ := newIO(strings.NewReader(""))
	if err := cmd.Run(ctx, []string{"1"}, stdio); err != nil {
		t.Fatalf("err=%v", err)
	}
	if !strings.Contains(out.String(), "\"examples\"") {
		t.Errorf("stdout=%q", out.String())
	}
}

func TestExamplesMissingPositional(t *testing.T) {
	cmd := &examplesCmd{deps: (&fakeDeps{}).deps()}
	stdio, _, _ := newIO(strings.NewReader(""))
	err := cmd.Run(context.Background(), nil, stdio)
	if !errors.Is(err, cli.ErrUsage) {
		t.Errorf("err=%v", err)
	}
}

func TestExamplesNonInt(t *testing.T) {
	cmd := &examplesCmd{deps: (&fakeDeps{}).deps()}
	stdio, _, _ := newIO(strings.NewReader(""))
	err := cmd.Run(context.Background(), []string{"abc"}, stdio)
	if !errors.Is(err, cli.ErrUsage) {
		t.Errorf("err=%v", err)
	}
}

func TestExamplesBadFlag(t *testing.T) {
	cmd := &examplesCmd{deps: (&fakeDeps{}).deps()}
	stdio, _, _ := newIO(strings.NewReader(""))
	err := cmd.Run(context.Background(), []string{"--nope"}, stdio)
	if !errors.Is(err, cli.ErrUsage) {
		t.Errorf("err=%v", err)
	}
}

func TestExamplesUnaryErr(t *testing.T) {
	f := &fakeDeps{unaryFn: func(_ context.Context, _ string, _, _ any) error { return errors.New("boom") }}
	cmd := &examplesCmd{deps: f.deps()}
	stdio, _, _ := newIO(strings.NewReader(""))
	err := cmd.Run(context.Background(), []string{"1"}, stdio)
	if err == nil || !strings.Contains(err.Error(), "boom") {
		t.Errorf("err=%v", err)
	}
}

func TestExamplesRemarshalErr(t *testing.T) {
	f := &fakeDeps{
		unaryFn: func(_ context.Context, _ string, _, resp any) error {
			out := resp.(*map[string]any)
			*out = map[string]any{"examples": "nope"}
			return nil
		},
	}
	cmd := &examplesCmd{deps: f.deps()}
	stdio, _, _ := newIO(strings.NewReader(""))
	err := cmd.Run(context.Background(), []string{"1"}, stdio)
	if err == nil || !strings.Contains(err.Error(), "decode response") {
		t.Errorf("err=%v", err)
	}
}

// --- direct util tests ---

func TestFlagWasSet(t *testing.T) {
	fs := cli.NewFlagSet("t")
	_ = fs.String("a", "", "")
	_ = fs.String("b", "", "")
	if err := fs.Parse([]string{"--a", "x"}); err != nil {
		t.Fatal(err)
	}
	if !flagWasSet(fs, "a") {
		t.Errorf("a should be set")
	}
	if flagWasSet(fs, "b") {
		t.Errorf("b should not be set")
	}
}
