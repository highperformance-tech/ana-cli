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
	t.Run("build request error", func(t *testing.T) {
		wantErr(t, downloadFile(context.Background(), http.DefaultClient, "http://\x7f", filepath.Join(t.TempDir(), "x")), "build request")
	})
	t.Run("do error", func(t *testing.T) {
		doer := &fakeDoer{handler: func(*http.Request) (*http.Response, error) { return nil, errors.New("dial") }}
		wantErr(t, downloadFile(context.Background(), doer, "http://x", filepath.Join(t.TempDir(), "x")), "dial")
	})
	t.Run("create error", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte("ok")) }))
		defer srv.Close()
		wantErr(t, downloadFile(context.Background(), http.DefaultClient, srv.URL, filepath.Join(t.TempDir(), "nope", "x")), "create")
	})
	t.Run("copy error", func(t *testing.T) {
		doer := &fakeDoer{handler: func(*http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: 200, Body: errBody{err: errors.New("body boom")}, Header: make(http.Header)}, nil
		}}
		wantErr(t, downloadFile(context.Background(), doer, "http://x", filepath.Join(t.TempDir(), "x")), "body boom")
	})
}

func TestDownloadBody_DoError(t *testing.T) {
	t.Parallel()
	doer := &fakeDoer{handler: func(*http.Request) (*http.Response, error) { return nil, errors.New("dial") }}
	_, err := downloadBody(context.Background(), doer, "http://x")
	wantErr(t, err, "dial")
}

func TestVerifyChecksum_OpenError(t *testing.T) {
	t.Parallel()
	wantErr(t, verifyChecksum(filepath.Join(t.TempDir(), "nope"), "a", []byte("dead  a\n")), "open")
}
