package audit

import (
	"context"
	"encoding/json"
	"fmt"
	"text/tabwriter"
	"time"

	"github.com/highperformance-tech/ana-cli/internal/cli"
)

// tailCmd implements `ana audit tail` — ListAuditLogs. Flags:
//
//	--since <dur|RFC3339>  e.g. 1h, 24h, or 2026-04-18T00:00:00Z; emitted as
//	                       `{"since": <RFC3339>}` after normalising to UTC
//	--limit <int>          pass through as `"limit": N` when >0
//
// Catalog confirms the response envelope is `{ "entries": [ ... ] }` with
// snake_case action strings such as `api_key.created`. With --json the
// command emits each entry as a JSON Lines record.
type tailCmd struct{ deps Deps }

func (c *tailCmd) Help() string {
	return "tail   Tail audit logs (TIME/ACTOR/ACTION/TARGET table, --json for JSON Lines).\n" +
		"Usage: ana audit tail [--since <dur|RFC3339>] [--limit <n>]"
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
	fs := cli.NewFlagSet("audit tail")
	var since string
	var limit int
	fs.StringVar(&since, "since", "", "lower bound: relative duration ago (e.g. 1h, 24h) or absolute RFC3339 timestamp")
	fs.IntVar(&limit, "limit", 0, "maximum number of entries to request")
	if err := cli.ParseFlags(fs, args); err != nil {
		return err
	}

	// Build the wire payload. We use a map so we can add fields conditionally
	// without defining an additive set of struct tags — the request shape is
	// naturally sparse.
	body := map[string]any{}
	if since != "" {
		// Accept either a relative duration ("1h", "24h") or an absolute
		// RFC3339 timestamp ("2026-04-18T00:00:00Z"). Try duration first
		// because it is the more common shape. Note: Go's time.RFC3339
		// parser already tolerates the fractional-second form ("…00.123Z"),
		// so a separate RFC3339Nano fallback would be dead code.
		if dur, err := time.ParseDuration(since); err == nil {
			body["since"] = c.deps.Now().Add(-dur).UTC().Format(time.RFC3339)
		} else if ts, tsErr := time.Parse(time.RFC3339, since); tsErr == nil {
			body["since"] = ts.UTC().Format(time.RFC3339)
		} else {
			return cli.UsageErrf(
				"audit tail: invalid --since %q (expected duration like 1h or RFC3339 timestamp)",
				since,
			)
		}
	}
	if limit > 0 {
		body["limit"] = limit
	}

	var raw map[string]any
	if err := c.deps.Unary(ctx, auditServicePath+"/ListAuditLogs", body, &raw); err != nil {
		return fmt.Errorf("audit tail: %w", err)
	}
	var typed tailResp
	if err := cli.Remarshal(raw, &typed); err != nil {
		return fmt.Errorf("audit tail: decode response: %w", err)
	}
	if cli.GlobalFrom(ctx).JSON {
		// Emit one JSON object per audit record (JSON Lines), not the pretty
		// envelope: easier to pipe into jq / append to a log without re-
		// parsing the array wrapper. encoding/json's Encoder writes a
		// trailing newline after each value, which is exactly the JSONL
		// frame separator.
		enc := json.NewEncoder(stdio.Stdout)
		for _, e := range typed.Entries {
			if err := enc.Encode(e); err != nil {
				return fmt.Errorf("audit tail: encode json line: %w", err)
			}
		}
		return nil
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
