package connector

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/highperformance-tech/ana-cli/internal/cli"
	"github.com/highperformance-tech/ana-cli/internal/testcli"
)

func TestDeleteHappy(t *testing.T) {
	t.Parallel()
	f := &fakeDeps{}
	cmd := &deleteCmd{deps: f.deps()}
	stdio, out, _ := testcli.NewIO(strings.NewReader(""))
	if err := cmd.Run(context.Background(), []string{"7"}, stdio); err != nil {
		t.Fatalf("err=%v", err)
	}
	if !strings.Contains(out.String(), "deleted 7") {
		t.Errorf("stdout=%q", out.String())
	}
	if !strings.Contains(string(f.lastRawReq), `"connectorId":7`) {
		t.Errorf("req=%s", string(f.lastRawReq))
	}
	if f.lastPath != servicePath+"/DeleteConnector" {
		t.Errorf("path=%s", f.lastPath)
	}
}

func TestDeleteMissingPositional(t *testing.T) {
	t.Parallel()
	cmd := &deleteCmd{deps: (&fakeDeps{}).deps()}
	stdio, _, _ := testcli.NewIO(strings.NewReader(""))
	err := cmd.Run(context.Background(), nil, stdio)
	if !errors.Is(err, cli.ErrUsage) {
		t.Errorf("err=%v", err)
	}
}

func TestDeleteNonInt(t *testing.T) {
	t.Parallel()
	cmd := &deleteCmd{deps: (&fakeDeps{}).deps()}
	stdio, _, _ := testcli.NewIO(strings.NewReader(""))
	err := cmd.Run(context.Background(), []string{"abc"}, stdio)
	if !errors.Is(err, cli.ErrUsage) {
		t.Errorf("err=%v", err)
	}
}

func TestDeleteUnaryErr(t *testing.T) {
	t.Parallel()
	f := &fakeDeps{unaryFn: func(_ context.Context, _ string, _, _ any) error { return errors.New("boom") }}
	cmd := &deleteCmd{deps: f.deps()}
	stdio, _, _ := testcli.NewIO(strings.NewReader(""))
	err := cmd.Run(context.Background(), []string{"1"}, stdio)
	if err == nil || !strings.Contains(err.Error(), "boom") {
		t.Errorf("err=%v", err)
	}
}

func TestDeleteBadFlag(t *testing.T) {
	t.Parallel()
	cmd := &deleteCmd{deps: (&fakeDeps{}).deps()}
	stdio, _, _ := testcli.NewIO(strings.NewReader(""))
	err := cmd.Run(context.Background(), []string{"--nope"}, stdio)
	if !errors.Is(err, cli.ErrUsage) {
		t.Errorf("err=%v", err)
	}
}
