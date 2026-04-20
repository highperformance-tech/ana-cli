package audit

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/highperformance-tech/ana-cli/internal/cli"
)

// tailCmd implements `ana audit tail` — ListAuditLogs. Flags:
//
//	--since <dur|RFC3339>  e.g. 1h, 24h, or 2026-04-18T00:00:00Z; emitted as
//	                       `{"since": <RFC3339>}` after normalising to UTC
//	--limit <int>          pass through as `"limit": N` when >0; must be >=0
//	                       (negative values are rejected with a usage error,
//	                       zero means "unspecified" and is omitted from the
//	                       wire payload)
//
// Negative --since durations are rejected: `time.ParseDuration("-1h")` is
// technically valid, but interpreting it would produce a timestamp in the
// future rather than surfacing an obvious operator typo.
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

// tailReq is the ListAuditLogs wire payload. `omitempty` lets callers leave
// either field zero; the server then treats them as unspecified. Kept as a
// typed struct rather than `map[string]any` so the JSON keys are checked at
// build time.
type tailReq struct {
	Since string `json:"since,omitempty"`
	Limit int    `json:"limit,omitempty"`
}

func (c *tailCmd) Run(ctx context.Context, args []string, stdio cli.IO) error {
	fs := cli.NewFlagSet("audit tail")
	var since string
	var limit int
	fs.StringVar(&since, "since", "", "lower bound: relative duration ago (e.g. 1h, 24h) or absolute RFC3339 timestamp")
	fs.IntVar(&limit, "limit", 0, "maximum number of entries to request (must be >= 0; 0 means unspecified)")
	if err := cli.ParseFlags(fs, args); err != nil {
		return err
	}
	// Reject negative --limit explicitly. The wire payload's `omitempty` would
	// otherwise silently drop a negative value and behave as "unspecified",
	// masking the operator's input error. Surface it as a usage error so the
	// intent is obvious at the CLI surface.
	if limit < 0 {
		return cli.UsageErrf("audit tail: --limit must be >= 0 (got %d)", limit)
	}

	// Build the wire payload. `omitempty` on both fields keeps the request
	// shape naturally sparse: a zero Since string and zero Limit both drop
	// off the wire.
	var body tailReq
	if since != "" {
		// Accept either a relative duration ("1h", "24h") or an absolute
		// RFC3339 timestamp ("2026-04-18T00:00:00Z"). Try duration first
		// because it is the more common shape. Note: Go's time.RFC3339
		// parser already tolerates the fractional-second form ("…00.123Z"),
		// so a separate RFC3339Nano fallback would be dead code.
		if dur, err := time.ParseDuration(since); err == nil {
			// Reject negative durations. `time.ParseDuration("-1h")` succeeds,
			// and `Now().Add(-(-1h))` would quietly turn that into a timestamp
			// in the future — the exact shape of "from the future" bug that
			// hides operator typos. Surface it as a usage error instead.
			if dur < 0 {
				return cli.UsageErrf("audit tail: --since duration must be >= 0 (got %q)", since)
			}
			body.Since = c.deps.Now().Add(-dur).UTC().Format(time.RFC3339)
		} else if ts, tsErr := time.Parse(time.RFC3339, since); tsErr == nil {
			body.Since = ts.UTC().Format(time.RFC3339)
		} else {
			return cli.UsageErrf(
				"audit tail: invalid --since %q (expected duration like 1h or RFC3339 timestamp)",
				since,
			)
		}
	}
	if limit > 0 {
		body.Limit = limit
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
	tw := cli.NewTableWriter(stdio.Stdout)
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
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n",
			cli.DashIfEmpty(e.CreatedAt),
			cli.DashIfEmpty(e.ActorEmail),
			cli.DashIfEmpty(e.Action),
			cli.DashIfEmpty(target))
	}
	return tw.Flush()
}
