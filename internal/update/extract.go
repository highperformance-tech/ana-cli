package update

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// extractBinary extracts exeName from archivePath to dst (0755).
// archiveExt is "tar.gz" (Unix) or "zip" (Windows).
func extractBinary(archivePath, archiveExt, exeName, dst string) error {
	if archiveExt == "zip" {
		return extractFromZip(archivePath, exeName, dst)
	}
	return extractFromTarGz(archivePath, exeName, dst)
}

func extractFromTarGz(archivePath, exeName, dst string) error {
	f, err := os.Open(archivePath)
	if err != nil {
		return fmt.Errorf("update: open %s: %w", archivePath, err)
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		return fmt.Errorf("update: gzip %s: %w", archivePath, err)
	}
	defer gz.Close()
	tr := tar.NewReader(gz)
	for {
		h, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return fmt.Errorf("update: tar %s: %w", archivePath, err)
		}
		if filepath.Base(h.Name) != exeName {
			continue
		}
		return writeBinary(dst, tr)
	}
	return fmt.Errorf("update: archive %s missing %s", archivePath, exeName)
}

func extractFromZip(archivePath, exeName, dst string) error {
	zr, err := zip.OpenReader(archivePath)
	if err != nil {
		return fmt.Errorf("update: zip %s: %w", archivePath, err)
	}
	defer zr.Close()
	for _, zf := range zr.File {
		if filepath.Base(zf.Name) != exeName {
			continue
		}
		rc, err := zf.Open()
		if err != nil {
			return fmt.Errorf("update: open %s in %s: %w", zf.Name, archivePath, err)
		}
		defer rc.Close()
		return writeBinary(dst, rc)
	}
	return fmt.Errorf("update: archive %s missing %s", archivePath, exeName)
}

// writeBinary copies src to dst with 0755 so the extracted binary is
// immediately executable.
func writeBinary(dst string, src io.Reader) (err error) {
	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o755)
	if err != nil {
		return fmt.Errorf("update: create %s: %w", dst, err)
	}
	// Capture Close error via named return so a failed flush doesn't leave a
	// silently-truncated binary staged for atomicReplace.
	defer func() {
		if cerr := out.Close(); err == nil {
			err = cerr
		}
	}()
	if _, err := io.Copy(out, src); err != nil {
		return fmt.Errorf("update: write %s: %w", dst, err)
	}
	return nil
}
