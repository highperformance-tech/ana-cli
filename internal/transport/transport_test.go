package transport

import (
	"bufio"
	"context"
	"encoding/binary"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

// newTestServer starts an httptest.Server wired to handler and returns the
// server plus a Client pointed at it. Tests defer server.Close() themselves.
func newTestServer(t *testing.T, handler http.HandlerFunc, opts ...Option) (*httptest.Server, *Client) {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	c := New(srv.URL, nil, opts...)
	return srv, c
}

// writeFrame writes a single Connect server-streaming frame with the given
// flags and payload, flushing between the header and payload so tests exercise
// partial-read paths in StreamReader.
func writeFrame(w *bufio.Writer, flags byte, payload []byte) error {
	header := make([]byte, 5)
	header[0] = flags
	binary.BigEndian.PutUint32(header[1:5], uint32(len(payload)))
	if _, err := w.Write(header); err != nil {
		return err
	}
	if err := w.Flush(); err != nil {
		return err
	}
	// Split payload into halves to force a partial read between flushes.
	if len(payload) == 0 {
		return nil
	}
	mid := len(payload) / 2
	if mid > 0 {
		if _, err := w.Write(payload[:mid]); err != nil {
			return err
		}
		if err := w.Flush(); err != nil {
			return err
		}
	}
	if _, err := w.Write(payload[mid:]); err != nil {
		return err
	}
	return w.Flush()
}

// mustJSON marshals v to JSON or t.Fatals.
func mustJSON(t *testing.T, v any) []byte {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return b
}

// drainBody reads and discards the request body and returns it.
func drainBody(t *testing.T, r *http.Request) []byte {
	t.Helper()
	b, err := io.ReadAll(r.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	return b
}

// staticToken returns a tokenFn that always yields the given token with nil
// err.
func staticToken(tok string) func(context.Context) (string, error) {
	return func(context.Context) (string, error) { return tok, nil }
}
