package cli

import (
	"context"
	"errors"
	"flag"
	"strings"
	"testing"
)

// resolveLeaf is a Flagger leaf used across resolver tests. Targets it
// declares are observable on the parsed merged FlagSet — tests assert against
// the local pointers to verify routing.
type resolveLeaf struct {
	endpoint string
	count    int
	verbose  bool
	help     string
}

func (l *resolveLeaf) Flags(fs *flag.FlagSet) {
	fs.StringVar(&l.endpoint, "endpoint", "", "leaf endpoint")
	fs.IntVar(&l.count, "count", 0, "leaf count")
	fs.BoolVar(&l.verbose, "verbose", false, "verbose")
}

func (l *resolveLeaf) Help() string { return l.help }

func (l *resolveLeaf) Run(ctx context.Context, args []string, _ IO) error { return nil }

// rootGroupWith builds a *Group whose only persistent flags are the four
// well-known globals. Most tests use this as the root.
func rootGroupWith(children map[string]Command) *Group {
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

func TestResolve_LeafLevelFlag(t *testing.T) {
	t.Parallel()
	leaf := &resolveLeaf{}
	root := rootGroupWith(map[string]Command{"profile": &Group{Children: map[string]Command{"add": leaf}}})
	res, err := Resolve(root, []string{"profile", "add", "myprof", "--endpoint", "https://x"})
	if err != nil {
		t.Fatalf("err=%v", err)
	}
	if leaf.endpoint != "https://x" {
		t.Errorf("leaf.endpoint=%q want https://x", leaf.endpoint)
	}
	if len(res.Args) != 1 || res.Args[0] != "myprof" {
		t.Errorf("Args=%v want [myprof]", res.Args)
	}
}

func TestResolve_AncestorPersistent(t *testing.T) {
	t.Parallel()
	leaf := &fakeCmd{}
	root := rootGroupWith(map[string]Command{"org": &Group{Children: map[string]Command{"show": leaf}}})
	res, err := Resolve(root, []string{"org", "show", "--json"})
	if err != nil {
		t.Fatalf("err=%v", err)
	}
	if g := globalFromFlagSet(res.MergedFS); !g.JSON {
		t.Errorf("global.JSON should be true: %+v", g)
	}
}

func TestResolve_ShadowedAtLeaf(t *testing.T) {
	t.Parallel()
	// Both root and leaf declare --endpoint. Leaf wins: the value lands in
	// the leaf's struct field, the root closure's binding is left untouched.
	var rootEndpoint string
	leaf := &resolveLeaf{}
	root := &Group{
		Flags: func(fs *flag.FlagSet) {
			fs.StringVar(&rootEndpoint, "endpoint", "", "root endpoint")
		},
		Children: map[string]Command{"profile": &Group{Children: map[string]Command{"add": leaf}}},
	}
	_, err := Resolve(root, []string{"profile", "add", "--endpoint", "leaf-value"})
	if err != nil {
		t.Fatalf("err=%v", err)
	}
	if leaf.endpoint != "leaf-value" {
		t.Errorf("leaf.endpoint=%q want leaf-value", leaf.endpoint)
	}
	if rootEndpoint != "" {
		t.Errorf("rootEndpoint=%q want empty (shadowed by leaf)", rootEndpoint)
	}
}

func TestResolve_GlobalBeforeVerb(t *testing.T) {
	t.Parallel()
	leaf := &fakeCmd{}
	root := rootGroupWith(map[string]Command{"org": &Group{Children: map[string]Command{"list": leaf}}})
	res, err := Resolve(root, []string{"--endpoint", "https://api", "org", "list"})
	if err != nil {
		t.Fatalf("err=%v", err)
	}
	if g := globalFromFlagSet(res.MergedFS); g.Endpoint != "https://api" {
		t.Errorf("global.Endpoint=%q", g.Endpoint)
	}
}

func TestResolve_ValueWithEquals(t *testing.T) {
	t.Parallel()
	leaf := &resolveLeaf{}
	root := rootGroupWith(map[string]Command{"verb": leaf})
	res, err := Resolve(root, []string{"verb", "--endpoint=eq-value"})
	if err != nil {
		t.Fatalf("err=%v", err)
	}
	if leaf.endpoint != "eq-value" {
		t.Errorf("leaf.endpoint=%q", leaf.endpoint)
	}
	if len(res.Args) != 0 {
		t.Errorf("Args=%v want empty", res.Args)
	}
}

func TestResolve_BoolFlag(t *testing.T) {
	t.Parallel()
	leaf := &fakeCmd{}
	root := rootGroupWith(map[string]Command{"verb": leaf})
	// `--json` is bool — must NOT consume `pos` as its value.
	res, err := Resolve(root, []string{"verb", "--json", "pos"})
	if err != nil {
		t.Fatalf("err=%v", err)
	}
	if g := globalFromFlagSet(res.MergedFS); !g.JSON {
		t.Errorf("global.JSON should be true")
	}
	if len(res.Args) != 1 || res.Args[0] != "pos" {
		t.Errorf("Args=%v want [pos]", res.Args)
	}
}

func TestResolve_UnknownFlag(t *testing.T) {
	t.Parallel()
	leaf := &resolveLeaf{}
	root := rootGroupWith(map[string]Command{"verb": leaf})
	_, err := Resolve(root, []string{"verb", "--no-such"})
	if err == nil {
		t.Fatalf("want error")
	}
	if !errors.Is(err, ErrUsage) {
		t.Errorf("err should be ErrUsage: %v", err)
	}
}

func TestResolve_UnknownVerb(t *testing.T) {
	t.Parallel()
	leaf := &fakeCmd{}
	root := rootGroupWith(map[string]Command{"verb": leaf})
	_, err := Resolve(root, []string{"nope"})
	if err == nil {
		t.Fatalf("want error")
	}
	if !errors.Is(err, ErrUsage) {
		t.Errorf("err should be ErrUsage: %v", err)
	}
	if !strings.Contains(err.Error(), "nope") {
		t.Errorf("err should mention offending token: %v", err)
	}
}

func TestResolve_HelpAtLeaf(t *testing.T) {
	t.Parallel()
	leaf := &resolveLeaf{help: "leaf help"}
	root := rootGroupWith(map[string]Command{"verb": leaf})
	res, err := Resolve(root, []string{"verb", "--help"})
	if !errors.Is(err, ErrHelp) {
		t.Fatalf("err=%v want ErrHelp", err)
	}
	if res == nil || res.Leaf != leaf {
		t.Fatalf("Leaf=%v want %v", res, leaf)
	}
}

func TestResolve_DoubleDashTerminator(t *testing.T) {
	t.Parallel()
	leaf := &resolveLeaf{}
	root := rootGroupWith(map[string]Command{"verb": leaf})
	res, err := Resolve(root, []string{"verb", "--", "--looks-like-flag", "x"})
	if err != nil {
		t.Fatalf("err=%v", err)
	}
	want := []string{"--looks-like-flag", "x"}
	if len(res.Args) != len(want) {
		t.Fatalf("Args=%v want %v", res.Args, want)
	}
	for i, w := range want {
		if res.Args[i] != w {
			t.Errorf("Args[%d]=%q want %q", i, res.Args[i], w)
		}
	}
}

func TestResolve_GroupPersistentFlag(t *testing.T) {
	t.Parallel()
	var middleV string
	leaf := &resolveLeaf{}
	middle := &Group{
		Flags: func(fs *flag.FlagSet) {
			fs.StringVar(&middleV, "middle", "", "middle group flag")
		},
		Children: map[string]Command{"leaf": leaf},
	}
	root := rootGroupWith(map[string]Command{"mid": middle})
	_, err := Resolve(root, []string{"mid", "leaf", "--middle", "hello"})
	if err != nil {
		t.Fatalf("err=%v", err)
	}
	if middleV != "hello" {
		t.Errorf("middleV=%q want hello", middleV)
	}
}

func TestResolve_LeafIsGroupNoFlagger(t *testing.T) {
	t.Parallel()
	// User typed `ana profile` (group-only). Resolve returns the group as
	// Leaf; Path stops at the group; MergedFS holds only ancestor flags.
	g := &Group{Children: map[string]Command{"add": &fakeCmd{}}}
	root := rootGroupWith(map[string]Command{"profile": g})
	res, err := Resolve(root, []string{"profile"})
	if err != nil {
		t.Fatalf("err=%v", err)
	}
	if res.Leaf != g {
		t.Errorf("Leaf should be the group, got %v", res.Leaf)
	}
}

func TestResolve_FlagBeforeVerbValue(t *testing.T) {
	t.Parallel()
	// `--profile prod org show` — `prod` is the value of `--profile`, not a
	// verb. Walker must consume both tokens, then descend `org` → `show`.
	leaf := &fakeCmd{}
	root := rootGroupWith(map[string]Command{"org": &Group{Children: map[string]Command{"show": leaf}}})
	res, err := Resolve(root, []string{"--profile", "prod", "org", "show"})
	if err != nil {
		t.Fatalf("err=%v", err)
	}
	if g := globalFromFlagSet(res.MergedFS); g.Profile != "prod" {
		t.Errorf("global.Profile=%q want prod", g.Profile)
	}
}

func TestResolve_NilRoot(t *testing.T) {
	t.Parallel()
	if _, err := Resolve(nil, nil); err == nil || !strings.Contains(err.Error(), "nil root") {
		t.Errorf("err=%v", err)
	}
}

func TestResolved_Execute_RunsLeaf(t *testing.T) {
	t.Parallel()
	leaf := &fakeCmd{}
	root := rootGroupWith(map[string]Command{"verb": leaf})
	res, err := Resolve(root, []string{"verb", "pos"})
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	stdio, _, _ := testIO()
	if err := res.Execute(context.Background(), stdio); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !leaf.ran {
		t.Errorf("leaf did not run")
	}
}

func TestResolved_Execute_GroupRendersHelp(t *testing.T) {
	t.Parallel()
	g := &Group{Children: map[string]Command{"sub": &fakeCmd{help: "sub"}}}
	root := rootGroupWith(map[string]Command{"grp": g})
	res, err := Resolve(root, []string{"grp"})
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	stdio, out, _ := testIO()
	err = res.Execute(context.Background(), stdio)
	if !errors.Is(err, ErrHelp) {
		t.Fatalf("Execute: err=%v want ErrHelp", err)
	}
	if !strings.Contains(out.String(), "Commands:") {
		t.Errorf("expected group help on stdout, got %q", out.String())
	}
}

func TestResolved_Execute_PreservesPriorFlagSet(t *testing.T) {
	t.Parallel()
	leaf := &fakeCmd{}
	root := rootGroupWith(map[string]Command{"verb": leaf})
	res, err := Resolve(root, []string{"verb"})
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	// Prior FlagSet on ctx must NOT be replaced by Execute.
	priorFS := flag.NewFlagSet("prior", flag.ContinueOnError)
	ctx := WithFlagSet(context.Background(), priorFS)
	stdio, _, _ := testIO()
	if err := res.Execute(ctx, stdio); err != nil {
		t.Fatalf("Execute: %v", err)
	}
}

func TestWithFlagSetRoundTrip(t *testing.T) {
	t.Parallel()
	fs := flag.NewFlagSet("t", flag.ContinueOnError)
	ctx := WithFlagSet(context.Background(), fs)
	if got := FlagSetFrom(ctx); got != fs {
		t.Errorf("round-trip mismatch")
	}
	if got := FlagSetFrom(context.Background()); got != nil {
		t.Errorf("absent should be nil, got %v", got)
	}
}
