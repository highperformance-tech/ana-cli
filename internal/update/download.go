package update

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
)

// httpGet performs a GET and returns the response body on 200.
func httpGet(ctx context.Context, client HTTPDoer, url string) (io.ReadCloser, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("update: build request: %w", err)
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("update: download %s: %w", url, err)
	}
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return nil, fmt.Errorf("update: download %s: status %d", url, resp.StatusCode)
	}
	return resp.Body, nil
}

func downloadFile(ctx context.Context, client HTTPDoer, url, dst string) (err error) {
	body, err := httpGet(ctx, client, url)
	if err != nil {
		return err
	}
	defer body.Close()
	f, err := os.Create(dst)
	if err != nil {
		return fmt.Errorf("update: create %s: %w", dst, err)
	}
	// Capture Close error via named return: a failed flush on a buffered fs
	// (NFS, disk full) can silently truncate the downloaded archive and then
	// surface as a bogus checksum mismatch. Surface the real cause instead.
	defer func() {
		if cerr := f.Close(); err == nil {
			err = cerr
		}
	}()
	if _, err := io.Copy(f, body); err != nil {
		return fmt.Errorf("update: write %s: %w", dst, err)
	}
	return nil
}

// maxChecksumsSize caps the checksums.txt buffer so a hostile / broken CDN
// can't force us to allocate arbitrarily large memory. A goreleaser
// checksums.txt for this project is well under 4 KB in practice; 1 MiB is
// generous.
const maxChecksumsSize = 1 << 20

func downloadBody(ctx context.Context, client HTTPDoer, url string) ([]byte, error) {
	body, err := httpGet(ctx, client, url)
	if err != nil {
		return nil, err
	}
	defer body.Close()
	return io.ReadAll(io.LimitReader(body, maxChecksumsSize))
}

// verifyChecksum compares the sha256 of archivePath against the matching
// entry in a goreleaser-format checksums.txt ("<hex>  <filename>").
func verifyChecksum(archivePath, archiveName string, checksums []byte) error {
	// goreleaser emits "<hex>  <filename>"; sha256sum's binary-mode format is
	// "<hex> *<filename>". Accept either, and allow extra whitespace so any
	// future format tweak that keeps hex-first/name-last keeps working.
	want := ""
	for _, line := range strings.Split(string(checksums), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		name := strings.TrimPrefix(fields[len(fields)-1], "*")
		if name == archiveName {
			want = fields[0]
			break
		}
	}
	if want == "" {
		return fmt.Errorf("update: no checksum entry for %s", archiveName)
	}
	data, err := os.ReadFile(archivePath)
	if err != nil {
		return fmt.Errorf("update: open %s: %w", archivePath, err)
	}
	sum := sha256.Sum256(data)
	got := hex.EncodeToString(sum[:])
	if got != want {
		return fmt.Errorf("update: checksum mismatch for %s: expected %s, got %s", archiveName, want, got)
	}
	return nil
}
