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
	fs, _, _, _ := newTestFS(t)
	if err := ParseFlags(fs, nil); err != nil {
		t.Fatalf("err=%v", err)
	}
	if fs.NArg() != 0 {
		t.Errorf("expected zero positionals")
	}
}

func TestParseFlags_UnknownFlagWrapsErrUsage(t *testing.T) {
	fs, _, _, _ := newTestFS(t)
	err := ParseFlags(fs, []string{"--nope"})
	if !errors.Is(err, ErrUsage) {
		t.Errorf("err=%v, want wrapping ErrUsage", err)
	}
}

func TestParseFlags_UnknownFlagAfterPositionalWrapsErrUsage(t *testing.T) {
	fs, _, _, _ := newTestFS(t)
	err := ParseFlags(fs, []string{"pos", "--nope"})
	if !errors.Is(err, ErrUsage) {
		t.Errorf("err=%v, want wrapping ErrUsage", err)
	}
}

func TestParseFlags_DoubleDashTerminator(t *testing.T) {
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
