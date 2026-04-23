// Command ana is the TextQL platform CLI. This file is pure wiring: it reads
// global flags + config, constructs a transport.Client, assembles the verb map
// by injecting adapted Deps into each internal/<verb> package, then hands off
// to cli.Dispatch. All domain logic lives under internal/.
package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/signal"
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

// run is the testable entrypoint: same signature as main() but with args,
// stdio, and env injected. Returns the error that main() feeds to ExitCode.
func run(args []string, stdio cli.IO, env func(string) string) error {
	// Short-circuit --version / -V: rewrite to the `version` verb so the
	// same code path renders the banner whether the user typed the flag or
	// the subcommand. Done before global-flag parsing so the flag doesn't
	// need to be declared in cli.Global (which would ripple into every test
	// that constructs a Global).
	if len(args) == 1 && (args[0] == "--version" || args[0] == "-V") {
		args = []string{"version"}
	}

	// Parse global flags up front so the token-file/endpoint/profile
	// overrides are available before we touch config on disk. cli.Dispatch
	// re-parses globals, but doing it here lets us wire deps correctly
	// before dispatch. StripGlobals (rather than ParseGlobal) so operators
	// can place `--profile`/`--endpoint`/`--token-file` after the verb and
	// still have config resolution honour them — ParseGlobal stops at the
	// first positional, which would leave those flags invisible here.
	global, verbArgs, err := cli.StripGlobals(args)
	if err != nil {
		// Don't return here — let Dispatch produce the canonical usage error
		// (it prints root help + err to stderr). Fall through with zero
		// global; Dispatch will hit the same parse error and handle it.
		global = cli.Global{}
	}

	cfgPath := global.TokenFile
	if cfgPath == "" {
		p, err := config.DefaultPath(env)
		if err != nil {
			// No XDG_CONFIG_HOME or HOME set: treat as unconfigured rather
			// than fatal — commands that need a token will fail informatively
			// at call time via auth.ErrNotLoggedIn.
			cfgPath = ""
		} else {
			cfgPath = p
		}
	}

	var loaded config.Config
	if cfgPath != "" {
		if c, lerr := config.Load(cfgPath); lerr == nil {
			loaded = c
		}
		// A load error (malformed JSON, permission denied) is intentionally
		// swallowed here; the resolved endpoint still defaults correctly and
		// auth commands will surface a clearer message downstream.
	}

	resolved, profileName, rerr := config.Resolve(env, loaded, global.Profile)
	if rerr != nil {
		// The only documented error from Resolve is ErrUnknownProfile — a
		// user-visible mistake. Print it on stderr and map to exit 1 (usage)
		// by wrapping cli.ErrUsage.
		if errors.Is(rerr, config.ErrUnknownProfile) {
			fmt.Fprintf(stdio.Stderr, "ana: unknown profile %q\n", global.Profile)
			return errors.Join(fmt.Errorf("%w: %w", cli.ErrUsage, rerr), cli.ErrReported)
		}
		return rerr
	}
	if global.Endpoint != "" {
		resolved.Endpoint = global.Endpoint
	}

	token := resolved.Token
	tokenFn := func(ctx context.Context) (string, error) {
		return token.Value(), nil
	}
	client := transport.New(resolved.Endpoint, tokenFn)

	verbs := buildVerbs(client, env, cfgPath, profileName, resolved.Endpoint)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	// Kick the passive update-check goroutine BEFORE Dispatch so the HTTP
	// round-trip overlaps the verb's work. drainNudge picks it up after
	// Dispatch returns. nudgeCh is nil whenever we decide not to check at
	// all (dev build, --json, disabled interval, or no cache path); that
	// nil flows through drainNudge as a no-op.
	nudgeCh := startNudge(env, loaded, global)
	err = cli.Dispatch(ctx, verbs, args, stdio)
	drainNudge(nudgeCh, 500*time.Millisecond, err, firstVerb(verbArgs), stdio.Stderr)
	return err
}

// firstVerb returns the leading positional from StripGlobals' verb-args slice,
// or "" for an empty slice. Keeps the drainNudge call site readable.
func firstVerb(verbArgs []string) string {
	if len(verbArgs) == 0 {
		return ""
	}
	return verbArgs[0]
}

// startNudge launches the passive update-check goroutine when enabled and
// returns a buffered channel that drainNudge reads. Returns nil whenever the
// check is skipped so drainNudge can short-circuit without touching the
// channel.
//
// Skip predicates (any true disables the check):
//   - version == "dev" — source checkout, no corresponding GitHub release.
//   - global.JSON — automation pipeline, extra stderr line would break parsers.
//   - ParseInterval reports disabled (config value "0" / "disable").
//   - CachePath fails — no XDG_CACHE_HOME and no HOME means we have nowhere
//     to stash freshness state, and we refuse to re-hit the network on every
//     run.
func startNudge(env func(string) string, loaded config.Config, global cli.Global) chan string {
	if version == "dev" || global.JSON {
		return nil
	}
	ttl, enabled := update.ParseInterval(loaded.UpdateCheckInterval)
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

// drainNudge waits up to timeout for startNudge's goroutine to report. When
// it produces a non-empty message and the verb did not return a help/usage
// error, the message is written to stderr — we intentionally suppress the
// nudge on help/usage paths so help text doesn't get crowded by an upgrade
// prompt. A successful `ana update` is also suppressed: the goroutine captured
// the pre-swap version at process start and can't know the binary was just
// replaced, so its "new version available" line would contradict the verb's
// own success output. A failed `ana update` still nudges — the user needs the
// retry hint. A nil ch (check was skipped) or a timeout is a clean no-op.
func drainNudge(ch chan string, timeout time.Duration, verbErr error, verb string, stderr io.Writer) {
	if ch == nil {
		return
	}
	if errors.Is(verbErr, cli.ErrHelp) || errors.Is(verbErr, cli.ErrUsage) {
		return
	}
	if verb == "update" && verbErr == nil {
		return
	}
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case msg := <-ch:
		if msg != "" {
			fmt.Fprintln(stderr, msg)
		}
	case <-timer.C:
	}
}

// buildVerbs wires every verb package's Deps against the shared transport
// client and config path. Pulled out of run() so main_test can exercise it in
// isolation. profileName is the slot that auth.Load/SaveCfg should read from
// and write into — resolved upstream so login/logout always target the same
// profile the rest of the invocation used. endpoint is the resolved API base
// URL (not just the --endpoint override), so verbs whose user-facing output
// references the web app — e.g. connector OAuth success notes — can point
// self-hosted and non-prod profiles at the right place.
func buildVerbs(client *transport.Client, env func(string) string, cfgPath, profileName, endpoint string) map[string]cli.Command {
	return map[string]cli.Command{
		"api":       api.New(api.Deps{DoRaw: client.DoRaw}),
		"auth":      auth.New(authDeps(client, env, cfgPath, profileName)),
		"profile":   profile.New(profileDeps(env, cfgPath)),
		"org":       org.New(org.Deps{Unary: client.Unary}),
		"connector": connector.New(connector.Deps{Unary: client.Unary, Endpoint: endpoint}),
		"chat":      chat.New(chatDeps(client)),
		"dashboard": dashboard.New(dashboard.Deps{Unary: client.Unary}),
		"playbook":  playbook.New(playbook.Deps{Unary: client.Unary}),
		"ontology":  ontology.New(ontology.Deps{Unary: client.Unary}),
		"feed":      feed.New(feed.Deps{Unary: client.Unary}),
		"audit":     audit.New(audit.Deps{Unary: client.Unary, Now: time.Now}),
		"version":   versionCmd{},
		"update":    updateCmd{deps: update.DefaultDeps()},
	}
}

// authDeps adapts the process-level config.Config <-> auth.Config boundary and
// wraps config.DefaultPath/Load/Save with closures that capture env + cfgPath
// + the selected profile name. LoadCfg/SaveCfg operate on the active profile
// slot: on Save we read the whole file, upsert the profile, and write it
// back, preserving every other profile (and any existing OrgName on the
// target profile — login is not responsible for clobbering that label).
func authDeps(client *transport.Client, env func(string) string, cfgPath, profileName string) auth.Deps {
	// Fallback to "default" if no profile was resolved (e.g. first-run login
	// with no config file on disk). The resolver already supplies this in
	// practice, but we keep the defense so SaveCfg never writes into "".
	if profileName == "" {
		profileName = "default"
	}
	return auth.Deps{
		Unary: client.Unary,
		LoadCfg: func() (auth.Config, error) {
			if cfgPath == "" {
				return auth.Config{}, nil
			}
			c, err := config.Load(cfgPath)
			if err != nil {
				return auth.Config{}, err
			}
			p := c.Profiles[profileName]
			return profileToAuthConfig(p), nil
		},
		SaveCfg: func(ac auth.Config) error {
			path := cfgPath
			if path == "" {
				p, err := config.DefaultPath(env)
				if err != nil {
					return err
				}
				path = p
			}
			// Read-modify-write: preserve every other profile and any
			// existing OrgName on the target. Load errors (malformed file,
			// permissions) propagate so the user sees them rather than
			// silently overwriting their config.
			existing, err := config.Load(path)
			if err != nil {
				return err
			}
			prev := existing.Profiles[profileName]
			existing.Upsert(profileName, config.Profile{
				Endpoint: ac.Endpoint,
				Token:    ac.Token,
				OrgName:  prev.OrgName,
			})
			return config.Save(path, existing)
		},
		ConfigPath: func() (string, error) {
			if cfgPath != "" {
				return cfgPath, nil
			}
			return config.DefaultPath(env)
		},
	}
}

// profileDeps wires profile.Deps. Unlike authDeps there is no adapter between
// config.Config and a narrower type: the profile verb package imports
// internal/config directly because managing profiles IS the whole config
// surface. LoadCfg/SaveCfg therefore pass config.Config through as-is.
func profileDeps(env func(string) string, cfgPath string) profile.Deps {
	return profile.Deps{
		LoadCfg: func() (config.Config, error) {
			path := cfgPath
			if path == "" {
				p, err := config.DefaultPath(env)
				if err != nil {
					return config.Config{}, err
				}
				path = p
			}
			return config.Load(path)
		},
		SaveCfg: func(c config.Config) error {
			path := cfgPath
			if path == "" {
				p, err := config.DefaultPath(env)
				if err != nil {
					return err
				}
				path = p
			}
			return config.Save(path, c)
		},
		ConfigPath: func() (string, error) {
			if cfgPath != "" {
				return cfgPath, nil
			}
			return config.DefaultPath(env)
		},
	}
}

// chatDeps wires chat.Deps. The stream adapter exists to narrow the return
// type from *transport.StreamReader to chat.StreamSession so the chat package
// (which must not import transport) can type-check against its own interface.
func chatDeps(client *transport.Client) chat.Deps {
	return chat.Deps{
		Unary:  client.Unary,
		Stream: streamAdapter(client),
		UUIDFn: newUUID,
	}
}

// streamAdapter exposes client.Stream through chat.StreamSession.
func streamAdapter(client *transport.Client) func(ctx context.Context, path string, req any) (chat.StreamSession, error) {
	return func(ctx context.Context, path string, req any) (chat.StreamSession, error) {
		sr, err := client.Stream(ctx, path, req)
		if err != nil {
			return nil, err
		}
		return &boundStream{ctx: ctx, sr: sr}, nil
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
// random bytes from crypto/rand; if the source fails (e.g. the kernel pool is
// unavailable) it falls back to a time-based hex blob so callers never block
// or panic. The fallback is deterministic-enough for cellIds in chat sends,
// which is the only place we mint UUIDs from the CLI today.
func newUUID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		// Fallback: 16 bytes derived from the wall clock. Not cryptographic,
		// but good enough to avoid a hard failure in a chat send loop.
		now := time.Now().UnixNano()
		for i := range 16 {
			b[i] = byte(now >> (uint(i%8) * 8))
		}
	}
	// Apply the v4 version + RFC 4122 variant bits.
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	// Format as 8-4-4-4-12 hex with dashes.
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
