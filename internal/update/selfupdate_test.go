package update

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
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
	setup := func(t *testing.T) (string, Deps, func()) {
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

// TestSelfUpdate_CtxCancel asserts context cancellation mid-flow surfaces
// as a ctx error and doesn't replace the executable. Guards against a future
// regression that swaps ctx-aware helpers for context.Background().
func TestSelfUpdate_CtxCancel(t *testing.T) {
	exePath, deps := stageUpdate(t, "linux", "amd64", "ana")
	r := &releaseServer{tag: "v1.2.3", archiveName: "ana_1.2.3_linux_amd64.tar.gz"}
	r.archiveBody = fakeArchive(t, "tar.gz", "ana", []byte("NEW"))
	srv := r.serve(t)
	defer srv.Close()
	withURLs(t, srv)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := SelfUpdate(ctx, deps, "1.0.0", io.Discard, false)
	if err == nil {
		t.Fatal("expected cancellation error")
	}
	if got, _ := os.ReadFile(exePath); string(got) != "old" {
		t.Errorf("exe changed despite cancel: %q", got)
	}
}

func TestSelfUpdate_EarlyErrors(t *testing.T) {
	t.Run("exe path error", func(t *testing.T) {
		deps := Deps{ExePath: func() (string, error) { return "", errors.New("no exe") }}
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

// failingWriter errors on any Write call. Used to exercise emitStatus's
// WriteString-error branch without wiring a whole SelfUpdate invocation.
type failingWriter struct{}

func (failingWriter) Write([]byte) (int, error) { return 0, errors.New("write denied") }

func TestEmitStatus_WriteError(t *testing.T) {
	t.Parallel()
	err := emitStatus(failingWriter{}, false, updateStatus{Status: "x"}, "line\n")
	wantErr(t, err, "emit status")
}

// TestFormatReplaceErr_PermissionGuidance pins the user-facing wording for the
// EACCES branch on Unix and Windows. The strings are what users will see on
// perm-denied paths (e.g. /usr/local/bin/ana, C:\Program Files\ana), and the
// plan commits to exactly this copy — a silent wording change would erase the
// actionable hint that motivated this fix.
func TestFormatReplaceErr_PermissionGuidance(t *testing.T) {
	t.Parallel()
	t.Run("unix permission denied", func(t *testing.T) {
		err := formatReplaceErr("linux", "/usr/local/bin/ana", fs.ErrPermission)
		got := err.Error()
		if !strings.Contains(got, "cannot write to /usr/local/bin/ana") {
			t.Errorf("missing path: %q", got)
		}
		if !strings.Contains(got, "sudo") {
			t.Errorf("missing sudo hint: %q", got)
		}
		if strings.Contains(got, "/tmp") || strings.Contains(got, "ana-update-") {
			t.Errorf("leaked tempdir path: %q", got)
		}
	})
	t.Run("windows permission denied", func(t *testing.T) {
		err := formatReplaceErr("windows", `C:\Program Files\ana\ana.exe`, fs.ErrPermission)
		got := err.Error()
		if !strings.Contains(got, "Administrator") {
			t.Errorf("missing Administrator hint: %q", got)
		}
		if !strings.Contains(got, "LOCALAPPDATA") {
			t.Errorf("missing LOCALAPPDATA hint: %q", got)
		}
	})
	t.Run("non-permission error falls through", func(t *testing.T) {
		err := formatReplaceErr("linux", "/x/ana", errors.New("disk full"))
		got := err.Error()
		if !strings.Contains(got, "disk full") {
			t.Errorf("should preserve cause: %q", got)
		}
		if strings.Contains(got, "sudo") {
			t.Errorf("should not suggest sudo for non-permission errors: %q", got)
		}
	})
}

// permDeniedErr wraps a *PathError around fs.ErrPermission so errors.Is
// matches both the stdlib rename error shape (PathError) and the sentinel.
func permDeniedErr() error {
	return &fs.PathError{Op: "rename", Path: "x", Err: fs.ErrPermission}
}

// TestSelfUpdate_UnixEACCES drives the unix rename-permission branch end to end
// and asserts the user-facing message includes the sudo hint rather than the
// raw tempdir path.
func TestSelfUpdate_UnixEACCES(t *testing.T) {
	exePath, deps := stageUpdate(t, "linux", "amd64", "ana")
	deps.Rename = func(string, string) error { return permDeniedErr() }
	r := &releaseServer{tag: "v2.0.0", archiveName: "ana_2.0.0_linux_amd64.tar.gz"}
	r.archiveBody = fakeArchive(t, "tar.gz", "ana", []byte("NEW"))
	srv := r.serve(t)
	defer srv.Close()
	withURLs(t, srv)

	err := SelfUpdate(context.Background(), deps, "1.0.0", io.Discard, false)
	wantErr(t, err, "cannot write to "+exePath)
	if !strings.Contains(err.Error(), "sudo") {
		t.Errorf("missing sudo hint: %v", err)
	}
	if got, _ := os.ReadFile(exePath); string(got) != "old" {
		t.Errorf("exe must not change on EACCES: %q", got)
	}
}

// TestSelfUpdate_WindowsEACCES_AsideFails covers the first-rename permission
// denial on Windows (no .old yet, no rollback) and pins the Administrator hint.
func TestSelfUpdate_WindowsEACCES_AsideFails(t *testing.T) {
	exePath, deps := stageUpdate(t, "windows", "amd64", "ana.exe")
	deps.Rename = func(string, string) error { return permDeniedErr() }
	r := &releaseServer{tag: "v2.0.0", archiveName: "ana_2.0.0_windows_amd64.zip"}
	r.archiveBody = fakeArchive(t, "zip", "ana.exe", []byte("NEW"))
	srv := r.serve(t)
	defer srv.Close()
	withURLs(t, srv)

	err := SelfUpdate(context.Background(), deps, "1.0.0", io.Discard, false)
	wantErr(t, err, "cannot write to "+exePath)
	if !strings.Contains(err.Error(), "Administrator") {
		t.Errorf("missing Administrator hint: %v", err)
	}
	// No ".old" recovery wording — the aside rename never succeeded.
	if strings.Contains(err.Error(), "recover from") {
		t.Errorf("should not reference .old recovery when aside failed: %v", err)
	}
}

// TestSelfUpdate_WindowsEACCES_SecondFailsRollbackFails covers the pathological
// case: aside succeeded, rename-in failed with EACCES, rollback also failed.
// The old "recover from <.old>" wording is misleading there — .old sits in the
// same admin-only directory — so the message must point at elevation.
func TestSelfUpdate_WindowsEACCES_SecondFailsRollbackFails(t *testing.T) {
	exePath, deps := stageUpdate(t, "windows", "amd64", "ana.exe")
	var n int
	deps.Rename = func(o, np string) error {
		n++
		switch n {
		case 1:
			return os.Rename(o, np) // aside succeeds
		default:
			return permDeniedErr() // rename-in + rollback both fail EACCES
		}
	}
	r := &releaseServer{tag: "v2.0.0", archiveName: "ana_2.0.0_windows_amd64.zip"}
	r.archiveBody = fakeArchive(t, "zip", "ana.exe", []byte("NEW"))
	srv := r.serve(t)
	defer srv.Close()
	withURLs(t, srv)

	err := SelfUpdate(context.Background(), deps, "1.0.0", io.Discard, false)
	wantErr(t, err, "cannot write to "+exePath)
	if !strings.Contains(err.Error(), "Administrator") {
		t.Errorf("missing Administrator hint: %v", err)
	}
	if strings.Contains(err.Error(), "recover from") {
		t.Errorf("should not point at .old when both sides are admin-only: %v", err)
	}
}

func TestDeps(t *testing.T) {
	t.Parallel()
	t.Run("DefaultDeps populates everything", func(t *testing.T) {
		d := DefaultDeps()
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
		d := resolveDeps(Deps{})
		p, err := d.TempDir()
		if err != nil {
			t.Fatalf("tempdir: %v", err)
		}
		defer os.RemoveAll(p)
	})
	t.Run("resolveDeps preserves explicit", func(t *testing.T) {
		d := resolveDeps(Deps{
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
