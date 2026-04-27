package cli

import (
	"errors"
	"flag"
	"io"
	"testing"
	"time"
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

func TestRequireFlags_AllSet(t *testing.T) {
	t.Parallel()
	fs := NewFlagSet("t")
	_ = fs.String("a", "", "")
	_ = fs.String("b", "", "")
	if err := fs.Parse([]string{"--a", "x", "--b", "y"}); err != nil {
		t.Fatalf("parse: %v", err)
	}
	if err := RequireFlags(fs, "verb", "a", "b"); err != nil {
		t.Errorf("err=%v", err)
	}
}

func TestEnumFlag_Accept(t *testing.T) {
	t.Parallel()
	fs := NewFlagSet("t")
	var typ string
	fs.Var(EnumFlag(&typ, []string{"postgres", "mysql"}), "type", "")
	if err := fs.Parse([]string{"--type", "postgres"}); err != nil {
		t.Fatalf("parse: %v", err)
	}
	if typ != "postgres" {
		t.Errorf("typ=%q", typ)
	}
}

func TestEnumFlag_RejectUnknown(t *testing.T) {
	t.Parallel()
	// stdlib flag.Parse re-wraps the Set error with %v (not %w), so the
	// ErrUsage chain survives only through ParseFlags' own ErrUsage wrap.
	// Tests here exercise the production path (ParseFlags), not fs.Parse.
	fs := NewFlagSet("t")
	var typ string
	fs.Var(EnumFlag(&typ, []string{"postgres"}), "type", "")
	err := ParseFlags(fs, []string{"--type", "sqlite"})
	if err == nil {
		t.Fatalf("expected error")
	}
	if !errors.Is(err, ErrUsage) {
		t.Errorf("err=%v, want wrapping ErrUsage", err)
	}
}

func TestEnumFlag_StringRendersCurrentValue(t *testing.T) {
	t.Parallel()
	var typ string
	v := EnumFlag(&typ, []string{"postgres"})
	if got := v.String(); got != "" {
		t.Errorf("zero String=%q", got)
	}
	typ = "postgres"
	if got := v.String(); got != "postgres" {
		t.Errorf("set String=%q", got)
	}
	nilV := EnumFlag(nil, []string{"postgres"})
	if got := nilV.String(); got != "" {
		t.Errorf("nil target String=%q", got)
	}
}

func TestIntListFlag_ParsesAndTrims(t *testing.T) {
	t.Parallel()
	fs := NewFlagSet("t")
	var ids []int
	fs.Var(IntListFlag(&ids, ","), "connector", "")
	if err := fs.Parse([]string{"--connector", " 1, 2 ,3"}); err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(ids) != 3 || ids[0] != 1 || ids[1] != 2 || ids[2] != 3 {
		t.Errorf("ids=%v", ids)
	}
}

func TestIntListFlag_RejectEmpty(t *testing.T) {
	t.Parallel()
	fs := NewFlagSet("t")
	var ids []int
	fs.Var(IntListFlag(&ids, ","), "c", "")
	err := ParseFlags(fs, []string{"--c", ""})
	if err == nil || !errors.Is(err, ErrUsage) {
		t.Errorf("err=%v", err)
	}
}

func TestIntListFlag_RejectEmptyEntry(t *testing.T) {
	t.Parallel()
	fs := NewFlagSet("t")
	var ids []int
	fs.Var(IntListFlag(&ids, ","), "c", "")
	err := ParseFlags(fs, []string{"--c", "1,,2"})
	if err == nil || !errors.Is(err, ErrUsage) {
		t.Errorf("err=%v", err)
	}
}

func TestIntListFlag_RejectNonInt(t *testing.T) {
	t.Parallel()
	fs := NewFlagSet("t")
	var ids []int
	fs.Var(IntListFlag(&ids, ","), "c", "")
	err := ParseFlags(fs, []string{"--c", "1,x,3"})
	if err == nil || !errors.Is(err, ErrUsage) {
		t.Errorf("err=%v", err)
	}
}

func TestIntListFlag_String(t *testing.T) {
	t.Parallel()
	var ids []int
	v := IntListFlag(&ids, ",")
	if got := v.String(); got != "" {
		t.Errorf("empty String=%q", got)
	}
	ids = []int{1, 2, 3}
	if got := v.String(); got != "1,2,3" {
		t.Errorf("String=%q", got)
	}
	nilV := IntListFlag(nil, ",")
	if got := nilV.String(); got != "" {
		t.Errorf("nil target String=%q", got)
	}
}

func TestSinceFlag_Duration(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 4, 20, 12, 0, 0, 0, time.UTC)
	fs := NewFlagSet("t")
	var got time.Time
	fs.Var(SinceFlag(&got, func() time.Time { return now }), "since", "")
	if err := fs.Parse([]string{"--since", "1h"}); err != nil {
		t.Fatalf("parse: %v", err)
	}
	want := now.Add(-time.Hour).UTC()
	if !got.Equal(want) {
		t.Errorf("got=%v want=%v", got, want)
	}
}

func TestSinceFlag_RFC3339(t *testing.T) {
	t.Parallel()
	fs := NewFlagSet("t")
	var got time.Time
	fs.Var(SinceFlag(&got, time.Now), "since", "")
	if err := fs.Parse([]string{"--since", "2026-04-18T00:00:00Z"}); err != nil {
		t.Fatalf("parse: %v", err)
	}
	want, _ := time.Parse(time.RFC3339, "2026-04-18T00:00:00Z")
	if !got.Equal(want.UTC()) {
		t.Errorf("got=%v want=%v", got, want)
	}
}

func TestSinceFlag_NegativeDuration(t *testing.T) {
	t.Parallel()
	fs := NewFlagSet("t")
	var got time.Time
	fs.Var(SinceFlag(&got, time.Now), "since", "")
	err := ParseFlags(fs, []string{"--since", "-1h"})
	if err == nil || !errors.Is(err, ErrUsage) {
		t.Errorf("err=%v", err)
	}
}

func TestSinceFlag_Invalid(t *testing.T) {
	t.Parallel()
	fs := NewFlagSet("t")
	var got time.Time
	fs.Var(SinceFlag(&got, time.Now), "since", "")
	err := ParseFlags(fs, []string{"--since", "banana"})
	if err == nil || !errors.Is(err, ErrUsage) {
		t.Errorf("err=%v", err)
	}
}

func TestSinceFlag_String(t *testing.T) {
	t.Parallel()
	var got time.Time
	v := SinceFlag(&got, time.Now)
	if s := v.String(); s != "" {
		t.Errorf("zero String=%q", s)
	}
	got = time.Date(2026, 4, 20, 12, 0, 0, 0, time.UTC)
	if s := v.String(); s != "2026-04-20T12:00:00Z" {
		t.Errorf("set String=%q", s)
	}
	nilV := SinceFlag(nil, time.Now)
	if s := nilV.String(); s != "" {
		t.Errorf("nil target String=%q", s)
	}
}

func TestRequireFlags_MissingReportsSorted(t *testing.T) {
	t.Parallel()
	fs := NewFlagSet("t")
	_ = fs.String("a", "", "")
	_ = fs.String("b", "", "")
	_ = fs.String("c", "", "")
	if err := fs.Parse([]string{"--b", "set"}); err != nil {
		t.Fatalf("parse: %v", err)
	}
	err := RequireFlags(fs, "verb", "c", "a")
	if !errors.Is(err, ErrUsage) {
		t.Fatalf("err=%v want ErrUsage", err)
	}
	if got := err.Error(); got != "verb: missing required flags: --a, --c: "+ErrUsage.Error() {
		t.Errorf("err=%q", got)
	}
}

func TestRequireNoPositionals_Empty(t *testing.T) {
	t.Parallel()
	if err := RequireNoPositionals("verb", nil); err != nil {
		t.Errorf("err=%v want nil", err)
	}
	if err := RequireNoPositionals("verb", []string{}); err != nil {
		t.Errorf("err=%v want nil", err)
	}
}

func TestRequireNoPositionals_NonEmpty(t *testing.T) {
	t.Parallel()
	err := RequireNoPositionals("verb", []string{"x", "y"})
	if !errors.Is(err, ErrUsage) {
		t.Fatalf("err=%v want ErrUsage", err)
	}
	if got := err.Error(); got != "verb: unexpected positional arguments: [x y]: "+ErrUsage.Error() {
		t.Errorf("err=%q", got)
	}
}

func TestRequireMaxPositionals_WithinLimit(t *testing.T) {
	t.Parallel()
	if err := RequireMaxPositionals("verb", 1, []string{"only"}); err != nil {
		t.Errorf("err=%v want nil", err)
	}
	if err := RequireMaxPositionals("verb", 2, nil); err != nil {
		t.Errorf("err=%v want nil", err)
	}
}

func TestRequireMaxPositionals_OverLimit(t *testing.T) {
	t.Parallel()
	err := RequireMaxPositionals("verb", 1, []string{"id", "extra1", "extra2"})
	if !errors.Is(err, ErrUsage) {
		t.Fatalf("err=%v want ErrUsage", err)
	}
	if got := err.Error(); got != "verb: unexpected positional arguments: [extra1 extra2]: "+ErrUsage.Error() {
		t.Errorf("err=%q", got)
	}
}
