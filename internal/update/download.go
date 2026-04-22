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

func downloadFile(ctx context.Context, client HTTPDoer, url, dst string) error {
	body, err := httpGet(ctx, client, url)
	if err != nil {
		return err
	}
	defer body.Close()
	f, err := os.Create(dst)
	if err != nil {
		return fmt.Errorf("update: create %s: %w", dst, err)
	}
	defer f.Close()
	if _, err := io.Copy(f, body); err != nil {
		return fmt.Errorf("update: write %s: %w", dst, err)
	}
	return nil
}

func downloadBody(ctx context.Context, client HTTPDoer, url string) ([]byte, error) {
	body, err := httpGet(ctx, client, url)
	if err != nil {
		return nil, err
	}
	defer body.Close()
	return io.ReadAll(body)
}

// verifyChecksum compares the sha256 of archivePath against the matching
// entry in a goreleaser-format checksums.txt ("<hex>  <filename>").
func verifyChecksum(archivePath, archiveName string, checksums []byte) error {
	want := ""
	for _, line := range strings.Split(string(checksums), "\n") {
		fields := strings.Fields(line)
		if len(fields) == 2 && fields[1] == archiveName {
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
