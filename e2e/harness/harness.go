package harness

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/highperformance-tech/ana-cli/internal/cli"
	"github.com/highperformance-tech/ana-cli/internal/transport"
)

// H is the per-test live-smoke harness. Construct via Begin; the returned H
// holds a transport.Client plus a verb map bound to a temp config. Every
// mutation registered with the helpers gets reverted in LIFO order by End.
type H struct {
	t        *testing.T
	env      envSpec
	client   *transport.Client
	verbs    map[string]cli.Command
	envFn    func(string) string
	cfgPath  string
	Prefix   string
	ledger   *ManualRevertLog
	cleanups []func()
	mu       sync.Mutex

	// forbidden ids the test promised never to touch. Keyed by resource type.
	forbidden map[string]map[string]struct{}

	// latestKey points at the id slot the most recent CreateAPIKey registered
	// for deferred revoke. RotateAPIKey updates it in place so cleanup always
	// revokes whichever id is current at End-time.
	latestKey *string
}

// Begin sets up a harness for t. Skips the test unless both ANA_E2E_ENDPOINT
// and ANA_E2E_TOKEN are set. Validates the token against ANA_E2E_EXPECT_ORG_ID
// (aborts with t.Fatalf on mismatch) and sweeps any anacli-e2e-* leftovers
// from prior crashed runs before returning.
func Begin(t *testing.T) *H {
	t.Helper()
	env, ok := loadEnv()
	if !ok {
		t.Skip("e2e: ANA_E2E_ENDPOINT and ANA_E2E_TOKEN not set")
	}
	if env.expectOrgID == "" {
		t.Fatalf("e2e: ANA_E2E_EXPECT_ORG_ID must be set so the harness can refuse to touch the wrong org")
	}

	dir := t.TempDir()
	env.configHome = dir
	cfgPath, err := seedConfig(dir, env.endpoint, env.token)
	if err != nil {
		t.Fatalf("e2e: seed config: %v", err)
	}

	client := buildTransport(env.endpoint, env.token)
	envFn := makeEnv(dir)
	verbs := buildVerbs(client, envFn, cfgPath, env.endpoint)

	h := &H{
		t:         t,
		env:       env,
		client:    client,
		verbs:     verbs,
		envFn:     envFn,
		cfgPath:   cfgPath,
		Prefix:    fmt.Sprintf("anacli-e2e-%d-%s", time.Now().Unix(), shortRand()),
		ledger:    newManualRevertLog(env.expectOrgID),
		forbidden: map[string]map[string]struct{}{},
	}

	if env.dryRun {
		t.Logf("e2e dryrun: prefix=%q endpoint=%q expect_org_id=%q", h.Prefix, env.endpoint, env.expectOrgID)
		return h
	}

	if err := guardOrg(context.Background(), client, env.expectOrgID); err != nil {
		t.Fatalf("e2e: org guard: %v", err)
	}
	if err := sweepPrior(context.Background(), client); err != nil {
		t.Fatalf("e2e: pre-run sweep: %v", err)
	}
	if env.sweepOnly {
		t.Skip("e2e: ANA_E2E_SWEEP_ONLY=1 — sweep completed, skipping test body")
	}

	t.Cleanup(h.End)
	return h
}

// End runs every registered cleanup in LIFO order, then flushes the ledger
// and fails the test if any manual-revert entries were recorded.
func (h *H) End() {
	h.t.Helper()
	h.mu.Lock()
	funcs := h.cleanups
	h.cleanups = nil
	h.mu.Unlock()
	for i := len(funcs) - 1; i >= 0; i-- {
		func(fn func()) {
			defer func() {
				if r := recover(); r != nil {
					h.t.Errorf("e2e cleanup panicked: %v", r)
				}
			}()
			fn()
		}(funcs[i])
	}
	if h.ledger.HasEntries() {
		path, err := h.ledger.Flush(filepath.Dir(h.cfgPath))
		if err != nil {
			h.t.Errorf("e2e: ledger flush: %v", err)
		}
		h.t.Fatalf("e2e: manual-revert required, see %s (%d entries)", path, h.ledger.Count())
	}
}

// DryRun reports whether ANA_E2E_DRYRUN=1 is set.
func (h *H) DryRun() bool { return h.env.dryRun }

// Client exposes the raw transport.Client for out-of-band setup/teardown
// (sweep lists, parent-cascade deletes). Prefer Run for test body actions.
func (h *H) Client() *transport.Client { return h.client }

// ExpectOrgID returns the ANA_E2E_EXPECT_ORG_ID value the harness validated.
func (h *H) ExpectOrgID() string { return h.env.expectOrgID }

// defer registers fn to run in LIFO order when End fires. Exported indirectly
// via Register so callers can deregister on successful delete.
func (h *H) Register(fn func()) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.cleanups = append(h.cleanups, fn)
}

// RecordManualRevert appends a ledger entry; End will flush and fail.
func (h *H) RecordManualRevert(what, reason string) {
	h.ledger.Record(what, reason, time.Now())
}

// MustNotTouch marks id as off-limits. Any helper that mutates this id must
// call h.forbiddenCheck first; the default pathway through the typed helpers
// does this automatically.
func (h *H) MustNotTouch(resourceType string, id any) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.forbidden[resourceType] == nil {
		h.forbidden[resourceType] = map[string]struct{}{}
	}
	h.forbidden[resourceType][fmt.Sprint(id)] = struct{}{}
}

// forbiddenCheck fatals the test if id is in the don't-touch set.
func (h *H) forbiddenCheck(resourceType string, id any) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if set, ok := h.forbidden[resourceType]; ok {
		if _, bad := set[fmt.Sprint(id)]; bad {
			h.t.Fatalf("e2e: test attempted to mutate forbidden %s id %v — create a disposable parent instead", resourceType, id)
		}
	}
}

// Run invokes the ana CLI via cli.Dispatch with a verb and args. stdout and
// stderr are captured and returned. Global flags (--json, --profile, etc.)
// may appear in args; Dispatch handles them. If DryRun is set, Run records
// the intent and returns an empty result without calling the RPC.
func (h *H) Run(args ...string) (string, string, error) {
	h.t.Helper()
	return h.RunStdin("", args...)
}

// RunStdin is Run with a stdin payload, used by leaves that accept secrets via
// --*-stdin flags (e.g. connector create snowflake password
// --password-stdin). An empty stdin is equivalent to Run.
func (h *H) RunStdin(stdin string, args ...string) (string, string, error) {
	h.t.Helper()
	if h.env.dryRun {
		h.t.Logf("dryrun: ana %s", strings.Join(args, " "))
		return "", "", nil
	}
	var stdout, stderr bytes.Buffer
	stdio := cli.IO{
		Stdin:  strings.NewReader(stdin),
		Stdout: &stdout,
		Stderr: &stderr,
		Env:    h.envFn,
		Now:    time.Now,
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	err := cli.Dispatch(ctx, h.verbs, args, stdio)
	return stdout.String(), stderr.String(), err
}

// RunJSON runs the CLI with --json prepended (after any pre-verb globals),
// then decodes stdout as a map. Convenience over Run for tests that assert on
// structured output.
func (h *H) RunJSON(args ...string) (map[string]any, error) {
	h.t.Helper()
	// Insert --json before the verb. cli.ParseGlobal scans leading flags.
	full := append([]string{"--json"}, args...)
	stdout, stderr, err := h.Run(full...)
	if err != nil {
		return nil, fmt.Errorf("run: %w (stderr=%s)", err, stderr)
	}
	if stdout == "" {
		return map[string]any{}, nil
	}
	var out map[string]any
	if derr := json.Unmarshal([]byte(stdout), &out); derr != nil {
		return nil, fmt.Errorf("decode json: %w (stdout=%q)", derr, stdout)
	}
	return out, nil
}

// ResourceName returns a prefixed, test-owned name safe for server-side
// mutation. Passing the caller-specific suffix helps the sweep recognise
// leftovers without ambiguity.
func (h *H) ResourceName(suffix string) string {
	if suffix == "" {
		return h.Prefix
	}
	return h.Prefix + "-" + suffix
}
