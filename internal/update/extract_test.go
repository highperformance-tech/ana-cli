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

func TestWriteBinary(t *testing.T) {
	t.Parallel()
	t.Run("create error", func(t *testing.T) {
		wantErr(t, writeBinary(filepath.Join(t.TempDir(), "nope", "out"), strings.NewReader("x")), "create")
	})
	t.Run("copy error", func(t *testing.T) {
		wantErr(t, writeBinary(filepath.Join(t.TempDir(), "out"), errReader{err: errors.New("rd")}), "rd")
	})
}
