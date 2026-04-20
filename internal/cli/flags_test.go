package cli

import (
	"errors"
	"flag"
	"io"
	"testing"
)

func newTestFS(t *testing.T) (*flag.FlagSet, *string, *string, *bool) {
	t.Helper()
	fs := flag.NewFlagSet("test", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	name := fs.String("name", "", "")
	endpoint := fs.String("endpoint", "", "")
	dry := fs.Bool("dry", false, "")
	return fs, name, endpoint, dry
}

func TestParseFlags_FlagsBeforePositional(t *testing.T) {
	t.Parallel()
	fs, name, endpoint, _ := newTestFS(t)
	if err := ParseFlags(fs, []string{"--name", "n", "--endpoint", "u", "pos"}); err != nil {
		t.Fatalf("err=%v", err)
	}
	if *name != "n" || *endpoint != "u" {
		t.Errorf("flags=%q %q", *name, *endpoint)
	}
	if fs.NArg() != 1 || fs.Arg(0) != "pos" {
		t.Errorf("args=%v", fs.Args())
	}
}

func TestParseFlags_PositionalBeforeFlags(t *testing.T) {
	t.Parallel()
	// The regression case: bare fs.Parse stops at "pos" and drops --name/--endpoint.
	fs, name, endpoint, _ := newTestFS(t)
	if err := ParseFlags(fs, []string{"pos", "--name", "n", "--endpoint", "u"}); err != nil {
		t.Fatalf("err=%v", err)
	}
	if *name != "n" || *endpoint != "u" {
		t.Errorf("flags lost: name=%q endpoint=%q", *name, *endpoint)
	}
	if fs.NArg() != 1 || fs.Arg(0) != "pos" {
		t.Errorf("args=%v", fs.Args())
	}
}

func TestParseFlags_InterleavedWithMultiplePositionals(t *testing.T) {
	t.Parallel()
	fs, name, endpoint, dry := newTestFS(t)
	if err := ParseFlags(fs, []string{"--name", "n", "a", "--endpoint", "u", "b", "--dry"}); err != nil {
		t.Fatalf("err=%v", err)
	}
	if *name != "n" || *endpoint != "u" || !*dry {
		t.Errorf("flags=%q %q %v", *name, *endpoint, *dry)
	}
	if got := fs.Args(); len(got) != 2 || got[0] != "a" || got[1] != "b" {
		t.Errorf("positionals=%v", got)
	}
}

func TestParseFlags_NoArgs(t *testing.T) {
	t.Parallel()
	fs, _, _, _ := newTestFS(t)
	if err := ParseFlags(fs, nil); err != nil {
		t.Fatalf("err=%v", err)
	}
	if fs.NArg() != 0 {
		t.Errorf("expected zero positionals")
	}
}

func TestParseFlags_UnknownFlagWrapsErrUsage(t *testing.T) {
	t.Parallel()
	fs, _, _, _ := newTestFS(t)
	err := ParseFlags(fs, []string{"--nope"})
	if !errors.Is(err, ErrUsage) {
		t.Errorf("err=%v, want wrapping ErrUsage", err)
	}
}

func TestParseFlags_UnknownFlagAfterPositionalWrapsErrUsage(t *testing.T) {
	t.Parallel()
	fs, _, _, _ := newTestFS(t)
	err := ParseFlags(fs, []string{"pos", "--nope"})
	if !errors.Is(err, ErrUsage) {
		t.Errorf("err=%v, want wrapping ErrUsage", err)
	}
}

func TestParseFlags_DoubleDashTerminator(t *testing.T) {
	t.Parallel()
	// Arguments after -- must be preserved as positionals; the wrapper's own
	// re-seed must not clobber them or double-process them.
	fs, name, _, _ := newTestFS(t)
	if err := ParseFlags(fs, []string{"--name", "n", "--", "--not-a-flag", "raw"}); err != nil {
		t.Fatalf("err=%v", err)
	}
	if *name != "n" {
		t.Errorf("name=%q", *name)
	}
	got := fs.Args()
	if len(got) != 2 || got[0] != "--not-a-flag" || got[1] != "raw" {
		t.Errorf("args=%v", got)
	}
}

func TestFlagWasSet(t *testing.T) {
	t.Parallel()
	fs := NewFlagSet("t")
	_ = fs.String("a", "default-a", "")
	_ = fs.String("b", "", "")
	if err := fs.Parse([]string{"--a", "x"}); err != nil {
		t.Fatalf("parse: %v", err)
	}
	if !FlagWasSet(fs, "a") {
		t.Errorf("a should be reported as set")
	}
	if FlagWasSet(fs, "b") {
		t.Errorf("b has its default value and should not be reported as set")
	}
	// Unknown flag names must not panic — fs.Visit only iterates set flags,
	// so a typo in the caller silently returns false.
	if FlagWasSet(fs, "nope") {
		t.Errorf("unknown flag should not be reported as set")
	}
}

func TestFlagWasSet_ZeroValueStillReportsTrue(t *testing.T) {
	t.Parallel()
	// A user can pass the zero value explicitly (--port=0); FlagWasSet must
	// distinguish that from "flag not supplied" so partial-update verbs can
	// overlay the zero value when requested.
	fs := NewFlagSet("t")
	_ = fs.Int("port", 5432, "")
	if err := fs.Parse([]string{"--port", "0"}); err != nil {
		t.Fatalf("parse: %v", err)
	}
	if !FlagWasSet(fs, "port") {
		t.Errorf("--port=0 should be reported as set")
	}
}
