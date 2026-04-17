package audit

import (
	"context"
	"fmt"
	"text/tabwriter"
	"time"

	"github.com/highperformance-tech/ana-cli/internal/cli"
)

// tailCmd implements `ana audit tail` — ListAuditLogs. Flags:
//
//	--since <duration>  (e.g. 1h, 24h) — include `{"since": <RFC3339>}`
//	--limit <int>       (pass through as `"limit": N` when >0)
//
// Catalog confirms the response envelope is `{ "entries": [ ... ] }` with
// snake_case action strings such as `api_key.created`.
type tailCmd struct{ deps Deps }

func (c *tailCmd) Help() string {
	return "tail   Tail audit logs (TIME/ACTOR/ACTION/TARGET table, --json for raw).\n" +
		"Usage: ana audit tail [--since <dur>] [--limit <n>]"
}

// tailResp narrows the fields we render. The catalog shows many more
// (orgId, resourceId, details, category, ...) which the decoder drops.
type tailResp struct {
	Entries []struct {
		ActorEmail   string `json:"actorEmail"`
		Action       string `json:"action"`
		ResourceType string `json:"resourceType"`
		ResourceID   string `json:"resourceId"`
		CreatedAt    string `json:"createdAt"`
	} `json:"entries"`
}

func (c *tailCmd) Run(ctx context.Context, args []string, stdio cli.IO) error {
	fs := newFlagSet("audit tail")
	var since string
	var limit int
	fs.StringVar(&since, "since", "", "only include entries newer than this duration ago (e.g. 1h, 24h)")
	fs.IntVar(&limit, "limit", 0, "maximum number of entries to request")
	if err := parseFlags(fs, args); err != nil {
		return err
	}

	// Build the wire payload. We use a map so we can add fields conditionally
	// without defining an additive set of struct tags — the request shape is
	// naturally sparse.
	body := map[string]any{}
	if since != "" {
		dur, err := time.ParseDuration(since)
		if err != nil {
			return usageErrf("audit tail: invalid --since %q: %v", since, err)
		}
		body["since"] = c.deps.Now().Add(-dur).UTC().Format(time.RFC3339)
	}
	if limit > 0 {
		body["limit"] = limit
	}

	var raw map[string]any
	if err := c.deps.Unary(ctx, auditServicePath+"/ListAuditLogs", body, &raw); err != nil {
		return fmt.Errorf("audit tail: %w", err)
	}
	if cli.GlobalFrom(ctx).JSON {
		return writeJSON(stdio.Stdout, raw)
	}
	var typed tailResp
	if err := remarshal(raw, &typed); err != nil {
		return fmt.Errorf("audit tail: decode response: %w", err)
	}
	tw := tabwriter.NewWriter(stdio.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "TIME\tACTOR\tACTION\tTARGET")
	for _, e := range typed.Entries {
		target := e.ResourceType
		if e.ResourceID != "" {
			if target != "" {
				target = target + ":" + e.ResourceID
			} else {
				target = e.ResourceID
			}
		}
		if target == "" {
			target = "-"
		}
		t := e.CreatedAt
		if t == "" {
			t = "-"
		}
		actor := e.ActorEmail
		if actor == "" {
			actor = "-"
		}
		action := e.Action
		if action == "" {
			action = "-"
		}
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n", t, actor, action, target)
	}
	return tw.Flush()
}
