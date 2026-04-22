package update

import (
	"archive/zip"
	"bytes"
	"compress/gzip"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestExtractFromTarGz(t *testing.T) {
	t.Parallel()
	t.Run("open error", func(t *testing.T) {
		wantErr(t, extractFromTarGz(filepath.Join(t.TempDir(), "nope"), "ana", "/tmp/out"), "open")
	})
	t.Run("broken tar inside valid gzip", func(t *testing.T) {
		var buf bytes.Buffer
		gz := gzip.NewWriter(&buf)
		_, _ = gz.Write([]byte("garbage"))
		_ = gz.Close()
		path := filepath.Join(t.TempDir(), "broken.tar.gz")
		if err := os.WriteFile(path, buf.Bytes(), 0o600); err != nil {
			t.Fatalf("seed: %v", err)
		}
		wantErr(t, extractFromTarGz(path, "ana", filepath.Join(t.TempDir(), "out")), "")
	})
}

func TestExtractFromZip(t *testing.T) {
	t.Parallel()
	t.Run("open error", func(t *testing.T) {
		wantErr(t, extractFromZip(filepath.Join(t.TempDir(), "nope"), "ana.exe", "/tmp/out"), "zip")
	})
	t.Run("unsupported method", func(t *testing.T) {
		// Writer claims method 99 (registered as a nop); reader has no
		// matching decompressor, so zf.Open fails on that entry.
		var buf bytes.Buffer
		zw := zip.NewWriter(&buf)
		zw.RegisterCompressor(99, func(w io.Writer) (io.WriteCloser, error) {
			return nopWriteCloser{Writer: w}, nil
		})
		fh := &zip.FileHeader{Name: "ana.exe", Method: 99}
		w, err := zw.CreateHeader(fh)
		if err != nil {
			t.Fatalf("create header: %v", err)
		}
		if _, err := w.Write([]byte("payload")); err != nil {
			t.Fatalf("write: %v", err)
		}
		if err := zw.Close(); err != nil {
			t.Fatalf("close: %v", err)
		}
		archive := filepath.Join(t.TempDir(), "bad.zip")
		if err := os.WriteFile(archive, buf.Bytes(), 0o600); err != nil {
			t.Fatalf("seed: %v", err)
		}
		wantErr(t, extractFromZip(archive, "ana.exe", filepath.Join(t.TempDir(), "out")), "open")
	})
}

// TestExtract_PathTraversalSafe proves that a malicious archive entry
// named `../../evil/ana` still lands at the caller-provided `dst` and
// does NOT write outside dst's parent. Regression guard in case a future
// refactor swaps `filepath.Base` for raw `h.Name`.
func TestExtract_PathTraversalSafe(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name, ext, member string
	}{
		{"tar.gz", "tar.gz", "../../../evil/ana"},
		{"zip", "zip", "../../../evil/ana.exe"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			archive := filepath.Join(dir, "a")
			if err := os.WriteFile(archive, fakeArchive(t, tc.ext, tc.member, []byte("PAYLOAD")), 0o600); err != nil {
				t.Fatalf("seed: %v", err)
			}
			dst := filepath.Join(dir, "dst")
			target := filepath.Base(tc.member) // "ana" or "ana.exe"
			var err error
			if tc.ext == "zip" {
				err = extractFromZip(archive, target, dst)
			} else {
				err = extractFromTarGz(archive, target, dst)
			}
			if err != nil {
				t.Fatalf("extract: %v", err)
			}
			if got, _ := os.ReadFile(dst); string(got) != "PAYLOAD" {
				t.Fatalf("dst content = %q", got)
			}
			// Nothing outside dir (the archive parent) should have been written.
			entries, _ := os.ReadDir(filepath.Dir(dir))
			for _, e := range entries {
				if e.Name() == "evil" {
					t.Fatalf("path traversal wrote outside: %s", e.Name())
				}
			}
		})
	}
}

func TestWriteBinary(t *testing.T) {
	t.Parallel()
	t.Run("create error", func(t *testing.T) {
		wantErr(t, writeBinary(filepath.Join(t.TempDir(), "nope", "out"), strings.NewReader("x")), "create")
	})
	t.Run("copy error", func(t *testing.T) {
		wantErr(t, writeBinary(filepath.Join(t.TempDir(), "out"), errReader{err: errors.New("rd")}), "rd")
	})
}
