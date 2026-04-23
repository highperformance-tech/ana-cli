package update

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/highperformance-tech/ana-cli/internal/cli"
)

// Deps is the injection boundary for SelfUpdate; zero fields fall
// through to stdlib defaults via resolveDeps.
type Deps struct {
	HTTP    HTTPDoer
	GOOS    string
	GOARCH  string
	ExePath func() (string, error)
	Rename  func(old, newp string) error
	TempDir func() (string, error)
}

// DefaultDeps wires Deps against the process environment.
func DefaultDeps() Deps {
	return Deps{
		HTTP:    http.DefaultClient,
		GOOS:    runtime.GOOS,
		GOARCH:  runtime.GOARCH,
		ExePath: os.Executable,
		Rename:  os.Rename,
		TempDir: func() (string, error) { return os.MkdirTemp("", "ana-update-*") },
	}
}

// resolveDeps fills zero fields with stdlib defaults.
func resolveDeps(d Deps) Deps {
	if d.HTTP == nil {
		d.HTTP = http.DefaultClient
	}
	if d.GOOS == "" {
		d.GOOS = runtime.GOOS
	}
	if d.GOARCH == "" {
		d.GOARCH = runtime.GOARCH
	}
	if d.ExePath == nil {
		d.ExePath = os.Executable
	}
	if d.Rename == nil {
		d.Rename = os.Rename
	}
	if d.TempDir == nil {
		d.TempDir = func() (string, error) { return os.MkdirTemp("", "ana-update-*") }
	}
	return d
}

// updateStatus is the --json output shape. Named type keeps field tags
// stable for scripts that parse `ana update --json`.
type updateStatus struct {
	Status  string `json:"status"`
	From    string `json:"from,omitempty"`
	To      string `json:"to,omitempty"`
	Archive string `json:"archive,omitempty"`
}

// SelfUpdate runs the full self-update flow: resolve latest, compare, skip if
// already current, else download + verify + extract + atomic replace. jsonOut
// selects a single-object JSON summary on w; otherwise plain-text progress is
// written line by line. "Already up to date" is success (nil error).
func SelfUpdate(ctx context.Context, deps Deps, currentVersion string, w io.Writer, jsonOut bool) error {
	deps = resolveDeps(deps)

	exe, err := deps.ExePath()
	if err != nil {
		return fmt.Errorf("update: locate executable: %w", err)
	}
	// Best-effort cleanup of a .old left by a previous Windows update.
	// Missing file is expected; other errors just leave it around.
	if deps.GOOS == "windows" {
		_ = os.Remove(exe + ".old")
	}

	tag, err := LatestRelease(ctx, deps.HTTP)
	if err != nil {
		return err
	}
	latest := strings.TrimPrefix(tag, "v")
	if CmpSemver(currentVersion, latest) >= 0 {
		return emitStatus(w, jsonOut, updateStatus{Status: "up-to-date", From: currentVersion}, fmt.Sprintf("ana is already at version %s\n", currentVersion))
	}

	exeName := "ana"
	archiveExt := "tar.gz"
	if deps.GOOS == "windows" {
		exeName = "ana.exe"
		archiveExt = "zip"
	}
	archiveName := fmt.Sprintf("ana_%s_%s_%s.%s", latest, deps.GOOS, deps.GOARCH, archiveExt)
	base := releasesBaseURL + "/" + tag

	tmp, err := deps.TempDir()
	if err != nil {
		return fmt.Errorf("update: create tempdir: %w", err)
	}
	defer os.RemoveAll(tmp)

	archivePath := filepath.Join(tmp, archiveName)
	sum, err := downloadFile(ctx, deps.HTTP, base+"/"+archiveName, archivePath)
	if err != nil {
		return err
	}
	checksums, err := downloadBody(ctx, deps.HTTP, base+"/checksums.txt")
	if err != nil {
		return err
	}
	if err := verifyChecksum(sum, archiveName, checksums); err != nil {
		return err
	}

	newBinary := filepath.Join(tmp, exeName+".new")
	if err := extractBinary(archivePath, archiveExt, exeName, newBinary); err != nil {
		return err
	}

	if err := atomicReplace(deps, exe, newBinary); err != nil {
		return err
	}
	return emitStatus(w, jsonOut,
		updateStatus{Status: "updated", From: currentVersion, To: latest, Archive: archiveName},
		fmt.Sprintf("Updated ana %s → %s\n", currentVersion, latest),
	)
}

// emitStatus routes through cli.WriteJSON on --json so output stays
// byte-compatible with the rest of the CLI's --json verbs.
func emitStatus(w io.Writer, jsonOut bool, st updateStatus, plain string) error {
	if jsonOut {
		return cli.WriteJSON(w, st)
	}
	if _, err := io.WriteString(w, plain); err != nil {
		return fmt.Errorf("update: emit status: %w", err)
	}
	return nil
}

// atomicReplace installs newPath over exePath. Unix: a single rename works
// because the old inode stays resident while the process runs. Windows: an
// open .exe cannot be renamed over, so we rename it aside first (to .old)
// and rename the replacement in. If the second rename fails, we roll the
// .old back in place and surface an error that names the recovery path.
func atomicReplace(deps Deps, exePath, newPath string) error {
	if deps.GOOS != "windows" {
		if err := deps.Rename(newPath, exePath); err != nil {
			return formatReplaceErr(deps.GOOS, exePath, err)
		}
		return nil
	}
	oldPath := exePath + ".old"
	if err := deps.Rename(exePath, oldPath); err != nil {
		// No .old exists yet, so no rollback wording; route through the
		// permission-aware formatter for the EACCES-equivalent case.
		return formatReplaceErr(deps.GOOS, exePath, err)
	}
	if err := deps.Rename(newPath, exePath); err != nil {
		// Roll the old binary back; if that also fails and the root cause
		// is a permission denial, the `.old` we'd name in the recovery
		// message sits in the same admin-only directory — point the user
		// at elevation instead of a path they also can't touch.
		if rbErr := deps.Rename(oldPath, exePath); rbErr != nil {
			if errors.Is(err, fs.ErrPermission) {
				return formatReplaceErr(deps.GOOS, exePath, err)
			}
			return fmt.Errorf("update: replace %s failed (%w); rollback also failed (%w); recover from %s", exePath, err, rbErr, oldPath)
		}
		return formatReplaceErr(deps.GOOS, exePath, err)
	}
	return nil
}

// formatReplaceErr turns an os.Rename failure into an actionable message.
// On fs.ErrPermission we tell the user exactly how to retry (sudo on Unix,
// Administrator on Windows) instead of leaking the tempdir path the user
// never picked. Every other error falls through to the plain wrapped form.
func formatReplaceErr(goos, exePath string, err error) error {
	if errors.Is(err, fs.ErrPermission) {
		if goos == "windows" {
			return fmt.Errorf("update: cannot write to %s: permission denied. Re-run from an elevated (Administrator) shell, or reinstall ana into a user-writable directory (e.g. %%LOCALAPPDATA%%)", exePath)
		}
		return fmt.Errorf("update: cannot write to %s: permission denied. Re-run with sudo, or reinstall ana into a user-writable directory", exePath)
	}
	return fmt.Errorf("update: replace %s: %w", exePath, err)
}
