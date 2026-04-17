package profile

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/highperformance-tech/ana-cli/internal/cli"
	"github.com/highperformance-tech/ana-cli/internal/config"
)

// --- helpers ---

// newDeps returns profile.Deps backed by real config.Load/Save/DefaultPath
// closures targeting cfgPath. Tests exercise the actual round-trip rather
// than mocking the boundary so every test proves the on-disk shape is what
// the commands expect to read back.
func newDeps(cfgPath string) Deps {
	return Deps{
		LoadCfg:    func() (config.Config, error) { return config.Load(cfgPath) },
		SaveCfg:    func(c config.Config) error { return config.Save(cfgPath, c) },
		ConfigPath: func() (string, error) { return cfgPath, nil },
	}
}

// newIO builds a cli.IO with in-memory streams. stdin is optional; when nil
// an empty reader is used.
func newIO(stdin io.Reader) (cli.IO, *bytes.Buffer, *bytes.Buffer) {
	if stdin == nil {
		stdin = strings.NewReader("")
	}
	var out, errb bytes.Buffer
	stdio := cli.IO{
		Stdin:  stdin,
		Stdout: &out,
		Stderr: &errb,
		Env:    func(string) string { return "" },
		Now:    time.Now,
	}
	return stdio, &out, &errb
}

// seed writes a starter config with two profiles so list/show have something
// to render. "default" is active; "other" is inactive.
func seed(t *testing.T, cfgPath string) {
	t.Helper()
	c := config.Config{
		Profiles: map[string]config.Profile{
			"default": {Endpoint: "https://app.textql.com", Token: "abcdef1234", OrgName: "Example Org"},
			"other":   {Endpoint: "https://alt.textql.com", Token: "", OrgName: "TextQL Demo"},
		},
		Active: "default",
	}
	if err := config.Save(cfgPath, c); err != nil {
		t.Fatalf("seed: %v", err)
	}
}

// tmpCfg returns a fresh config path under t.TempDir().
func tmpCfg(t *testing.T) string {
	t.Helper()
	return filepath.Join(t.TempDir(), "config.json")
}

// --- New / Group ---

func TestNew_ReturnsGroupWithExpectedChildren(t *testing.T) {
	g := New(Deps{})
	for _, name := range []string{"list", "add", "use", "remove", "show"} {
		if _, ok := g.Children[name]; !ok {
			t.Errorf("missing child: %s", name)
		}
	}
	if g.Summary == "" {
		t.Error("Summary should be set so root help has something to print")
	}
	// Help renders non-empty output; the actual shape is covered by cli tests.
	if g.Help() == "" {
		t.Error("Help() returned empty string")
	}
}

// --- shared helpers ---

func TestParseFlags_WrapsErrors(t *testing.T) {
	fs := newFlagSet("x")
	fs.Bool("flag", false, "")
	err := parseFlags(fs, []string{"--nope"})
	if !errors.Is(err, cli.ErrUsage) {
		t.Fatalf("expected cli.ErrUsage, got %v", err)
	}
}

func TestUsageErrf_WrapsErrUsage(t *testing.T) {
	err := usageErrf("bad %s", "thing")
	if !errors.Is(err, cli.ErrUsage) {
		t.Fatalf("expected cli.ErrUsage, got %v", err)
	}
	if !strings.Contains(err.Error(), "bad thing") {
		t.Fatalf("msg: %q", err.Error())
	}
}

func TestWriteJSON_IndentsAndNewline(t *testing.T) {
	var buf bytes.Buffer
	if err := writeJSON(&buf, map[string]int{"x": 1}); err != nil {
		t.Fatalf("writeJSON: %v", err)
	}
	if !strings.Contains(buf.String(), "\n") {
		t.Fatal("expected trailing newline")
	}
	if !strings.Contains(buf.String(), "  ") {
		t.Fatal("expected 2-space indent")
	}
}

// writeJSON must surface encoder errors; a channel cannot be JSON-encoded.
func TestWriteJSON_EncodeError(t *testing.T) {
	if err := writeJSON(io.Discard, make(chan int)); err == nil {
		t.Fatal("expected encode error")
	}
}

// --- list ---

func TestList_Help(t *testing.T) {
	c := &listCmd{}
	if !strings.Contains(c.Help(), "list") {
		t.Fatalf("help: %q", c.Help())
	}
}

func TestList_Human(t *testing.T) {
	cfgPath := tmpCfg(t)
	seed(t, cfgPath)
	stdio, out, _ := newIO(nil)
	err := (&listCmd{deps: newDeps(cfgPath)}).Run(context.Background(), nil, stdio)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	s := out.String()
	if !strings.Contains(s, "NAME") || !strings.Contains(s, "ACTIVE") {
		t.Fatalf("missing header: %q", s)
	}
	if !strings.Contains(s, "default") || !strings.Contains(s, "*") {
		t.Fatalf("default/active marker missing: %q", s)
	}
	if !strings.Contains(s, "other") {
		t.Fatalf("other profile missing: %q", s)
	}
	// Tokens must NEVER appear in the list view.
	if strings.Contains(s, "abcdef1234") {
		t.Fatalf("token leaked into list output: %q", s)
	}
}

func TestList_JSON(t *testing.T) {
	cfgPath := tmpCfg(t)
	seed(t, cfgPath)
	stdio, out, _ := newIO(nil)
	ctx := cli.WithGlobal(context.Background(), cli.Global{JSON: true})
	err := (&listCmd{deps: newDeps(cfgPath)}).Run(ctx, nil, stdio)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	var got listPayload
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v (%q)", err, out.String())
	}
	if got.Active != "default" {
		t.Errorf("active = %q", got.Active)
	}
	if len(got.Profiles) != 2 {
		t.Fatalf("profiles: %+v", got.Profiles)
	}
	// hasToken must reflect token presence without emitting the value itself.
	var sawDefaultWithToken, sawOtherNoToken bool
	for _, p := range got.Profiles {
		if p.Name == "default" && p.HasToken {
			sawDefaultWithToken = true
		}
		if p.Name == "other" && !p.HasToken {
			sawOtherNoToken = true
		}
	}
	if !sawDefaultWithToken || !sawOtherNoToken {
		t.Fatalf("hasToken wrong: %+v", got.Profiles)
	}
	if strings.Contains(out.String(), "abcdef1234") {
		t.Fatal("token leaked into --json output")
	}
}

func TestList_BadFlag(t *testing.T) {
	stdio, _, _ := newIO(nil)
	err := (&listCmd{deps: newDeps(tmpCfg(t))}).Run(context.Background(), []string{"--nope"}, stdio)
	if !errors.Is(err, cli.ErrUsage) {
		t.Fatalf("err = %v", err)
	}
}

func TestList_LoadError(t *testing.T) {
	cfgPath := tmpCfg(t)
	if err := os.WriteFile(cfgPath, []byte("{not json"), 0o600); err != nil {
		t.Fatalf("seed: %v", err)
	}
	stdio, _, _ := newIO(nil)
	err := (&listCmd{deps: newDeps(cfgPath)}).Run(context.Background(), nil, stdio)
	if err == nil {
		t.Fatal("expected error on malformed config")
	}
}

// --- add ---

func TestAdd_Help(t *testing.T) {
	if !strings.Contains((&addCmd{}).Help(), "add") {
		t.Fatal("help missing")
	}
}

func TestAdd_CreatesProfile(t *testing.T) {
	cfgPath := tmpCfg(t)
	stdio, out, _ := newIO(strings.NewReader("secret-token\n"))
	err := (&addCmd{deps: newDeps(cfgPath)}).Run(context.Background(),
		[]string{"--endpoint", "https://custom", "--org", "Acme", "newprof"}, stdio)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !strings.Contains(out.String(), "saved profile newprof") {
		t.Fatalf("stdout: %q", out.String())
	}
	c, err := config.Load(cfgPath)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	p, ok := c.Profiles["newprof"]
	if !ok {
		t.Fatalf("newprof missing: %+v", c)
	}
	if p.Endpoint != "https://custom" || p.Token != "secret-token" || p.OrgName != "Acme" {
		t.Fatalf("profile wrong: %+v", p)
	}
	// First profile wins active.
	if c.Active != "newprof" {
		t.Fatalf("active = %q", c.Active)
	}
}

// Regression: stdlib flag.Parse stops at the first non-flag, so
// `profile add <name> --flag` would silently drop every trailing flag.
// The live symptom was an e2e profile saved without --org / --endpoint /
// --token-stdin despite them being passed on the command line. This test
// guards the shape that bit us in the field.
func TestAdd_RegressionPositionalBeforeFlags(t *testing.T) {
	cfgPath := tmpCfg(t)
	stdio, _, _ := newIO(strings.NewReader("secret\n"))
	err := (&addCmd{deps: newDeps(cfgPath)}).Run(context.Background(),
		[]string{"newprof", "--endpoint", "https://custom", "--org", "Acme"}, stdio)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	c, err := config.Load(cfgPath)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	p, ok := c.Profiles["newprof"]
	if !ok {
		t.Fatalf("newprof missing: %+v", c)
	}
	if p.Endpoint != "https://custom" {
		t.Errorf("endpoint dropped when placed after positional: got %q", p.Endpoint)
	}
	if p.OrgName != "Acme" {
		t.Errorf("org dropped when placed after positional: got %q", p.OrgName)
	}
	if p.Token != "secret" {
		t.Errorf("token = %q", p.Token)
	}
}

func TestAdd_DefaultEndpoint(t *testing.T) {
	cfgPath := tmpCfg(t)
	stdio, _, _ := newIO(strings.NewReader("tok\n"))
	err := (&addCmd{deps: newDeps(cfgPath)}).Run(context.Background(), []string{"p"}, stdio)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	c, _ := config.Load(cfgPath)
	if c.Profiles["p"].Endpoint != config.DefaultEndpoint {
		t.Fatalf("endpoint = %q, want default", c.Profiles["p"].Endpoint)
	}
}

func TestAdd_TokenStdin_FullStream(t *testing.T) {
	cfgPath := tmpCfg(t)
	stdio, _, _ := newIO(strings.NewReader("line1\nline2\n"))
	err := (&addCmd{deps: newDeps(cfgPath)}).Run(context.Background(),
		[]string{"--token-stdin", "p"}, stdio)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	c, _ := config.Load(cfgPath)
	if c.Profiles["p"].Token != "line1\nline2" {
		t.Fatalf("token = %q", c.Profiles["p"].Token)
	}
}

func TestAdd_OverwritesExisting(t *testing.T) {
	cfgPath := tmpCfg(t)
	seed(t, cfgPath)
	stdio, _, _ := newIO(strings.NewReader("new-token\n"))
	err := (&addCmd{deps: newDeps(cfgPath)}).Run(context.Background(),
		[]string{"--endpoint", "https://rewritten", "default"}, stdio)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	c, _ := config.Load(cfgPath)
	if c.Profiles["default"].Token != "new-token" {
		t.Fatalf("token not overwritten: %q", c.Profiles["default"].Token)
	}
	if c.Profiles["default"].Endpoint != "https://rewritten" {
		t.Fatalf("endpoint not overwritten: %q", c.Profiles["default"].Endpoint)
	}
	// "other" must still exist.
	if _, ok := c.Profiles["other"]; !ok {
		t.Fatal("other wiped on overwrite")
	}
}

func TestAdd_MissingName(t *testing.T) {
	stdio, _, _ := newIO(strings.NewReader("t\n"))
	err := (&addCmd{deps: newDeps(tmpCfg(t))}).Run(context.Background(), nil, stdio)
	if !errors.Is(err, cli.ErrUsage) {
		t.Fatalf("err = %v", err)
	}
}

func TestAdd_EmptyName(t *testing.T) {
	stdio, _, _ := newIO(strings.NewReader("t\n"))
	err := (&addCmd{deps: newDeps(tmpCfg(t))}).Run(context.Background(), []string{""}, stdio)
	if !errors.Is(err, cli.ErrUsage) {
		t.Fatalf("err = %v", err)
	}
}

func TestAdd_BadFlag(t *testing.T) {
	stdio, _, _ := newIO(strings.NewReader("t\n"))
	err := (&addCmd{deps: newDeps(tmpCfg(t))}).Run(context.Background(), []string{"--nope", "p"}, stdio)
	if !errors.Is(err, cli.ErrUsage) {
		t.Fatalf("err = %v", err)
	}
}

func TestAdd_NilStdinError(t *testing.T) {
	cfgPath := tmpCfg(t)
	stdio, _, _ := newIO(nil)
	stdio.Stdin = nil
	err := (&addCmd{deps: newDeps(cfgPath)}).Run(context.Background(), []string{"p"}, stdio)
	if err == nil {
		t.Fatal("expected error on nil stdin")
	}
}

func TestAdd_LoadError(t *testing.T) {
	cfgPath := tmpCfg(t)
	if err := os.WriteFile(cfgPath, []byte("{not json"), 0o600); err != nil {
		t.Fatalf("seed: %v", err)
	}
	stdio, _, _ := newIO(strings.NewReader("t\n"))
	err := (&addCmd{deps: newDeps(cfgPath)}).Run(context.Background(), []string{"p"}, stdio)
	if err == nil {
		t.Fatal("expected load error")
	}
}

func TestAdd_SaveError(t *testing.T) {
	cfgPath := tmpCfg(t)
	// Inject a failing SaveCfg directly. badDirCfg would normally trigger the
	// same branch, but a pure injection keeps the test robust across
	// filesystems that happen to allow the MkdirAll we relied on.
	d := Deps{
		LoadCfg:    func() (config.Config, error) { return config.Load(cfgPath) },
		SaveCfg:    func(config.Config) error { return errors.New("boom") },
		ConfigPath: func() (string, error) { return cfgPath, nil },
	}
	stdio, _, _ := newIO(strings.NewReader("t\n"))
	err := (&addCmd{deps: d}).Run(context.Background(), []string{"p"}, stdio)
	if err == nil {
		t.Fatal("expected save error")
	}
}

func TestAdd_PathError(t *testing.T) {
	cfgPath := tmpCfg(t)
	d := newDeps(cfgPath)
	d.ConfigPath = func() (string, error) { return "", errors.New("boom") }
	stdio, out, _ := newIO(strings.NewReader("t\n"))
	err := (&addCmd{deps: d}).Run(context.Background(), []string{"p"}, stdio)
	if err == nil {
		t.Fatal("expected path error")
	}
	// Save succeeded so the softer message should land on stdout.
	if !strings.Contains(out.String(), "saved profile p") {
		t.Fatalf("stdout missing soft success: %q", out.String())
	}
}

// TestAdd_TokenStdin_ReadError exercises the io.ReadAll error path.
func TestAdd_TokenStdin_ReadError(t *testing.T) {
	stdio, _, _ := newIO(&errReader{})
	err := (&addCmd{deps: newDeps(tmpCfg(t))}).Run(context.Background(),
		[]string{"--token-stdin", "p"}, stdio)
	if err == nil {
		t.Fatal("expected read error")
	}
}

// TestAdd_LineScanner_ReadError exercises the Scanner error path (line mode).
func TestAdd_LineScanner_ReadError(t *testing.T) {
	stdio, _, _ := newIO(&errReader{})
	err := (&addCmd{deps: newDeps(tmpCfg(t))}).Run(context.Background(), []string{"p"}, stdio)
	if err == nil {
		t.Fatal("expected scanner error")
	}
}

// errReader always returns an error so we can cover the read-failure branches
// in readToken without relying on filesystem quirks.
type errReader struct{}

func (errReader) Read([]byte) (int, error) { return 0, io.ErrClosedPipe }

// TestAdd_EmptyStdin covers the no-token branch: scanner returns false with
// no error and an empty token is persisted (the add command accepts it —
// login is the one that rejects empty; add lets you pre-create slots).
func TestAdd_EmptyStdin(t *testing.T) {
	cfgPath := tmpCfg(t)
	stdio, _, _ := newIO(strings.NewReader(""))
	err := (&addCmd{deps: newDeps(cfgPath)}).Run(context.Background(), []string{"p"}, stdio)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	c, _ := config.Load(cfgPath)
	if c.Profiles["p"].Token != "" {
		t.Fatalf("expected empty token, got %q", c.Profiles["p"].Token)
	}
}

// --- use ---

func TestUse_Help(t *testing.T) {
	if !strings.Contains((&useCmd{}).Help(), "use") {
		t.Fatal("help missing")
	}
}

func TestUse_Switches(t *testing.T) {
	cfgPath := tmpCfg(t)
	seed(t, cfgPath)
	stdio, out, _ := newIO(nil)
	err := (&useCmd{deps: newDeps(cfgPath)}).Run(context.Background(), []string{"other"}, stdio)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !strings.Contains(out.String(), "active profile: other") {
		t.Fatalf("stdout: %q", out.String())
	}
	c, _ := config.Load(cfgPath)
	if c.Active != "other" {
		t.Fatalf("active = %q", c.Active)
	}
}

func TestUse_Unknown(t *testing.T) {
	cfgPath := tmpCfg(t)
	seed(t, cfgPath)
	stdio, _, _ := newIO(nil)
	err := (&useCmd{deps: newDeps(cfgPath)}).Run(context.Background(), []string{"ghost"}, stdio)
	if !errors.Is(err, config.ErrUnknownProfile) {
		t.Fatalf("err = %v", err)
	}
}

func TestUse_MissingArg(t *testing.T) {
	stdio, _, _ := newIO(nil)
	err := (&useCmd{deps: newDeps(tmpCfg(t))}).Run(context.Background(), nil, stdio)
	if !errors.Is(err, cli.ErrUsage) {
		t.Fatalf("err = %v", err)
	}
}

func TestUse_EmptyArg(t *testing.T) {
	stdio, _, _ := newIO(nil)
	err := (&useCmd{deps: newDeps(tmpCfg(t))}).Run(context.Background(), []string{""}, stdio)
	if !errors.Is(err, cli.ErrUsage) {
		t.Fatalf("err = %v", err)
	}
}

func TestUse_BadFlag(t *testing.T) {
	stdio, _, _ := newIO(nil)
	err := (&useCmd{deps: newDeps(tmpCfg(t))}).Run(context.Background(), []string{"--nope"}, stdio)
	if !errors.Is(err, cli.ErrUsage) {
		t.Fatalf("err = %v", err)
	}
}

func TestUse_LoadError(t *testing.T) {
	cfgPath := tmpCfg(t)
	if err := os.WriteFile(cfgPath, []byte("{not json"), 0o600); err != nil {
		t.Fatalf("seed: %v", err)
	}
	stdio, _, _ := newIO(nil)
	err := (&useCmd{deps: newDeps(cfgPath)}).Run(context.Background(), []string{"x"}, stdio)
	if err == nil {
		t.Fatal("expected load error")
	}
}

func TestUse_SaveError(t *testing.T) {
	// Seed a good file at a path, then flip the save target to a bad one
	// while keeping load working.
	cfgPath := tmpCfg(t)
	seed(t, cfgPath)
	d := Deps{
		LoadCfg:    func() (config.Config, error) { return config.Load(cfgPath) },
		SaveCfg:    func(config.Config) error { return errors.New("boom") },
		ConfigPath: func() (string, error) { return cfgPath, nil },
	}
	stdio, _, _ := newIO(nil)
	err := (&useCmd{deps: d}).Run(context.Background(), []string{"other"}, stdio)
	if err == nil {
		t.Fatal("expected save error")
	}
}

// --- remove ---

func TestRemove_Help(t *testing.T) {
	if !strings.Contains((&removeCmd{}).Help(), "remove") {
		t.Fatal("help missing")
	}
}

func TestRemove_SwitchesActive(t *testing.T) {
	cfgPath := tmpCfg(t)
	seed(t, cfgPath)
	stdio, out, _ := newIO(nil)
	err := (&removeCmd{deps: newDeps(cfgPath)}).Run(context.Background(), []string{"default"}, stdio)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !strings.Contains(out.String(), "removed profile default") || !strings.Contains(out.String(), "active profile is now other") {
		t.Fatalf("stdout: %q", out.String())
	}
	c, _ := config.Load(cfgPath)
	if c.Active != "other" {
		t.Fatalf("active = %q", c.Active)
	}
	if _, ok := c.Profiles["default"]; ok {
		t.Fatal("default still present")
	}
}

func TestRemove_NoneRemaining(t *testing.T) {
	cfgPath := tmpCfg(t)
	// Single-profile config.
	c := config.Config{
		Profiles: map[string]config.Profile{"only": {Endpoint: "https://x", Token: "t"}},
		Active:   "only",
	}
	if err := config.Save(cfgPath, c); err != nil {
		t.Fatalf("seed: %v", err)
	}
	stdio, out, _ := newIO(nil)
	err := (&removeCmd{deps: newDeps(cfgPath)}).Run(context.Background(), []string{"only"}, stdio)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !strings.Contains(out.String(), "no profiles remain") {
		t.Fatalf("stdout: %q", out.String())
	}
}

func TestRemove_NotFound(t *testing.T) {
	cfgPath := tmpCfg(t)
	seed(t, cfgPath)
	stdio, _, _ := newIO(nil)
	err := (&removeCmd{deps: newDeps(cfgPath)}).Run(context.Background(), []string{"ghost"}, stdio)
	if err == nil || !strings.Contains(err.Error(), "not found") {
		t.Fatalf("err = %v", err)
	}
}

func TestRemove_MissingArg(t *testing.T) {
	stdio, _, _ := newIO(nil)
	err := (&removeCmd{deps: newDeps(tmpCfg(t))}).Run(context.Background(), nil, stdio)
	if !errors.Is(err, cli.ErrUsage) {
		t.Fatalf("err = %v", err)
	}
}

func TestRemove_EmptyArg(t *testing.T) {
	stdio, _, _ := newIO(nil)
	err := (&removeCmd{deps: newDeps(tmpCfg(t))}).Run(context.Background(), []string{""}, stdio)
	if !errors.Is(err, cli.ErrUsage) {
		t.Fatalf("err = %v", err)
	}
}

func TestRemove_BadFlag(t *testing.T) {
	stdio, _, _ := newIO(nil)
	err := (&removeCmd{deps: newDeps(tmpCfg(t))}).Run(context.Background(), []string{"--nope"}, stdio)
	if !errors.Is(err, cli.ErrUsage) {
		t.Fatalf("err = %v", err)
	}
}

func TestRemove_LoadError(t *testing.T) {
	cfgPath := tmpCfg(t)
	if err := os.WriteFile(cfgPath, []byte("{not json"), 0o600); err != nil {
		t.Fatalf("seed: %v", err)
	}
	stdio, _, _ := newIO(nil)
	err := (&removeCmd{deps: newDeps(cfgPath)}).Run(context.Background(), []string{"x"}, stdio)
	if err == nil {
		t.Fatal("expected load error")
	}
}

func TestRemove_SaveError(t *testing.T) {
	cfgPath := tmpCfg(t)
	seed(t, cfgPath)
	d := Deps{
		LoadCfg:    func() (config.Config, error) { return config.Load(cfgPath) },
		SaveCfg:    func(config.Config) error { return errors.New("boom") },
		ConfigPath: func() (string, error) { return cfgPath, nil },
	}
	stdio, _, _ := newIO(nil)
	err := (&removeCmd{deps: d}).Run(context.Background(), []string{"other"}, stdio)
	if err == nil {
		t.Fatal("expected save error")
	}
}

// --- show ---

func TestShow_Help(t *testing.T) {
	if !strings.Contains((&showCmd{}).Help(), "show") {
		t.Fatal("help missing")
	}
}

func TestShow_Active_Human(t *testing.T) {
	cfgPath := tmpCfg(t)
	seed(t, cfgPath)
	stdio, out, _ := newIO(nil)
	err := (&showCmd{deps: newDeps(cfgPath)}).Run(context.Background(), nil, stdio)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	s := out.String()
	if !strings.Contains(s, "name") || !strings.Contains(s, "default") {
		t.Fatalf("stdout missing name row: %q", s)
	}
	if !strings.Contains(s, "active") || !strings.Contains(s, "true") {
		t.Fatalf("stdout missing active row: %q", s)
	}
	if !strings.Contains(s, "********** (last 4: 1234)") {
		t.Fatalf("token redaction wrong: %q", s)
	}
	if strings.Contains(s, "abcdef1234") {
		t.Fatalf("token leaked: %q", s)
	}
}

func TestShow_Named_JSON(t *testing.T) {
	cfgPath := tmpCfg(t)
	seed(t, cfgPath)
	stdio, out, _ := newIO(nil)
	ctx := cli.WithGlobal(context.Background(), cli.Global{JSON: true})
	err := (&showCmd{deps: newDeps(cfgPath)}).Run(ctx, []string{"other"}, stdio)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	var got showPayload
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v (%q)", err, out.String())
	}
	if got.Name != "other" || got.Active {
		t.Fatalf("payload wrong: %+v", got)
	}
	if got.HasToken {
		t.Fatalf("other has empty token: %+v", got)
	}
	// Even under --json the raw token must never appear.
	if strings.Contains(out.String(), "abcdef1234") {
		t.Fatal("token leaked in --json")
	}
}

func TestShow_UnsetToken(t *testing.T) {
	cfgPath := tmpCfg(t)
	seed(t, cfgPath)
	stdio, out, _ := newIO(nil)
	err := (&showCmd{deps: newDeps(cfgPath)}).Run(context.Background(), []string{"other"}, stdio)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !strings.Contains(out.String(), "(unset)") {
		t.Fatalf("expected (unset): %q", out.String())
	}
}

func TestShow_ShortToken(t *testing.T) {
	// Covers the len(tok) < 4 branch in redactToken — we never expect this
	// from a real API key but the safety net should still print something.
	cfgPath := tmpCfg(t)
	c := config.Config{
		Profiles: map[string]config.Profile{"p": {Endpoint: "https://x", Token: "ab"}},
		Active:   "p",
	}
	if err := config.Save(cfgPath, c); err != nil {
		t.Fatalf("seed: %v", err)
	}
	stdio, out, _ := newIO(nil)
	err := (&showCmd{deps: newDeps(cfgPath)}).Run(context.Background(), nil, stdio)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !strings.Contains(out.String(), "last 4: ab") {
		t.Fatalf("stdout: %q", out.String())
	}
}

func TestShow_UnknownProfile(t *testing.T) {
	cfgPath := tmpCfg(t)
	seed(t, cfgPath)
	stdio, _, _ := newIO(nil)
	err := (&showCmd{deps: newDeps(cfgPath)}).Run(context.Background(), []string{"ghost"}, stdio)
	if !errors.Is(err, config.ErrUnknownProfile) {
		t.Fatalf("err = %v", err)
	}
}

// TestShow_NoActiveNoArg covers the "no name, no active" path: show should
// report unknown-profile rather than panic on an empty lookup.
func TestShow_NoActiveNoArg(t *testing.T) {
	cfgPath := tmpCfg(t)
	// Empty config: no profiles, no active.
	if err := config.Save(cfgPath, config.Config{}); err != nil {
		t.Fatalf("seed: %v", err)
	}
	stdio, _, _ := newIO(nil)
	err := (&showCmd{deps: newDeps(cfgPath)}).Run(context.Background(), nil, stdio)
	if !errors.Is(err, config.ErrUnknownProfile) {
		t.Fatalf("err = %v", err)
	}
}

func TestShow_EmptyArgFallsBackToActive(t *testing.T) {
	cfgPath := tmpCfg(t)
	seed(t, cfgPath)
	stdio, out, _ := newIO(nil)
	err := (&showCmd{deps: newDeps(cfgPath)}).Run(context.Background(), []string{""}, stdio)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !strings.Contains(out.String(), "default") {
		t.Fatalf("stdout: %q", out.String())
	}
}

func TestShow_BadFlag(t *testing.T) {
	stdio, _, _ := newIO(nil)
	err := (&showCmd{deps: newDeps(tmpCfg(t))}).Run(context.Background(), []string{"--nope"}, stdio)
	if !errors.Is(err, cli.ErrUsage) {
		t.Fatalf("err = %v", err)
	}
}

func TestShow_LoadError(t *testing.T) {
	cfgPath := tmpCfg(t)
	if err := os.WriteFile(cfgPath, []byte("{not json"), 0o600); err != nil {
		t.Fatalf("seed: %v", err)
	}
	stdio, _, _ := newIO(nil)
	err := (&showCmd{deps: newDeps(cfgPath)}).Run(context.Background(), nil, stdio)
	if err == nil {
		t.Fatal("expected load error")
	}
}
