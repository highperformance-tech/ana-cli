// Command ana is the TextQL platform CLI. This file is pure wiring: it
// declares the root verb tree (whose persistent flags own --json /
// --endpoint / --token-file / --profile), constructs lazy transport.Client +
// config closures that initialize on first use, and hands off to
// cli.Resolve + cli.Resolved.Execute. All domain logic lives under
// internal/.
package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"time"

	"github.com/highperformance-tech/ana-cli/internal/api"
	"github.com/highperformance-tech/ana-cli/internal/audit"
	"github.com/highperformance-tech/ana-cli/internal/auth"
	"github.com/highperformance-tech/ana-cli/internal/chat"
	"github.com/highperformance-tech/ana-cli/internal/cli"
	"github.com/highperformance-tech/ana-cli/internal/config"
	"github.com/highperformance-tech/ana-cli/internal/connector"
	"github.com/highperformance-tech/ana-cli/internal/dashboard"
	"github.com/highperformance-tech/ana-cli/internal/feed"
	"github.com/highperformance-tech/ana-cli/internal/ontology"
	"github.com/highperformance-tech/ana-cli/internal/org"
	"github.com/highperformance-tech/ana-cli/internal/playbook"
	"github.com/highperformance-tech/ana-cli/internal/profile"
	"github.com/highperformance-tech/ana-cli/internal/transport"
	"github.com/highperformance-tech/ana-cli/internal/update"
)

func main() {
	stdio := cli.DefaultIO()
	err := run(os.Args[1:], stdio, os.Getenv)
	if err != nil && !errors.Is(err, cli.ErrHelp) && !errors.Is(err, cli.ErrReported) {
		// ErrHelp means help text was already written to stdout; skip.
		// ErrReported means the callee already wrote its diagnostic to
		// stderr (Dispatch for bad globals / unknown commands, run() for
		// unknown profiles). Every other error — including ErrUsage from
		// leaves whose FlagSet output is io.Discard — still needs to
		// surface, otherwise misplaced flags exit 1 with silent output.
		fmt.Fprintln(stdio.Stderr, err)
	}
	os.Exit(cli.ExitCode(err))
}

// run is the testable entrypoint. Returns the error main() feeds to ExitCode.
//
// The flow is resolve-then-execute: build a root *cli.Group whose persistent
// Flags closure binds &global (mutated in place by cli.Resolve), then call
// Resolve to walk argv + parse every flag. After Resolve the global struct
// holds the user-supplied values; we use that to decide whether to spawn the
// passive update-check goroutine, then Execute the resolved leaf.
//
// Transport client + config state are computed lazily inside the verb-Deps
// closures so verbs like `ana profile add` never load existing config or
// open a transport for an org they're about to create.
func run(args []string, stdio cli.IO, env func(string) string) error {
	// Short-circuit --version / -V anywhere in argv: rewrite to the
	// `version` verb so the same code path renders the banner whether the
	// user typed the flag or the subcommand. Scanning all args (rather than
	// only args[0]) lets `ana --json --version`, `ana --version --endpoint X`,
	// etc. short-circuit before falling into normal resolution.
	for _, a := range args {
		if a == "--version" || a == "-V" {
			args = []string{"version"}
			break
		}
	}

	var global cli.Global
	state := newLazyState(env, &global, stdio)

	root := &cli.Group{
		Summary: "Manage TextQL via the public Connect-RPC API.",
		Flags: func(fs *flag.FlagSet) {
			fs.BoolVar(&global.JSON, "json", false, "emit JSON output")
			fs.StringVar(&global.Endpoint, "endpoint", "", "override API endpoint URL")
			fs.StringVar(&global.TokenFile, "token-file", "", "path to bearer-token file")
			fs.StringVar(&global.Profile, "profile", "", "config profile to use")
		},
		Children: buildVerbs(state),
	}

	// Empty / explicit-help-token at the front: render root help, exit 0.
	if len(args) == 0 || cli.IsHelpArg(args[0]) {
		fmt.Fprintln(stdio.Stdout, cli.RootHelp(root))
		return cli.ErrHelp
	}

	res, err := cli.Resolve(root, args)
	if err != nil {
		if errors.Is(err, cli.ErrHelp) {
			cli.RenderResolvedHelp(res, root, stdio.Stdout)
			return cli.ErrHelp
		}
		// Modern-CLI convention: error first, then the help for the deepest
		// scope the resolver reached so the user sees relevant syntax.
		cli.ReportUsageError(res, root, err, stdio.Stderr)
		return errors.Join(err, cli.ErrReported)
	}

	// Stash global on ctx so leaves' GlobalFrom(ctx) reads return it. Also
	// stash the merged FlagSet so leaves can use FlagSetFrom for RequireFlags
	// / FlagWasSet.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	ctx = cli.WithGlobal(ctx, global)
	ctx = cli.WithFlagSet(ctx, res.MergedFS)

	// Kick the passive update-check BEFORE the verb runs so the HTTP
	// round-trip overlaps the verb's work. drainNudge picks it up after.
	nudgeCh := startNudge(env, global)
	// Resolved.Execute is the single chokepoint that owns leaf invocation
	// AND the modern-CLI-convention annotation for any leaf-internal usage
	// error (RequireFlags / RequireStringID / UsageErrf / etc.). Anything
	// extra here would be a parallel implementation that could drift.
	runErr := res.Execute(ctx, stdio)
	drainCtx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	drainNudge(drainCtx, nudgeCh, runErr, firstVerb(args), stdio.Stderr)
	return runErr
}

// firstVerb returns the leading positional from args (skipping flag tokens
// at the front) so drainNudge can suppress nudges for `ana update` success.
func firstVerb(args []string) string {
	for i := 0; i < len(args); i++ {
		tok := args[i]
		if len(tok) >= 2 && tok[0] == '-' {
			// Flag token. If it consumes the next argv entry as its value,
			// skip both. We don't know shapes here so heuristic: if no `=`
			// and next exists and isn't a flag, skip both.
			if !contains(tok, '=') && i+1 < len(args) && (len(args[i+1]) == 0 || args[i+1][0] != '-') {
				// Bool globals would mis-skip; in practice the only
				// post-skip token is the verb name itself. Accept the small
				// error margin for nudge suppression.
				if isKnownValueGlobal(tok) {
					i++
				}
			}
			continue
		}
		return tok
	}
	return ""
}

// contains is a tiny strings.IndexByte == >= 0 with no import.
func contains(s string, b byte) bool {
	for i := 0; i < len(s); i++ {
		if s[i] == b {
			return true
		}
	}
	return false
}

// isKnownValueGlobal reports whether tok is `-name`/`--name` for one of the
// value-bearing root persistent flags. Bool --json doesn't match.
func isKnownValueGlobal(tok string) bool {
	body := tok
	if len(body) >= 2 && body[0] == '-' {
		body = body[1:]
	}
	if len(body) >= 1 && body[0] == '-' {
		body = body[1:]
	}
	switch body {
	case "endpoint", "token-file", "profile":
		return true
	}
	return false
}

// startNudge launches the passive update-check goroutine when enabled. The
// returned channel fires once with either an upgrade message or the empty
// string. Returns nil whenever the check is skipped so drainNudge can short-
// circuit without touching the channel.
//
// Skip predicates:
//   - version == "dev" — source checkout, no corresponding GitHub release.
//   - global.JSON — automation pipeline; extra stderr would break parsers.
//   - update.ParseInterval reports disabled.
//   - update.CachePath errors — no place to stash freshness state.
//
// Best-effort peek at config.UpdateCheckInterval so explicit "0"/"disable"
// or custom durations are honored. Both DefaultPath and Load errors are
// swallowed here (unlike lazyState.initConfig) — the nudge is best-effort
// background work, and a missing/unreadable config simply falls back to
// ParseInterval(nil) → (4h, true). Mirrors lazyState.initConfig's path
// precedence so --token-file selects the same config the rest of the
// command reads from.
func startNudge(env func(string) string, global cli.Global) chan string {
	if version == "dev" || global.JSON {
		return nil
	}
	path := global.TokenFile
	if path == "" {
		if p, err := config.DefaultPath(env); err == nil {
			path = p
		}
	}
	var interval *string
	if path != "" {
		if cfg, err := config.Load(path); err == nil {
			interval = cfg.UpdateCheckInterval
		}
	}
	ttl, enabled := update.ParseInterval(interval)
	if !enabled {
		return nil
	}
	if _, err := update.CachePath(env); err != nil {
		return nil
	}
	ch := make(chan string, 1)
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		tag, notify, _ := update.CachedCheck(ctx, update.CacheDeps{
			Env:  env,
			Now:  time.Now,
			HTTP: http.DefaultClient,
		}, ttl, version)
		if notify {
			ch <- fmt.Sprintf("A new version of ana-cli is available: v%s → %s  Run: ana update", version, tag)
		} else {
			ch <- ""
		}
	}()
	return ch
}

// drainNudge waits for startNudge's goroutine to report or for ctx to be
// done, whichever comes first. When the goroutine produces a non-empty
// message and the verb did not return a help/usage error, the message is
// written to stderr — we intentionally suppress the nudge on help/usage
// paths so help text doesn't get crowded by an upgrade prompt. A successful
// `ana update` is also suppressed: the goroutine captured the pre-swap
// version at process start and can't know the binary was just replaced. A
// failed `ana update` still nudges. A nil ch (check was skipped) or a
// canceled ctx is a clean no-op. Production callers wrap with a
// context.WithTimeout so a slow update check can't make the verb hang.
func drainNudge(ctx context.Context, ch chan string, verbErr error, verb string, stderr io.Writer) {
	if ch == nil {
		return
	}
	if errors.Is(verbErr, cli.ErrHelp) || errors.Is(verbErr, cli.ErrUsage) {
		return
	}
	if verb == "update" && verbErr == nil {
		return
	}
	select {
	case msg := <-ch:
		if msg != "" {
			fmt.Fprintln(stderr, msg)
		}
	case <-ctx.Done():
	}
}

// buildVerbs wires every verb package's Deps. Heavy state (config load,
// transport client) lives in lazyState; Deps closures fetch it on first call
// so non-RPC verbs (profile, update, version) never trigger client
// construction. profileName is supplied to the auth package so login/logout
// always target the same profile the rest of the invocation uses.
func buildVerbs(s *lazyState) map[string]cli.Command {
	return map[string]cli.Command{
		"api":       api.New(api.Deps{DoRaw: s.DoRaw}),
		"auth":      auth.New(s.AuthDeps()),
		"profile":   profile.New(s.ProfileDeps()),
		"org":       org.New(org.Deps{Unary: s.Unary}),
		"connector": connector.New(connector.Deps{Unary: s.Unary, Endpoint: s.EndpointFn}),
		"chat":      chat.New(s.ChatDeps()),
		"dashboard": dashboard.New(dashboard.Deps{Unary: s.Unary}),
		"playbook":  playbook.New(playbook.Deps{Unary: s.Unary}),
		"ontology":  ontology.New(ontology.Deps{Unary: s.Unary}),
		"feed":      feed.New(feed.Deps{Unary: s.Unary}),
		"audit":     audit.New(audit.Deps{Unary: s.Unary, Now: time.Now}),
		"version":   versionCmd{},
		"update":    updateCmd{deps: update.DefaultDeps()},
	}
}

// lazyState centralises every "needs config or transport" computation that
// historically ran eagerly in run(). Each accessor below uses sync.Once so
// the work happens at most once per invocation.
type lazyState struct {
	env    func(string) string
	global *cli.Global
	stdio  cli.IO

	cfgOnce     sync.Once
	cfgPath     string
	profileName string
	resolved    config.Profile
	cfgErr      error

	clientOnce sync.Once
	client     *transport.Client
}

// newLazyState builds a lazyState bound to env, the &global pointer that
// cli.Resolve will populate, and the stdio used to print the unknown-profile
// error before returning ErrReported.
func newLazyState(env func(string) string, global *cli.Global, stdio cli.IO) *lazyState {
	return &lazyState{env: env, global: global, stdio: stdio}
}

// initConfig resolves config state on first call. Subsequent calls return
// the cached error (if any). Unknown-profile is the canonical user mistake;
// we print the diagnostic ourselves and return an ErrUsage + ErrReported
// pair so main()'s fallback printer doesn't double-emit.
func (s *lazyState) initConfig() error {
	s.cfgOnce.Do(func() {
		s.cfgPath = s.global.TokenFile
		if s.cfgPath == "" {
			p, err := config.DefaultPath(s.env)
			if err != nil {
				s.cfgErr = err
				return
			}
			s.cfgPath = p
		}
		var loaded config.Config
		if s.cfgPath != "" {
			c, err := config.Load(s.cfgPath)
			if err != nil {
				s.cfgErr = err
				return
			}
			loaded = c
		}
		resolved, name, rerr := config.Resolve(s.env, loaded, s.global.Profile)
		if rerr != nil {
			if errors.Is(rerr, config.ErrUnknownProfile) {
				// Use name (the effective profile chosen via the precedence
				// chain in pickProfileName) rather than s.global.Profile.
				// Today config.Resolve only returns ErrUnknownProfile when
				// the flag was set, so the two values agree — but using name
				// is correct under the precedence contract regardless of how
				// Resolve's gate evolves.
				fmt.Fprintf(s.stdio.Stderr, "ana: unknown profile %q\n", name)
				s.cfgErr = errors.Join(fmt.Errorf("%w: %w", cli.ErrUsage, rerr), cli.ErrReported)
				return
			}
			s.cfgErr = rerr
			return
		}
		if s.global.Endpoint != "" {
			resolved.Endpoint = s.global.Endpoint
		}
		s.resolved = resolved
		s.profileName = name
		if s.profileName == "" {
			s.profileName = "default"
		}
	})
	return s.cfgErr
}

// transportClient returns the shared transport.Client built from the resolved
// config; the build runs at most once per invocation.
func (s *lazyState) transportClient() (*transport.Client, error) {
	if err := s.initConfig(); err != nil {
		return nil, err
	}
	s.clientOnce.Do(func() {
		tokenFn := func(_ context.Context) (string, error) {
			return s.resolved.Token.Value(), nil
		}
		s.client = transport.New(s.resolved.Endpoint, tokenFn)
	})
	return s.client, nil
}

// Unary forwards to the lazily-built transport.Client. Used as the Deps.Unary
// closure for every RPC verb package.
func (s *lazyState) Unary(ctx context.Context, path string, req, resp any) error {
	c, err := s.transportClient()
	if err != nil {
		return err
	}
	return c.Unary(ctx, path, req, resp)
}

// DoRaw forwards to the lazily-built transport.Client. Used by api.Deps.
func (s *lazyState) DoRaw(ctx context.Context, method, path string, body []byte) (int, []byte, error) {
	c, err := s.transportClient()
	if err != nil {
		return 0, nil, err
	}
	return c.DoRaw(ctx, method, path, body)
}

// Stream forwards to the lazily-built transport.Client wrapped in a
// chat.StreamSession adapter (so the chat package never imports
// internal/transport).
func (s *lazyState) Stream(ctx context.Context, path string, req any) (chat.StreamSession, error) {
	c, err := s.transportClient()
	if err != nil {
		return nil, err
	}
	sr, serr := c.Stream(ctx, path, req)
	if serr != nil {
		return nil, serr
	}
	return &boundStream{ctx: ctx, sr: sr}, nil
}

// EndpointFn returns the resolved API base URL for connector OAuth notes.
// Returns "" on init failure — connector only consults this in already-
// successful create-leaf paths, so a fallback isn't structurally needed.
func (s *lazyState) EndpointFn() string {
	if err := s.initConfig(); err != nil {
		return ""
	}
	return s.resolved.Endpoint
}

// AuthDeps builds the auth.Deps with closures targeting the profile slot
// resolved by initConfig. SaveCfg is read-modify-write so it preserves every
// other profile and any existing OrgName on the target.
func (s *lazyState) AuthDeps() auth.Deps {
	return auth.Deps{
		Unary: s.Unary,
		LoadCfg: func() (auth.Config, error) {
			if err := s.initConfig(); err != nil {
				return auth.Config{}, err
			}
			if s.cfgPath == "" {
				return auth.Config{}, nil
			}
			c, err := config.Load(s.cfgPath)
			if err != nil {
				return auth.Config{}, err
			}
			p := c.Profiles[s.profileName]
			return profileToAuthConfig(p), nil
		},
		SaveCfg: func(ac auth.Config) error {
			if err := s.initConfig(); err != nil {
				return err
			}
			path := s.cfgPath
			if path == "" {
				p, err := config.DefaultPath(s.env)
				if err != nil {
					return err
				}
				path = p
			}
			existing, err := config.Load(path)
			if err != nil {
				return err
			}
			prev := existing.Profiles[s.profileName]
			existing.Upsert(s.profileName, config.Profile{
				Endpoint: ac.Endpoint,
				Token:    ac.Token,
				OrgName:  prev.OrgName,
			})
			return config.Save(path, existing)
		},
		ConfigPath: func() (string, error) {
			if s.cfgPath != "" {
				return s.cfgPath, nil
			}
			// Don't invoke initConfig here — ConfigPath should work even when
			// the user has --token-file pointing at an empty path.
			path := s.global.TokenFile
			if path != "" {
				return path, nil
			}
			return config.DefaultPath(s.env)
		},
	}
}

// ProfileDeps wires profile.Deps. Unlike auth there's no projection: the
// profile verb owns the whole config.Config surface. We do NOT call
// initConfig here — profile commands manage the file directly.
func (s *lazyState) ProfileDeps() profile.Deps {
	return profile.Deps{
		LoadCfg: func() (config.Config, error) {
			path := s.global.TokenFile
			if path == "" {
				p, err := config.DefaultPath(s.env)
				if err != nil {
					return config.Config{}, err
				}
				path = p
			}
			return config.Load(path)
		},
		SaveCfg: func(c config.Config) error {
			path := s.global.TokenFile
			if path == "" {
				p, err := config.DefaultPath(s.env)
				if err != nil {
					return err
				}
				path = p
			}
			return config.Save(path, c)
		},
		ConfigPath: func() (string, error) {
			if s.global.TokenFile != "" {
				return s.global.TokenFile, nil
			}
			return config.DefaultPath(s.env)
		},
	}
}

// ChatDeps wires chat.Deps. Stream uses the lazy transport adapter; UUIDFn
// is the local newUUID so chat sends are deterministic in tests via
// dependency injection.
func (s *lazyState) ChatDeps() chat.Deps {
	return chat.Deps{
		Unary:  s.Unary,
		Stream: s.Stream,
		UUIDFn: newUUID,
	}
}

// boundStream binds ctx alongside the StreamReader so chat.StreamSession.Next
// stays ctx-free.
type boundStream struct {
	ctx context.Context
	sr  *transport.StreamReader
}

func (b *boundStream) Next(out any) (bool, error) { return b.sr.Next(b.ctx, out) }
func (b *boundStream) Close() error               { return b.sr.Close() }

// profileToAuthConfig projects a config.Profile down to the subset
// internal/auth cares about. Keeping the projection in main.go means auth
// never imports internal/config — the contract stays narrow.
func profileToAuthConfig(p config.Profile) auth.Config {
	return auth.Config{Endpoint: p.Endpoint, Token: p.Token}
}

// newUUID returns a canonical RFC 4122 version-4 UUID string. It draws 16
// random bytes from crypto/rand; if the source fails (e.g. the kernel pool
// is unavailable) it falls back to a time-based hex blob so callers never
// block or panic. The fallback is deterministic-enough for cellIds in chat
// sends, which is the only place we mint UUIDs from the CLI today.
func newUUID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		now := time.Now().UnixNano()
		for i := range 16 {
			b[i] = byte(now >> (uint(i%8) * 8))
		}
	}
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	var buf [36]byte
	hex.Encode(buf[0:8], b[0:4])
	buf[8] = '-'
	hex.Encode(buf[9:13], b[4:6])
	buf[13] = '-'
	hex.Encode(buf[14:18], b[6:8])
	buf[18] = '-'
	hex.Encode(buf[19:23], b[8:10])
	buf[23] = '-'
	hex.Encode(buf[24:36], b[10:16])
	return string(buf[:])
}
