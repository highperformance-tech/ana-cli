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

// withRootGlobals returns a *Group whose Flags closure declares the four
// well-known root persistent flags. Most dispatch tests use this so the
// merged FlagSet behaves like the real binary's.
func withRootGlobals(children map[string]Command) *Group {
	return &Group{
		Flags: func(fs *flag.FlagSet) {
			fs.Bool("json", false, "emit JSON output")
			fs.String("endpoint", "", "override API endpoint URL")
			fs.String("token-file", "", "path to bearer-token file")
			fs.String("profile", "", "config profile to use")
		},
		Children: children,
	}
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
	if !strings.Contains(errb.String(), "nope") {
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

func TestDispatchHappyPath(t *testing.T) {
	t.Parallel()
	child := &fakeCmd{help: "child"}
	root := withRootGlobals(map[string]Command{"run": child})
	stdio, _, _ := testIO()
	err := Dispatch(context.Background(), root, []string{"--json", "run", "a", "b"}, stdio)
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
	root := withRootGlobals(map[string]Command{"x": &fakeCmd{help: "x"}})
	stdio, out, _ := testIO()
	err := Dispatch(context.Background(), root, nil, stdio)
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
			root := withRootGlobals(map[string]Command{"x": &fakeCmd{help: "x"}})
			stdio, out, _ := testIO()
			err := Dispatch(context.Background(), root, []string{tok}, stdio)
			if !errors.Is(err, ErrHelp) {
				t.Fatalf("err=%v want ErrHelp", err)
			}
			if !strings.Contains(out.String(), "Commands:") {
				t.Errorf("stdout missing root help: %q", out.String())
			}
		})
	}
}

func TestDispatchUnknownVerb(t *testing.T) {
	t.Parallel()
	root := withRootGlobals(map[string]Command{"x": &fakeCmd{help: "x"}})
	stdio, _, errb := testIO()
	err := Dispatch(context.Background(), root, []string{"zzz"}, stdio)
	if !errors.Is(err, ErrUsage) {
		t.Fatalf("err=%v", err)
	}
	if !errors.Is(err, ErrReported) {
		t.Errorf("err should carry ErrReported: %v", err)
	}
	if !strings.Contains(errb.String(), "zzz") {
		t.Errorf("stderr missing unknown msg: %q", errb.String())
	}
}

func TestDispatchBadGlobalFlag(t *testing.T) {
	t.Parallel()
	root := withRootGlobals(map[string]Command{"x": &fakeCmd{help: "x"}})
	stdio, _, errb := testIO()
	err := Dispatch(context.Background(), root, []string{"--no-such-flag"}, stdio)
	if !errors.Is(err, ErrUsage) {
		t.Fatalf("err=%v want ErrUsage", err)
	}
	if errb.Len() == 0 {
		t.Errorf("stderr should describe error")
	}
}

func TestDispatchPropagatesChildError(t *testing.T) {
	t.Parallel()
	want := errors.New("inner")
	child := &fakeCmd{err: want}
	root := withRootGlobals(map[string]Command{"run": child})
	stdio, _, _ := testIO()
	err := Dispatch(context.Background(), root, []string{"run"}, stdio)
	if !errors.Is(err, want) {
		t.Fatalf("err=%v want %v", err, want)
	}
}

func TestDispatchStoresGlobalInContext(t *testing.T) {
	t.Parallel()
	child := &fakeCmd{help: "c"}
	root := withRootGlobals(map[string]Command{"run": child})
	stdio, _, _ := testIO()
	args := []string{"--endpoint", "https://api", "--token-file=/t", "--profile", "prod", "--json", "run"}
	err := Dispatch(context.Background(), root, args, stdio)
	if err != nil {
		t.Fatalf("err=%v", err)
	}
	got := child.gotGlobal
	want := Global{JSON: true, Endpoint: "https://api", TokenFile: "/t", Profile: "prod"}
	if got != want {
		t.Errorf("gotGlobal=%+v want %+v", got, want)
	}
}

func TestRootHelpSortsAndShowsGlobals(t *testing.T) {
	t.Parallel()
	root := withRootGlobals(map[string]Command{
		"zebra": &fakeCmd{help: "zebra\nmore"},
		"ant":   &fakeCmd{help: "ant cmd"},
	})
	s := RootHelp(root)
	ai := strings.Index(s, "ant")
	zi := strings.Index(s, "zebra")
	if ai < 0 || zi < 0 || ai >= zi {
		t.Errorf("sort wrong: %q", s)
	}
	if strings.Contains(s, "more") {
		t.Errorf("only first line of child help should appear: %q", s)
	}
	for _, name := range []string{"--json", "--endpoint", "--token-file", "--profile"} {
		if n := strings.Count(s, name); n != 1 {
			t.Errorf("flag %q appeared %d times, want 1: %q", name, n, s)
		}
	}
}

func TestDispatchLeafHelpShowsAncestorFlags(t *testing.T) {
	t.Parallel()
	child := &fakeCmd{help: "run leaf help"}
	root := withRootGlobals(map[string]Command{"run": child})
	stdio, out, _ := testIO()
	err := Dispatch(context.Background(), root, []string{"run", "--help"}, stdio)
	if !errors.Is(err, ErrHelp) {
		t.Fatalf("err=%v want ErrHelp", err)
	}
	s := out.String()
	if !strings.Contains(s, "run leaf help") {
		t.Errorf("leaf help missing: %q", s)
	}
	for _, name := range []string{"--json", "--endpoint", "--token-file", "--profile"} {
		if n := strings.Count(s, name); n != 1 {
			t.Errorf("flag %q appeared %d times, want 1: %q", name, n, s)
		}
	}
}

func TestWithGlobalAndFrom(t *testing.T) {
	t.Parallel()
	ctx := WithGlobal(context.Background(), Global{JSON: true})
	if g := GlobalFrom(ctx); !g.JSON {
		t.Errorf("GlobalFrom returned %+v", g)
	}
	if g := GlobalFrom(context.Background()); g != (Global{}) {
		t.Errorf("GlobalFrom(bg)=%+v want zero", g)
	}
}

func TestWithGlobalNilCtxPanics(t *testing.T) {
	t.Parallel()
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
	root := withRootGlobals(map[string]Command{"run": child})
	stdio, out, _ := testIO()
	err := Dispatch(context.Background(), root, []string{"run", "--help"}, stdio)
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
	root := withRootGlobals(map[string]Command{"run": child})
	stdio, out, _ := testIO()
	err := Dispatch(context.Background(), root, []string{"run", "-h"}, stdio)
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
	root := withRootGlobals(map[string]Command{"run": child})
	stdio, out, _ := testIO()
	err := Dispatch(context.Background(), root, []string{"run", "id", "--help"}, stdio)
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
	root := withRootGlobals(map[string]Command{"run": child})
	stdio, out, _ := testIO()
	err := Dispatch(context.Background(), root, []string{"--json", "run", "--help"}, stdio)
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
	root := withRootGlobals(map[string]Command{"run": child})
	stdio, out, _ := testIO()
	err := Dispatch(context.Background(), root, []string{"run", "help"}, stdio)
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
	if got := ExitCode(stubAuthErr{auth: false}); got != 2 {
		t.Errorf("authErr(false)=%d want 2", got)
	}
	if got := ExitCode(errors.New("random")); got != 2 {
		t.Errorf("other=%d want 2", got)
	}
}

// Compile-time interface checks guarding against future drift.
var _ io.Reader = DefaultIO().Stdin
var _ io.Writer = DefaultIO().Stdout
var _ io.Writer = DefaultIO().Stderr

// flagLeaf is a Flagger-implementing leaf used by the resolver+merge tests.
// The fields it declares end up on the merged FlagSet via the resolver.
type flagLeaf struct {
	declareOwn func(fs *flag.FlagSet)
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
	l.parsedArgs = args
	l.ran = true
	return nil
}

func TestGroupFlagsPropagateToLeaf(t *testing.T) {
	t.Parallel()
	var leafFoo string
	leaf := &flagLeaf{declareOwn: func(fs *flag.FlagSet) {
		fs.StringVar(new(string), "leaf-only", "", "leaf-only flag")
	}}
	g := &Group{
		Flags: func(fs *flag.FlagSet) {
			fs.StringVar(&leafFoo, "foo", "", "inherited foo flag")
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
			fs.String("foo", "", "the foo `VALUE`")
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
			fs.StringVar(&middleV, "middle", "", "middle flag")
		},
		Children: map[string]Command{"leaf": leaf},
	}
	outer := &Group{
		Flags: func(fs *flag.FlagSet) {
			fs.StringVar(&outerV, "outer", "", "outer flag")
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
	// Both ancestor and leaf declare --foo. The contract: the parsed value
	// lands on the leaf's target. (The ancestor's target is left at its
	// default-value initialization side effect; the parser never writes to
	// it because the leaf's binding shadows in MergedFS.)
	var ancestorV string
	var leafV string
	leaf := &flagLeaf{declareOwn: func(fs *flag.FlagSet) {
		fs.StringVar(&leafV, "foo", "leafdef", "leaf foo")
	}}
	g := &Group{
		Flags: func(fs *flag.FlagSet) {
			fs.StringVar(&ancestorV, "foo", "ancdef", "ancestor foo")
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
	if ancestorV == "x" {
		t.Errorf("ancestorV=%q — parser should NOT have written the parsed value to ancestor target", ancestorV)
	}
}

func TestGroupFlagsHelpForGroupItself(t *testing.T) {
	t.Parallel()
	g := &Group{
		Summary: "group sum",
		Flags: func(fs *flag.FlagSet) {
			fs.String("groupflag", "", "the group flag")
		},
		Children: map[string]Command{"c": &fakeCmd{help: "c help"}},
	}
	stdio, out, _ := testIO()
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

func TestRootHelpNil(t *testing.T) {
	t.Parallel()
	if got := RootHelp(nil); got != "" {
		t.Errorf("RootHelp(nil)=%q want \"\"", got)
	}
}

func TestFlagWasSetNil(t *testing.T) {
	t.Parallel()
	if FlagWasSet(nil, "any") {
		t.Errorf("FlagWasSet(nil) should be false")
	}
}

func TestDispatchLandsOnGroupLeaf(t *testing.T) {
	t.Parallel()
	g := &Group{Children: map[string]Command{"sub": &fakeCmd{help: "sub help"}}}
	root := withRootGlobals(map[string]Command{"grp": g})
	stdio, out, _ := testIO()
	err := Dispatch(context.Background(), root, []string{"grp"}, stdio)
	if !errors.Is(err, ErrHelp) {
		t.Fatalf("err=%v want ErrHelp", err)
	}
	if !strings.Contains(out.String(), "sub help") {
		t.Errorf("expected group help on stdout: %q", out.String())
	}
}

func TestGroupRun_LandsOnInnerGroup(t *testing.T) {
	t.Parallel()
	inner := &Group{Children: map[string]Command{"sub": &fakeCmd{help: "sub help"}}}
	outer := &Group{Children: map[string]Command{"inner": inner}}
	stdio, out, _ := testIO()
	err := outer.Run(context.Background(), []string{"inner"}, stdio)
	if !errors.Is(err, ErrHelp) {
		t.Fatalf("err=%v want ErrHelp", err)
	}
	if !strings.Contains(out.String(), "sub") {
		t.Errorf("expected inner help on stdout: %q", out.String())
	}
}

func TestGroupRun_BadFlagPropagates(t *testing.T) {
	t.Parallel()
	leaf := &flagLeaf{declareOwn: func(fs *flag.FlagSet) {
		fs.String("known", "", "known")
	}}
	g := &Group{Children: map[string]Command{"leaf": leaf}}
	stdio, _, errb := testIO()
	err := g.Run(context.Background(), []string{"leaf", "--nope"}, stdio)
	if !errors.Is(err, ErrUsage) {
		t.Fatalf("err=%v want ErrUsage", err)
	}
	if errb.Len() == 0 {
		t.Errorf("stderr should describe error")
	}
}

func TestRenderResolvedHelp_GroupLeaf(t *testing.T) {
	t.Parallel()
	g := &Group{Summary: "grp sum", Children: map[string]Command{"a": &fakeCmd{help: "a"}}}
	root := withRootGlobals(map[string]Command{"grp": g})
	res, err := Resolve(root, []string{"grp"})
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	var buf strings.Builder
	RenderResolvedHelp(res, root, &buf)
	if !strings.Contains(buf.String(), "grp sum") {
		t.Errorf("expected group summary in output: %q", buf.String())
	}
}

func TestRenderResolvedHelpNilRes(t *testing.T) {
	t.Parallel()
	root := withRootGlobals(map[string]Command{"x": &fakeCmd{help: "x"}})
	var buf strings.Builder
	RenderResolvedHelp(nil, root, &buf)
	if !strings.Contains(buf.String(), "Commands:") {
		t.Errorf("nil-res should fall back to RootHelp: %q", buf.String())
	}
}

func TestDispatchNilRoot(t *testing.T) {
	t.Parallel()
	stdio, _, _ := testIO()
	err := Dispatch(context.Background(), nil, []string{"x"}, stdio)
	if err == nil || !strings.Contains(err.Error(), "nil root") {
		t.Errorf("err=%v", err)
	}
}

func TestParseFlagToken_NotAFlag(t *testing.T) {
	t.Parallel()
	cases := []string{"", "-", "--", "x"}
	for _, tok := range cases {
		_, _, _, isLong := parseFlagToken(tok)
		if isLong {
			t.Errorf("parseFlagToken(%q): isLong=true, want false", tok)
		}
	}
}

func TestRenderFlagsAsTextDefaultAndEmptyType(t *testing.T) {
	t.Parallel()
	fs := flag.NewFlagSet("t", flag.ContinueOnError)
	fs.Bool("v", false, "verbose flag")
	fs.String("name", "mydef", "a `NAME`")
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
