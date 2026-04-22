package update

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"
)

func TestSelfUpdate_UnixHappyPath(t *testing.T) {
	exePath, deps := stageUpdate(t, "linux", "amd64", "ana")
	r := &releaseServer{tag: "v1.2.3", archiveName: "ana_1.2.3_linux_amd64.tar.gz"}
	r.archiveBody = fakeArchive(t, "tar.gz", "ana", []byte("NEW_BINARY_BYTES"))
	srv := r.serve(t)
	defer srv.Close()
	withURLs(t, srv)

	var rnCalls int
	deps.Rename = func(o, n string) error { rnCalls++; return os.Rename(o, n) }

	var out bytes.Buffer
	if err := SelfUpdate(context.Background(), deps, "1.0.0", &out, false); err != nil {
		t.Fatalf("SelfUpdate: %v", err)
	}
	if got, _ := os.ReadFile(exePath); string(got) != "NEW_BINARY_BYTES" {
		t.Errorf("exe not replaced: %q", got)
	}
	if rnCalls != 1 {
		t.Errorf("expected 1 rename on unix, got %d", rnCalls)
	}
	if !strings.Contains(out.String(), "Updated ana 1.0.0 → 1.2.3") {
		t.Errorf("unexpected stdout: %q", out.String())
	}
}

func TestSelfUpdate_JSONAndUpToDate(t *testing.T) {
	t.Run("updated", func(t *testing.T) {
		_, deps := stageUpdate(t, "linux", "amd64", "ana")
		r := &releaseServer{tag: "v1.2.3", archiveName: "ana_1.2.3_linux_amd64.tar.gz"}
		r.archiveBody = fakeArchive(t, "tar.gz", "ana", []byte("NEW"))
		srv := r.serve(t)
		defer srv.Close()
		withURLs(t, srv)

		var out bytes.Buffer
		if err := SelfUpdate(context.Background(), deps, "1.0.0", &out, true); err != nil {
			t.Fatalf("SelfUpdate: %v", err)
		}
		var got updateStatus
		if err := json.Unmarshal(out.Bytes(), &got); err != nil {
			t.Fatalf("json: %v (%q)", err, out.String())
		}
		if got.Status != "updated" || got.From != "1.0.0" || got.To != "1.2.3" {
			t.Errorf("status: %+v", got)
		}
	})

	for _, jsonOut := range []bool{false, true} {
		t.Run(fmt.Sprintf("up-to-date json=%v", jsonOut), func(t *testing.T) {
			exePath, deps := stageUpdate(t, "linux", "amd64", "ana")
			r := &releaseServer{tag: "v1.0.0", archiveName: "ana_1.0.0_linux_amd64.tar.gz"}
			r.archiveBody = fakeArchive(t, "tar.gz", "ana", []byte("NEW"))
			srv := r.serve(t)
			defer srv.Close()
			withURLs(t, srv)

			var out bytes.Buffer
			if err := SelfUpdate(context.Background(), deps, "1.0.0", &out, jsonOut); err != nil {
				t.Fatalf("SelfUpdate: %v", err)
			}
			if jsonOut {
				var got updateStatus
				if err := json.Unmarshal(out.Bytes(), &got); err != nil {
					t.Fatalf("json: %v", err)
				}
				if got.Status != "up-to-date" {
					t.Errorf("status: %+v", got)
				}
			} else if !strings.Contains(out.String(), "already at version 1.0.0") {
				t.Errorf("unexpected output: %q", out.String())
			}
			if got, _ := os.ReadFile(exePath); string(got) != "old" {
				t.Errorf("exe should not change: %q", got)
			}
		})
	}
}

// TestSelfUpdate_SadPaths table-drives the one-axis failure modes against an
// otherwise-valid release server. Each case overrides a single field to
// inject the specific failure.
func TestSelfUpdate_SadPaths(t *testing.T) {
	validTarGz := fakeArchive(t, "tar.gz", "ana", []byte("NEW"))
	wrongMember := fakeArchive(t, "tar.gz", "README.md", []byte("x"))
	validZip := fakeArchive(t, "zip", "ana.exe", []byte("NEW"))
	zipWrongMember := fakeArchive(t, "zip", "README.md", []byte("x"))
	cases := []struct {
		name      string
		goos      string
		ext       string
		overrides func(r *releaseServer)
		wantErr   string
	}{
		{"archive 404", "", "tar.gz", func(r *releaseServer) { r.archive404 = true }, "status 404"},
		{"checksums 404", "", "tar.gz", func(r *releaseServer) { r.checksums404 = true }, "status 404"},
		{"checksum mismatch", "", "tar.gz", func(r *releaseServer) { r.checksums = "deadbeef  " + r.archiveName + "\n" }, "checksum mismatch"},
		{"no checksum entry", "", "tar.gz", func(r *releaseServer) { r.checksums = "deadbeef  unrelated.tar.gz\n" }, "no checksum entry"},
		{"archive missing member (tar.gz)", "", "tar.gz", func(r *releaseServer) { r.archiveBody = wrongMember }, "missing ana"},
		{"archive missing member (zip)", "windows", "zip", func(r *releaseServer) { r.archiveBody = zipWrongMember }, "missing ana.exe"},
		{"corrupt gzip", "", "tar.gz", func(r *releaseServer) { r.archiveBody = []byte("not a gzip stream") }, "gzip"},
		{"corrupt zip", "windows", "zip", func(r *releaseServer) { r.archiveBody = []byte("not a zip either") }, "zip"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			goos := tc.goos
			if goos == "" {
				goos = "linux"
			}
			exeName := "ana"
			body := validTarGz
			if tc.ext == "zip" {
				exeName = "ana.exe"
				body = validZip
			}
			_, deps := stageUpdate(t, goos, "amd64", exeName)
			r := &releaseServer{
				tag:         "v2.0.0",
				archiveName: fmt.Sprintf("ana_2.0.0_%s_amd64.%s", goos, tc.ext),
				archiveBody: body,
			}
			if tc.overrides != nil {
				tc.overrides(r)
			}
			srv := r.serve(t)
			defer srv.Close()
			withURLs(t, srv)
			wantErr(t, SelfUpdate(context.Background(), deps, "1.0.0", io.Discard, false), tc.wantErr)
		})
	}
}

func TestSelfUpdate_WindowsAtomicReplace(t *testing.T) {
	setup := func(t *testing.T) (string, UpdateDeps, func()) {
		exePath, deps := stageUpdate(t, "windows", "amd64", "ana.exe")
		r := &releaseServer{tag: "v2.0.0", archiveName: "ana_2.0.0_windows_amd64.zip"}
		r.archiveBody = fakeArchive(t, "zip", "ana.exe", []byte("NEW_BINARY_BYTES"))
		srv := r.serve(t)
		withURLs(t, srv)
		return exePath, deps, srv.Close
	}

	t.Run("happy path: rename aside + in, .old cleanup", func(t *testing.T) {
		exePath, deps, done := setup(t)
		defer done()
		if err := os.WriteFile(exePath+".old", []byte("stale"), 0o600); err != nil {
			t.Fatalf("seed .old: %v", err)
		}
		var renames [][2]string
		deps.Rename = func(o, n string) error { renames = append(renames, [2]string{o, n}); return os.Rename(o, n) }

		if err := SelfUpdate(context.Background(), deps, "1.0.0", io.Discard, false); err != nil {
			t.Fatalf("SelfUpdate: %v", err)
		}
		if len(renames) != 2 || renames[0][1] != exePath+".old" || renames[1][1] != exePath {
			t.Fatalf("rename sequence wrong: %v", renames)
		}
		if got, _ := os.ReadFile(exePath); string(got) != "NEW_BINARY_BYTES" {
			t.Errorf("exe not replaced: %q", got)
		}
		// Pre-seeded .old ("stale") was removed; aside-rename then wrote the
		// prior exe ("old") into .old for rollback.
		if got, _ := os.ReadFile(exePath + ".old"); string(got) != "old" {
			t.Errorf(".old content = %q; want \"old\"", got)
		}
	})

	t.Run("rename-in fails: rollback succeeds", func(t *testing.T) {
		exePath, deps, done := setup(t)
		defer done()
		var n int
		deps.Rename = func(o, np string) error {
			n++
			if n == 2 {
				return errors.New("simulated replace failure")
			}
			return os.Rename(o, np)
		}
		wantErr(t, SelfUpdate(context.Background(), deps, "1.0.0", io.Discard, false), "simulated replace failure")
		if got, _ := os.ReadFile(exePath); string(got) != "old" {
			t.Errorf("rollback did not restore: %q", got)
		}
	})

	t.Run("rename-in fails: rollback also fails", func(t *testing.T) {
		_, deps, done := setup(t)
		defer done()
		var n int
		deps.Rename = func(o, np string) error {
			n++
			if n >= 2 {
				return errors.New("all renames fail")
			}
			return os.Rename(o, np)
		}
		wantErr(t, SelfUpdate(context.Background(), deps, "1.0.0", io.Discard, false), "recover from")
	})

	t.Run("rename aside fails", func(t *testing.T) {
		_, deps, done := setup(t)
		defer done()
		deps.Rename = func(string, string) error { return errors.New("locked") }
		wantErr(t, SelfUpdate(context.Background(), deps, "1.0.0", io.Discard, false), "locked")
	})
}

func TestSelfUpdate_EarlyErrors(t *testing.T) {
	t.Run("exe path error", func(t *testing.T) {
		deps := UpdateDeps{ExePath: func() (string, error) { return "", errors.New("no exe") }}
		wantErr(t, SelfUpdate(context.Background(), deps, "1.0.0", io.Discard, false), "no exe")
	})
	t.Run("latest release error", func(t *testing.T) {
		_, deps := stageUpdate(t, "linux", "amd64", "ana")
		deps.HTTP = &fakeDoer{handler: func(*http.Request) (*http.Response, error) { return nil, errors.New("net down") }}
		wantErr(t, SelfUpdate(context.Background(), deps, "1.0.0", io.Discard, false), "net down")
	})
	t.Run("tempdir error", func(t *testing.T) {
		_, deps := stageUpdate(t, "linux", "amd64", "ana")
		deps.TempDir = func() (string, error) { return "", errors.New("tmp down") }
		r := &releaseServer{tag: "v2.0.0", archiveName: "ana_2.0.0_linux_amd64.tar.gz"}
		r.archiveBody = fakeArchive(t, "tar.gz", "ana", []byte("NEW"))
		srv := r.serve(t)
		defer srv.Close()
		withURLs(t, srv)
		wantErr(t, SelfUpdate(context.Background(), deps, "1.0.0", io.Discard, false), "tmp down")
	})
	t.Run("unix rename fails", func(t *testing.T) {
		_, deps := stageUpdate(t, "linux", "amd64", "ana")
		deps.Rename = func(string, string) error { return errors.New("denied") }
		r := &releaseServer{tag: "v2.0.0", archiveName: "ana_2.0.0_linux_amd64.tar.gz"}
		r.archiveBody = fakeArchive(t, "tar.gz", "ana", []byte("NEW"))
		srv := r.serve(t)
		defer srv.Close()
		withURLs(t, srv)
		wantErr(t, SelfUpdate(context.Background(), deps, "1.0.0", io.Discard, false), "denied")
	})
}

func TestDeps(t *testing.T) {
	t.Parallel()
	t.Run("DefaultUpdateDeps populates everything", func(t *testing.T) {
		d := DefaultUpdateDeps()
		if d.HTTP == nil || d.ExePath == nil || d.Rename == nil || d.TempDir == nil || d.GOOS == "" || d.GOARCH == "" {
			t.Fatalf("missing field: %+v", d)
		}
		p, err := d.TempDir()
		if err != nil {
			t.Fatalf("tempdir: %v", err)
		}
		defer os.RemoveAll(p)
	})
	t.Run("resolveDeps zero fills and runs", func(t *testing.T) {
		d := resolveDeps(UpdateDeps{})
		p, err := d.TempDir()
		if err != nil {
			t.Fatalf("tempdir: %v", err)
		}
		defer os.RemoveAll(p)
	})
	t.Run("resolveDeps preserves explicit", func(t *testing.T) {
		d := resolveDeps(UpdateDeps{
			HTTP: http.DefaultClient, GOOS: "plan9", GOARCH: "arm",
			ExePath: func() (string, error) { return "/x", nil },
			Rename:  func(string, string) error { return nil },
			TempDir: func() (string, error) { return "/t", nil },
		})
		if d.GOOS != "plan9" || d.GOARCH != "arm" {
			t.Fatalf("explicit overwritten: %+v", d)
		}
	})
}
