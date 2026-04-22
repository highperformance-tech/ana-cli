package update

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestParseInterval(t *testing.T) {
	t.Parallel()
	cases := []struct {
		in     *string
		wantD  time.Duration
		wantEn bool
	}{
		{nil, defaultCheckInterval, true},
		{ptr("4h"), 4 * time.Hour, true},
		{ptr("30m"), 30 * time.Minute, true},
		{ptr("0"), 0, false},
		{ptr("disable"), 0, false},
		{ptr("  DISABLE  "), 0, false},
		{ptr("garbage"), defaultCheckInterval, true},
		{ptr("-5m"), defaultCheckInterval, true},
	}
	for _, tc := range cases {
		d, en := ParseInterval(tc.in)
		if d != tc.wantD || en != tc.wantEn {
			t.Errorf("ParseInterval(%v) = (%v,%v), want (%v,%v)", tc.in, d, en, tc.wantD, tc.wantEn)
		}
	}
}

func TestCachePath(t *testing.T) {
	t.Parallel()
	cases := []struct {
		env  map[string]string
		want string
		err  bool
	}{
		{map[string]string{"XDG_CACHE_HOME": "/xdg", "HOME": "/h"}, filepath.Join("/xdg", "ana", "update-check.json"), false},
		{map[string]string{"HOME": "/h"}, filepath.Join("/h", ".cache", "ana", "update-check.json"), false},
		{nil, "", true},
	}
	for _, tc := range cases {
		got, err := CachePath(mapEnv(tc.env))
		if (err != nil) != tc.err || got != tc.want {
			t.Errorf("CachePath(%v) = (%q,%v); want (%q,err=%v)", tc.env, got, err, tc.want, tc.err)
		}
	}
}

func TestLatestRelease(t *testing.T) {
	cases := []struct {
		name       string
		handler    http.HandlerFunc
		doer       HTTPDoer
		badBaseURL bool
		wantTag    string
		wantErr    string
	}{
		{
			name: "happy",
			handler: func(w http.ResponseWriter, r *http.Request) {
				if !strings.Contains(r.Header.Get("Accept"), "vnd.github") {
					t.Errorf("missing Accept header")
				}
				_ = json.NewEncoder(w).Encode(map[string]string{"tag_name": "v1.2.3"})
			},
			wantTag: "v1.2.3",
		},
		{
			name:    "non-200",
			handler: func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(503) },
			wantErr: "503",
		},
		{
			name:    "bad json",
			handler: func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte("not json")) },
			wantErr: "decode",
		},
		{
			name: "empty tag",
			handler: func(w http.ResponseWriter, _ *http.Request) {
				_ = json.NewEncoder(w).Encode(map[string]string{"tag_name": ""})
			},
			wantErr: "empty tag_name",
		},
		{
			name:    "do error",
			doer:    &fakeDoer{handler: func(*http.Request) (*http.Response, error) { return nil, errors.New("net down") }},
			wantErr: "net down",
		},
		{
			name:       "build request error",
			badBaseURL: true,
			wantErr:    "build request",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var client HTTPDoer = http.DefaultClient
			if tc.handler != nil {
				srv := httptest.NewServer(tc.handler)
				defer srv.Close()
				prev := latestReleaseURL
				latestReleaseURL = srv.URL
				defer func() { latestReleaseURL = prev }()
			}
			if tc.doer != nil {
				client = tc.doer
			}
			if tc.badBaseURL {
				prev := latestReleaseURL
				latestReleaseURL = "http://\x7f"
				defer func() { latestReleaseURL = prev }()
			}
			tag, err := LatestRelease(context.Background(), client)
			if tc.wantErr != "" {
				wantErr(t, err, tc.wantErr)
				return
			}
			if err != nil {
				t.Fatalf("err: %v", err)
			}
			if tag != tc.wantTag {
				t.Fatalf("tag = %q, want %q", tag, tc.wantTag)
			}
		})
	}
}

func TestCachedCheck(t *testing.T) {
	freshServerTag := "v1.5.0"
	serveFresh := func(t *testing.T) *httptest.Server {
		return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			_ = json.NewEncoder(w).Encode(map[string]string{"tag_name": freshServerTag})
		}))
	}

	t.Run("fresh hit: no HTTP, notify when behind", func(t *testing.T) {
		dir := t.TempDir()
		cachePath := filepath.Join(dir, "ana", "update-check.json")
		if err := writeCache(cachePath, cacheFile{CheckedAt: time.Now(), LatestTag: "v2.0.0"}); err != nil {
			t.Fatalf("seed: %v", err)
		}
		doer := &fakeDoer{handler: func(*http.Request) (*http.Response, error) {
			t.Fatal("HTTP should not be called")
			return nil, nil
		}}
		tag, notify, err := CachedCheck(context.Background(), CacheDeps{
			Env:  mapEnv(map[string]string{"XDG_CACHE_HOME": dir}),
			Now:  time.Now,
			HTTP: doer,
		}, time.Hour, "1.0.0")
		if err != nil || tag != "v2.0.0" || !notify || doer.calls != 0 {
			t.Fatalf("tag=%q notify=%v calls=%d err=%v", tag, notify, doer.calls, err)
		}
	})

	t.Run("stale refresh rewrites cache", func(t *testing.T) {
		srv := serveFresh(t)
		defer srv.Close()
		prev := latestReleaseURL
		latestReleaseURL = srv.URL
		defer func() { latestReleaseURL = prev }()

		dir := t.TempDir()
		cachePath := filepath.Join(dir, "ana", "update-check.json")
		if err := writeCache(cachePath, cacheFile{CheckedAt: time.Now().Add(-time.Hour), LatestTag: "v1.0.0"}); err != nil {
			t.Fatalf("seed: %v", err)
		}
		now := time.Now()
		tag, notify, err := CachedCheck(context.Background(), CacheDeps{
			Env:  mapEnv(map[string]string{"XDG_CACHE_HOME": dir}),
			Now:  func() time.Time { return now },
			HTTP: http.DefaultClient,
		}, time.Second, "1.0.0")
		if err != nil || tag != freshServerTag || !notify {
			t.Fatalf("tag=%q notify=%v err=%v", tag, notify, err)
		}
		c, ok := readCache(cachePath)
		if !ok || c.LatestTag != freshServerTag || !c.CheckedAt.Equal(now) {
			t.Fatalf("cache not rewritten: %+v ok=%v", c, ok)
		}
	})

	t.Run("first run writes cache + notifies", func(t *testing.T) {
		srv := serveFresh(t)
		defer srv.Close()
		prev := latestReleaseURL
		latestReleaseURL = srv.URL
		defer func() { latestReleaseURL = prev }()

		dir := t.TempDir()
		_, notify, err := CachedCheck(context.Background(), CacheDeps{
			Env:  mapEnv(map[string]string{"XDG_CACHE_HOME": dir}),
			Now:  time.Now,
			HTTP: http.DefaultClient,
		}, time.Hour, "0.0.9")
		if err != nil || !notify {
			t.Fatalf("notify=%v err=%v", notify, err)
		}
		if _, ok := readCache(filepath.Join(dir, "ana", "update-check.json")); !ok {
			t.Fatal("cache not written")
		}
	})

	t.Run("current equal or ahead → no notify", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			_ = json.NewEncoder(w).Encode(map[string]string{"tag_name": "v1.0.0"})
		}))
		defer srv.Close()
		prev := latestReleaseURL
		latestReleaseURL = srv.URL
		defer func() { latestReleaseURL = prev }()

		for _, cur := range []string{"1.0.0", "2.0.0"} {
			dir := t.TempDir()
			_, notify, err := CachedCheck(context.Background(), CacheDeps{
				Env:  mapEnv(map[string]string{"XDG_CACHE_HOME": dir}),
				Now:  time.Now,
				HTTP: http.DefaultClient,
			}, time.Hour, cur)
			if err != nil {
				t.Fatalf("err: %v", err)
			}
			if notify {
				t.Errorf("cur=%q notify=true; want false", cur)
			}
		}
	})

	t.Run("empty cached tag → no notify", func(t *testing.T) {
		dir := t.TempDir()
		cachePath := filepath.Join(dir, "ana", "update-check.json")
		if err := writeCache(cachePath, cacheFile{CheckedAt: time.Now(), LatestTag: ""}); err != nil {
			t.Fatalf("seed: %v", err)
		}
		_, notify, err := CachedCheck(context.Background(), CacheDeps{
			Env:  mapEnv(map[string]string{"XDG_CACHE_HOME": dir}),
			Now:  time.Now,
			HTTP: &fakeDoer{handler: func(*http.Request) (*http.Response, error) { t.Fatal("no HTTP"); return nil, nil }},
		}, time.Hour, "1.0.0")
		if err != nil || notify {
			t.Fatalf("notify=%v err=%v", notify, err)
		}
	})

	t.Run("path error", func(t *testing.T) {
		_, _, err := CachedCheck(context.Background(), CacheDeps{
			Env:  mapEnv(nil),
			Now:  time.Now,
			HTTP: http.DefaultClient,
		}, time.Hour, "1.0.0")
		wantErr(t, err, "neither XDG_CACHE_HOME")
	})

	t.Run("latest release error", func(t *testing.T) {
		doer := &fakeDoer{handler: func(*http.Request) (*http.Response, error) { return nil, errors.New("offline") }}
		dir := t.TempDir()
		_, notify, err := CachedCheck(context.Background(), CacheDeps{
			Env:  mapEnv(map[string]string{"XDG_CACHE_HOME": dir}),
			Now:  time.Now,
			HTTP: doer,
		}, time.Hour, "1.0.0")
		wantErr(t, err, "offline")
		if notify {
			t.Error("notify should be false on error")
		}
	})

	t.Run("write cache error still returns fetched tag", func(t *testing.T) {
		dir := t.TempDir()
		// Pre-create a directory at the .tmp path so WriteFile can't create the file.
		cachePath := filepath.Join(dir, "ana", "update-check.json")
		if err := os.MkdirAll(cachePath+".tmp", 0o700); err != nil {
			t.Fatalf("seed: %v", err)
		}
		srv := serveFresh(t)
		defer srv.Close()
		prev := latestReleaseURL
		latestReleaseURL = srv.URL
		defer func() { latestReleaseURL = prev }()

		tag, notify, err := CachedCheck(context.Background(), CacheDeps{
			Env:  mapEnv(map[string]string{"XDG_CACHE_HOME": dir}),
			Now:  time.Now,
			HTTP: http.DefaultClient,
		}, time.Hour, "1.0.0")
		if err == nil {
			t.Fatal("expected write error")
		}
		if tag != freshServerTag || !notify {
			t.Fatalf("tag/notify should reflect fetched value: tag=%q notify=%v", tag, notify)
		}
	})
}

func TestReadCache_Corrupt(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "c.json")
	if err := os.WriteFile(path, []byte("{not json"), 0o600); err != nil {
		t.Fatalf("seed: %v", err)
	}
	if _, ok := readCache(path); ok {
		t.Fatal("corrupt cache should yield ok=false")
	}
}

func TestWriteCache_Errors(t *testing.T) {
	t.Parallel()
	t.Run("mkdir", func(t *testing.T) {
		dir := t.TempDir()
		blocker := filepath.Join(dir, "blocker")
		if err := os.WriteFile(blocker, []byte("x"), 0o600); err != nil {
			t.Fatalf("seed: %v", err)
		}
		wantErr(t, writeCache(filepath.Join(blocker, "sub", "c.json"), cacheFile{}), "mkdir")
	})
	t.Run("write", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "c.json")
		if err := os.MkdirAll(path+".tmp", 0o700); err != nil {
			t.Fatalf("seed: %v", err)
		}
		wantErr(t, writeCache(path, cacheFile{}), "write")
	})
	t.Run("rename", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "c.json")
		if err := os.MkdirAll(path, 0o700); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		if err := os.WriteFile(filepath.Join(path, "x"), []byte("x"), 0o600); err != nil {
			t.Fatalf("populate: %v", err)
		}
		wantErr(t, writeCache(path, cacheFile{}), "rename")
	})
}
