package harness

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// ManualRevertLog collects entries for mutations the harness could not
// auto-revert. End flushes them to a markdown file and fails the test.
type ManualRevertLog struct {
	org     string
	started time.Time
	mu      sync.Mutex
	entries []ledgerEntry
}

type ledgerEntry struct {
	What   string
	Reason string
	When   time.Time
}

func newManualRevertLog(org string) *ManualRevertLog {
	return &ManualRevertLog{org: org, started: time.Now()}
}

// Record appends one entry. Safe for concurrent use.
func (l *ManualRevertLog) Record(what, reason string, when time.Time) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.entries = append(l.entries, ledgerEntry{What: what, Reason: reason, When: when})
}

// HasEntries reports whether any entries were recorded.
func (l *ManualRevertLog) HasEntries() bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	return len(l.entries) > 0
}

// Count returns the number of entries.
func (l *ManualRevertLog) Count() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return len(l.entries)
}

// Flush writes a `e2e-manual-revert-<ts>.md` file in dir and returns its
// path. The repo .gitignore should exclude that pattern.
func (l *ManualRevertLog) Flush(dir string) (string, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	ended := time.Now()
	name := fmt.Sprintf("e2e-manual-revert-%d.md", ended.Unix())
	// Prefer the repo root for discoverability. Callers pass cfg dir today;
	// we write next to the config file which lives under tmpdir during tests.
	// A stable filename pattern keeps CI easy.
	path := filepath.Join(dir, name)

	var b strings.Builder
	fmt.Fprintf(&b, "# Manual revert required\n\n")
	fmt.Fprintf(&b, "Suite started: %s   Suite ended: %s\n", l.started.Format(time.RFC3339), ended.Format(time.RFC3339))
	fmt.Fprintf(&b, "Org:           %s\n\n", l.org)
	fmt.Fprintf(&b, "The following mutations could not be auto-reverted. Please review:\n\n")
	for _, e := range l.entries {
		fmt.Fprintf(&b, "- [ ] %s\n      reason: %s\n      recorded at: %s\n",
			e.What, e.Reason, e.When.Format(time.RFC3339))
	}
	if err := os.WriteFile(path, []byte(b.String()), 0o600); err != nil {
		return "", err
	}
	return path, nil
}

// shortRand returns 4 lowercase-hex chars. Used for run-unique prefixes.
func shortRand() string {
	var b [2]byte
	if _, err := rand.Read(b[:]); err != nil {
		// crypto/rand is reliable on the platforms we ship to; on failure we
		// fall back to the monotonic nanosecond clock so the prefix still
		// distinguishes parallel runs.
		n := time.Now().UnixNano()
		b[0] = byte(n)
		b[1] = byte(n >> 8)
	}
	return hex.EncodeToString(b[:])
}
