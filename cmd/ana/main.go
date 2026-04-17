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
	"os"
	"os/signal"
	"time"

	"github.com/textql/ana-cli/internal/audit"
	"github.com/textql/ana-cli/internal/auth"
	"github.com/textql/ana-cli/internal/chat"
	"github.com/textql/ana-cli/internal/cli"
	"github.com/textql/ana-cli/internal/config"
	"github.com/textql/ana-cli/internal/connector"
	"github.com/textql/ana-cli/internal/dashboard"
	"github.com/textql/ana-cli/internal/feed"
	"github.com/textql/ana-cli/internal/ontology"
	"github.com/textql/ana-cli/internal/org"
	"github.com/textql/ana-cli/internal/playbook"
	"github.com/textql/ana-cli/internal/transport"
)

func main() {
	stdio := cli.DefaultIO()
	err := run(os.Args[1:], stdio, os.Getenv)
	if err != nil && !errors.Is(err, cli.ErrUsage) {
		// cli.ErrUsage means help/usage text has already been emitted to the
		// appropriate stream; any other error is a runtime failure that
		// hasn't been reported yet, so surface it on stderr.
		fmt.Fprintln(stdio.Stderr, err)
	}
	os.Exit(cli.ExitCode(err))
}

// run is the testable entrypoint: same signature as main() but with args,
// stdio, and env injected. Returns the error that main() feeds to ExitCode.
func run(args []string, stdio cli.IO, env func(string) string) error {
	// Parse global flags up front so the token-file/endpoint overrides are
	// available before we touch config on disk. cli.Dispatch re-parses
	// globals, but doing it here lets us wire deps correctly before dispatch.
	global, _, err := cli.ParseGlobal(args)
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
	resolved := config.Resolve(env, loaded)
	if global.Endpoint != "" {
		resolved.Endpoint = global.Endpoint
	}

	token := resolved.Token
	tokenFn := func(ctx context.Context) (string, error) {
		return token, nil
	}
	client := transport.New(resolved.Endpoint, tokenFn)

	verbs := buildVerbs(client, env, cfgPath)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	return cli.Dispatch(ctx, verbs, args, stdio)
}

// buildVerbs wires every verb package's Deps against the shared transport
// client and config path. Pulled out of run() so main_test can exercise it in
// isolation.
func buildVerbs(client *transport.Client, env func(string) string, cfgPath string) map[string]cli.Command {
	return map[string]cli.Command{
		"auth":      auth.New(authDeps(client, env, cfgPath)),
		"org":       org.New(org.Deps{Unary: client.Unary}),
		"connector": connector.New(connector.Deps{Unary: client.Unary}),
		"chat":      chat.New(chatDeps(client)),
		"dashboard": dashboard.New(dashboard.Deps{Unary: client.Unary}),
		"playbook":  playbook.New(playbook.Deps{Unary: client.Unary}),
		"ontology":  ontology.New(ontology.Deps{Unary: client.Unary}),
		"feed":      feed.New(feed.Deps{Unary: client.Unary}),
		"audit":     audit.New(audit.Deps{Unary: client.Unary, Now: time.Now}),
	}
}

// authDeps adapts the process-level config.Config <-> auth.Config boundary and
// wraps config.DefaultPath/Load/Save with closures that capture env + cfgPath.
func authDeps(client *transport.Client, env func(string) string, cfgPath string) auth.Deps {
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
			return toAuthConfig(c), nil
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
			return config.Save(path, fromAuthConfig(ac))
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

// streamAdapter exposes client.Stream through chat.StreamSession. Factored as
// a named helper so main_test can cover it without standing up a live server.
func streamAdapter(client *transport.Client) func(ctx context.Context, path string, req any) (chat.StreamSession, error) {
	return func(ctx context.Context, path string, req any) (chat.StreamSession, error) {
		sr, err := client.Stream(ctx, path, req)
		if err != nil {
			return nil, err
		}
		return sr, nil
	}
}

// toAuthConfig projects the persisted config.Config down to the subset
// internal/auth cares about. Keeping the projection in main.go means auth
// never imports internal/config — the contract stays narrow.
func toAuthConfig(c config.Config) auth.Config {
	return auth.Config{Endpoint: c.Endpoint, Token: c.Token}
}

// fromAuthConfig is the inverse of toAuthConfig. Any fields config.Config adds
// later (telemetry prefs, org id) are preserved by reading them first, but
// today the two shapes are equal so a direct copy is fine.
func fromAuthConfig(ac auth.Config) config.Config {
	return config.Config{Endpoint: ac.Endpoint, Token: ac.Token}
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
		for i := 0; i < 16; i++ {
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
