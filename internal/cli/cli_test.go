package cli

import (
	"bytes"
	"context"
	"errors"
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
	child := &fakeCmd{help: "do child"}
	g := &Group{Summary: "a group", Children: map[string]Command{"c": child}}
	stdio, out, errb := testIO()
	err := g.Run(context.Background(), nil, stdio)
	if !errors.Is(err, ErrUsage) {
		t.Fatalf("err=%v want ErrUsage", err)
	}
	if !strings.Contains(out.String(), "a group") {
		t.Errorf("stdout missing summary: %q", out.String())
	}
	if errb.Len() != 0 {
		t.Errorf("stderr should be empty: %q", errb.String())
	}
}

func TestGroupRunHelpTokens(t *testing.T) {
	for _, tok := range []string{"-h", "--help", "help"} {
		tok := tok
		t.Run(tok, func(t *testing.T) {
			g := &Group{Summary: "S", Children: map[string]Command{"c": &fakeCmd{help: "do c"}}}
			stdio, out, errb := testIO()
			err := g.Run(context.Background(), []string{tok}, stdio)
			if !errors.Is(err, ErrUsage) {
				t.Fatalf("err=%v want ErrUsage", err)
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
	args := []string{"--json", "--endpoint=https://x", "--token-file=/tmp/t", "--verbose", "verb", "a"}
	g, rest, err := ParseGlobal(args)
	if err != nil {
		t.Fatalf("err=%v", err)
	}
	if !g.JSON || !g.Verbose {
		t.Errorf("bool flags not set: %+v", g)
	}
	if g.Endpoint != "https://x" || g.TokenFile != "/tmp/t" {
		t.Errorf("string flags wrong: %+v", g)
	}
	if len(rest) != 2 || rest[0] != "verb" || rest[1] != "a" {
		t.Errorf("rest=%v", rest)
	}
}

func TestParseGlobalAllFlagsSpaceForm(t *testing.T) {
	args := []string{"--endpoint", "https://y", "--token-file", "/p", "-v", "verb"}
	g, rest, err := ParseGlobal(args)
	if err != nil {
		t.Fatalf("err=%v", err)
	}
	if g.Endpoint != "https://y" || g.TokenFile != "/p" || !g.Verbose {
		t.Errorf("global=%+v", g)
	}
	if len(rest) != 1 || rest[0] != "verb" {
		t.Errorf("rest=%v", rest)
	}
}

func TestParseGlobalStopsAtPositional(t *testing.T) {
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
	_, _, err := ParseGlobal([]string{"--nope"})
	if err == nil {
		t.Fatalf("want error")
	}
	if !strings.Contains(err.Error(), "parse global flags") {
		t.Errorf("err should be wrapped: %v", err)
	}
}

func TestDispatchHappyPath(t *testing.T) {
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
	verbs := map[string]Command{"x": &fakeCmd{help: "x"}}
	stdio, out, _ := testIO()
	err := Dispatch(context.Background(), verbs, nil, stdio)
	if !errors.Is(err, ErrUsage) {
		t.Fatalf("err=%v", err)
	}
	if !strings.Contains(out.String(), "Commands:") {
		t.Errorf("stdout missing root help: %q", out.String())
	}
}

func TestDispatchHelp(t *testing.T) {
	for _, tok := range []string{"help", "-h", "--help"} {
		tok := tok
		t.Run(tok, func(t *testing.T) {
			verbs := map[string]Command{"x": &fakeCmd{help: "x"}}
			stdio, out, _ := testIO()
			args := []string{tok}
			if tok == "help" {
				// help as a verb should not be consumed by ParseGlobal.
				args = []string{"help"}
			}
			err := Dispatch(context.Background(), verbs, args, stdio)
			if !errors.Is(err, ErrUsage) {
				t.Fatalf("err=%v", err)
			}
			if !strings.Contains(out.String(), "Commands:") {
				t.Errorf("stdout missing root help: %q", out.String())
			}
		})
	}
}

func TestDispatchHelpAfterGlobalFlag(t *testing.T) {
	// Globals parse successfully, then the remainder starts with "help".
	verbs := map[string]Command{"x": &fakeCmd{help: "x"}}
	stdio, out, _ := testIO()
	err := Dispatch(context.Background(), verbs, []string{"--json", "help"}, stdio)
	if !errors.Is(err, ErrUsage) {
		t.Fatalf("err=%v", err)
	}
	if !strings.Contains(out.String(), "Commands:") {
		t.Errorf("stdout missing root help: %q", out.String())
	}
}

func TestDispatchUnknownVerb(t *testing.T) {
	verbs := map[string]Command{"x": &fakeCmd{help: "x"}}
	stdio, _, errb := testIO()
	err := Dispatch(context.Background(), verbs, []string{"zzz"}, stdio)
	if !errors.Is(err, ErrUsage) {
		t.Fatalf("err=%v", err)
	}
	if !strings.Contains(errb.String(), "unknown command: zzz") {
		t.Errorf("stderr missing unknown msg: %q", errb.String())
	}
}

func TestDispatchBadGlobalFlag(t *testing.T) {
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

func TestDispatchPropagatesChildError(t *testing.T) {
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
	child := &fakeCmd{help: "c"}
	verbs := map[string]Command{"run": child}
	stdio, _, _ := testIO()
	args := []string{"--endpoint", "https://api", "--token-file=/t", "-v", "--json", "run"}
	err := Dispatch(context.Background(), verbs, args, stdio)
	if err != nil {
		t.Fatalf("err=%v", err)
	}
	got := child.gotGlobal
	want := Global{JSON: true, Endpoint: "https://api", TokenFile: "/t", Verbose: true}
	if got != want {
		t.Errorf("gotGlobal=%+v want %+v", got, want)
	}
}

func TestRootHelpWritesSortedList(t *testing.T) {
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
}

func TestWithGlobalAndFrom(t *testing.T) {
	// nil context: WithGlobal should still succeed.
	ctx := WithGlobal(nil, Global{JSON: true})
	if g := GlobalFrom(ctx); !g.JSON {
		t.Errorf("GlobalFrom returned %+v", g)
	}
	// nil context on GlobalFrom returns zero value.
	if g := GlobalFrom(nil); g != (Global{}) {
		t.Errorf("GlobalFrom(nil)=%+v want zero", g)
	}
	// Empty context returns zero value.
	if g := GlobalFrom(context.Background()); g != (Global{}) {
		t.Errorf("GlobalFrom(bg)=%+v want zero", g)
	}
}

// stubAuthErr implements authError for ExitCode tests.
type stubAuthErr struct{ auth bool }

func (s stubAuthErr) Error() string       { return "auth" }
func (s stubAuthErr) IsAuthError() bool   { return s.auth }

func TestExitCode(t *testing.T) {
	if got := ExitCode(nil); got != 0 {
		t.Errorf("nil=%d want 0", got)
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
