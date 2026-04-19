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
	// channel cannot be marshalled.
	err := Remarshal(make(chan int), &struct{}{})
	if err == nil {
		t.Fatal("expected marshal error")
	}
}

func TestRemarshalUnmarshalErrorPropagates(t *testing.T) {
	// destination not a pointer triggers an unmarshal error.
	var dst int
	err := Remarshal(map[string]any{"x": 1}, dst)
	if err == nil {
		t.Fatal("expected unmarshal error")
	}
}

func TestRequireStringID(t *testing.T) {
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
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
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
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
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

func TestRenderTwoColScalarsThenNested(t *testing.T) {
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
	err := RenderTwoCol(failingWriter{}, map[string]any{"a": 1})
	if err == nil {
		t.Fatal("expected flush error")
	}
}
