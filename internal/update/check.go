package update

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// defaultCheckInterval is the cadence of the passive update nudge. 4h
// matches the issue's ask — short enough to notice a fresh release within a
// workday, long enough that CI loops don't spam api.github.com.
const defaultCheckInterval = 4 * time.Hour

// LatestRelease returns the `tag_name` from GitHub's /releases/latest.
func LatestRelease(ctx context.Context, client HTTPDoer) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, latestReleaseURL, nil)
	if err != nil {
		return "", fmt.Errorf("update: build request: %w", err)
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("update: fetch latest release: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("update: fetch latest release: status %d", resp.StatusCode)
	}
	var body struct {
		TagName string `json:"tag_name"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return "", fmt.Errorf("update: decode latest release: %w", err)
	}
	if body.TagName == "" {
		return "", errors.New("update: empty tag_name in response")
	}
	return body.TagName, nil
}

// CacheDeps is the injection boundary for CachedCheck.
type CacheDeps struct {
	Env  func(string) string
	Now  func() time.Time
	HTTP HTTPDoer
}

// cacheFile carries the last-observed release so fresh-cache hits can
// still decide whether to nudge without another HTTP call.
type cacheFile struct {
	CheckedAt time.Time `json:"checkedAt"`
	LatestTag string    `json:"latestTag"`
}

// CachePath resolves $XDG_CACHE_HOME/ana/update-check.json with a
// $HOME/.cache/... fallback; both unset → error so callers skip the check.
func CachePath(env func(string) string) (string, error) {
	if xdg := env("XDG_CACHE_HOME"); xdg != "" {
		return filepath.Join(xdg, "ana", "update-check.json"), nil
	}
	if home := env("HOME"); home != "" {
		return filepath.Join(home, ".cache", "ana", "update-check.json"), nil
	}
	return "", errors.New("update: neither XDG_CACHE_HOME nor HOME is set")
}

// ParseInterval interprets the Config.UpdateCheckInterval pointer: nil →
// (4h, true). "0" or "disable" → (0, false). Any time.ParseDuration-friendly
// string → (d, true). A malformed duration silently falls back to the
// default — we never want a bad config value to block the user's verb.
func ParseInterval(s *string) (time.Duration, bool) {
	if s == nil {
		return defaultCheckInterval, true
	}
	v := strings.TrimSpace(*s)
	if v == "0" || strings.EqualFold(v, "disable") {
		return 0, false
	}
	d, err := time.ParseDuration(v)
	if err != nil || d <= 0 {
		return defaultCheckInterval, true
	}
	return d, true
}

// CachedCheck returns the newest observed release tag plus a notify flag that
// is true only when currentVersion is older than that tag. When the cache is
// fresh (age < ttl) no HTTP call is made. A stale or missing cache triggers a
// LatestRelease call and an atomic cache rewrite. Any error short-circuits
// to (_, false, err); callers ignore the error since the nudge is best-effort.
func CachedCheck(ctx context.Context, deps CacheDeps, ttl time.Duration, currentVersion string) (string, bool, error) {
	now := deps.Now()
	path, err := CachePath(deps.Env)
	if err != nil {
		return "", false, err
	}
	if cached, ok := readCache(path); ok && now.Sub(cached.CheckedAt) < ttl {
		return cached.LatestTag, shouldNotify(currentVersion, cached.LatestTag), nil
	}
	tag, err := LatestRelease(ctx, deps.HTTP)
	if err != nil {
		return "", false, err
	}
	if werr := writeCache(path, cacheFile{CheckedAt: now, LatestTag: tag}); werr != nil {
		// Cache-write failure doesn't invalidate the tag we just fetched;
		// worst case the next invocation refetches. Still surface it so
		// callers can log.
		return tag, shouldNotify(currentVersion, tag), werr
	}
	return tag, shouldNotify(currentVersion, tag), nil
}

// shouldNotify reports true only when tag strictly exceeds currentVersion.
func shouldNotify(currentVersion, tag string) bool {
	if tag == "" {
		return false
	}
	return CmpSemver(currentVersion, strings.TrimPrefix(tag, "v")) < 0
}

// readCache returns ok=false on missing/corrupt — the first-run path.
func readCache(path string) (cacheFile, bool) {
	data, err := os.ReadFile(path)
	if err != nil {
		return cacheFile{}, false
	}
	var c cacheFile
	if err := json.Unmarshal(data, &c); err != nil {
		return cacheFile{}, false
	}
	return c, true
}

// writeCache atomically writes the cache file with 0700 dir + 0600 file
// perms — same pattern as config.Save so permissions stay consistent across
// ana's on-disk state.
func writeCache(path string, c cacheFile) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("update: mkdir %s: %w", dir, err)
	}
	data, _ := json.Marshal(c)
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return fmt.Errorf("update: write %s: %w", tmp, err)
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("update: rename %s -> %s: %w", tmp, path, err)
	}
	return nil
}
