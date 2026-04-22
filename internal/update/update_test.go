package update

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Shared test helpers used across semver_test / check_test / selfupdate_test
// / download_test / extract_test. Per the project convention, helpers used by
// more than one test file live in <pkg>_test.go.

type fakeDoer struct {
	handler func(*http.Request) (*http.Response, error)
	calls   int
}

func (f *fakeDoer) Do(r *http.Request) (*http.Response, error) {
	f.calls++
	return f.handler(r)
}

type errReader struct{ err error }

func (r errReader) Read([]byte) (int, error) { return 0, r.err }

type errBody struct{ err error }

func (b errBody) Read([]byte) (int, error) { return 0, b.err }
func (b errBody) Close() error             { return nil }

type nopWriteCloser struct{ io.Writer }

func (nopWriteCloser) Close() error { return nil }

func ptr(s string) *string { return &s }

func mapEnv(m map[string]string) func(string) string {
	return func(k string) string { return m[k] }
}

func sha256Hex(b []byte) string {
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}

// withURLs points both package-level URL vars at a test server. Tests that
// use it MUST NOT call t.Parallel() — they share global state.
func withURLs(t *testing.T, srv *httptest.Server) {
	t.Helper()
	prevLatest, prevBase := latestReleaseURL, releasesBaseURL
	latestReleaseURL = srv.URL + "/api/releases/latest"
	releasesBaseURL = srv.URL + "/releases"
	t.Cleanup(func() {
		latestReleaseURL = prevLatest
		releasesBaseURL = prevBase
	})
}

// fakeArchive builds a single-member tar.gz or zip in memory.
func fakeArchive(t *testing.T, ext, name string, payload []byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	switch ext {
	case "tar.gz":
		gz := gzip.NewWriter(&buf)
		tw := tar.NewWriter(gz)
		if err := tw.WriteHeader(&tar.Header{Name: name, Mode: 0o755, Size: int64(len(payload))}); err != nil {
			t.Fatalf("tar header: %v", err)
		}
		if _, err := tw.Write(payload); err != nil {
			t.Fatalf("tar write: %v", err)
		}
		if err := tw.Close(); err != nil {
			t.Fatalf("tar close: %v", err)
		}
		if err := gz.Close(); err != nil {
			t.Fatalf("gz close: %v", err)
		}
	case "zip":
		zw := zip.NewWriter(&buf)
		f, err := zw.Create(name)
		if err != nil {
			t.Fatalf("zip create: %v", err)
		}
		if _, err := f.Write(payload); err != nil {
			t.Fatalf("zip write: %v", err)
		}
		if err := zw.Close(); err != nil {
			t.Fatalf("zip close: %v", err)
		}
	}
	return buf.Bytes()
}

// releaseServer holds the three routes the update flow expects. Zero-value
// fields use sensible defaults; tests override individual fields to simulate
// specific failure modes.
type releaseServer struct {
	tag          string
	archiveName  string
	latestStatus int
	latestBody   string
	archiveBody  []byte
	archive404   bool
	checksums    string
	checksums404 bool
}

func (r *releaseServer) serve(t *testing.T) *httptest.Server {
	t.Helper()
	sum := sha256Hex(r.archiveBody)
	if r.checksums == "" {
		r.checksums = fmt.Sprintf("%s  %s\n", sum, r.archiveName)
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/api/releases/latest", func(w http.ResponseWriter, _ *http.Request) {
		if r.latestStatus != 0 {
			w.WriteHeader(r.latestStatus)
		}
		if r.latestBody != "" {
			fmt.Fprint(w, r.latestBody)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]string{"tag_name": r.tag})
	})
	mux.HandleFunc("/releases/"+r.tag+"/"+r.archiveName, func(w http.ResponseWriter, _ *http.Request) {
		if r.archive404 {
			w.WriteHeader(404)
			return
		}
		_, _ = w.Write(r.archiveBody)
	})
	mux.HandleFunc("/releases/"+r.tag+"/checksums.txt", func(w http.ResponseWriter, _ *http.Request) {
		if r.checksums404 {
			w.WriteHeader(404)
			return
		}
		fmt.Fprint(w, r.checksums)
	})
	return httptest.NewServer(mux)
}

// stageUpdate seeds an exe + staging root and returns UpdateDeps wired at
// those paths. Callers mutate the returned struct for per-test tweaks.
func stageUpdate(t *testing.T, goos, goarch, exeName string) (string, UpdateDeps) {
	t.Helper()
	tmp := t.TempDir()
	exePath := filepath.Join(tmp, exeName)
	if err := os.WriteFile(exePath, []byte("old"), 0o755); err != nil {
		t.Fatalf("seed exe: %v", err)
	}
	return exePath, UpdateDeps{
		GOOS:    goos,
		GOARCH:  goarch,
		ExePath: func() (string, error) { return exePath, nil },
		TempDir: func() (string, error) { return os.MkdirTemp(tmp, "stage-*") },
	}
}

// wantErr fatally fails the test when err is nil or (when substr is
// non-empty) when it doesn't contain substr.
func wantErr(t *testing.T, err error, substr string) {
	t.Helper()
	if err == nil {
		t.Fatalf("expected error containing %q, got nil", substr)
	}
	if substr != "" && !strings.Contains(err.Error(), substr) {
		t.Fatalf("err = %v; want substring %q", err, substr)
	}
}
