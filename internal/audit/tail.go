package audit

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"time"

	"github.com/highperformance-tech/ana-cli/internal/cli"
)

// tailCmd implements `ana audit tail` — ListAuditLogs. Flags:
//
//	--since <dur|RFC3339>  e.g. 1h, 24h, or 2026-04-18T00:00:00Z; emitted as
//	                       `{"since": <RFC3339>}` after normalising to UTC.
//	--limit <int>          pass through as `"limit": N` when >0; must be >=0.
type tailCmd struct {
	deps Deps

	since time.Time
	limit int
}

func (c *tailCmd) Help() string {
	return "tail   Tail audit logs (TIME/ACTOR/ACTION/TARGET table, --json for JSON Lines).\n" +
		"Usage: ana audit tail [--since <dur|RFC3339>] [--limit <n>]"
}

func (c *tailCmd) Flags(fs *flag.FlagSet) {
	fs.Var(cli.SinceFlag(&c.since, c.deps.Now), "since", "lower bound: relative duration ago (e.g. 1h, 24h) or absolute RFC3339 timestamp")
	fs.IntVar(&c.limit, "limit", 0, "maximum number of entries to request (must be >= 0; 0 means unspecified)")
}

type tailResp struct {
	Entries []struct {
		ActorEmail   string `json:"actorEmail"`
		Action       string `json:"action"`
		ResourceType string `json:"resourceType"`
		ResourceID   string `json:"resourceId"`
		CreatedAt    string `json:"createdAt"`
	} `json:"entries"`
}

type tailReq struct {
	Since string `json:"since,omitempty"`
	Limit int    `json:"limit,omitempty"`
}

func (c *tailCmd) Run(ctx context.Context, args []string, stdio cli.IO) error {
	if err := cli.RequireNoPositionals("audit tail", args); err != nil {
		return err
	}
	if c.limit < 0 {
		return cli.UsageErrf("audit tail: --limit must be >= 0 (got %d)", c.limit)
	}

	var body tailReq
	if !c.since.IsZero() {
		body.Since = c.since.UTC().Format(time.RFC3339)
	}
	if c.limit > 0 {
		body.Limit = c.limit
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
