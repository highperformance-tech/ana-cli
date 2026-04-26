package chat

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/highperformance-tech/ana-cli/internal/cli"
	"github.com/highperformance-tech/ana-cli/internal/testcli"
)

// sendFake builds a fakeDeps wired to return cellId=X from SendMessage and
// forward Stream calls to the caller-supplied fakeStream.
func sendFake(cellID string, stream *fakeStream) (*fakeDeps, StreamSession) {
	f := &fakeDeps{
		uuidFn: func() string { return cellID },
		unaryFn: func(_ context.Context, _ string, _, resp any) error {
			out := resp.(*sendMessageResp)
			out.CellID = cellID
			return nil
		},
		streamFn: func(_ context.Context, _ string, _ any) (StreamSession, error) {
			return stream, nil
		},
	}
	return f, stream
}

func TestSendHappyPath(t *testing.T) {
	t.Parallel()
	// Target cell X progresses through three lifecycles and terminates the
	// loop at EXECUTED. The wait-all flag is OFF so only X's executed event
	// matters — intermediate frames from other cells would not block us.
	stream := &fakeStream{frames: []map[string]any{
		{"id": "X", "lifecycle": "LIFECYCLE_CREATED", "mdCell": map[string]any{"content": "hello"}},
		{"id": "X", "lifecycle": "LIFECYCLE_EXECUTING", "mdCell": map[string]any{"content": "more"}},
		{"id": "X", "lifecycle": "LIFECYCLE_EXECUTED", "mdCell": map[string]any{"content": "final"}},
		// Extra frame past EXECUTED should never be consumed.
		{"id": "Y", "lifecycle": "LIFECYCLE_CREATED", "mdCell": map[string]any{"content": "tail"}},
	}}
	f, _ := sendFake("X", stream)
	cmd := &sendCmd{deps: f.deps()}
	stdio, out, _ := testcli.NewIO(nil)
	if err := cmd.Run(context.Background(), []string{"chat-id", "hello?"}, stdio); err != nil {
		t.Fatalf("err=%v", err)
	}
	if !stream.closed {
		t.Errorf("stream should be closed")
	}
	if stream.i != 3 {
		t.Errorf("consumed frames=%d, want 3", stream.i)
	}
	if !strings.Contains(out.String(), "LIFECYCLE_EXECUTED") {
		t.Errorf("stdout=%q", out.String())
	}
	if !strings.Contains(string(f.lastRaw), `"messageId":"X"`) {
		t.Errorf("messageId not wired through: %s", f.lastRaw)
	}
	if f.streamPth != chatServicePath+"/StreamChat" {
		t.Errorf("stream path=%s", f.streamPth)
	}
	if !strings.Contains(string(f.streamRaw), `"chatId":"chat-id"`) {
		t.Errorf("stream body=%s", f.streamRaw)
	}
}

func TestSendWaitAll(t *testing.T) {
	t.Parallel()
	// Two cells; must not exit until BOTH have EXECUTED. Interleave so the
	// target cell reaches EXECUTED before Y does — with --wait-all we keep
	// reading until Y also EXECUTEs.
	stream := &fakeStream{frames: []map[string]any{
		{"id": "X", "lifecycle": "LIFECYCLE_CREATED", "pyCell": map[string]any{"code": "code"}},
		{"id": "Y", "lifecycle": "LIFECYCLE_CREATED", "statusCell": map[string]any{"status": "s"}},
		{"id": "X", "lifecycle": "LIFECYCLE_EXECUTED", "pyCell": map[string]any{"code": "code"}},
		{"id": "Y", "lifecycle": "LIFECYCLE_EXECUTED", "statusCell": map[string]any{"status": "s"}},
	}}
	f, _ := sendFake("X", stream)
	stdio, _, _ := testcli.NewIO(nil)
	if err := New(f.deps()).Run(context.Background(), []string{"send", "c", "hi", "--wait-all"}, stdio); err != nil {
		t.Fatalf("err=%v", err)
	}
	if stream.i != 4 {
		t.Errorf("wait-all should drain all 4 frames, got %d", stream.i)
	}
}

func TestSendNaturalEndOfStream(t *testing.T) {
	t.Parallel()
	// EXECUTED never arrives; stream runs out cleanly → nil return.
	stream := &fakeStream{frames: []map[string]any{
		{"id": "X", "lifecycle": "LIFECYCLE_CREATED", "mdCell": map[string]any{"content": "a"}},
		{"id": "X", "lifecycle": "LIFECYCLE_EXECUTING", "mdCell": map[string]any{"content": "b"}},
	}}
	f, _ := sendFake("X", stream)
	cmd := &sendCmd{deps: f.deps()}
	stdio, _, _ := testcli.NewIO(nil)
	if err := cmd.Run(context.Background(), []string{"c", "hi"}, stdio); err != nil {
		t.Errorf("err=%v", err)
	}
	if !stream.closed {
		t.Errorf("stream not closed on natural end")
	}
}

func TestSendStreamError(t *testing.T) {
	t.Parallel()
	stream := &fakeStream{err: errors.New("mid-way boom")}
	f, _ := sendFake("X", stream)
	cmd := &sendCmd{deps: f.deps()}
	stdio, _, _ := testcli.NewIO(nil)
	err := cmd.Run(context.Background(), []string{"c", "hi"}, stdio)
	if err == nil || !strings.Contains(err.Error(), "mid-way boom") {
		t.Errorf("err=%v", err)
	}
	if !stream.closed {
		t.Errorf("stream not closed on error (defer missed)")
	}
}

func TestSendMessageError(t *testing.T) {
	t.Parallel()
	// SendMessage fails → no Stream call at all.
	f := &fakeDeps{
		uuidFn:  func() string { return "X" },
		unaryFn: func(_ context.Context, _ string, _, _ any) error { return errors.New("send-boom") },
		streamFn: func(_ context.Context, _ string, _ any) (StreamSession, error) {
			return nil, errors.New("should not be called")
		},
	}
	cmd := &sendCmd{deps: f.deps()}
	stdio, _, _ := testcli.NewIO(nil)
	err := cmd.Run(context.Background(), []string{"c", "hi"}, stdio)
	if err == nil || !strings.Contains(err.Error(), "send-boom") {
		t.Errorf("err=%v", err)
	}
	if f.streamPth != "" {
		t.Errorf("stream should not have been opened")
	}
}

func TestSendStreamOpenError(t *testing.T) {
	t.Parallel()
	// SendMessage fine, but opening the stream itself fails.
	f := &fakeDeps{
		uuidFn:  func() string { return "X" },
		unaryFn: func(_ context.Context, _ string, _, _ any) error { return nil },
		streamFn: func(_ context.Context, _ string, _ any) (StreamSession, error) {
			return nil, errors.New("open-boom")
		},
	}
	cmd := &sendCmd{deps: f.deps()}
	stdio, _, _ := testcli.NewIO(nil)
	err := cmd.Run(context.Background(), []string{"c", "hi"}, stdio)
	if err == nil || !strings.Contains(err.Error(), "open-boom") {
		t.Errorf("err=%v", err)
	}
}

// Regression: positionals BEFORE trailing flag must not drop the flag. The
// stdlib fs.Parse stops at the first non-flag token, so the naive
// implementation would silently ignore --wait-all here. 100% coverage on the
// central cli.ParseFlags helper didn't catch the prod bug because the verb
// wrapper was bypassing it — this explicit test locks in the verb-level path.
func TestSendRegressionPositionalBeforeFlags(t *testing.T) {
	t.Parallel()
	// --wait-all appears AFTER both positionals; we must read past X's
	// EXECUTED until Y also EXECUTEs. If the flag were dropped the loop would
	// exit after frame 3 (X EXECUTED) and we'd consume fewer frames.
	stream := &fakeStream{frames: []map[string]any{
		{"id": "X", "lifecycle": "LIFECYCLE_CREATED", "mdCell": map[string]any{"content": "a"}},
		{"id": "Y", "lifecycle": "LIFECYCLE_CREATED", "mdCell": map[string]any{"content": "b"}},
		{"id": "X", "lifecycle": "LIFECYCLE_EXECUTED", "mdCell": map[string]any{"content": "c"}},
		{"id": "Y", "lifecycle": "LIFECYCLE_EXECUTED", "mdCell": map[string]any{"content": "d"}},
	}}
	f, _ := sendFake("X", stream)
	stdio, _, _ := testcli.NewIO(nil)
	if err := New(f.deps()).Run(context.Background(), []string{"send", "chat-id", "hello?", "--wait-all"}, stdio); err != nil {
		t.Fatalf("err=%v", err)
	}
	if stream.i != 4 {
		t.Errorf("--wait-all dropped when placed after positionals: consumed=%d want=4", stream.i)
	}
}

func TestSendBadFlag(t *testing.T) {
	t.Parallel()
	stdio, _, _ := testcli.NewIO(nil)
	err := New((&fakeDeps{}).deps()).Run(context.Background(), []string{"send", "--nope", "chat-x", "msg"}, stdio)
	if !errors.Is(err, cli.ErrUsage) {
		t.Errorf("err=%v", err)
	}
}

func TestSendMissingID(t *testing.T) {
	t.Parallel()
	cmd := &sendCmd{deps: (&fakeDeps{}).deps()}
	stdio, _, _ := testcli.NewIO(nil)
	err := cmd.Run(context.Background(), nil, stdio)
	if !errors.Is(err, cli.ErrUsage) {
		t.Errorf("err=%v", err)
	}
}

func TestSendNoMessage(t *testing.T) {
	t.Parallel()
	cmd := &sendCmd{deps: (&fakeDeps{}).deps()}
	stdio, _, _ := testcli.NewIO(nil)
	err := cmd.Run(context.Background(), []string{"chat-id"}, stdio)
	if !errors.Is(err, cli.ErrUsage) {
		t.Errorf("err=%v", err)
	}
}

// TestSendRejectsExtraPositionals exercises the `len(args) > 2` branch:
// extra trailing tokens beyond `<id> <message>` must yield ErrUsage so the
// operator quotes multi-word messages instead of having them silently dropped.
func TestSendRejectsExtraPositionals(t *testing.T) {
	t.Parallel()
	f := &fakeDeps{}
	cmd := &sendCmd{deps: f.deps()}
	stdio, _, _ := testcli.NewIO(nil)
	err := cmd.Run(context.Background(), []string{"id1", "msg1", "extra-msg"}, stdio)
	if !errors.Is(err, cli.ErrUsage) || !strings.Contains(err.Error(), "exactly one positional <message>") {
		t.Errorf("err=%v want strict-arity ErrUsage", err)
	}
	if f.lastPath != "" || f.streamPth != "" {
		t.Errorf("Unary/Stream should not be called on positional-arity failure: path=%q stream=%q", f.lastPath, f.streamPth)
	}
}

func TestSendMessageFilePath(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	p := filepath.Join(dir, "msg.txt")
	if err := os.WriteFile(p, []byte("hello from file"), 0o600); err != nil {
		t.Fatal(err)
	}
	stream := &fakeStream{frames: []map[string]any{
		{"id": "X", "lifecycle": "LIFECYCLE_EXECUTED", "mdCell": map[string]any{"content": "ok"}},
	}}
	f, _ := sendFake("X", stream)
	stdio, _, _ := testcli.NewIO(nil)
	if err := New(f.deps()).Run(context.Background(), []string{"send", "c", "--message-file", p}, stdio); err != nil {
		t.Fatalf("err=%v", err)
	}
	if !strings.Contains(string(f.lastRaw), `"message":"hello from file"`) {
		t.Errorf("body=%s", f.lastRaw)
	}
}

func TestSendMessageFileMissing(t *testing.T) {
	t.Parallel()
	stdio, _, _ := testcli.NewIO(nil)
	err := New((&fakeDeps{}).deps()).Run(context.Background(), []string{"send", "c", "--message-file", "/nope/does-not-exist-12345"}, stdio)
	if err == nil || !strings.Contains(err.Error(), "read message file") {
		t.Errorf("err=%v", err)
	}
}

func TestSendMessageFileStdin(t *testing.T) {
	t.Parallel()
	stream := &fakeStream{frames: []map[string]any{
		{"id": "X", "lifecycle": "LIFECYCLE_EXECUTED", "mdCell": map[string]any{"content": "ok"}},
	}}
	f, _ := sendFake("X", stream)
	stdio, _, _ := testcli.NewIO(strings.NewReader("from-stdin-input"))
	if err := New(f.deps()).Run(context.Background(), []string{"send", "c", "--message-file", "-"}, stdio); err != nil {
		t.Fatalf("err=%v", err)
	}
	if !strings.Contains(string(f.lastRaw), `"message":"from-stdin-input"`) {
		t.Errorf("body=%s", f.lastRaw)
	}
}

func TestSendStdinEmpty(t *testing.T) {
	t.Parallel()
	stdio, _, _ := testcli.NewIO(strings.NewReader(""))
	err := New((&fakeDeps{}).deps()).Run(context.Background(), []string{"send", "c", "--message-file", "-"}, stdio)
	if !errors.Is(err, cli.ErrUsage) {
		t.Errorf("err=%v", err)
	}
}

// errReader forces the ReadAll branch of stdin reading to error out.
type errReader struct{ err error }

func (e errReader) Read([]byte) (int, error) { return 0, e.err }

func TestSendStdinReadErr(t *testing.T) {
	t.Parallel()
	stdio, _, _ := testcli.NewIO(errReader{err: errors.New("read-fail")})
	err := New((&fakeDeps{}).deps()).Run(context.Background(), []string{"send", "c", "--message-file", "-"}, stdio)
	if err == nil || !strings.Contains(err.Error(), "read-fail") {
		t.Errorf("err=%v", err)
	}
}

func TestSendStdinNilReader(t *testing.T) {
	t.Parallel()
	// Direct helper call — the run path always has a non-nil Stdin because
	// cli.IO sets os.Stdin at root. This exercises the nil branch.
	if _, err := resolveMessage("", "-", nil); !errors.Is(err, cli.ErrUsage) {
		t.Errorf("want usage err: %v", err)
	}
}

func TestSendBothPositionalAndFile(t *testing.T) {
	t.Parallel()
	stdio, _, _ := testcli.NewIO(nil)
	err := New((&fakeDeps{}).deps()).Run(context.Background(), []string{"send", "c", "positional", "--message-file", "/tmp/anything"}, stdio)
	if !errors.Is(err, cli.ErrUsage) {
		t.Errorf("err=%v", err)
	}
}

func TestSendUUIDFnNil(t *testing.T) {
	t.Parallel()
	// With nil UUIDFn the messageId is empty; server's CellID response becomes
	// the authoritative target. Cover the "CellID empty → fall back to msgID"
	// branch by having the stub return empty and asserting we still proceed
	// (the fallback yields "" as target, which won't match any cell; stream
	// ends naturally).
	stream := &fakeStream{frames: []map[string]any{
		{"id": "X", "lifecycle": "LIFECYCLE_EXECUTING", "mdCell": map[string]any{"content": "mid"}},
	}}
	f := &fakeDeps{
		uuidFn: nil, // explicit
		unaryFn: func(_ context.Context, _ string, _, resp any) error {
			out := resp.(*sendMessageResp)
			out.CellID = "" // server returns empty → fallback triggers
			return nil
		},
		streamFn: func(_ context.Context, _ string, _ any) (StreamSession, error) {
			return stream, nil
		},
	}
	cmd := &sendCmd{deps: f.deps()}
	stdio, _, _ := testcli.NewIO(nil)
	if err := cmd.Run(context.Background(), []string{"c", "hi"}, stdio); err != nil {
		t.Fatalf("err=%v", err)
	}
}

// TestFrameContentAllVariants exercises every branch of frameContent. The send
// renderer picks md > py > status > summary > playbook; we construct a frame
// per variant (minus the leaders already covered in streaming tests) plus the
// default empty case so the method reaches 100%.
func TestFrameContentAllVariants(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		f    streamFrame
		want string
	}{
		{"md", streamFrame{MdCell: &struct {
			Content string `json:"content"`
		}{Content: "m"}}, "m"},
		{"py", streamFrame{PyCell: &struct {
			Code string `json:"code"`
		}{Code: "c"}}, "c"},
		{"status", streamFrame{StatusCell: &struct {
			Status string `json:"status"`
		}{Status: "s"}}, "s"},
		{"summary", streamFrame{SummaryCell: &struct {
			Summary string `json:"summary"`
		}{Summary: "su"}}, "su"},
		{"playbook", streamFrame{PlaybookEditorCell: &struct {
			Action string `json:"action"`
		}{Action: "a"}}, "a"},
		{"empty", streamFrame{}, ""},
	}
	for _, tc := range cases {
		if got := tc.f.frameContent(); got != tc.want {
			t.Errorf("%s: got=%q want=%q", tc.name, got, tc.want)
		}
	}
}
