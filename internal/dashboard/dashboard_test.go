package dashboard

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/textql/ana-cli/internal/cli"
)

// --- fakes and helpers ---

// fakeDeps captures each Unary invocation's path + encoded request bytes so
// tests can assert on both the endpoint and the wire-level JSON shape
// (camelCase field names, array-vs-scalar, etc.).
type fakeDeps struct {
	unaryFn    func(ctx context.Context, path string, req, resp any) error
	lastPath   string
	lastRawReq []byte
}

func (f *fakeDeps) deps() Deps {
	return Deps{
		Unary: func(ctx context.Context, path string, req, resp any) error {
			f.lastPath = path
			if b, err := json.Marshal(req); err == nil {
				f.lastRawReq = b
			}
			if f.unaryFn != nil {
				return f.unaryFn(ctx, path, req, resp)
			}
			return nil
		},
	}
}

// newIO returns a cli.IO with in-memory streams.
func newIO(stdin io.Reader) (cli.IO, *bytes.Buffer, *bytes.Buffer) {
	var out, errb bytes.Buffer
	return cli.IO{
		Stdin:  stdin,
		Stdout: &out,
		Stderr: &errb,
		Env:    func(string) string { return "" },
		Now:    func() time.Time { return time.Unix(0, 0) },
	}, &out, &errb
}

// failingWriter trips writeJSON's encoder error path.
type failingWriter struct{}

func (failingWriter) Write([]byte) (int, error) { return 0, errors.New("w boom") }

// --- New / Group surface ---

func TestNewReturnsGroupWithExpectedChildren(t *testing.T) {
	g := New(Deps{})
	if g == nil || g.Children == nil {
		t.Fatalf("New returned empty group")
	}
	for _, name := range []string{"list", "folders", "get", "spawn", "health"} {
		if _, ok := g.Children[name]; !ok {
			t.Errorf("missing child %q", name)
		}
	}
	if g.Summary == "" {
		t.Errorf("Summary should be non-empty")
	}
	// folders is itself a group with a list child.
	folders, ok := g.Children["folders"].(*cli.Group)
	if !ok {
		t.Fatalf("folders child must be *cli.Group")
	}
	if _, ok := folders.Children["list"]; !ok {
		t.Errorf("folders group missing list child")
	}
	if folders.Summary == "" {
		t.Errorf("folders Summary should be non-empty")
	}
}

func TestHelpStringsNonEmpty(t *testing.T) {
	cases := map[string]cli.Command{
		"list":         &listCmd{},
		"folders-list": &foldersListCmd{},
		"get":          &getCmd{},
		"spawn":        &spawnCmd{},
		"health":       &healthCmd{},
	}
	for n, c := range cases {
		h := c.Help()
		if h == "" {
			t.Errorf("%s: empty help", n)
		}
		if !strings.Contains(strings.ToLower(h), "usage") {
			t.Errorf("%s: help missing usage: %q", n, h)
		}
	}
}

// --- list ---

func TestListTable(t *testing.T) {
	f := &fakeDeps{
		unaryFn: func(_ context.Context, _ string, _, resp any) error {
			out := resp.(*map[string]any)
			*out = map[string]any{
				"dashboards": []any{
					map[string]any{"id": "d1", "name": "alpha", "folderName": "F1"},
					map[string]any{"id": "d2", "name": "beta", "folderId": "fid-2"},
					map[string]any{"id": "d3", "name": "gamma"},
				},
			}
			return nil
		},
	}
	cmd := &listCmd{deps: f.deps()}
	stdio, out, _ := newIO(strings.NewReader(""))
	if err := cmd.Run(context.Background(), nil, stdio); err != nil {
		t.Fatalf("err=%v", err)
	}
	s := out.String()
	for _, want := range []string{"ID", "NAME", "FOLDER", "alpha", "beta", "gamma", "F1", "fid-2", "-"} {
		if !strings.Contains(s, want) {
			t.Errorf("missing %q in %q", want, s)
		}
	}
	if f.lastPath != servicePath+"/ListDashboards" {
		t.Errorf("path=%s", f.lastPath)
	}
	if string(f.lastRawReq) != "{}" {
		t.Errorf("req=%s", string(f.lastRawReq))
	}
}

func TestListJSON(t *testing.T) {
	f := &fakeDeps{
		unaryFn: func(_ context.Context, _ string, _, resp any) error {
			out := resp.(*map[string]any)
			*out = map[string]any{"dashboards": []any{}}
			return nil
		},
	}
	cmd := &listCmd{deps: f.deps()}
	ctx := cli.WithGlobal(context.Background(), cli.Global{JSON: true})
	stdio, out, _ := newIO(strings.NewReader(""))
	if err := cmd.Run(ctx, nil, stdio); err != nil {
		t.Fatalf("err=%v", err)
	}
	if !strings.Contains(out.String(), "\"dashboards\"") {
		t.Errorf("stdout=%q", out.String())
	}
}

func TestListUnaryErr(t *testing.T) {
	f := &fakeDeps{unaryFn: func(_ context.Context, _ string, _, _ any) error { return errors.New("boom") }}
	cmd := &listCmd{deps: f.deps()}
	stdio, _, _ := newIO(strings.NewReader(""))
	err := cmd.Run(context.Background(), nil, stdio)
	if err == nil || !strings.Contains(err.Error(), "boom") || !strings.Contains(err.Error(), "dashboard list") {
		t.Errorf("err=%v", err)
	}
}

func TestListBadFlag(t *testing.T) {
	cmd := &listCmd{deps: (&fakeDeps{}).deps()}
	stdio, _, _ := newIO(strings.NewReader(""))
	err := cmd.Run(context.Background(), []string{"--nope"}, stdio)
	if !errors.Is(err, cli.ErrUsage) {
		t.Errorf("err=%v", err)
	}
}

func TestListRemarshalErr(t *testing.T) {
	f := &fakeDeps{
		unaryFn: func(_ context.Context, _ string, _, resp any) error {
			out := resp.(*map[string]any)
			*out = map[string]any{"dashboards": "nope"}
			return nil
		},
	}
	cmd := &listCmd{deps: f.deps()}
	stdio, _, _ := newIO(strings.NewReader(""))
	err := cmd.Run(context.Background(), nil, stdio)
	if err == nil || !strings.Contains(err.Error(), "decode response") {
		t.Errorf("err=%v", err)
	}
}

// --- folders list ---

func TestFoldersListTable(t *testing.T) {
	f := &fakeDeps{
		unaryFn: func(_ context.Context, _ string, _, resp any) error {
			out := resp.(*map[string]any)
			*out = map[string]any{
				"folders": []any{
					map[string]any{"id": "b", "name": "Beta"},
					map[string]any{"id": "a", "name": "Alpha"},
				},
			}
			return nil
		},
	}
	cmd := &foldersListCmd{deps: f.deps()}
	stdio, out, _ := newIO(strings.NewReader(""))
	if err := cmd.Run(context.Background(), nil, stdio); err != nil {
		t.Fatalf("err=%v", err)
	}
	s := out.String()
	// Alpha must appear before Beta thanks to the sort.
	if ai, bi := strings.Index(s, "Alpha"), strings.Index(s, "Beta"); ai < 0 || bi < 0 || ai > bi {
		t.Errorf("sort broken: %q", s)
	}
	if f.lastPath != servicePath+"/ListDashboardFolders" {
		t.Errorf("path=%s", f.lastPath)
	}
	if string(f.lastRawReq) != "{}" {
		t.Errorf("req=%s", string(f.lastRawReq))
	}
}

func TestFoldersListJSON(t *testing.T) {
	f := &fakeDeps{
		unaryFn: func(_ context.Context, _ string, _, resp any) error {
			out := resp.(*map[string]any)
			*out = map[string]any{"folders": []any{}}
			return nil
		},
	}
	cmd := &foldersListCmd{deps: f.deps()}
	ctx := cli.WithGlobal(context.Background(), cli.Global{JSON: true})
	stdio, out, _ := newIO(strings.NewReader(""))
	if err := cmd.Run(ctx, nil, stdio); err != nil {
		t.Fatalf("err=%v", err)
	}
	if !strings.Contains(out.String(), "\"folders\"") {
		t.Errorf("stdout=%q", out.String())
	}
}

func TestFoldersListUnaryErr(t *testing.T) {
	f := &fakeDeps{unaryFn: func(_ context.Context, _ string, _, _ any) error { return errors.New("boom") }}
	cmd := &foldersListCmd{deps: f.deps()}
	stdio, _, _ := newIO(strings.NewReader(""))
	err := cmd.Run(context.Background(), nil, stdio)
	if err == nil || !strings.Contains(err.Error(), "boom") {
		t.Errorf("err=%v", err)
	}
}

func TestFoldersListBadFlag(t *testing.T) {
	cmd := &foldersListCmd{deps: (&fakeDeps{}).deps()}
	stdio, _, _ := newIO(strings.NewReader(""))
	err := cmd.Run(context.Background(), []string{"--nope"}, stdio)
	if !errors.Is(err, cli.ErrUsage) {
		t.Errorf("err=%v", err)
	}
}

func TestFoldersListRemarshalErr(t *testing.T) {
	f := &fakeDeps{
		unaryFn: func(_ context.Context, _ string, _, resp any) error {
			out := resp.(*map[string]any)
			*out = map[string]any{"folders": "nope"}
			return nil
		},
	}
	cmd := &foldersListCmd{deps: f.deps()}
	stdio, _, _ := newIO(strings.NewReader(""))
	err := cmd.Run(context.Background(), nil, stdio)
	if err == nil || !strings.Contains(err.Error(), "decode response") {
		t.Errorf("err=%v", err)
	}
}

// --- get ---

func TestGetSummary(t *testing.T) {
	f := &fakeDeps{
		unaryFn: func(_ context.Context, _ string, _, resp any) error {
			out := resp.(*map[string]any)
			*out = map[string]any{
				"dashboard": map[string]any{
					"id":        "d1",
					"name":      "HPT",
					"orgId":     "o1",
					"creatorId": "c1",
					"code":      "print(1)",
				},
			}
			return nil
		},
	}
	cmd := &getCmd{deps: f.deps()}
	stdio, out, _ := newIO(strings.NewReader(""))
	if err := cmd.Run(context.Background(), []string{"d1"}, stdio); err != nil {
		t.Fatalf("err=%v", err)
	}
	s := out.String()
	for _, want := range []string{"id:", "name:", "HPT", "orgId:", "creatorId:", "code:", "8 bytes"} {
		if !strings.Contains(s, want) {
			t.Errorf("missing %q: %q", want, s)
		}
	}
	if !strings.Contains(string(f.lastRawReq), `"dashboardId":"d1"`) {
		t.Errorf("req=%s", string(f.lastRawReq))
	}
	if f.lastPath != servicePath+"/GetDashboard" {
		t.Errorf("path=%s", f.lastPath)
	}
}

func TestGetJSON(t *testing.T) {
	f := &fakeDeps{
		unaryFn: func(_ context.Context, _ string, _, resp any) error {
			out := resp.(*map[string]any)
			*out = map[string]any{"dashboard": map[string]any{"id": "x"}}
			return nil
		},
	}
	cmd := &getCmd{deps: f.deps()}
	ctx := cli.WithGlobal(context.Background(), cli.Global{JSON: true})
	stdio, out, _ := newIO(strings.NewReader(""))
	if err := cmd.Run(ctx, []string{"x"}, stdio); err != nil {
		t.Fatalf("err=%v", err)
	}
	if !strings.Contains(out.String(), "\"dashboard\"") {
		t.Errorf("stdout=%q", out.String())
	}
}

func TestGetNoDashboardKeyFallback(t *testing.T) {
	f := &fakeDeps{
		unaryFn: func(_ context.Context, _ string, _, resp any) error {
			out := resp.(*map[string]any)
			*out = map[string]any{"other": 1.0}
			return nil
		},
	}
	cmd := &getCmd{deps: f.deps()}
	stdio, out, _ := newIO(strings.NewReader(""))
	if err := cmd.Run(context.Background(), []string{"x"}, stdio); err != nil {
		t.Fatalf("err=%v", err)
	}
	if !strings.Contains(out.String(), "\"other\"") {
		t.Errorf("stdout=%q", out.String())
	}
}

func TestGetMissingPositional(t *testing.T) {
	cmd := &getCmd{deps: (&fakeDeps{}).deps()}
	stdio, _, _ := newIO(strings.NewReader(""))
	err := cmd.Run(context.Background(), nil, stdio)
	if !errors.Is(err, cli.ErrUsage) {
		t.Errorf("err=%v", err)
	}
}

func TestGetUnaryErr(t *testing.T) {
	f := &fakeDeps{unaryFn: func(_ context.Context, _ string, _, _ any) error { return errors.New("boom") }}
	cmd := &getCmd{deps: f.deps()}
	stdio, _, _ := newIO(strings.NewReader(""))
	err := cmd.Run(context.Background(), []string{"x"}, stdio)
	if err == nil || !strings.Contains(err.Error(), "boom") {
		t.Errorf("err=%v", err)
	}
}

func TestGetBadFlag(t *testing.T) {
	cmd := &getCmd{deps: (&fakeDeps{}).deps()}
	stdio, _, _ := newIO(strings.NewReader(""))
	err := cmd.Run(context.Background(), []string{"--nope"}, stdio)
	if !errors.Is(err, cli.ErrUsage) {
		t.Errorf("err=%v", err)
	}
}

// --- spawn ---

func TestSpawnHappy(t *testing.T) {
	f := &fakeDeps{
		unaryFn: func(_ context.Context, _ string, _, resp any) error {
			out := resp.(*map[string]any)
			*out = map[string]any{"refreshedAt": "2026-04-16T16:00:18Z"}
			return nil
		},
	}
	cmd := &spawnCmd{deps: f.deps()}
	stdio, out, _ := newIO(strings.NewReader(""))
	if err := cmd.Run(context.Background(), []string{"d1"}, stdio); err != nil {
		t.Fatalf("err=%v", err)
	}
	if !strings.Contains(out.String(), "spawned d1") || !strings.Contains(out.String(), "2026-04-16T16:00:18Z") {
		t.Errorf("stdout=%q", out.String())
	}
	if f.lastPath != servicePath+"/SpawnDashboard" {
		t.Errorf("path=%s", f.lastPath)
	}
	if !strings.Contains(string(f.lastRawReq), `"dashboardId":"d1"`) {
		t.Errorf("req=%s", string(f.lastRawReq))
	}
}

func TestSpawnJSON(t *testing.T) {
	f := &fakeDeps{
		unaryFn: func(_ context.Context, _ string, _, resp any) error {
			out := resp.(*map[string]any)
			*out = map[string]any{"refreshedAt": "t"}
			return nil
		},
	}
	cmd := &spawnCmd{deps: f.deps()}
	ctx := cli.WithGlobal(context.Background(), cli.Global{JSON: true})
	stdio, out, _ := newIO(strings.NewReader(""))
	if err := cmd.Run(ctx, []string{"d1"}, stdio); err != nil {
		t.Fatalf("err=%v", err)
	}
	if !strings.Contains(out.String(), "\"refreshedAt\"") {
		t.Errorf("stdout=%q", out.String())
	}
}

func TestSpawnNoRefreshedAtFallback(t *testing.T) {
	f := &fakeDeps{
		unaryFn: func(_ context.Context, _ string, _, resp any) error {
			out := resp.(*map[string]any)
			*out = map[string]any{"weird": true}
			return nil
		},
	}
	cmd := &spawnCmd{deps: f.deps()}
	stdio, out, _ := newIO(strings.NewReader(""))
	if err := cmd.Run(context.Background(), []string{"d1"}, stdio); err != nil {
		t.Fatalf("err=%v", err)
	}
	if !strings.Contains(out.String(), "\"weird\"") {
		t.Errorf("stdout=%q", out.String())
	}
}

func TestSpawnMissingPositional(t *testing.T) {
	cmd := &spawnCmd{deps: (&fakeDeps{}).deps()}
	stdio, _, _ := newIO(strings.NewReader(""))
	err := cmd.Run(context.Background(), nil, stdio)
	if !errors.Is(err, cli.ErrUsage) {
		t.Errorf("err=%v", err)
	}
}

func TestSpawnUnaryErr(t *testing.T) {
	f := &fakeDeps{unaryFn: func(_ context.Context, _ string, _, _ any) error { return errors.New("boom") }}
	cmd := &spawnCmd{deps: f.deps()}
	stdio, _, _ := newIO(strings.NewReader(""))
	err := cmd.Run(context.Background(), []string{"d1"}, stdio)
	if err == nil || !strings.Contains(err.Error(), "boom") {
		t.Errorf("err=%v", err)
	}
}

func TestSpawnBadFlag(t *testing.T) {
	cmd := &spawnCmd{deps: (&fakeDeps{}).deps()}
	stdio, _, _ := newIO(strings.NewReader(""))
	err := cmd.Run(context.Background(), []string{"--nope"}, stdio)
	if !errors.Is(err, cli.ErrUsage) {
		t.Errorf("err=%v", err)
	}
}

// --- health ---

func TestHealthHealthy(t *testing.T) {
	f := &fakeDeps{
		unaryFn: func(_ context.Context, _ string, _, resp any) error {
			out := resp.(*map[string]any)
			*out = map[string]any{
				"dashboards": []any{
					map[string]any{
						"dashboardId":  "d1",
						"status":       "HEALTH_STATUS_HEALTHY",
						"streamlitUrl": "x:8501",
						"embedUrl":     "/sandbox/proxy/x/8501/",
					},
				},
			}
			return nil
		},
	}
	cmd := &healthCmd{deps: f.deps()}
	stdio, out, _ := newIO(strings.NewReader(""))
	if err := cmd.Run(context.Background(), []string{"d1"}, stdio); err != nil {
		t.Fatalf("err=%v", err)
	}
	s := out.String()
	if !strings.Contains(s, "d1 HEALTHY") || !strings.Contains(s, "streamlitUrl: x:8501") || !strings.Contains(s, "embedUrl: /sandbox/proxy/x/8501/") {
		t.Errorf("stdout=%q", s)
	}
	if f.lastPath != servicePath+"/CheckDashboardHealth" {
		t.Errorf("path=%s", f.lastPath)
	}
	// Catalog-shape check: wire body must be plural + array.
	if !strings.Contains(string(f.lastRawReq), `"dashboardIds":["d1"]`) {
		t.Errorf("req=%s", string(f.lastRawReq))
	}
}

func TestHealthUnhealthyWithMessage(t *testing.T) {
	f := &fakeDeps{
		unaryFn: func(_ context.Context, _ string, _, resp any) error {
			out := resp.(*map[string]any)
			*out = map[string]any{
				"dashboards": []any{
					map[string]any{
						"dashboardId": "d1",
						"status":      "HEALTH_STATUS_UNHEALTHY",
						"message":     "container crashed",
					},
				},
			}
			return nil
		},
	}
	cmd := &healthCmd{deps: f.deps()}
	stdio, out, _ := newIO(strings.NewReader(""))
	if err := cmd.Run(context.Background(), []string{"d1"}, stdio); err != nil {
		t.Fatalf("err=%v", err)
	}
	if !strings.Contains(out.String(), "d1 UNHEALTHY: container crashed") {
		t.Errorf("stdout=%q", out.String())
	}
}

func TestHealthUnknownAndCustomStatusLabels(t *testing.T) {
	cases := map[string]string{
		"":                          "UNKNOWN",
		"HEALTH_STATUS_UNSPECIFIED": "UNKNOWN",
		"HEALTH_STATUS_HEALTHY":     "HEALTHY",
		"HEALTH_STATUS_UNHEALTHY":   "UNHEALTHY",
		"HEALTH_STATUS_DEGRADED":    "DEGRADED", // TrimPrefix fallback
		"totally-other":             "totally-other",
	}
	for in, want := range cases {
		if got := healthLabel(in); got != want {
			t.Errorf("healthLabel(%q)=%q want %q", in, got, want)
		}
	}
}

func TestHealthJSON(t *testing.T) {
	f := &fakeDeps{
		unaryFn: func(_ context.Context, _ string, _, resp any) error {
			out := resp.(*map[string]any)
			*out = map[string]any{"dashboards": []any{map[string]any{"dashboardId": "d1"}}}
			return nil
		},
	}
	cmd := &healthCmd{deps: f.deps()}
	ctx := cli.WithGlobal(context.Background(), cli.Global{JSON: true})
	stdio, out, _ := newIO(strings.NewReader(""))
	if err := cmd.Run(ctx, []string{"d1"}, stdio); err != nil {
		t.Fatalf("err=%v", err)
	}
	if !strings.Contains(out.String(), "\"dashboards\"") {
		t.Errorf("stdout=%q", out.String())
	}
}

func TestHealthEmptyDashboards(t *testing.T) {
	f := &fakeDeps{
		unaryFn: func(_ context.Context, _ string, _, resp any) error {
			out := resp.(*map[string]any)
			*out = map[string]any{"dashboards": []any{}}
			return nil
		},
	}
	cmd := &healthCmd{deps: f.deps()}
	stdio, _, _ := newIO(strings.NewReader(""))
	err := cmd.Run(context.Background(), []string{"d1"}, stdio)
	if !errors.Is(err, cli.ErrUsage) {
		t.Errorf("err=%v", err)
	}
}

func TestHealthMissingPositional(t *testing.T) {
	cmd := &healthCmd{deps: (&fakeDeps{}).deps()}
	stdio, _, _ := newIO(strings.NewReader(""))
	err := cmd.Run(context.Background(), nil, stdio)
	if !errors.Is(err, cli.ErrUsage) {
		t.Errorf("err=%v", err)
	}
}

func TestHealthWhitespacePositional(t *testing.T) {
	// requireID also rejects a pure-whitespace positional so we don't POST
	// a meaningless request.
	cmd := &healthCmd{deps: (&fakeDeps{}).deps()}
	stdio, _, _ := newIO(strings.NewReader(""))
	err := cmd.Run(context.Background(), []string{"   "}, stdio)
	if !errors.Is(err, cli.ErrUsage) {
		t.Errorf("err=%v", err)
	}
}

func TestHealthUnaryErr(t *testing.T) {
	f := &fakeDeps{unaryFn: func(_ context.Context, _ string, _, _ any) error { return errors.New("boom") }}
	cmd := &healthCmd{deps: f.deps()}
	stdio, _, _ := newIO(strings.NewReader(""))
	err := cmd.Run(context.Background(), []string{"d1"}, stdio)
	if err == nil || !strings.Contains(err.Error(), "boom") {
		t.Errorf("err=%v", err)
	}
}

func TestHealthBadFlag(t *testing.T) {
	cmd := &healthCmd{deps: (&fakeDeps{}).deps()}
	stdio, _, _ := newIO(strings.NewReader(""))
	err := cmd.Run(context.Background(), []string{"--nope"}, stdio)
	if !errors.Is(err, cli.ErrUsage) {
		t.Errorf("err=%v", err)
	}
}

func TestHealthRemarshalErr(t *testing.T) {
	f := &fakeDeps{
		unaryFn: func(_ context.Context, _ string, _, resp any) error {
			out := resp.(*map[string]any)
			*out = map[string]any{"dashboards": "nope"}
			return nil
		},
	}
	cmd := &healthCmd{deps: f.deps()}
	stdio, _, _ := newIO(strings.NewReader(""))
	err := cmd.Run(context.Background(), []string{"d1"}, stdio)
	if err == nil || !strings.Contains(err.Error(), "decode response") {
		t.Errorf("err=%v", err)
	}
}

// --- direct util tests ---

func TestWriteJSONErr(t *testing.T) {
	// Channel value → Marshal fails inside writeJSON (via encoder).
	if err := writeJSON(&bytes.Buffer{}, make(chan int)); err == nil {
		t.Errorf("want encode error")
	}
}

func TestWriteJSONWriterErr(t *testing.T) {
	// failingWriter trips the encoder's write path.
	if err := writeJSON(failingWriter{}, map[string]any{"a": 1}); err == nil {
		t.Errorf("want writer error")
	}
}

func TestRemarshalMarshalErr(t *testing.T) {
	if err := remarshal(make(chan int), &struct{}{}); err == nil {
		t.Errorf("want marshal error")
	}
}

func TestRequireIDPaths(t *testing.T) {
	if _, err := requireID("x", nil); !errors.Is(err, cli.ErrUsage) {
		t.Errorf("nil: %v", err)
	}
	if _, err := requireID("x", []string{""}); !errors.Is(err, cli.ErrUsage) {
		t.Errorf("empty: %v", err)
	}
	if _, err := requireID("x", []string{"   "}); !errors.Is(err, cli.ErrUsage) {
		t.Errorf("ws: %v", err)
	}
	if v, err := requireID("x", []string{"abc"}); err != nil || v != "abc" {
		t.Errorf("good: v=%q err=%v", v, err)
	}
}

func TestUsageErrfWraps(t *testing.T) {
	err := usageErrf("boom %d", 1)
	if !errors.Is(err, cli.ErrUsage) {
		t.Errorf("not wrapped: %v", err)
	}
	if !strings.Contains(err.Error(), "boom 1") {
		t.Errorf("msg=%q", err.Error())
	}
}
