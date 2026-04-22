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

// downloadFile streams url into dst and returns the hex sha256 of the body
// (computed via TeeReader so the archive doesn't need to be re-read for
// verification). Close-on-error is wired via named return so a failed flush
// surfaces the real cause instead of producing a silently-truncated archive.
func downloadFile(ctx context.Context, client HTTPDoer, url, dst string) (sha string, err error) {
	body, err := httpGet(ctx, client, url)
	if err != nil {
		return "", err
	}
	defer body.Close()
	f, err := os.Create(dst)
	if err != nil {
		return "", fmt.Errorf("update: create %s: %w", dst, err)
	}
	defer func() {
		if cerr := f.Close(); err == nil {
			err = cerr
		}
	}()
	h := sha256.New()
	if _, err = io.Copy(f, io.TeeReader(body, h)); err != nil {
		return "", fmt.Errorf("update: write %s: %w", dst, err)
	}
	return hex.EncodeToString(h.Sum(nil)), nil
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

// verifyChecksum compares got against the entry for archiveName in a
// goreleaser-format checksums.txt ("<hex>  <filename>" or sha256sum's
// "<hex> *<filename>" binary-mode variant). Caller computes got — typically
// via downloadFile's TeeReader output.
func verifyChecksum(got, archiveName string, checksums []byte) error {
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
	if got != want {
		return fmt.Errorf("update: checksum mismatch for %s: expected %s, got %s", archiveName, want, got)
	}
	return nil
}
