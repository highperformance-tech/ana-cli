// Package harness wires the ana CLI against a real TextQL endpoint for
// live smoke tests. It duplicates the cmd/ana/main.go verb-building logic
// so tests can call CLI verbs and raw RPCs through the same transport.
package harness

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"time"

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
)

// envSpec is the env-var contract required by the harness. See e2e/README.md.
type envSpec struct {
	endpoint    string
	token       string
	expectOrgID string
	dryRun      bool
	sweepOnly   bool
	configHome  string
}

func loadEnv() (envSpec, bool) {
	e := envSpec{
		endpoint:    os.Getenv("ANA_E2E_ENDPOINT"),
		token:       os.Getenv("ANA_E2E_TOKEN"),
		expectOrgID: os.Getenv("ANA_E2E_EXPECT_ORG_ID"),
		dryRun:      os.Getenv("ANA_E2E_DRYRUN") == "1",
		sweepOnly:   os.Getenv("ANA_E2E_SWEEP_ONLY") == "1",
	}
	return e, e.endpoint != "" && e.token != ""
}

// buildTransport constructs a transport.Client bound to the live endpoint
// with a static token. Mirrors cmd/ana/main.go:run's wiring.
func buildTransport(endpoint, token string) *transport.Client {
	tokenFn := func(ctx context.Context) (string, error) { return token, nil }
	return transport.New(endpoint, tokenFn, transport.WithUserAgent("ana-e2e"))
}

// seedConfig writes a one-profile config file into dir so CLI invocations
// resolve endpoint+token through the same path a logged-in user would.
func seedConfig(dir, endpoint, token string) (string, error) {
	cfgPath := fmt.Sprintf("%s/ana/config.json", dir)
	cfg := config.Config{
		Profiles: map[string]config.Profile{
			"default": {Endpoint: endpoint, Token: token, OrgName: "e2e"},
		},
		Active: "default",
	}
	if err := config.Save(cfgPath, cfg); err != nil {
		return "", err
	}
	return cfgPath, nil
}

// makeEnv builds the env lookup func the harness hands to cli.Dispatch and to
// the wiring closures. XDG_CONFIG_HOME is pinned to the per-test tmpdir so the
// real ~/.config/ana/config.json never gets touched.
func makeEnv(configHome string) func(string) string {
	return func(k string) string {
		switch k {
		case "XDG_CONFIG_HOME":
			return configHome
		}
		return ""
	}
}

// buildVerbs duplicates cmd/ana/main.go:buildVerbs. The logic is kept in sync
// by hand; TestBuildVerbs_Shape in cmd/ana/main_test.go guards the verb set.
func buildVerbs(client *transport.Client, env func(string) string, cfgPath string) map[string]cli.Command {
	return map[string]cli.Command{
		"auth":      auth.New(authDeps(client, env, cfgPath)),
		"profile":   profile.New(profileDeps(env, cfgPath)),
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

func authDeps(client *transport.Client, env func(string) string, cfgPath string) auth.Deps {
	return auth.Deps{
		Unary: client.Unary,
		LoadCfg: func() (auth.Config, error) {
			c, err := config.Load(cfgPath)
			if err != nil {
				return auth.Config{}, err
			}
			p := c.Profiles["default"]
			return auth.Config{Endpoint: p.Endpoint, Token: p.Token}, nil
		},
		SaveCfg: func(ac auth.Config) error {
			existing, err := config.Load(cfgPath)
			if err != nil {
				return err
			}
			prev := existing.Profiles["default"]
			existing.Upsert("default", config.Profile{
				Endpoint: ac.Endpoint,
				Token:    ac.Token,
				OrgName:  prev.OrgName,
			})
			return config.Save(cfgPath, existing)
		},
		ConfigPath: func() (string, error) { return cfgPath, nil },
	}
}

func profileDeps(env func(string) string, cfgPath string) profile.Deps {
	return profile.Deps{
		LoadCfg:    func() (config.Config, error) { return config.Load(cfgPath) },
		SaveCfg:    func(c config.Config) error { return config.Save(cfgPath, c) },
		ConfigPath: func() (string, error) { return cfgPath, nil },
	}
}

func chatDeps(client *transport.Client) chat.Deps {
	return chat.Deps{
		Unary: client.Unary,
		Stream: func(ctx context.Context, path string, req any) (chat.StreamSession, error) {
			sr, err := client.Stream(ctx, path, req)
			if err != nil {
				return nil, err
			}
			return sr, nil
		},
		UUIDFn: newUUID,
	}
}

// newUUID mirrors cmd/ana/main.go:newUUID with the same fallback — crypto/rand
// may fail on locked-down hosts and we do not want a streamed send test to
// panic mid-run. Duplicated here so harness/client.go has zero cmd/ana deps.
func newUUID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		now := time.Now().UnixNano()
		for i := 0; i < 16; i++ {
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
