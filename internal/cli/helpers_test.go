package cli

import (
	"bytes"
	"errors"
	"flag"
	"io"
	"strings"
	"testing"
)

func TestNewFlagSet(t *testing.T) {
	t.Parallel()
	fs := NewFlagSet("verb")
	if fs.Name() != "verb" {
		t.Errorf("Name=%q want verb", fs.Name())
	}
	if fs.ErrorHandling() != flag.ContinueOnError {
		t.Errorf("ErrorHandling=%v want ContinueOnError", fs.ErrorHandling())
	}
	if fs.Output() != io.Discard {
		t.Errorf("Output should be io.Discard")
	}
	// A bad flag must NOT print to stderr (output is silenced); the error
	// is returned to the caller.
	err := fs.Parse([]string{"--nope"})
	if err == nil {
		t.Errorf("expected parse error on unknown flag")
	}
}

func TestUsageErrfWrapsErrUsage(t *testing.T) {
	t.Parallel()
	err := UsageErrf("verb: %s required", "<id>")
	if err == nil {
		t.Fatal("nil error")
	}
	if !errors.Is(err, ErrUsage) {
		t.Errorf("want errors.Is(err, ErrUsage), got %v", err)
	}
	if got, want := err.Error(), "verb: <id> required: usage"; got != want {
		t.Errorf("err.Error()=%q want %q", got, want)
	}
}

func TestWriteJSONIndentedAndTrailingNewline(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	if err := WriteJSON(&buf, map[string]any{"a": 1, "b": "two"}); err != nil {
		t.Fatalf("err=%v", err)
	}
	got := buf.String()
	want := "{\n  \"a\": 1,\n  \"b\": \"two\"\n}\n"
	if got != want {
		t.Errorf("WriteJSON=%q\nwant %q", got, want)
	}
}

func TestWriteJSONUnencodableValueWraps(t *testing.T) {
	t.Parallel()
	// channels are not JSON-encodable.
	err := WriteJSON(io.Discard, make(chan int))
	if err == nil {
		t.Fatal("expected error encoding channel")
	}
	if !strings.Contains(err.Error(), "encode response") {
		t.Errorf("want wrapped 'encode response', got %v", err)
	}
}

func TestRemarshalRoundTripsMapToStruct(t *testing.T) {
	t.Parallel()
	src := map[string]any{"name": "alpha", "count": 3}
	type out struct {
		Name  string `json:"name"`
		Count int    `json:"count"`
	}
	var dst out
	if err := Remarshal(src, &dst); err != nil {
		t.Fatalf("err=%v", err)
	}
	if dst.Name != "alpha" || dst.Count != 3 {
		t.Errorf("dst=%+v", dst)
	}
}

func TestRemarshalMarshalErrorPropagates(t *testing.T) {
	t.Parallel()
	// channel cannot be marshalled.
	err := Remarshal(make(chan int), &struct{}{})
	if err == nil {
		t.Fatal("expected marshal error")
	}
}

func TestRemarshalUnmarshalErrorPropagates(t *testing.T) {
	t.Parallel()
	// destination not a pointer triggers an unmarshal error.
	var dst int
	err := Remarshal(map[string]any{"x": 1}, dst)
	if err == nil {
		t.Fatal("expected unmarshal error")
	}
}

func TestRequireStringID(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name    string
		args    []string
		wantErr bool
		wantID  string
	}{
		{"happy", []string{"abc"}, false, "abc"},
		{"extra-args-ok", []string{"abc", "def"}, false, "abc"},
		{"missing", nil, true, ""},
		{"empty-string", []string{""}, true, ""},
		{"whitespace-only", []string{"   "}, true, ""},
		{"tab-only", []string{"\t"}, true, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			id, err := RequireStringID("verb", tc.args)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("want error, got id=%q", id)
				}
				if !errors.Is(err, ErrUsage) {
					t.Errorf("err should wrap ErrUsage: %v", err)
				}
				if !strings.Contains(err.Error(), "verb: <id> positional argument required") {
					t.Errorf("err msg=%q", err.Error())
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected err: %v", err)
			}
			if id != tc.wantID {
				t.Errorf("id=%q want %q", id, tc.wantID)
			}
		})
	}
}

func TestRequireIntID(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name       string
		args       []string
		wantErr    bool
		wantID     int
		wantMsgSub string
	}{
		{"happy", []string{"42"}, false, 42, ""},
		{"happy-negative", []string{"-7"}, false, -7, ""},
		{"extra-args-ok", []string{"99", "more"}, false, 99, ""},
		{"missing", nil, true, 0, "verb: <id> positional argument required"},
		{"empty-string", []string{""}, true, 0, "verb: <id> positional argument required"},
		{"whitespace-only", []string{"   "}, true, 0, "verb: <id> positional argument required"},
		{"tab-only", []string{"\t"}, true, 0, "verb: <id> positional argument required"},
		{"non-numeric", []string{"abc"}, true, 0, "verb: <id> must be an integer:"},
		{"trailing-junk", []string{"12x"}, true, 0, "verb: <id> must be an integer:"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			id, err := RequireIntID("verb", tc.args)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("want error, got id=%d", id)
				}
				if !errors.Is(err, ErrUsage) {
					t.Errorf("err should wrap ErrUsage: %v", err)
				}
				if !strings.Contains(err.Error(), tc.wantMsgSub) {
					t.Errorf("err msg=%q want substring %q", err.Error(), tc.wantMsgSub)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected err: %v", err)
			}
			if id != tc.wantID {
				t.Errorf("id=%d want %d", id, tc.wantID)
			}
		})
	}
}

func TestReadTokenNilReader(t *testing.T) {
	t.Parallel()
	if _, err := ReadToken(nil, false); err == nil {
		t.Errorf("want error on nil reader")
	}
}

func TestReadTokenLineMode(t *testing.T) {
	t.Parallel()
	tok, err := ReadToken(strings.NewReader("  my-token  \n"), false)
	if err != nil {
		t.Fatalf("err=%v", err)
	}
	if tok != "my-token" {
		t.Errorf("tok=%q", tok)
	}
}

func TestReadTokenStdinMode(t *testing.T) {
	t.Parallel()
	tok, err := ReadToken(strings.NewReader("line1\nline2\n  \n"), true)
	if err != nil {
		t.Fatalf("err=%v", err)
	}
	if tok != "line1\nline2" {
		t.Errorf("tok=%q", tok)
	}
}

func TestReadTokenEmptyStream(t *testing.T) {
	t.Parallel()
	tok, err := ReadToken(strings.NewReader(""), false)
	if err != nil {
		t.Fatalf("err=%v", err)
	}
	if tok != "" {
		t.Errorf("tok=%q", tok)
	}
}

// errReader returns err on every Read so ReadToken exercises its error paths.
type errReader struct{ err error }

func (e errReader) Read([]byte) (int, error) { return 0, e.err }

func TestReadTokenStdinReadError(t *testing.T) {
	t.Parallel()
	_, err := ReadToken(errReader{err: errors.New("boom")}, true)
	if err == nil || !strings.Contains(err.Error(), "boom") {
		t.Errorf("err=%v", err)
	}
}

func TestReadTokenLineReadError(t *testing.T) {
	t.Parallel()
	_, err := ReadToken(errReader{err: errors.New("boom")}, false)
	if err == nil || !strings.Contains(err.Error(), "boom") {
		t.Errorf("err=%v", err)
	}
}

func TestReadPasswordNilReader(t *testing.T) {
	t.Parallel()
	if _, err := ReadPassword(nil); err == nil {
		t.Errorf("want error on nil reader")
	}
}

// TestReadPasswordPreservesSurroundingWhitespace confirms the opposite of
// ReadToken's contract: a password may legitimately start/end with spaces or
// tabs, and those bytes must survive intact.
func TestReadPasswordPreservesSurroundingWhitespace(t *testing.T) {
	t.Parallel()
	got, err := ReadPassword(strings.NewReader(" secret\twith\tabs \n"))
	if err != nil {
		t.Fatalf("err=%v", err)
	}
	if want := " secret\twith\tabs "; got != want {
		t.Errorf("ReadPassword=%q want %q", got, want)
	}
}

// TestReadPasswordStripsLF strips the trailing newline but nothing else.
func TestReadPasswordStripsLF(t *testing.T) {
	t.Parallel()
	got, err := ReadPassword(strings.NewReader("hunter2\n"))
	if err != nil {
		t.Fatalf("err=%v", err)
	}
	if got != "hunter2" {
		t.Errorf("ReadPassword=%q want hunter2", got)
	}
}

// TestReadPasswordStripsCRLF strips a Windows-style line terminator cleanly.
func TestReadPasswordStripsCRLF(t *testing.T) {
	t.Parallel()
	got, err := ReadPassword(strings.NewReader("hunter2\r\n"))
	if err != nil {
		t.Fatalf("err=%v", err)
	}
	if got != "hunter2" {
		t.Errorf("ReadPassword=%q want hunter2", got)
	}
}

func TestReadPasswordEmptyStream(t *testing.T) {
	t.Parallel()
	got, err := ReadPassword(strings.NewReader(""))
	if err != nil {
		t.Fatalf("err=%v", err)
	}
	if got != "" {
		t.Errorf("ReadPassword=%q want empty", got)
	}
}

func TestReadPasswordReadError(t *testing.T) {
	t.Parallel()
	_, err := ReadPassword(errReader{err: errors.New("boom")})
	if err == nil || !strings.Contains(err.Error(), "boom") {
		t.Errorf("err=%v", err)
	}
}

func TestNewTableWriterFormats(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	tw := NewTableWriter(&buf)
	if _, err := tw.Write([]byte("A\tB\nalpha\tbeta\n")); err != nil {
		t.Fatalf("write: %v", err)
	}
	if err := tw.Flush(); err != nil {
		t.Fatalf("flush: %v", err)
	}
	// Two-space gutter between column 1 and column 2.
	want := "A      B\nalpha  beta\n"
	if buf.String() != want {
		t.Errorf("got=%q want %q", buf.String(), want)
	}
}

func TestRenderTwoColScalarsThenNested(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	m := map[string]any{
		"name":    "prod-pg",
		"id":      7,
		"dialect": "postgres",
		"postgresMetadata": map[string]any{
			"host": "db.internal",
			"port": 5432,
		},
		"emptyMeta": map[string]any{},
	}
	if err := RenderTwoCol(&buf, m); err != nil {
		t.Fatalf("err=%v", err)
	}
	got := buf.String()
	// Scalars come first, sorted; nested maps follow, sorted. Tabwriter
	// aligns the second column across every row in the writer (including
	// the indented nested-block rows), so the gutter is the maximum of all
	// left-column widths.
	want := "" +
		"dialect:           postgres\n" +
		"id:                7\n" +
		"name:              prod-pg\n" +
		"emptyMeta:         \n" +
		"postgresMetadata:  \n" +
		"  host:            db.internal\n" +
		"  port:            5432\n"
	if got != want {
		t.Errorf("RenderTwoCol mismatch.\n got:\n%s\nwant:\n%s", got, want)
	}
}

func TestRenderTwoColEmpty(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	if err := RenderTwoCol(&buf, map[string]any{}); err != nil {
		t.Fatalf("err=%v", err)
	}
	if buf.Len() != 0 {
		t.Errorf("empty map should produce no output, got %q", buf.String())
	}
}

// failingWriter forces tabwriter.Flush to surface a write error so the
// returned-error branch of RenderTwoCol is exercised.
type failingWriter struct{}

func (failingWriter) Write(p []byte) (int, error) { return 0, errors.New("boom") }

func TestRenderTwoColFlushError(t *testing.T) {
	t.Parallel()
	err := RenderTwoCol(failingWriter{}, map[string]any{"a": 1})
	if err == nil {
		t.Fatal("expected flush error")
	}
}

func TestFirstLine(t *testing.T) {
	t.Parallel()
	if got := FirstLine("one\ntwo"); got != "one" {
		t.Errorf("got %q", got)
	}
	if got := FirstLine("only"); got != "only" {
		t.Errorf("got %q", got)
	}
	if got := FirstLine(""); got != "" {
		t.Errorf("got %q", got)
	}
}

func TestDashIfEmpty(t *testing.T) {
	t.Parallel()
	if got := DashIfEmpty(""); got != "-" {
		t.Errorf("empty -> %q want -", got)
	}
	if got := DashIfEmpty("x"); got != "x" {
		t.Errorf("x -> %q want x", got)
	}
}

func TestFirstNonEmpty(t *testing.T) {
	t.Parallel()
	if got := FirstNonEmpty("", "", "c"); got != "c" {
		t.Errorf("got %q want c", got)
	}
	if got := FirstNonEmpty("a", "b"); got != "a" {
		t.Errorf("got %q want a", got)
	}
	if got := FirstNonEmpty("", ""); got != "" {
		t.Errorf("got %q want empty", got)
	}
	if got := FirstNonEmpty(); got != "" {
		t.Errorf("got %q want empty", got)
	}
}

func TestRedactToken(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name  string
		token string
		want  string
	}{
		{"empty", "", "(unset)"},
		// Short tokens (< 4 bytes) must also get the fully-masked form;
		// emitting the raw value would leak the secret for short, malformed,
		// or test tokens — defeating the redaction entirely.
		{"short-one", "a", "********** (last 4: ****)"},
		{"short-two", "ab", "********** (last 4: ****)"},
		{"short-three", "abc", "********** (last 4: ****)"},
		{"boundary-four", "abcd", "********** (last 4: abcd)"},
		{"typical", "abcdef1234", "********** (last 4: 1234)"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := RedactToken(tc.token)
			if got != tc.want {
				t.Errorf("RedactToken(%q)=%q want %q", tc.token, got, tc.want)
			}
			// Invariant: a short, non-empty input must never leak into the
			// "last 4: ..." slot — emitting the raw value would defeat the
			// whole point of redaction for short/malformed/test tokens.
			// The fixed prefix `last 4: ` happens to contain letters that
			// can legitimately overlap with the test token, so restrict
			// the leakage check to the bytes following that prefix.
			if tc.token != "" && len(tc.token) < 4 {
				const marker = "last 4: "
				if i := strings.Index(got, marker); i >= 0 {
					tail := got[i+len(marker):]
					if strings.Contains(tail, tc.token) {
						t.Errorf("RedactToken(%q)=%q leaked raw token into `last 4` slot", tc.token, got)
					}
				}
			}
		})
	}
}
