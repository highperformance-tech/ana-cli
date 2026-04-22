package update

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
)

func TestDownloadFile(t *testing.T) {
	t.Parallel()
	call := func(t *testing.T, doer HTTPDoer, url, dst string) error {
		t.Helper()
		_, err := downloadFile(context.Background(), doer, url, dst)
		return err
	}
	t.Run("build request error", func(t *testing.T) {
		wantErr(t, call(t, http.DefaultClient, "http://\x7f", filepath.Join(t.TempDir(), "x")), "build request")
	})
	t.Run("do error", func(t *testing.T) {
		doer := &fakeDoer{handler: func(*http.Request) (*http.Response, error) { return nil, errors.New("dial") }}
		wantErr(t, call(t, doer, "http://x", filepath.Join(t.TempDir(), "x")), "dial")
	})
	t.Run("create error", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte("ok")) }))
		defer srv.Close()
		wantErr(t, call(t, http.DefaultClient, srv.URL, filepath.Join(t.TempDir(), "nope", "x")), "create")
	})
	t.Run("copy error", func(t *testing.T) {
		doer := &fakeDoer{handler: func(*http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: 200, Body: errBody{err: errors.New("body boom")}, Header: make(http.Header)}, nil
		}}
		wantErr(t, call(t, doer, "http://x", filepath.Join(t.TempDir(), "x")), "body boom")
	})
}

func TestDownloadBody_DoError(t *testing.T) {
	t.Parallel()
	doer := &fakeDoer{handler: func(*http.Request) (*http.Response, error) { return nil, errors.New("dial") }}
	_, err := downloadBody(context.Background(), doer, "http://x")
	wantErr(t, err, "dial")
}

func TestVerifyChecksum(t *testing.T) {
	t.Parallel()
	t.Run("no entry", func(t *testing.T) {
		wantErr(t, verifyChecksum("deadbeef", "a.tar.gz", []byte("dead  other.tar.gz\n")), "no checksum entry")
	})
	t.Run("binary-mode filename prefix accepted", func(t *testing.T) {
		// Reaching "checksum mismatch" (rather than "no checksum entry")
		// proves the `*` prefix was stripped and the name match succeeded.
		wantErr(t, verifyChecksum("cafef00d", "a.tar.gz", []byte("deadbeef *a.tar.gz\n")), "checksum mismatch")
	})
	t.Run("happy path: match", func(t *testing.T) {
		if err := verifyChecksum("deadbeef", "a.tar.gz", []byte("deadbeef  a.tar.gz\n")); err != nil {
			t.Fatalf("err: %v", err)
		}
	})
}
