package transport

import (
	"bufio"
	"context"
	"encoding/binary"
	"errors"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"
)

type streamEvent struct {
	N int `json:"n"`
}

func TestStreamHappyPath(t *testing.T) {
	t.Parallel()
	_, c := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		bw := bufio.NewWriter(w)
		for i := 1; i <= 3; i++ {
			if err := writeFrame(bw, 0, mustJSON(t, streamEvent{N: i})); err != nil {
				t.Fatalf("writeFrame: %v", err)
			}
		}
		if err := writeFrame(bw, trailerFlag, nil); err != nil {
			t.Fatalf("writeFrame trailer: %v", err)
		}
		bw.Flush()
	})
	ctx := context.Background()
	sr, err := c.Stream(ctx, "/", nil)
	if err != nil {
		t.Fatalf("Stream: %v", err)
	}
	defer sr.Close()
	for i := 1; i <= 3; i++ {
		var ev streamEvent
		ok, err := sr.Next(ctx, &ev)
		if err != nil || !ok {
			t.Fatalf("Next(%d) = (%v,%v)", i, ok, err)
		}
		if ev.N != i {
			t.Fatalf("N = %d, want %d", ev.N, i)
		}
	}
	ok, err := sr.Next(ctx, &streamEvent{})
	if err != nil || ok {
		t.Fatalf("terminal Next = (%v, %v), want (false, nil)", ok, err)
	}
	// Subsequent calls after done keep returning (false, nil).
	ok, err = sr.Next(ctx, &streamEvent{})
	if err != nil || ok {
		t.Fatalf("post-done Next = (%v, %v)", ok, err)
	}
}

func TestStreamTrailerError(t *testing.T) {
	t.Parallel()
	_, c := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		bw := bufio.NewWriter(w)
		writeFrame(bw, 0, mustJSON(t, streamEvent{N: 1}))
		writeFrame(bw, trailerFlag, []byte(`{"code":"aborted","message":"nope"}`))
		bw.Flush()
	})
	ctx := context.Background()
	sr, err := c.Stream(ctx, "/", nil)
	if err != nil {
		t.Fatalf("Stream: %v", err)
	}
	defer sr.Close()
	var ev streamEvent
	ok, err := sr.Next(ctx, &ev)
	if err != nil || !ok {
		t.Fatalf("first Next = (%v,%v)", ok, err)
	}
	ok, err = sr.Next(ctx, &ev)
	if ok || err == nil {
		t.Fatalf("want error, got (%v,%v)", ok, err)
	}
	var te *Error
	if !errors.As(err, &te) {
		t.Fatalf("want *Error, got %T %v", err, err)
	}
	if te.Code != "aborted" || te.Message != "nope" {
		t.Fatalf("got %+v", te)
	}
}

func TestStreamTrailerMalformedBody(t *testing.T) {
	t.Parallel()
	_, c := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		bw := bufio.NewWriter(w)
		// Trailer with payload that isn't JSON.
		writeFrame(bw, trailerFlag, []byte("not-json"))
		bw.Flush()
	})
	ctx := context.Background()
	sr, err := c.Stream(ctx, "/", nil)
	if err != nil {
		t.Fatalf("Stream: %v", err)
	}
	defer sr.Close()
	ok, err := sr.Next(ctx, &streamEvent{})
	if ok || err == nil {
		t.Fatalf("want err, got (%v,%v)", ok, err)
	}
	var te *Error
	if !errors.As(err, &te) {
		t.Fatalf("want *Error, got %T", err)
	}
	if te.Message == "" {
		t.Fatalf("Message empty on malformed trailer")
	}
}

func TestStreamTrailerEmptyEnvelope(t *testing.T) {
	t.Parallel()
	// Trailer with a JSON body that parses to an empty envelope → clean end.
	_, c := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		bw := bufio.NewWriter(w)
		writeFrame(bw, trailerFlag, []byte(`{}`))
		bw.Flush()
	})
	ctx := context.Background()
	sr, err := c.Stream(ctx, "/", nil)
	if err != nil {
		t.Fatalf("Stream: %v", err)
	}
	defer sr.Close()
	ok, err := sr.Next(ctx, &streamEvent{})
	if ok || err != nil {
		t.Fatalf("want (false,nil), got (%v,%v)", ok, err)
	}
}

func TestStreamTruncatedHeader(t *testing.T) {
	t.Parallel()
	// Write only 2 header bytes then hang up.
	_, c := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte{0x00, 0x00})
	})
	ctx := context.Background()
	sr, err := c.Stream(ctx, "/", nil)
	if err != nil {
		t.Fatalf("Stream: %v", err)
	}
	defer sr.Close()
	ok, err := sr.Next(ctx, &streamEvent{})
	if ok || err == nil {
		t.Fatalf("want err, got (%v,%v)", ok, err)
	}
	if !strings.Contains(err.Error(), "truncated frame header") {
		t.Fatalf("err = %v", err)
	}
}

func TestStreamTruncatedPayload(t *testing.T) {
	t.Parallel()
	// Declare a 100-byte payload but send only 10.
	_, c := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		header := make([]byte, 5)
		binary.BigEndian.PutUint32(header[1:], 100)
		w.Write(header)
		w.Write(make([]byte, 10))
	})
	ctx := context.Background()
	sr, err := c.Stream(ctx, "/", nil)
	if err != nil {
		t.Fatalf("Stream: %v", err)
	}
	defer sr.Close()
	ok, err := sr.Next(ctx, &streamEvent{})
	if ok || err == nil {
		t.Fatalf("want err")
	}
	if !strings.Contains(err.Error(), "truncated frame payload") {
		t.Fatalf("err = %v", err)
	}
}

func TestStreamNonJSONDataFrame(t *testing.T) {
	t.Parallel()
	_, c := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		bw := bufio.NewWriter(w)
		writeFrame(bw, 0, []byte("not-json"))
		bw.Flush()
	})
	ctx := context.Background()
	sr, err := c.Stream(ctx, "/", nil)
	if err != nil {
		t.Fatalf("Stream: %v", err)
	}
	defer sr.Close()
	var ev streamEvent
	ok, err := sr.Next(ctx, &ev)
	if ok || err == nil {
		t.Fatalf("want decode err")
	}
	if !strings.Contains(err.Error(), "decode frame") {
		t.Fatalf("err = %v", err)
	}
}

func TestStreamNextWithNilOut(t *testing.T) {
	t.Parallel()
	// Nil out parameter skips JSON decoding — useful when a caller just wants
	// frame counts.
	_, c := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		bw := bufio.NewWriter(w)
		writeFrame(bw, 0, []byte(`{"n":1}`))
		writeFrame(bw, trailerFlag, nil)
		bw.Flush()
	})
	ctx := context.Background()
	sr, err := c.Stream(ctx, "/", nil)
	if err != nil {
		t.Fatalf("Stream: %v", err)
	}
	defer sr.Close()
	ok, err := sr.Next(ctx, nil)
	if !ok || err != nil {
		t.Fatalf("want true/nil, got (%v,%v)", ok, err)
	}
	ok, err = sr.Next(ctx, nil)
	if ok || err != nil {
		t.Fatalf("want false/nil, got (%v,%v)", ok, err)
	}
}

func TestStreamNonSuccessOnOpen(t *testing.T) {
	t.Parallel()
	_, c := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(500)
		w.Write([]byte(`{"code":"internal","message":"broken"}`))
	})
	sr, err := c.Stream(context.Background(), "/", nil)
	if sr != nil {
		t.Fatalf("want nil StreamReader on error")
	}
	var te *Error
	if !errors.As(err, &te) {
		t.Fatalf("want *Error, got %T %v", err, err)
	}
	if te.HTTPStatus != 500 || te.Code != "internal" {
		t.Fatalf("got %+v", te)
	}
}

// blockingBody lets a test release chunks of data on demand and also blocks
// reads until unblocked, so we can fire a ctx cancel mid-read.
type blockingBody struct {
	mu     sync.Mutex
	ch     chan []byte
	closed bool
}

func newBlockingBody() *blockingBody { return &blockingBody{ch: make(chan []byte, 4)} }

func (b *blockingBody) Read(p []byte) (int, error) {
	chunk, ok := <-b.ch
	if !ok {
		return 0, io.EOF
	}
	n := copy(p, chunk)
	return n, nil
}

func (b *blockingBody) Close() error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed {
		return nil
	}
	b.closed = true
	close(b.ch)
	return nil
}

// ctxRT returns a response whose body blocks on read; used for ctx-cancel.
type ctxRT struct{ body io.ReadCloser }

func (c ctxRT) RoundTrip(req *http.Request) (*http.Response, error) {
	return &http.Response{StatusCode: 200, Body: c.body, Header: make(http.Header), Request: req}, nil
}

func TestStreamContextCancelDuringRead(t *testing.T) {
	t.Parallel()
	bb := newBlockingBody()
	rt := ctxRT{body: bb}
	c := New("http://example.invalid", nil, WithHTTPClient(&http.Client{Transport: rt}))
	ctx, cancel := context.WithCancel(context.Background())
	sr, err := c.Stream(ctx, "/", nil)
	if err != nil {
		t.Fatalf("Stream: %v", err)
	}
	// Cancel first so Next's pre-check trips.
	cancel()
	ok, err := sr.Next(ctx, &streamEvent{})
	if ok || err == nil || !errors.Is(err, context.Canceled) {
		t.Fatalf("want ctx canceled, got (%v,%v)", ok, err)
	}
	// Make sure Close is safe.
	if cerr := sr.Close(); cerr != nil {
		t.Fatalf("Close: %v", cerr)
	}
	// Double-close returns nil.
	if cerr := sr.Close(); cerr != nil {
		t.Fatalf("double Close: %v", cerr)
	}
}

// readBlockerBody blocks Read until Close is called, then returns an io.EOF
// — used to exercise the "context cancelled while blocked in ReadFull" path.
type readBlockerBody struct {
	closed chan struct{}
	once   sync.Once
}

func newReadBlockerBody() *readBlockerBody { return &readBlockerBody{closed: make(chan struct{})} }

func (r *readBlockerBody) Read(p []byte) (int, error) {
	<-r.closed
	return 0, errors.New("body closed")
}

func (r *readBlockerBody) Close() error {
	r.once.Do(func() { close(r.closed) })
	return nil
}

func TestStreamCtxErrDuringHeaderRead(t *testing.T) {
	t.Parallel()
	// Read errors mid-header; ctx is cancelled → ctx err takes precedence.
	body := newReadBlockerBody()
	c := New("http://example.invalid", nil, WithHTTPClient(&http.Client{Transport: ctxRT{body: body}}))
	ctx, cancel := context.WithCancel(context.Background())
	sr, err := c.Stream(ctx, "/", nil)
	if err != nil {
		t.Fatalf("Stream: %v", err)
	}

	done := make(chan error, 1)
	go func() {
		_, err := sr.Next(ctx, &streamEvent{})
		done <- err
	}()
	time.Sleep(10 * time.Millisecond)
	cancel()
	// Close the body so the read returns.
	body.Close()
	select {
	case err := <-done:
		if err == nil || !errors.Is(err, context.Canceled) {
			t.Fatalf("want ctx.Canceled, got %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("Next did not return")
	}
}

// genericReadErrBody returns a non-EOF error on Read.
type genericReadErrBody struct{}

func (genericReadErrBody) Read(p []byte) (int, error) { return 0, errors.New("network boom") }
func (genericReadErrBody) Close() error               { return nil }

func TestStreamGenericReadError(t *testing.T) {
	t.Parallel()
	c := New("http://example.invalid", nil, WithHTTPClient(&http.Client{Transport: ctxRT{body: genericReadErrBody{}}}))
	ctx := context.Background()
	sr, err := c.Stream(ctx, "/", nil)
	if err != nil {
		t.Fatalf("Stream: %v", err)
	}
	ok, err := sr.Next(ctx, &streamEvent{})
	if ok || err == nil {
		t.Fatalf("want err")
	}
	if !strings.Contains(err.Error(), "read header") {
		t.Fatalf("err = %v", err)
	}
}

// partialHeaderThenErrBody returns a few header bytes, then a payload-phase
// generic error to exercise the "read payload" branch (non-EOF, non-ctx).
type partialHeaderThenErrBody struct {
	stage int
}

func (b *partialHeaderThenErrBody) Read(p []byte) (int, error) {
	switch b.stage {
	case 0:
		// Return a full 5-byte header declaring a 10-byte payload.
		b.stage = 1
		header := make([]byte, 5)
		binary.BigEndian.PutUint32(header[1:], 10)
		n := copy(p, header)
		return n, nil
	default:
		return 0, errors.New("payload boom")
	}
}
func (b *partialHeaderThenErrBody) Close() error { return nil }

func TestStreamPayloadReadError(t *testing.T) {
	t.Parallel()
	c := New("http://example.invalid", nil, WithHTTPClient(&http.Client{Transport: ctxRT{body: &partialHeaderThenErrBody{}}}))
	ctx := context.Background()
	sr, err := c.Stream(ctx, "/", nil)
	if err != nil {
		t.Fatalf("Stream: %v", err)
	}
	ok, err := sr.Next(ctx, &streamEvent{})
	if ok || err == nil {
		t.Fatalf("want err")
	}
	if !strings.Contains(err.Error(), "read payload") {
		t.Fatalf("err = %v", err)
	}
}

// payloadCtxErrBody returns a header and then, on payload read, both returns
// an error AND the ctx has been cancelled — ctx err branch inside payload read.
type payloadCtxErrBody struct {
	stage  int
	cancel func()
}

func (b *payloadCtxErrBody) Read(p []byte) (int, error) {
	switch b.stage {
	case 0:
		b.stage = 1
		header := make([]byte, 5)
		binary.BigEndian.PutUint32(header[1:], 10)
		n := copy(p, header)
		return n, nil
	default:
		b.cancel()
		return 0, errors.New("payload boom")
	}
}
func (b *payloadCtxErrBody) Close() error { return nil }

func TestStreamPayloadReadCtxErr(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(context.Background())
	body := &payloadCtxErrBody{cancel: cancel}
	c := New("http://example.invalid", nil, WithHTTPClient(&http.Client{Transport: ctxRT{body: body}}))
	sr, err := c.Stream(ctx, "/", nil)
	if err != nil {
		t.Fatalf("Stream: %v", err)
	}
	ok, err := sr.Next(ctx, &streamEvent{})
	if ok || err == nil || !errors.Is(err, context.Canceled) {
		t.Fatalf("want ctx canceled, got (%v,%v)", ok, err)
	}
}

// headerCtxErrBody returns ctx err on first Read + ctx cancelled.
type headerCtxErrBody struct{ cancel func() }

func (b *headerCtxErrBody) Read(p []byte) (int, error) {
	b.cancel()
	return 0, errors.New("header boom")
}
func (b *headerCtxErrBody) Close() error { return nil }

func TestStreamHeaderReadCtxErr(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(context.Background())
	c := New("http://example.invalid", nil, WithHTTPClient(&http.Client{Transport: ctxRT{body: &headerCtxErrBody{cancel: cancel}}}))
	sr, err := c.Stream(ctx, "/", nil)
	if err != nil {
		t.Fatalf("Stream: %v", err)
	}
	ok, err := sr.Next(ctx, &streamEvent{})
	if ok || err == nil || !errors.Is(err, context.Canceled) {
		t.Fatalf("want ctx canceled, got (%v,%v)", ok, err)
	}
}

func TestStreamBuildRequestError(t *testing.T) {
	t.Parallel()
	// Unserializable body → buildRequest returns a marshal error which Stream
	// should propagate directly.
	c := New("http://example.invalid", nil)
	sr, err := c.Stream(context.Background(), "/", unserializable{Ch: make(chan int)})
	if sr != nil || err == nil {
		t.Fatalf("want build err, got (%v, %v)", sr, err)
	}
	if !strings.Contains(err.Error(), "marshal request") {
		t.Fatalf("err = %v", err)
	}
}

// TestStreamWithBody covers the non-nil req marshal path + header plumbing:
// User-Agent (via WithUserAgent) and Authorization (via tokenFn returning a
// non-empty token). Three gaps closed in one server round-trip.
func TestStreamWithBody(t *testing.T) {
	t.Parallel()
	tokenFn := func(context.Context) (string, error) { return "tok-123", nil }
	srv, _ := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("User-Agent"); got != "ana/x" {
			t.Errorf("UA=%q", got)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer tok-123" {
			t.Errorf("Auth=%q", got)
		}
		// Verify non-nil req was framed + JSON-encoded.
		body, _ := io.ReadAll(r.Body)
		if len(body) < 5 || !strings.Contains(string(body[5:]), `"n":7`) {
			t.Errorf("body=%q", body)
		}
		// Empty stream; client sees EOF.
	}, WithUserAgent("ana/x"))
	c := New(srv.URL, tokenFn, WithUserAgent("ana/x"))
	sr, err := c.Stream(context.Background(), "/", streamEvent{N: 7})
	if err != nil {
		t.Fatalf("Stream: %v", err)
	}
	sr.Close()
}

// TestStreamTokenFnError covers the tokenFn-returned-error branch.
func TestStreamTokenFnError(t *testing.T) {
	t.Parallel()
	c := New("http://example.invalid", func(context.Context) (string, error) {
		return "", errors.New("token-boom")
	})
	_, err := c.Stream(context.Background(), "/", nil)
	if err == nil || !strings.Contains(err.Error(), "token-boom") {
		t.Fatalf("err=%v", err)
	}
}

// TestStreamBuildRequestInvalidURL covers http.NewRequestWithContext failure
// (control-char in URL is rejected before the transport is touched).
func TestStreamBuildRequestInvalidURL(t *testing.T) {
	t.Parallel()
	c := New("http://example.invalid\x7f", nil)
	_, err := c.Stream(context.Background(), "/", nil)
	if err == nil || !strings.Contains(err.Error(), "build request") {
		t.Fatalf("err=%v", err)
	}
}

func TestStreamEOFBeforeAnyFrame(t *testing.T) {
	t.Parallel()
	// Server writes nothing and hangs up cleanly.
	_, c := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {})
	ctx := context.Background()
	sr, err := c.Stream(ctx, "/", nil)
	if err != nil {
		t.Fatalf("Stream: %v", err)
	}
	defer sr.Close()
	ok, err := sr.Next(ctx, &streamEvent{})
	if ok || err == nil {
		t.Fatalf("want err, got (%v,%v)", ok, err)
	}
	if !strings.Contains(err.Error(), "unexpected EOF before frame header") {
		t.Fatalf("err = %v", err)
	}
}

// TestStreamFrameTooLarge covers the server-controlled-length guard. A header
// declaring a payload larger than maxFrameBytes must short-circuit with a
// *Error before the reader allocates anything, otherwise a hostile/buggy
// server could drive us into an OOM by setting length to ~4 GiB.
func TestStreamFrameTooLarge(t *testing.T) {
	t.Parallel()
	// Write a header declaring maxFrameBytes+1 bytes. No payload follows —
	// the guard must trip before any io.ReadFull is attempted.
	_, c := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		header := make([]byte, 5)
		binary.BigEndian.PutUint32(header[1:], uint32(maxFrameBytes+1))
		w.Write(header)
	})
	ctx := context.Background()
	sr, err := c.Stream(ctx, "/", nil)
	if err != nil {
		t.Fatalf("Stream: %v", err)
	}
	defer sr.Close()
	ok, err := sr.Next(ctx, &streamEvent{})
	if ok || err == nil {
		t.Fatalf("want err, got (%v,%v)", ok, err)
	}
	var te *Error
	if !errors.As(err, &te) {
		t.Fatalf("want *Error, got %T %v", err, err)
	}
	if !strings.Contains(te.Message, "frame too large") {
		t.Fatalf("Message = %q, want contains \"frame too large\"", te.Message)
	}
	// Subsequent calls stay quiet — guard flipped done=true.
	ok, err = sr.Next(ctx, &streamEvent{})
	if ok || err != nil {
		t.Fatalf("post-guard Next = (%v, %v), want (false, nil)", ok, err)
	}
}
