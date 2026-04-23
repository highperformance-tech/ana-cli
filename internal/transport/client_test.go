package transport

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

type echoPayload struct {
	Name string `json:"name"`
	N    int    `json:"n"`
}

func TestUnaryHappyPath(t *testing.T) {
	t.Parallel()
	_, c := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method = %q, want POST", r.Method)
		}
		if ct := r.Header.Get("content-type"); ct != "application/json" {
			t.Errorf("content-type = %q", ct)
		}
		if a := r.Header.Get("accept"); a != "application/json" {
			t.Errorf("accept = %q", a)
		}
		body := drainBody(t, r)
		w.Header().Set("content-type", "application/json")
		w.WriteHeader(200)
		w.Write(body)
	})

	req := echoPayload{Name: "ana", N: 7}
	var resp echoPayload
	if err := c.Unary(context.Background(), "/foo", req, &resp); err != nil {
		t.Fatalf("Unary: %v", err)
	}
	if resp != req {
		t.Fatalf("resp = %+v, want %+v", resp, req)
	}
}

func TestUnaryAuthorizationHeader(t *testing.T) {
	t.Parallel()
	_, srv := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer sekret" {
			t.Errorf("Authorization = %q", got)
		}
		w.Write([]byte("{}"))
	})
	srv.tokenFn = staticToken("sekret")
	if err := srv.Unary(context.Background(), "/", nil, nil); err != nil {
		t.Fatalf("Unary: %v", err)
	}
}

func TestUnaryNoAuthWhenTokenEmpty(t *testing.T) {
	t.Parallel()
	_, srv := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		if _, ok := r.Header["Authorization"]; ok {
			t.Errorf("unexpected Authorization header")
		}
		w.Write([]byte("{}"))
	})
	srv.tokenFn = staticToken("")
	if err := srv.Unary(context.Background(), "/", nil, nil); err != nil {
		t.Fatalf("Unary: %v", err)
	}
}

func TestUnaryTokenError(t *testing.T) {
	t.Parallel()
	var called bool
	_, srv := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.Write([]byte("{}"))
	})
	tokenErr := errors.New("no creds")
	srv.tokenFn = func(context.Context) (string, error) { return "", tokenErr }
	err := srv.Unary(context.Background(), "/", nil, nil)
	if err == nil {
		t.Fatalf("want error, got nil")
	}
	if !errors.Is(err, tokenErr) {
		t.Fatalf("want wrapped %v, got %v", tokenErr, err)
	}
	if called {
		t.Fatalf("server should not have been called")
	}
}

func TestUnaryNilRequestBody(t *testing.T) {
	t.Parallel()
	_, c := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		body := drainBody(t, r)
		if string(body) != "{}" {
			t.Errorf("body = %q, want {}", body)
		}
		w.Write([]byte("{}"))
	})
	if err := c.Unary(context.Background(), "/", nil, nil); err != nil {
		t.Fatalf("Unary: %v", err)
	}
}

func TestUnaryNilResponseIgnoresBody(t *testing.T) {
	t.Parallel()
	_, c := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		// Return malformed JSON — with resp==nil it should be ignored.
		w.Write([]byte("not json"))
	})
	if err := c.Unary(context.Background(), "/", nil, nil); err != nil {
		t.Fatalf("Unary: %v", err)
	}
}

func TestUnaryConnectErrorBody(t *testing.T) {
	t.Parallel()
	_, c := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(400)
		w.Write([]byte(`{"code":"invalid_argument","message":"bad"}`))
	})
	err := c.Unary(context.Background(), "/", nil, nil)
	var te *Error
	if !errors.As(err, &te) {
		t.Fatalf("want *Error, got %T %v", err, err)
	}
	if te.HTTPStatus != 400 || te.Code != "invalid_argument" || te.Message != "bad" {
		t.Fatalf("got %+v", te)
	}
}

func TestUnaryOathkeeperErrorBody(t *testing.T) {
	t.Parallel()
	_, c := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(502)
		w.Write([]byte(`{"error":{"code":502,"status":"Bad Gateway","message":"upstream"}}`))
	})
	err := c.Unary(context.Background(), "/", nil, nil)
	var te *Error
	if !errors.As(err, &te) {
		t.Fatalf("want *Error, got %T %v", err, err)
	}
	if te.HTTPStatus != 502 || te.Code != "Bad Gateway" || te.Message != "upstream" {
		t.Fatalf("got %+v", te)
	}
}

func TestUnaryNonJSONErrorBody(t *testing.T) {
	t.Parallel()
	_, c := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(500)
		w.Write([]byte("<html>oops</html>"))
	})
	err := c.Unary(context.Background(), "/", nil, nil)
	var te *Error
	if !errors.As(err, &te) {
		t.Fatalf("want *Error, got %T %v", err, err)
	}
	if te.HTTPStatus != 500 || te.Code != "" || string(te.Raw) != "<html>oops</html>" {
		t.Fatalf("got %+v", te)
	}
}

// TestUnaryAmbiguousErrorBody covers a body that carries both a Connect
// `"code"` string AND an Oathkeeper-shaped `"error"` object. The shape
// dispatch pins Connect first so classification is deterministic.
func TestUnaryAmbiguousErrorBody(t *testing.T) {
	t.Parallel()
	_, c := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(400)
		w.Write([]byte(`{"code":"invalid_argument","message":"bad","error":{"status":"Bad Gateway","message":"upstream"}}`))
	})
	err := c.Unary(context.Background(), "/", nil, nil)
	var te *Error
	if !errors.As(err, &te) {
		t.Fatalf("want *Error, got %T %v", err, err)
	}
	if te.Code != "invalid_argument" || te.Message != "bad" {
		t.Fatalf("ambiguous body: want Connect classification, got %+v", te)
	}
}

// TestUnaryUnknownJSONErrorBody covers a valid JSON body that matches neither
// Connect nor Oathkeeper shapes. Code/Message stay empty so callers can still
// rely on Raw and the HTTP status for context.
func TestUnaryUnknownJSONErrorBody(t *testing.T) {
	t.Parallel()
	_, c := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(503)
		w.Write([]byte(`{"detail":"service unavailable"}`))
	})
	err := c.Unary(context.Background(), "/", nil, nil)
	var te *Error
	if !errors.As(err, &te) {
		t.Fatalf("want *Error, got %T %v", err, err)
	}
	if te.HTTPStatus != 503 || te.Code != "" || te.Message != "" {
		t.Fatalf("got %+v", te)
	}
	if string(te.Raw) != `{"detail":"service unavailable"}` {
		t.Fatalf("raw = %q", string(te.Raw))
	}
}

// TestUnaryNonStringConnectCode covers a body whose `"code"` is NOT a JSON
// string (e.g. an integer). The shape probe rejects it as a Connect envelope,
// so we fall through to the neither-matches path rather than mis-classifying.
func TestUnaryNonStringConnectCode(t *testing.T) {
	t.Parallel()
	_, c := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(400)
		w.Write([]byte(`{"code":42,"message":"numeric-code"}`))
	})
	err := c.Unary(context.Background(), "/", nil, nil)
	var te *Error
	if !errors.As(err, &te) {
		t.Fatalf("want *Error, got %T %v", err, err)
	}
	if te.Code != "" || te.Message != "" {
		t.Fatalf("non-string code must not classify: %+v", te)
	}
}

func TestUnaryContextCancel(t *testing.T) {
	t.Parallel()
	block := make(chan struct{})
	_, c := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		<-block
	})
	t.Cleanup(func() { close(block) })
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- c.Unary(ctx, "/", nil, nil) }()
	time.Sleep(20 * time.Millisecond)
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("want context.Canceled, got %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("Unary did not return after cancel")
	}
}

func TestUnaryMalformedResponse(t *testing.T) {
	t.Parallel()
	_, c := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("not json"))
	})
	var out echoPayload
	err := c.Unary(context.Background(), "/", nil, &out)
	if err == nil || !strings.Contains(err.Error(), "decode response") {
		t.Fatalf("want decode error, got %v", err)
	}
}

func TestUnaryUserAgent(t *testing.T) {
	t.Parallel()
	_, c := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("User-Agent"); got != "ana/0.0.1" {
			t.Errorf("User-Agent = %q", got)
		}
		w.Write([]byte("{}"))
	}, WithUserAgent("ana/0.0.1"))
	if err := c.Unary(context.Background(), "/", nil, nil); err != nil {
		t.Fatalf("Unary: %v", err)
	}
}

// recordingRT captures the last outbound request and returns a canned 200.
type recordingRT struct {
	lastReq *http.Request
	lastURL string
}

func (r *recordingRT) RoundTrip(req *http.Request) (*http.Response, error) {
	r.lastReq = req
	r.lastURL = req.URL.String()
	body := io.NopCloser(bytes.NewReader([]byte(`{"ok":true}`)))
	return &http.Response{
		StatusCode: 200,
		Body:       body,
		Header:     make(http.Header),
		Request:    req,
	}, nil
}

func TestWithHTTPClient(t *testing.T) {
	t.Parallel()
	rt := &recordingRT{}
	httpClient := &http.Client{Transport: rt}
	c := New("https://example.invalid", nil, WithHTTPClient(httpClient))
	var out map[string]any
	if err := c.Unary(context.Background(), "/ping", nil, &out); err != nil {
		t.Fatalf("Unary: %v", err)
	}
	if rt.lastReq == nil {
		t.Fatalf("recording RT not invoked")
	}
	if out["ok"] != true {
		t.Fatalf("out = %+v", out)
	}
}

func TestWithHTTPClientNilIgnored(t *testing.T) {
	t.Parallel()
	// A nil http.Client must not crash or install a bad client; the
	// bearerTransport middleware wraps http.DefaultTransport in that case
	// (we clone DefaultClient so DefaultClient itself stays unmutated).
	c := New("http://example.invalid", nil, WithHTTPClient(nil))
	if c.httpClient == http.DefaultClient {
		t.Fatalf("expected a clone, not DefaultClient itself")
	}
	bt, ok := c.httpClient.Transport.(*bearerTransport)
	if !ok {
		t.Fatalf("expected bearerTransport wrap, got %T", c.httpClient.Transport)
	}
	if bt.next != http.DefaultTransport {
		t.Fatalf("expected next == http.DefaultTransport, got %T", bt.next)
	}
	// DefaultClient's Transport must remain untouched (nil).
	if http.DefaultClient.Transport != nil {
		t.Fatalf("http.DefaultClient.Transport mutated")
	}
}

func TestUnaryURLBuildNoDoubleSlash(t *testing.T) {
	t.Parallel()
	rt := &recordingRT{}
	c := New("https://example.invalid/", nil, WithHTTPClient(&http.Client{Transport: rt}))
	if err := c.Unary(context.Background(), "/rpc/Svc/Method", nil, nil); err != nil {
		t.Fatalf("Unary: %v", err)
	}
	if rt.lastURL != "https://example.invalid/rpc/Svc/Method" {
		t.Fatalf("URL = %q", rt.lastURL)
	}
}

func TestUnaryURLBuildNoLeadingOrTrailingSlash(t *testing.T) {
	t.Parallel()
	rt := &recordingRT{}
	c := New("https://example.invalid/api", nil, WithHTTPClient(&http.Client{Transport: rt}))
	if err := c.Unary(context.Background(), "/rpc", nil, nil); err != nil {
		t.Fatalf("Unary: %v", err)
	}
	if rt.lastURL != "https://example.invalid/api/rpc" {
		t.Fatalf("URL = %q", rt.lastURL)
	}
}

func TestUnaryMalformedBaseURL(t *testing.T) {
	t.Parallel()
	c := New("http://\x7f/bad", nil) // control byte → NewRequest rejects
	err := c.Unary(context.Background(), "/x", nil, nil)
	if err == nil || !strings.Contains(err.Error(), "build request") {
		t.Fatalf("want build request error, got %v", err)
	}
}

// unserializable triggers json.Marshal to fail: channels aren't JSON-encodable.
type unserializable struct {
	Ch chan int `json:"ch"`
}

func TestUnaryMarshalError(t *testing.T) {
	t.Parallel()
	c := New("http://example.invalid", nil)
	err := c.Unary(context.Background(), "/", unserializable{Ch: make(chan int)}, nil)
	if err == nil || !strings.Contains(err.Error(), "marshal request") {
		t.Fatalf("want marshal error, got %v", err)
	}
}

// readErrRT returns a response whose body errors on Read so we can exercise
// the io.ReadAll failure path in Unary.
type readErrRT struct{}

type errReader struct{}

func (errReader) Read(_ []byte) (int, error) { return 0, errors.New("boom read") }
func (errReader) Close() error               { return nil }

func (readErrRT) RoundTrip(req *http.Request) (*http.Response, error) {
	return &http.Response{StatusCode: 200, Body: errReader{}, Header: make(http.Header), Request: req}, nil
}

func TestUnaryReadBodyError(t *testing.T) {
	t.Parallel()
	c := New("http://example.invalid", nil, WithHTTPClient(&http.Client{Transport: readErrRT{}}))
	err := c.Unary(context.Background(), "/", nil, nil)
	if err == nil || !strings.Contains(err.Error(), "read response") {
		t.Fatalf("want read response error, got %v", err)
	}
}

// doErrRT always returns a transport error. Used to test the non-context
// error path in Unary/Stream.
type doErrRT struct{ err error }

func (d doErrRT) RoundTrip(*http.Request) (*http.Response, error) { return nil, d.err }

func TestUnaryTransportError(t *testing.T) {
	t.Parallel()
	want := errors.New("dial fail")
	c := New("http://example.invalid", nil, WithHTTPClient(&http.Client{Transport: doErrRT{err: want}}))
	err := c.Unary(context.Background(), "/", nil, nil)
	if err == nil || !errors.Is(err, want) {
		t.Fatalf("want wrapped %v, got %v", want, err)
	}
}

func TestStreamTransportErrorCtxCancel(t *testing.T) {
	t.Parallel()
	// Transport returns error AND the ctx is cancelled → ctx err surfaces.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	c := New("http://example.invalid", nil, WithHTTPClient(&http.Client{Transport: doErrRT{err: errors.New("x")}}))
	_, err := c.Stream(ctx, "/", nil)
	if err == nil || !errors.Is(err, context.Canceled) {
		t.Fatalf("want ctx cancel, got %v", err)
	}
}

func TestStreamTransportErrorNonCtx(t *testing.T) {
	t.Parallel()
	want := errors.New("dial fail")
	c := New("http://example.invalid", nil, WithHTTPClient(&http.Client{Transport: doErrRT{err: want}}))
	_, err := c.Stream(context.Background(), "/", nil)
	if err == nil || !errors.Is(err, want) {
		t.Fatalf("want wrapped %v, got %v", want, err)
	}
}

// withTokenFn helper sets tokenFn after construction for negative-token tests.
func (c *Client) withTokenFn(fn func(context.Context) (string, error)) *Client {
	c.tokenFn = fn
	return c
}

// Verify Unary error when request marshalling/build fails due to path.
func TestUnaryNewRequestError(t *testing.T) {
	t.Parallel()
	c := New("http://example.invalid", nil)
	err := c.Unary(context.Background(), "http://[::1%zone]/bad", nil, nil)
	if err == nil {
		t.Fatalf("want error")
	}
}

// sanity: captured catalog-style payloads round-trip through Unary.
func TestUnaryCatalogShape(t *testing.T) {
	t.Parallel()
	type theme struct {
		Bg string `json:"bg"`
	}
	type org struct {
		OrgID string `json:"orgId"`
		Theme theme  `json:"theme"`
	}
	type resp struct {
		Organization org `json:"organization"`
	}
	expect := resp{Organization: org{OrgID: "abc", Theme: theme{Bg: "#fff"}}}
	_, c := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("content-type", "application/json")
		json.NewEncoder(w).Encode(expect)
	})
	var got resp
	if err := c.Unary(context.Background(), "/svc", nil, &got); err != nil {
		t.Fatalf("Unary: %v", err)
	}
	if got != expect {
		t.Fatalf("got %+v, want %+v", got, expect)
	}
}

// Confirm the httptest-based tokenFn error path drops the server call entirely.
func TestUnaryTokenErrorNoServerCall(t *testing.T) {
	t.Parallel()
	var hit bool
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		hit = true
	}))
	t.Cleanup(srv.Close)
	c := New(srv.URL, nil).withTokenFn(func(context.Context) (string, error) {
		return "", fmt.Errorf("kaboom")
	})
	if err := c.Unary(context.Background(), "/", nil, nil); err == nil {
		t.Fatalf("want error")
	}
	if hit {
		t.Fatalf("server hit despite token error")
	}
}

// TestBearerTransportPreservesUserAgent verifies a caller-supplied User-Agent
// is not clobbered by the middleware (existing header wins).
func TestBearerTransportPreservesUserAgent(t *testing.T) {
	t.Parallel()
	_, c := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("User-Agent"); got != "caller/1.0" {
			t.Errorf("User-Agent = %q, want caller/1.0", got)
		}
		w.Write([]byte("{}"))
	}, WithUserAgent("ana/0.0.1"))

	// Drive a raw request with a pre-set User-Agent to prove the middleware
	// doesn't overwrite it.
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, c.baseURL, nil)
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	req.Header.Set("User-Agent", "caller/1.0")
	resp, err := c.httpClient.Do(req)
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	resp.Body.Close()
}

// TestBearerTransportPreservesAuthorization verifies a caller-supplied
// Authorization header is not clobbered by the middleware (existing header
// wins).
func TestBearerTransportPreservesAuthorization(t *testing.T) {
	t.Parallel()
	_, c := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer caller-token" {
			t.Errorf("Authorization = %q, want Bearer caller-token", got)
		}
		w.Write([]byte("{}"))
	})
	c.tokenFn = staticToken("middleware-token")

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, c.baseURL, nil)
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	req.Header.Set("Authorization", "Bearer caller-token")
	resp, err := c.httpClient.Do(req)
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	resp.Body.Close()
}

// TestWithHTTPClientWrapsCallerTransport proves a caller-supplied
// *http.Client with its own Transport still gets auth applied: the middleware
// wraps the caller's Transport, it isn't swapped out.
func TestWithHTTPClientWrapsCallerTransport(t *testing.T) {
	t.Parallel()
	rt := &recordingRT{}
	c := New("http://example.invalid", staticToken("tok"),
		WithHTTPClient(&http.Client{Transport: rt}))
	if err := c.Unary(context.Background(), "/x", nil, nil); err != nil {
		t.Fatalf("Unary: %v", err)
	}
	if rt.lastReq == nil {
		t.Fatalf("caller's Transport was not reached")
	}
	if got := rt.lastReq.Header.Get("Authorization"); got != "Bearer tok" {
		t.Fatalf("Authorization = %q, want Bearer tok", got)
	}
}

// TestDoRawGET exercises the happy GET path: no body sent, method + path
// preserved, response bytes returned verbatim.
func TestDoRawGET(t *testing.T) {
	t.Parallel()
	_, c := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("method = %q, want GET", r.Method)
		}
		if r.URL.Path != "/v1/things" {
			t.Errorf("path = %q", r.URL.Path)
		}
		if _, ok := r.Header["Content-Type"]; ok {
			t.Errorf("unexpected content-type on no-body request")
		}
		if r.ContentLength != 0 {
			t.Errorf("ContentLength = %d, want 0 (nil body)", r.ContentLength)
		}
		w.Header().Set("content-type", "application/json")
		w.Write([]byte(`{"ok":true}`))
	})
	status, body, err := c.DoRaw(context.Background(), http.MethodGet, "/v1/things", nil)
	if err != nil {
		t.Fatalf("DoRaw: %v", err)
	}
	if status != 200 {
		t.Errorf("status = %d", status)
	}
	if string(body) != `{"ok":true}` {
		t.Errorf("body = %q", body)
	}
}

// TestDoRawPOSTWithBody covers a body-bearing POST: Content-Type is set and
// the body bytes are round-tripped.
func TestDoRawPOSTWithBody(t *testing.T) {
	t.Parallel()
	_, c := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		if ct := r.Header.Get("content-type"); ct != "application/json" {
			t.Errorf("content-type = %q", ct)
		}
		got := drainBody(t, r)
		if string(got) != `{"x":1}` {
			t.Errorf("body = %q", got)
		}
		w.WriteHeader(200)
		w.Write(got) // echo
	})
	status, body, err := c.DoRaw(context.Background(), http.MethodPost, "/rpc/foo", []byte(`{"x":1}`))
	if err != nil {
		t.Fatalf("DoRaw: %v", err)
	}
	if status != 200 || string(body) != `{"x":1}` {
		t.Fatalf("status=%d body=%q", status, body)
	}
}

// TestDoRawNon2xxPassthrough: the body is returned intact on a 4xx/5xx; no
// error-envelope parsing happens at this layer (DoRaw's whole job).
func TestDoRawNon2xxPassthrough(t *testing.T) {
	t.Parallel()
	_, c := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(404)
		w.Write([]byte(`{"code":"not_found","message":"gone"}`))
	})
	status, body, err := c.DoRaw(context.Background(), http.MethodGet, "/missing", nil)
	if err != nil {
		t.Fatalf("DoRaw: %v", err)
	}
	if status != 404 {
		t.Errorf("status = %d", status)
	}
	if string(body) != `{"code":"not_found","message":"gone"}` {
		t.Errorf("body = %q", body)
	}
}

// TestDoRawAuthApplied: proves the middleware attaches bearer auth to DoRaw
// requests too — the whole point of the refactor.
func TestDoRawAuthApplied(t *testing.T) {
	t.Parallel()
	_, c := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer tok" {
			t.Errorf("Authorization = %q", got)
		}
		w.Write([]byte("{}"))
	})
	c.tokenFn = staticToken("tok")
	if _, _, err := c.DoRaw(context.Background(), http.MethodGet, "/", nil); err != nil {
		t.Fatalf("DoRaw: %v", err)
	}
}

// TestDoRawBuildRequestError covers the http.NewRequestWithContext failure
// branch — a control-byte URL is rejected by stdlib.
func TestDoRawBuildRequestError(t *testing.T) {
	t.Parallel()
	c := New("http://\x7f/bad", nil)
	_, _, err := c.DoRaw(context.Background(), http.MethodGet, "/x", nil)
	if err == nil || !strings.Contains(err.Error(), "build request") {
		t.Fatalf("want build request error, got %v", err)
	}
}

// TestDoRawTransportError covers the non-context transport error branch.
func TestDoRawTransportError(t *testing.T) {
	t.Parallel()
	want := errors.New("dial fail")
	c := New("http://example.invalid", nil,
		WithHTTPClient(&http.Client{Transport: doErrRT{err: want}}))
	_, _, err := c.DoRaw(context.Background(), http.MethodGet, "/", nil)
	if err == nil || !errors.Is(err, want) {
		t.Fatalf("want wrapped %v, got %v", want, err)
	}
}

// TestDoRawContextCancel covers the ctx-cancelled branch of DoRaw.
func TestDoRawContextCancel(t *testing.T) {
	t.Parallel()
	block := make(chan struct{})
	_, c := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		<-block
	})
	t.Cleanup(func() { close(block) })
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		_, _, err := c.DoRaw(ctx, http.MethodGet, "/", nil)
		done <- err
	}()
	time.Sleep(20 * time.Millisecond)
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("want context.Canceled, got %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("DoRaw did not return after cancel")
	}
}

// TestDoRawReadBodyError covers the io.ReadAll failure path on a 200 body.
func TestDoRawReadBodyError(t *testing.T) {
	t.Parallel()
	c := New("http://example.invalid", nil,
		WithHTTPClient(&http.Client{Transport: readErrRT{}}))
	status, _, err := c.DoRaw(context.Background(), http.MethodGet, "/", nil)
	if err == nil || !strings.Contains(err.Error(), "read response") {
		t.Fatalf("want read response error, got %v", err)
	}
	if status != 200 {
		t.Errorf("status = %d, want 200 (surfaced even on read err)", status)
	}
}

// TestDoRawTokenError covers the tokenFn-error branch propagating through the
// middleware → http.Client.Do → DoRaw wrap.
func TestDoRawTokenError(t *testing.T) {
	t.Parallel()
	tokenErr := errors.New("no creds")
	_, c := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("server should not have been called")
	})
	c.tokenFn = func(context.Context) (string, error) { return "", tokenErr }
	_, _, err := c.DoRaw(context.Background(), http.MethodGet, "/", nil)
	if err == nil || !errors.Is(err, tokenErr) {
		t.Fatalf("want wrapped %v, got %v", tokenErr, err)
	}
}
