package cli

import (
	"bytes"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"testing"
	"time"
)

// fakeCmd is a minimal Command used across dispatch tests.
type fakeCmd struct {
	ran       bool
	gotArgs   []string
	gotGlobal Global
	err       error
	help      string
}

func (f *fakeCmd) Run(ctx context.Context, args []string, _ IO) error {
	f.ran = true
	f.gotArgs = args
	f.gotGlobal = GlobalFrom(ctx)
	return f.err
}

func (f *fakeCmd) Help() string { return f.help }

// testIO builds an IO with in-memory streams for assertions.
func testIO() (IO, *bytes.Buffer, *bytes.Buffer) {
	var out, errb bytes.Buffer
	return IO{
		Stdin:  strings.NewReader(""),
		Stdout: &out,
		Stderr: &errb,
		Env:    func(string) string { return "" },
		Now:    func() time.Time { return time.Unix(0, 0) },
	}, &out, &errb
}

func TestDefaultIO(t *testing.T) {
	io := DefaultIO()
	if io.Stdin != os.Stdin {
		t.Errorf("Stdin: want os.Stdin")
	}
	if io.Stdout != os.Stdout {
		t.Errorf("Stdout: want os.Stdout")
	}
	if io.Stderr != os.Stderr {
		t.Errorf("Stderr: want os.Stderr")
	}
	if io.Env == nil {
		t.Errorf("Env: want non-nil")
	}
	// Verify Env is backed by os.Getenv by setting a known var.
	t.Setenv("ANA_CLI_TEST_VAR", "yes")
	if got := io.Env("ANA_CLI_TEST_VAR"); got != "yes" {
		t.Errorf("Env(ANA_CLI_TEST_VAR)=%q want yes", got)
	}
	if io.Now == nil {
		t.Errorf("Now: want non-nil")
	}
	if io.Now().IsZero() {
		t.Errorf("Now(): should not be zero time")
	}
}

func TestGroupRunEmptyArgs(t *testing.T) {
	t.Parallel()
	child := &fakeCmd{help: "do child"}
	g := &Group{Summary: "a group", Children: map[string]Command{"c": child}}
	stdio, out, errb := testIO()
	err := g.Run(context.Background(), nil, stdio)
	if !errors.Is(err, ErrHelp) {
		t.Fatalf("err=%v want ErrHelp", err)
	}
	if !strings.Contains(out.String(), "a group") {
		t.Errorf("stdout missing summary: %q", out.String())
	}
	if errb.Len() != 0 {
		t.Errorf("stderr should be empty: %q", errb.String())
	}
}

func TestGroupRunHelpTokens(t *testing.T) {
	t.Parallel()
	for _, tok := range []string{"-h", "--help", "help"} {
		t.Run(tok, func(t *testing.T) {
			t.Parallel()
			g := &Group{Summary: "S", Children: map[string]Command{"c": &fakeCmd{help: "do c"}}}
			stdio, out, errb := testIO()
			err := g.Run(context.Background(), []string{tok}, stdio)
			if !errors.Is(err, ErrHelp) {
				t.Fatalf("err=%v want ErrHelp", err)
			}
			if !strings.Contains(out.String(), "Commands:") {
				t.Errorf("stdout missing Commands: %q", out.String())
			}
			if errb.Len() != 0 {
				t.Errorf("stderr expected empty, got %q", errb.String())
			}
		})
	}
}

func TestGroupRunUnknownChild(t *testing.T) {
	t.Parallel()
	g := &Group{Children: map[string]Command{"c": &fakeCmd{help: "c help"}}}
	stdio, out, errb := testIO()
	err := g.Run(context.Background(), []string{"nope"}, stdio)
	if !errors.Is(err, ErrUsage) {
		t.Fatalf("err=%v want ErrUsage", err)
	}
	if !strings.Contains(errb.String(), "unknown subcommand: nope") {
		t.Errorf("stderr missing unknown msg: %q", errb.String())
	}
	if out.Len() != 0 {
		t.Errorf("stdout should be empty: %q", out.String())
	}
}

func TestGroupRunKnownChildDelegates(t *testing.T) {
	t.Parallel()
	child := &fakeCmd{help: "c"}
	g := &Group{Children: map[string]Command{"c": child}}
	stdio, _, _ := testIO()
	err := g.Run(context.Background(), []string{"c", "x", "y"}, stdio)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if !child.ran {
		t.Errorf("child.ran = false")
	}
	if got := child.gotArgs; len(got) != 2 || got[0] != "x" || got[1] != "y" {
		t.Errorf("gotArgs = %v", got)
	}
}

func TestGroupRunChildReturnsError(t *testing.T) {
	t.Parallel()
	want := errors.New("boom")
	child := &fakeCmd{err: want}
	g := &Group{Children: map[string]Command{"c": child}}
	stdio, _, _ := testIO()
	err := g.Run(context.Background(), []string{"c"}, stdio)
	if !errors.Is(err, want) {
		t.Errorf("err=%v want %v", err, want)
	}
}

func TestGroupHelpSortedWithSummary(t *testing.T) {
	t.Parallel()
	g := &Group{
		Summary: "the summary",
		Children: map[string]Command{
			"beta":  &fakeCmd{help: "beta does beta\ndetails"},
			"alpha": &fakeCmd{help: "alpha does alpha"},
		},
	}
	h := g.Help()
	if !strings.HasPrefix(h, "the summary\n") {
		t.Errorf("help missing leading summary: %q", h)
	}
	ai := strings.Index(h, "alpha")
	bi := strings.Index(h, "beta")
	if ai < 0 || bi < 0 || ai >= bi {
		t.Errorf("alpha should appear before beta: %q", h)
	}
	// First-line only for multi-line child help.
	if strings.Contains(h, "details") {
		t.Errorf("help should only show first line of child help: %q", h)
	}
}

func TestGroupHelpNoSummary(t *testing.T) {
	t.Parallel()
	g := &Group{Children: map[string]Command{"c": &fakeCmd{help: "c"}}}
	h := g.Help()
	if strings.HasPrefix(h, "\n") {
		t.Errorf("help should not start with a blank line when summary is empty: %q", h)
	}
	if !strings.HasPrefix(h, "Commands:") {
		t.Errorf("help should start with Commands: when no summary: %q", h)
	}
}

func TestParseGlobalAllFlagsEqualsForm(t *testing.T) {
	t.Parallel()
	args := []string{"--json", "--endpoint=https://x", "--token-file=/tmp/t", "--profile=prod", "verb", "a"}
	g, rest, err := ParseGlobal(args)
	if err != nil {
		t.Fatalf("err=%v", err)
	}
	if !g.JSON {
		t.Errorf("bool flags not set: %+v", g)
	}
	if g.Endpoint != "https://x" || g.TokenFile != "/tmp/t" || g.Profile != "prod" {
		t.Errorf("string flags wrong: %+v", g)
	}
	if len(rest) != 2 || rest[0] != "verb" || rest[1] != "a" {
		t.Errorf("rest=%v", rest)
	}
}

func TestParseGlobalAllFlagsSpaceForm(t *testing.T) {
	t.Parallel()
	args := []string{"--endpoint", "https://y", "--token-file", "/p", "verb"}
	g, rest, err := ParseGlobal(args)
	if err != nil {
		t.Fatalf("err=%v", err)
	}
	if g.Endpoint != "https://y" || g.TokenFile != "/p" {
		t.Errorf("global=%+v", g)
	}
	if len(rest) != 1 || rest[0] != "verb" {
		t.Errorf("rest=%v", rest)
	}
}

func TestParseGlobalStopsAtPositional(t *testing.T) {
	t.Parallel()
	g, rest, err := ParseGlobal([]string{"--json", "chat", "send"})
	if err != nil {
		t.Fatalf("err=%v", err)
	}
	if !g.JSON {
		t.Errorf("JSON should be true")
	}
	if len(rest) != 2 || rest[0] != "chat" || rest[1] != "send" {
		t.Errorf("rest=%v", rest)
	}
}

func TestParseGlobalDoubleDash(t *testing.T) {
	t.Parallel()
	g, rest, err := ParseGlobal([]string{"--json", "--", "--looks-like-flag", "x"})
	if err != nil {
		t.Fatalf("err=%v", err)
	}
	if !g.JSON {
		t.Errorf("JSON should be true")
	}
	if len(rest) != 2 || rest[0] != "--looks-like-flag" || rest[1] != "x" {
		t.Errorf("rest=%v", rest)
	}
}

func TestParseGlobalUnknownFlag(t *testing.T) {
	t.Parallel()
	_, _, err := ParseGlobal([]string{"--nope"})
	if err == nil {
		t.Fatalf("want error")
	}
	if !strings.Contains(err.Error(), "parse global flags") {
		t.Errorf("err should be wrapped: %v", err)
	}
}

func TestStripGlobalsBeforeVerb(t *testing.T) {
	t.Parallel()
	g, rest, err := StripGlobals([]string{"--json", "org", "show"})
	if err != nil {
		t.Fatalf("err=%v", err)
	}
	if !g.JSON {
		t.Errorf("JSON not set: %+v", g)
	}
	if len(rest) != 2 || rest[0] != "org" || rest[1] != "show" {
		t.Errorf("rest=%v", rest)
	}
}

func TestStripGlobalsAfterVerb(t *testing.T) {
	t.Parallel()
	g, rest, err := StripGlobals([]string{"org", "show", "--json"})
	if err != nil {
		t.Fatalf("err=%v", err)
	}
	if !g.JSON {
		t.Errorf("JSON not set: %+v", g)
	}
	if len(rest) != 2 || rest[0] != "org" || rest[1] != "show" {
		t.Errorf("rest=%v", rest)
	}
}

func TestStripGlobalsInterleaved(t *testing.T) {
	t.Parallel()
	// Global interleaved with a leaf positional; the positional must reach
	// the leaf unchanged.
	g, rest, err := StripGlobals([]string{"chat", "send", "--json", "id-123", "hello"})
	if err != nil {
		t.Fatalf("err=%v", err)
	}
	if !g.JSON {
		t.Errorf("JSON not set: %+v", g)
	}
	want := []string{"chat", "send", "id-123", "hello"}
	if len(rest) != len(want) {
		t.Fatalf("rest=%v want %v", rest, want)
	}
	for i := range want {
		if rest[i] != want[i] {
			t.Errorf("rest[%d]=%q want %q", i, rest[i], want[i])
		}
	}
}

func TestStripGlobalsEqualsForm(t *testing.T) {
	t.Parallel()
	g, rest, err := StripGlobals([]string{"org", "show", "--endpoint=https://x", "--json"})
	if err != nil {
		t.Fatalf("err=%v", err)
	}
	if g.Endpoint != "https://x" {
		t.Errorf("Endpoint=%q", g.Endpoint)
	}
	if !g.JSON {
		t.Errorf("JSON not set")
	}
	if len(rest) != 2 || rest[0] != "org" || rest[1] != "show" {
		t.Errorf("rest=%v", rest)
	}
}

func TestStripGlobalsSpaceForm(t *testing.T) {
	t.Parallel()
	g, rest, err := StripGlobals([]string{"--endpoint", "https://x", "org", "show"})
	if err != nil {
		t.Fatalf("err=%v", err)
	}
	if g.Endpoint != "https://x" {
		t.Errorf("Endpoint=%q", g.Endpoint)
	}
	if len(rest) != 2 || rest[0] != "org" || rest[1] != "show" {
		t.Errorf("rest=%v", rest)
	}
}

func TestStripGlobalsDoubleDashTerminator(t *testing.T) {
	t.Parallel()
	// After `--`, `--json` must be passed through as a positional so the leaf
	// receives it and reports an unknown-flag error. StripGlobals must not
	// consume it as the global.
	g, rest, err := StripGlobals([]string{"org", "show", "--", "--json"})
	if err != nil {
		t.Fatalf("err=%v", err)
	}
	if g.JSON {
		t.Errorf("JSON should NOT be set after --")
	}
	want := []string{"org", "show", "--", "--json"}
	if len(rest) != len(want) {
		t.Fatalf("rest=%v want %v", rest, want)
	}
	for i := range want {
		if rest[i] != want[i] {
			t.Errorf("rest[%d]=%q want %q", i, rest[i], want[i])
		}
	}
}

func TestStripGlobalsDuplicateLastWins(t *testing.T) {
	t.Parallel()
	g, _, err := StripGlobals([]string{"--endpoint", "https://a", "org", "--endpoint", "https://b", "show"})
	if err != nil {
		t.Fatalf("err=%v", err)
	}
	if g.Endpoint != "https://b" {
		t.Errorf("Endpoint=%q want https://b", g.Endpoint)
	}
}

func TestStripGlobalsUnknownFlagPassesThrough(t *testing.T) {
	t.Parallel()
	// Unknown flags stay in rest unchanged so the leaf's FlagSet emits the
	// canonical `flag provided but not defined: --xyz` error.
	g, rest, err := StripGlobals([]string{"org", "show", "--xyz"})
	if err != nil {
		t.Fatalf("err=%v", err)
	}
	if g.JSON || g.Endpoint != "" {
		t.Errorf("nothing should be consumed: %+v", g)
	}
	want := []string{"org", "show", "--xyz"}
	if len(rest) != len(want) {
		t.Fatalf("rest=%v want %v", rest, want)
	}
}

func TestStripGlobalsMissingValue(t *testing.T) {
	t.Parallel()
	_, _, err := StripGlobals([]string{"org", "show", "--endpoint"})
	if err == nil {
		t.Fatalf("want error for trailing value-less flag")
	}
	if !strings.Contains(err.Error(), "flag needs an argument") {
		t.Errorf("err=%v", err)
	}
}

// Stdlib `flag.FlagSet.Parse` treats `-name` and `--name` as equivalent.
// StripGlobals must do the same so `ana -json org show` works the same as
// `ana --json org show` — otherwise a user used to the single-dash form
// would have their flag silently passed through to the leaf's FlagSet.
func TestStripGlobalsSingleDashForm(t *testing.T) {
	t.Parallel()
	g, rest, err := StripGlobals([]string{"-json", "org", "-endpoint=https://x", "show"})
	if err != nil {
		t.Fatalf("err=%v", err)
	}
	if !g.JSON || g.Endpoint != "https://x" {
		t.Errorf("global=%+v", g)
	}
	if len(rest) != 2 || rest[0] != "org" || rest[1] != "show" {
		t.Errorf("rest=%v", rest)
	}
}

// Pathological dash-only tokens (`-`, `---`, `-=val`) are not flags; they
// must be passed through to rest unchanged so the leaf can reject or accept
// them on its own terms.
func TestStripGlobalsDashNoise(t *testing.T) {
	t.Parallel()
	_, rest, err := StripGlobals([]string{"-", "org", "---", "-=val", "show"})
	if err != nil {
		t.Fatalf("err=%v", err)
	}
	want := []string{"-", "org", "---", "-=val", "show"}
	if len(rest) != len(want) {
		t.Fatalf("rest=%v want %v", rest, want)
	}
	for i := range want {
		if rest[i] != want[i] {
			t.Errorf("rest[%d]=%q want %q", i, rest[i], want[i])
		}
	}
}

// Bool-valued globals accept an explicit `--name=value`. A non-bool-parseable
// value should surface as a usage error rather than being silently coerced.
func TestStripGlobalsBoolEqualsInvalid(t *testing.T) {
	t.Parallel()
	_, _, err := StripGlobals([]string{"--json=notbool", "org", "show"})
	if err == nil {
		t.Fatalf("want error for invalid bool value")
	}
	if !strings.Contains(err.Error(), "invalid value") {
		t.Errorf("err=%v", err)
	}
}

// Bool-valued globals accept `--name=false` to explicitly disable (stdlib
// semantics). Confirms the hasEquals branch propagates the literal value.
func TestStripGlobalsBoolEqualsFalse(t *testing.T) {
	t.Parallel()
	g, _, err := StripGlobals([]string{"--json=false", "org", "show"})
	if err != nil {
		t.Fatalf("err=%v", err)
	}
	if g.JSON {
		t.Errorf("JSON=true, want false for --json=false")
	}
}

func TestStripGlobalsAllFourFlags(t *testing.T) {
	t.Parallel()
	args := []string{
		"connector", "list",
		"--json",
		"--endpoint=https://api",
		"--token-file", "/tmp/t",
		"--profile=prod",
	}
	g, rest, err := StripGlobals(args)
	if err != nil {
		t.Fatalf("err=%v", err)
	}
	want := Global{JSON: true, Endpoint: "https://api", TokenFile: "/tmp/t", Profile: "prod"}
	if g != want {
		t.Errorf("global=%+v want %+v", g, want)
	}
	if len(rest) != 2 || rest[0] != "connector" || rest[1] != "list" {
		t.Errorf("rest=%v", rest)
	}
}

// TestGlobalFlagsRegistrySync catches drift between ParseGlobal's FlagSet and
// globalFlagRegistry. Any new global flag must appear in both; if the registry
// lags, StripGlobals would silently pass the flag through to the leaf (which
// would fail with `flag provided but not defined`).
func TestGlobalFlagsRegistrySync(t *testing.T) {
	t.Parallel()
	fs := flag.NewFlagSet("sync", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	var g Global
	fs.BoolVar(&g.JSON, "json", false, "")
	fs.StringVar(&g.Endpoint, "endpoint", "", "")
	fs.StringVar(&g.TokenFile, "token-file", "", "")
	fs.StringVar(&g.Profile, "profile", "", "")
	// Reflect ParseGlobal: every flag it declares must have an entry in the
	// registry with the correct takesValue classification.
	fs.VisitAll(func(f *flag.Flag) {
		spec, ok := lookupGlobal(f.Name)
		if !ok {
			t.Errorf("global flag %q missing from globalFlagRegistry", f.Name)
			return
		}
		// Stdlib flag.Flag.Value is a flag.Value; bool-typed values satisfy
		// flag.boolFlag (unexported) with IsBoolFlag() bool. Use the adapter
		// getter below to detect bool-ness without reaching into stdlib.
		boolish, isBool := f.Value.(interface{ IsBoolFlag() bool })
		takesValue := !(isBool && boolish.IsBoolFlag())
		if spec.takesValue != takesValue {
			t.Errorf("flag %q takesValue=%v want %v", f.Name, spec.takesValue, takesValue)
		}
	})
	// And no extras in the registry that ParseGlobal doesn't declare.
	for _, spec := range globalFlagRegistry {
		if fs.Lookup(spec.name) == nil {
			t.Errorf("registry has %q but ParseGlobal does not declare it", spec.name)
		}
	}
}

func TestDispatchHappyPath(t *testing.T) {
	t.Parallel()
	child := &fakeCmd{help: "child"}
	verbs := map[string]Command{"run": child}
	stdio, _, _ := testIO()
	err := Dispatch(context.Background(), verbs, []string{"--json", "run", "a", "b"}, stdio)
	if err != nil {
		t.Fatalf("err=%v", err)
	}
	if !child.ran {
		t.Fatalf("child not run")
	}
	if got := child.gotArgs; len(got) != 2 || got[0] != "a" || got[1] != "b" {
		t.Errorf("gotArgs=%v", got)
	}
	if !child.gotGlobal.JSON {
		t.Errorf("gotGlobal.JSON should be true: %+v", child.gotGlobal)
	}
}

func TestDispatchNoVerb(t *testing.T) {
	t.Parallel()
	verbs := map[string]Command{"x": &fakeCmd{help: "x"}}
	stdio, out, _ := testIO()
	err := Dispatch(context.Background(), verbs, nil, stdio)
	if !errors.Is(err, ErrHelp) {
		t.Fatalf("err=%v want ErrHelp", err)
	}
	if !strings.Contains(out.String(), "Commands:") {
		t.Errorf("stdout missing root help: %q", out.String())
	}
}

func TestDispatchHelp(t *testing.T) {
	t.Parallel()
	for _, tok := range []string{"help", "-h", "--help"} {
		t.Run(tok, func(t *testing.T) {
			t.Parallel()
			verbs := map[string]Command{"x": &fakeCmd{help: "x"}}
			stdio, out, _ := testIO()
			args := []string{tok}
			if tok == "help" {
				// help as a verb should not be consumed by ParseGlobal.
				args = []string{"help"}
			}
			err := Dispatch(context.Background(), verbs, args, stdio)
			if !errors.Is(err, ErrHelp) {
				t.Fatalf("err=%v want ErrHelp", err)
			}
			if !strings.Contains(out.String(), "Commands:") {
				t.Errorf("stdout missing root help: %q", out.String())
			}
		})
	}
}

func TestDispatchHelpAfterGlobalFlag(t *testing.T) {
	t.Parallel()
	// Globals parse successfully, then the remainder starts with "help".
	verbs := map[string]Command{"x": &fakeCmd{help: "x"}}
	stdio, out, _ := testIO()
	err := Dispatch(context.Background(), verbs, []string{"--json", "help"}, stdio)
	if !errors.Is(err, ErrHelp) {
		t.Fatalf("err=%v want ErrHelp", err)
	}
	if !strings.Contains(out.String(), "Commands:") {
		t.Errorf("stdout missing root help: %q", out.String())
	}
}

func TestDispatchUnknownVerb(t *testing.T) {
	t.Parallel()
	verbs := map[string]Command{"x": &fakeCmd{help: "x"}}
	stdio, _, errb := testIO()
	err := Dispatch(context.Background(), verbs, []string{"zzz"}, stdio)
	if !errors.Is(err, ErrUsage) {
		t.Fatalf("err=%v", err)
	}
	if !errors.Is(err, ErrReported) {
		t.Errorf("err should carry ErrReported: %v", err)
	}
	if !strings.Contains(errb.String(), "unknown command: zzz") {
		t.Errorf("stderr missing unknown msg: %q", errb.String())
	}
}

func TestDispatchBadGlobalFlag(t *testing.T) {
	t.Parallel()
	verbs := map[string]Command{"x": &fakeCmd{help: "x"}}
	stdio, _, errb := testIO()
	err := Dispatch(context.Background(), verbs, []string{"--no-such-flag"}, stdio)
	if !errors.Is(err, ErrUsage) {
		t.Fatalf("err=%v want ErrUsage", err)
	}
	if errb.Len() == 0 {
		t.Errorf("stderr should describe error")
	}
}

// StripGlobals-level failures (missing value, invalid bool) must surface to
// stderr and map to ErrUsage — distinct from leaf-level flag errors which
// TestDispatchBadGlobalFlag covers via the pass-through path. The returned
// err must also carry ErrReported so main() knows not to double-print.
func TestDispatchStripGlobalsError(t *testing.T) {
	t.Parallel()
	verbs := map[string]Command{"x": &fakeCmd{help: "x"}}
	stdio, _, errb := testIO()
	err := Dispatch(context.Background(), verbs, []string{"--endpoint"}, stdio)
	if !errors.Is(err, ErrUsage) {
		t.Fatalf("err=%v want ErrUsage", err)
	}
	if !errors.Is(err, ErrReported) {
		t.Errorf("err should carry ErrReported: %v", err)
	}
	if !strings.Contains(errb.String(), "flag needs an argument") {
		t.Errorf("stderr missing global-parse msg: %q", errb.String())
	}
}

func TestDispatchPropagatesChildError(t *testing.T) {
	t.Parallel()
	want := errors.New("inner")
	child := &fakeCmd{err: want}
	verbs := map[string]Command{"run": child}
	stdio, _, _ := testIO()
	err := Dispatch(context.Background(), verbs, []string{"run"}, stdio)
	if !errors.Is(err, want) {
		t.Fatalf("err=%v want %v", err, want)
	}
}

func TestDispatchStoresGlobalInContext(t *testing.T) {
	t.Parallel()
	child := &fakeCmd{help: "c"}
	verbs := map[string]Command{"run": child}
	stdio, _, _ := testIO()
	args := []string{"--endpoint", "https://api", "--token-file=/t", "--profile", "prod", "--json", "run"}
	err := Dispatch(context.Background(), verbs, args, stdio)
	if err != nil {
		t.Fatalf("err=%v", err)
	}
	got := child.gotGlobal
	want := Global{JSON: true, Endpoint: "https://api", TokenFile: "/t", Profile: "prod"}
	if got != want {
		t.Errorf("gotGlobal=%+v want %+v", got, want)
	}
}

func TestRootHelpWritesSortedList(t *testing.T) {
	t.Parallel()
	verbs := map[string]Command{
		"zebra": &fakeCmd{help: "zebra\nmore"},
		"ant":   &fakeCmd{help: "ant cmd"},
	}
	var buf bytes.Buffer
	RootHelp(&buf, verbs)
	s := buf.String()
	if !strings.Contains(s, "Usage: ana") {
		t.Errorf("missing usage: %q", s)
	}
	ai := strings.Index(s, "ant")
	zi := strings.Index(s, "zebra")
	if ai < 0 || zi < 0 || ai >= zi {
		t.Errorf("sort wrong: %q", s)
	}
	if strings.Contains(s, "more") {
		t.Errorf("only first line of child help should appear: %q", s)
	}
	// Global flags block must appear with every known flag, each exactly once.
	if !strings.Contains(s, "Global Flags:") {
		t.Errorf("missing Global Flags section: %q", s)
	}
	for _, name := range []string{"--json", "--endpoint", "--token-file", "--profile"} {
		if n := strings.Count(s, name); n != 1 {
			t.Errorf("flag %q appeared %d times, want 1: %q", name, n, s)
		}
	}
}

func TestDispatchLeafHelpShowsGlobalFlags(t *testing.T) {
	t.Parallel()
	child := &fakeCmd{help: "run leaf help"}
	verbs := map[string]Command{"run": child}
	stdio, out, _ := testIO()
	err := Dispatch(context.Background(), verbs, []string{"run", "--help"}, stdio)
	if !errors.Is(err, ErrHelp) {
		t.Fatalf("err=%v want ErrHelp", err)
	}
	s := out.String()
	if !strings.Contains(s, "run leaf help") {
		t.Errorf("leaf help missing: %q", s)
	}
	if !strings.Contains(s, "Global Flags:") {
		t.Errorf("leaf --help should append Global Flags block: %q", s)
	}
	// No double-emit: each flag appears exactly once.
	for _, name := range []string{"--json", "--endpoint", "--token-file", "--profile"} {
		if n := strings.Count(s, name); n != 1 {
			t.Errorf("flag %q appeared %d times, want 1: %q", name, n, s)
		}
	}
}

func TestWithGlobalAndFrom(t *testing.T) {
	t.Parallel()
	// Round-trip a Global through a non-nil parent ctx.
	ctx := WithGlobal(context.Background(), Global{JSON: true})
	if g := GlobalFrom(ctx); !g.JSON {
		t.Errorf("GlobalFrom returned %+v", g)
	}
	// Empty context returns zero value.
	if g := GlobalFrom(context.Background()); g != (Global{}) {
		t.Errorf("GlobalFrom(bg)=%+v want zero", g)
	}
}

func TestWithGlobalNilCtxPanics(t *testing.T) {
	t.Parallel()
	// Matches context.WithValue's stdlib contract: nil parent ctx panics.
	defer func() {
		if recover() == nil {
			t.Errorf("WithGlobal(nil,…) should panic")
		}
	}()
	//lint:ignore SA1012 intentional: asserting stdlib-style nil-ctx panic
	_ = WithGlobal(nil, Global{})
}

func TestGlobalFromNilCtxPanics(t *testing.T) {
	t.Parallel()
	// Matches context.Value's stdlib contract: nil ctx panics.
	defer func() {
		if recover() == nil {
			t.Errorf("GlobalFrom(nil) should panic")
		}
	}()
	//lint:ignore SA1012 intentional: asserting stdlib-style nil-ctx panic
	_ = GlobalFrom(nil)
}

func TestDispatchLeafHelp(t *testing.T) {
	t.Parallel()
	child := &fakeCmd{help: "run  Do the run thing.\nUsage: ana run"}
	verbs := map[string]Command{"run": child}
	stdio, out, _ := testIO()
	err := Dispatch(context.Background(), verbs, []string{"run", "--help"}, stdio)
	if !errors.Is(err, ErrHelp) {
		t.Fatalf("err=%v want ErrHelp", err)
	}
	if child.ran {
		t.Errorf("child should not run when --help short-circuits")
	}
	if !strings.Contains(out.String(), "Do the run thing.") {
		t.Errorf("stdout missing leaf help: %q", out.String())
	}
}

func TestDispatchLeafShortHelp(t *testing.T) {
	t.Parallel()
	child := &fakeCmd{help: "run leaf help"}
	verbs := map[string]Command{"run": child}
	stdio, out, _ := testIO()
	err := Dispatch(context.Background(), verbs, []string{"run", "-h"}, stdio)
	if !errors.Is(err, ErrHelp) {
		t.Fatalf("err=%v want ErrHelp", err)
	}
	if child.ran {
		t.Errorf("child should not run")
	}
	if !strings.Contains(out.String(), "run leaf help") {
		t.Errorf("stdout missing leaf help: %q", out.String())
	}
}

func TestGroupRunLeafHelpNested(t *testing.T) {
	t.Parallel()
	leaf := &fakeCmd{help: "leaf help"}
	g := &Group{Children: map[string]Command{"run": leaf}}
	stdio, out, _ := testIO()
	err := g.Run(context.Background(), []string{"run", "--help"}, stdio)
	if !errors.Is(err, ErrHelp) {
		t.Fatalf("err=%v want ErrHelp", err)
	}
	if leaf.ran {
		t.Errorf("leaf should not run")
	}
	if !strings.Contains(out.String(), "leaf help") {
		t.Errorf("stdout missing leaf help: %q", out.String())
	}
}

func TestDispatchLeafHelpMidArgs(t *testing.T) {
	t.Parallel()
	child := &fakeCmd{help: "run leaf help"}
	verbs := map[string]Command{"run": child}
	stdio, out, _ := testIO()
	err := Dispatch(context.Background(), verbs, []string{"run", "id", "--help"}, stdio)
	if !errors.Is(err, ErrHelp) {
		t.Fatalf("err=%v want ErrHelp", err)
	}
	if child.ran {
		t.Errorf("child should not run")
	}
	if !strings.Contains(out.String(), "run leaf help") {
		t.Errorf("stdout missing leaf help: %q", out.String())
	}
}

func TestDispatchLeafHelpWithGlobal(t *testing.T) {
	t.Parallel()
	child := &fakeCmd{help: "run leaf help"}
	verbs := map[string]Command{"run": child}
	stdio, out, _ := testIO()
	err := Dispatch(context.Background(), verbs, []string{"--json", "run", "--help"}, stdio)
	if !errors.Is(err, ErrHelp) {
		t.Fatalf("err=%v want ErrHelp", err)
	}
	if child.ran {
		t.Errorf("child should not run")
	}
	if !strings.Contains(out.String(), "run leaf help") {
		t.Errorf("stdout missing leaf help: %q", out.String())
	}
}

func TestDispatchLeafPositionalHelpDoesNotShortCircuit(t *testing.T) {
	t.Parallel()
	// Bare "help" is a legitimate positional (e.g. a message body). Only
	// --help/-h should trigger the leaf help path.
	child := &fakeCmd{help: "run leaf help"}
	verbs := map[string]Command{"run": child}
	stdio, out, _ := testIO()
	err := Dispatch(context.Background(), verbs, []string{"run", "help"}, stdio)
	if err != nil {
		t.Fatalf("err=%v want nil", err)
	}
	if !child.ran {
		t.Fatalf("child should run with positional \"help\"")
	}
	if got := child.gotArgs; len(got) != 1 || got[0] != "help" {
		t.Errorf("gotArgs=%v want [help]", got)
	}
	if out.Len() != 0 {
		t.Errorf("stdout should be empty: %q", out.String())
	}
}

// stubAuthErr implements authError for ExitCode tests.
type stubAuthErr struct{ auth bool }

func (s stubAuthErr) Error() string     { return "auth" }
func (s stubAuthErr) IsAuthError() bool { return s.auth }

func TestExitCode(t *testing.T) {
	t.Parallel()
	if got := ExitCode(nil); got != 0 {
		t.Errorf("nil=%d want 0", got)
	}
	if got := ExitCode(ErrHelp); got != 0 {
		t.Errorf("ErrHelp=%d want 0", got)
	}
	if got := ExitCode(fmt.Errorf("wrap: %w", ErrHelp)); got != 0 {
		t.Errorf("wrapped ErrHelp=%d want 0", got)
	}
	if got := ExitCode(ErrUsage); got != 1 {
		t.Errorf("ErrUsage=%d want 1", got)
	}
	if got := ExitCode(fmt.Errorf("wrap: %w", ErrUsage)); got != 1 {
		t.Errorf("wrapped ErrUsage=%d want 1", got)
	}
	if got := ExitCode(stubAuthErr{auth: true}); got != 3 {
		t.Errorf("authErr=%d want 3", got)
	}
	if got := ExitCode(fmt.Errorf("wrap: %w", stubAuthErr{auth: true})); got != 3 {
		t.Errorf("wrapped authErr=%d want 3", got)
	}
	// authError that reports false should NOT get the 3 treatment.
	if got := ExitCode(stubAuthErr{auth: false}); got != 2 {
		t.Errorf("authErr(false)=%d want 2", got)
	}
	if got := ExitCode(errors.New("random")); got != 2 {
		t.Errorf("other=%d want 2", got)
	}
}

// Ensure the IO struct's io.Reader/Writer fields implement the expected
// stdlib interfaces — a compile-time check guarding against future drift.
var _ io.Reader = DefaultIO().Stdin
var _ io.Writer = DefaultIO().Stdout
var _ io.Writer = DefaultIO().Stderr

// flagLeaf is a Flagger-implementing leaf used by the Phase 2 tests. It
// declares its own flags in Flags, then in Run declares them on a private
// FlagSet, calls ApplyAncestorFlags, and records what parsed. Keeping the
// declaration inside Flags (rather than repeating it inline in Run) mirrors
// the contract the real verb packages will adopt when they migrate.
type flagLeaf struct {
	declareOwn func(fs *flag.FlagSet)
	ancestorFS *flag.FlagSet
	parsedArgs []string
	ran        bool
}

func (l *flagLeaf) Flags(fs *flag.FlagSet) {
	if l.declareOwn != nil {
		l.declareOwn(fs)
	}
}

func (l *flagLeaf) Help() string { return "leaf help line" }

func (l *flagLeaf) Run(ctx context.Context, args []string, _ IO) error {
	fs := flag.NewFlagSet("leaf", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	l.Flags(fs)
	ApplyAncestorFlags(ctx, fs)
	l.ancestorFS = fs
	if err := fs.Parse(args); err != nil {
		return err
	}
	l.parsedArgs = fs.Args()
	l.ran = true
	return nil
}

func TestWithAncestorFlagsPreservesOrder(t *testing.T) {
	t.Parallel()
	var order []string
	ctx := context.Background()
	ctx = WithAncestorFlags(ctx, func(fs *flag.FlagSet) { order = append(order, "outer") })
	ctx = WithAncestorFlags(ctx, func(fs *flag.FlagSet) { order = append(order, "inner") })
	fs := flag.NewFlagSet("t", flag.ContinueOnError)
	ApplyAncestorFlags(ctx, fs)
	if len(order) != 2 || order[0] != "outer" || order[1] != "inner" {
		t.Errorf("order=%v want [outer inner]", order)
	}
}

func TestApplyAncestorFlagsNoRegistrars(t *testing.T) {
	t.Parallel()
	// Zero-registrar ctx must not panic — the absence path matters for
	// leaves dispatched directly (not under a Group with Flags set).
	fs := flag.NewFlagSet("t", flag.ContinueOnError)
	ApplyAncestorFlags(context.Background(), fs)
	// sanity: no flags were declared
	count := 0
	fs.VisitAll(func(*flag.Flag) { count++ })
	if count != 0 {
		t.Errorf("VisitAll count=%d want 0", count)
	}
}

func TestDeclareStringGuardsAgainstRedeclare(t *testing.T) {
	t.Parallel()
	fs := flag.NewFlagSet("t", flag.ContinueOnError)
	var leafT, ancT string
	fs.StringVar(&leafT, "foo", "leafdef", "leaf usage")
	DeclareString(fs, &ancT, "foo", "ancdef", "anc usage")
	if err := fs.Parse([]string{"--foo", "x"}); err != nil {
		t.Fatalf("parse err=%v", err)
	}
	if leafT != "x" {
		t.Errorf("leafT=%q want x", leafT)
	}
	if ancT != "" {
		t.Errorf("ancT=%q want '' (ancestor should not have been bound)", ancT)
	}
}

func TestDeclareBoolGuardsAgainstRedeclare(t *testing.T) {
	t.Parallel()
	fs := flag.NewFlagSet("t", flag.ContinueOnError)
	var leafT, ancT bool
	fs.BoolVar(&leafT, "v", false, "leaf usage")
	DeclareBool(fs, &ancT, "v", false, "anc usage")
	if err := fs.Parse([]string{"--v"}); err != nil {
		t.Fatalf("parse err=%v", err)
	}
	if !leafT {
		t.Errorf("leafT=false want true")
	}
	if ancT {
		t.Errorf("ancT=true want false (ancestor should not have been bound)")
	}
}

func TestDeclareBoolFreshDeclaration(t *testing.T) {
	t.Parallel()
	// When no prior declaration exists, DeclareBool must bind the target.
	fs := flag.NewFlagSet("t", flag.ContinueOnError)
	var target bool
	DeclareBool(fs, &target, "v", false, "usage")
	if err := fs.Parse([]string{"--v"}); err != nil {
		t.Fatalf("parse err=%v", err)
	}
	if !target {
		t.Errorf("target=false want true")
	}
}

func TestDeclareStringFreshDeclaration(t *testing.T) {
	t.Parallel()
	fs := flag.NewFlagSet("t", flag.ContinueOnError)
	var target string
	DeclareString(fs, &target, "s", "def", "usage")
	if err := fs.Parse([]string{"--s", "x"}); err != nil {
		t.Fatalf("parse err=%v", err)
	}
	if target != "x" {
		t.Errorf("target=%q want x", target)
	}
}

func TestDeclareIntGuardsAgainstRedeclare(t *testing.T) {
	t.Parallel()
	fs := flag.NewFlagSet("t", flag.ContinueOnError)
	var leafT, ancT int
	fs.IntVar(&leafT, "port", 1, "leaf usage")
	DeclareInt(fs, &ancT, "port", 2, "anc usage")
	if err := fs.Parse([]string{"--port", "7"}); err != nil {
		t.Fatalf("parse err=%v", err)
	}
	if leafT != 7 {
		t.Errorf("leafT=%d want 7", leafT)
	}
	if ancT != 0 {
		t.Errorf("ancT=%d want 0 (ancestor should not have been bound)", ancT)
	}
}

func TestDeclareIntFreshDeclaration(t *testing.T) {
	t.Parallel()
	fs := flag.NewFlagSet("t", flag.ContinueOnError)
	var target int
	DeclareInt(fs, &target, "port", 443, "usage")
	if err := fs.Parse([]string{"--port", "5432"}); err != nil {
		t.Fatalf("parse err=%v", err)
	}
	if target != 5432 {
		t.Errorf("target=%d want 5432", target)
	}
}

func TestDeclareIntDefault(t *testing.T) {
	t.Parallel()
	fs := flag.NewFlagSet("t", flag.ContinueOnError)
	var target int
	DeclareInt(fs, &target, "port", 443, "usage")
	if err := fs.Parse(nil); err != nil {
		t.Fatalf("parse err=%v", err)
	}
	if target != 443 {
		t.Errorf("target=%d want 443 default", target)
	}
}

func TestRenderFlagsAsTextDefaultAndEmptyType(t *testing.T) {
	t.Parallel()
	fs := flag.NewFlagSet("t", flag.ContinueOnError)
	// Bool with zero-value default "false" → no "(default: X)" suffix
	fs.Bool("v", false, "verbose flag")
	// String with non-empty default → suffix emitted
	fs.String("name", "mydef", "a `NAME`")
	// String with empty default → no suffix
	fs.String("other", "", "other flag")
	got := renderFlagsAsText(fs)
	if !strings.Contains(got, "--name") || !strings.Contains(got, "(default: mydef)") {
		t.Errorf("expected --name with default: %q", got)
	}
	if strings.Contains(got, "--v ") && strings.Contains(got, "(default: false)") {
		t.Errorf("bool false default should not render: %q", got)
	}
	if strings.Contains(got, "--other  ") && strings.Contains(got, "(default:") {
		t.Errorf("empty string default should not render: %q", got)
	}
}

func TestGroupFlagsPropagateToLeaf(t *testing.T) {
	t.Parallel()
	var leafFoo string
	leaf := &flagLeaf{declareOwn: func(fs *flag.FlagSet) {
		// leaf-only flag so ancestor's declaration wins via DeclareString
		fs.StringVar(new(string), "leaf-only", "", "leaf-only flag")
	}}
	g := &Group{
		Flags: func(fs *flag.FlagSet) {
			DeclareString(fs, &leafFoo, "foo", "", "inherited foo flag")
		},
		Children: map[string]Command{"leaf": leaf},
	}
	stdio, _, _ := testIO()
	err := g.Run(context.Background(), []string{"leaf", "--foo", "x", "--leaf-only", "y"}, stdio)
	if err != nil {
		t.Fatalf("err=%v", err)
	}
	if !leaf.ran {
		t.Fatalf("leaf did not run")
	}
	if leafFoo != "x" {
		t.Errorf("leafFoo=%q want x", leafFoo)
	}
}

func TestGroupFlagsVisibleInLeafHelp(t *testing.T) {
	t.Parallel()
	leaf := &flagLeaf{declareOwn: func(fs *flag.FlagSet) {
		fs.String("leaf-only", "", "leaf flag `NAME`")
	}}
	g := &Group{
		Flags: func(fs *flag.FlagSet) {
			DeclareString(fs, new(string), "foo", "", "the foo `VALUE`")
		},
		Children: map[string]Command{"leaf": leaf},
	}
	stdio, out, _ := testIO()
	err := g.Run(context.Background(), []string{"leaf", "--help"}, stdio)
	if !errors.Is(err, ErrHelp) {
		t.Fatalf("err=%v want ErrHelp", err)
	}
	s := out.String()
	if !strings.Contains(s, "--foo") {
		t.Errorf("leaf --help missing ancestor --foo: %q", s)
	}
	if !strings.Contains(s, "--leaf-only") {
		t.Errorf("leaf --help missing leaf --leaf-only: %q", s)
	}
	if !strings.Contains(s, "Flags:") {
		t.Errorf("leaf --help missing Flags: header: %q", s)
	}
}

func TestGroupFlagsNestedTwoLevels(t *testing.T) {
	t.Parallel()
	var outerV, middleV string
	leaf := &flagLeaf{declareOwn: func(fs *flag.FlagSet) {
		fs.String("leaf-only", "", "leaf-only flag")
	}}
	middle := &Group{
		Flags: func(fs *flag.FlagSet) {
			DeclareString(fs, &middleV, "middle", "", "middle flag")
		},
		Children: map[string]Command{"leaf": leaf},
	}
	outer := &Group{
		Flags: func(fs *flag.FlagSet) {
			DeclareString(fs, &outerV, "outer", "", "outer flag")
		},
		Children: map[string]Command{"mid": middle},
	}
	stdio, _, _ := testIO()
	err := outer.Run(context.Background(),
		[]string{"mid", "leaf", "--outer", "o", "--middle", "m", "--leaf-only", "l"}, stdio)
	if err != nil {
		t.Fatalf("err=%v", err)
	}
	if !leaf.ran {
		t.Fatalf("leaf did not run")
	}
	if outerV != "o" {
		t.Errorf("outerV=%q want o", outerV)
	}
	if middleV != "m" {
		t.Errorf("middleV=%q want m", middleV)
	}
}

func TestGroupFlagsPrecedenceLeafWins(t *testing.T) {
	t.Parallel()
	// Both ancestor and leaf declare --foo; leaf declares FIRST (via its
	// Flags method called from Run), so DeclareString in the ancestor
	// registrar is a no-op. The leaf's target pointer receives the value.
	var ancestorV string
	var leafV string
	leaf := &flagLeaf{declareOwn: func(fs *flag.FlagSet) {
		fs.StringVar(&leafV, "foo", "leafdef", "leaf foo")
	}}
	g := &Group{
		Flags: func(fs *flag.FlagSet) {
			DeclareString(fs, &ancestorV, "foo", "ancdef", "ancestor foo")
		},
		Children: map[string]Command{"leaf": leaf},
	}
	stdio, _, _ := testIO()
	err := g.Run(context.Background(), []string{"leaf", "--foo", "x"}, stdio)
	if err != nil {
		t.Fatalf("err=%v", err)
	}
	if leafV != "x" {
		t.Errorf("leafV=%q want x", leafV)
	}
	if ancestorV != "" {
		t.Errorf("ancestorV=%q want '' (ancestor target should not bind)", ancestorV)
	}
}

func TestGroupFlagsHelpForGroupItself(t *testing.T) {
	t.Parallel()
	g := &Group{
		Summary: "group sum",
		Flags: func(fs *flag.FlagSet) {
			DeclareString(fs, new(string), "groupflag", "", "the group flag")
		},
		Children: map[string]Command{"c": &fakeCmd{help: "c help"}},
	}
	stdio, out, _ := testIO()
	// "--help" on the group itself shows group help (including group Flags)
	err := g.Run(context.Background(), []string{"--help"}, stdio)
	if !errors.Is(err, ErrHelp) {
		t.Fatalf("err=%v want ErrHelp", err)
	}
	s := out.String()
	if !strings.Contains(s, "group sum") {
		t.Errorf("group help missing summary: %q", s)
	}
	if !strings.Contains(s, "Flags:") {
		t.Errorf("group help missing Flags: block: %q", s)
	}
	if !strings.Contains(s, "--groupflag") {
		t.Errorf("group help missing --groupflag: %q", s)
	}
}

func TestRenderFlagsAsTextEmpty(t *testing.T) {
	t.Parallel()
	fs := flag.NewFlagSet("t", flag.ContinueOnError)
	if got := renderFlagsAsText(fs); got != "" {
		t.Errorf("empty fs should render '', got %q", got)
	}
}

func TestRenderFlagsAsTextSorted(t *testing.T) {
	t.Parallel()
	fs := flag.NewFlagSet("t", flag.ContinueOnError)
	fs.String("zebra", "", "the zebra")
	fs.String("alpha", "", "the alpha")
	got := renderFlagsAsText(fs)
	ai := strings.Index(got, "--alpha")
	zi := strings.Index(got, "--zebra")
	if ai < 0 || zi < 0 || ai >= zi {
		t.Errorf("flags should be sorted alpha→zebra: %q", got)
	}
}
