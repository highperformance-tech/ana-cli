package auth

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/highperformance-tech/ana-cli/internal/cli"
	"github.com/highperformance-tech/ana-cli/internal/testcli"
)

func TestLogoutClearsToken(t *testing.T) {
	t.Parallel()
	f := &fakeDeps{loadFn: func() (Config, error) {
		return Config{Endpoint: "https://x", Token: "secret"}, nil
	}}
	cmd := &logoutCmd{deps: f.deps()}
	stdio, out, _ := testcli.NewIO(strings.NewReader(""))
	if err := cmd.Run(context.Background(), nil, stdio); err != nil {
		t.Fatalf("err=%v", err)
	}
	if f.saved == nil || f.saved.Token != "" {
		t.Errorf("saved=%+v want empty token", f.saved)
	}
	if f.saved.Endpoint != "https://x" {
		t.Errorf("endpoint lost: %+v", f.saved)
	}
	if !strings.Contains(out.String(), "logged out") {
		t.Errorf("stdout=%q", out.String())
	}
}

func TestLogoutLoadErr(t *testing.T) {
	t.Parallel()
	f := &fakeDeps{loadFn: func() (Config, error) { return Config{}, errors.New("load boom") }}
	cmd := &logoutCmd{deps: f.deps()}
	stdio, _, _ := testcli.NewIO(strings.NewReader(""))
	err := cmd.Run(context.Background(), nil, stdio)
	if err == nil || !strings.Contains(err.Error(), "load boom") {
		t.Errorf("err=%v", err)
	}
}

func TestLogoutSaveErr(t *testing.T) {
	t.Parallel()
	f := &fakeDeps{saveFn: func(Config) error { return errors.New("save boom") }}
	cmd := &logoutCmd{deps: f.deps()}
	stdio, _, _ := testcli.NewIO(strings.NewReader(""))
	err := cmd.Run(context.Background(), nil, stdio)
	if err == nil || !strings.Contains(err.Error(), "save boom") {
		t.Errorf("err=%v", err)
	}
}

func TestLogoutUnexpectedArgs(t *testing.T) {
	t.Parallel()
	f := &fakeDeps{}
	cmd := &logoutCmd{deps: f.deps()}
	stdio, _, _ := testcli.NewIO(strings.NewReader(""))
	err := cmd.Run(context.Background(), []string{"extra"}, stdio)
	if !errors.Is(err, cli.ErrUsage) {
		t.Errorf("err=%v want ErrUsage", err)
	}
}

func TestLogoutBadFlag(t *testing.T) {
	t.Parallel()
	f := &fakeDeps{}
	stdio, _, _ := testcli.NewIO(strings.NewReader(""))
	err := New(f.deps()).Run(context.Background(), []string{"logout", "--nope"}, stdio)
	if !errors.Is(err, cli.ErrUsage) {
		t.Errorf("err=%v want ErrUsage", err)
	}
}
