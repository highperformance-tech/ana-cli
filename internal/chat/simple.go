package chat

import (
	"context"
	"fmt"
	"io"

	"github.com/highperformance-tech/ana-cli/internal/cli"
)

// This file gathers the small single-RPC verbs — rename / bookmark /
// unbookmark / duplicate / delete. Each follows the same shape: read one
// positional id, build a typed request, Unary, print a one-liner. None
// declare flags, so none implement Flagger.

// renameReq matches UpdateChat's wire shape. The field is `summary` — the
// catalog shows no `title` key; the brief's `title` is a user-facing alias.
type renameReq struct {
	ChatID  string `json:"chatId"`
	Summary string `json:"summary"`
}

// renameCmd implements `ana chat rename <id> <title>`.
type renameCmd struct{ deps Deps }

func (c *renameCmd) Help() string {
	return "rename   Rename a chat's summary/title.\n" +
		"Usage: ana chat rename <id> <title>"
}

func (c *renameCmd) Run(ctx context.Context, args []string, stdio cli.IO) error {
	id, err := cli.RequireStringID("chat rename", args)
	if err != nil {
		return err
	}
	if len(args) < 2 || args[1] == "" {
		return cli.UsageErrf("chat rename: <title> positional argument required")
	}
	if err := cli.RequireMaxPositionals("chat rename", 2, args); err != nil {
		return err
	}
	title := args[1]
	var raw map[string]any
	if err := c.deps.Unary(ctx, chatServicePath+"/UpdateChat",
		renameReq{ChatID: id, Summary: title}, &raw); err != nil {
		return fmt.Errorf("chat rename: %w", err)
	}
	var typed struct{}
	if err := cli.RenderOutput(stdio.Stdout, raw, cli.GlobalFrom(ctx).JSON, &typed, func(w io.Writer, _ *struct{}) error {
		_, err := fmt.Fprintln(w, "ok")
		return err
	}); err != nil {
		return fmt.Errorf("chat rename: %w", err)
	}
	return nil
}

// chatIDReq is the shared {chatId} body used by bookmark/unbookmark/
// duplicate/delete. Defined once because duplicating a single-field struct
// five times makes refactors noisy.
type chatIDReq struct {
	ChatID string `json:"chatId"`
}

// bookmarkCmd implements `ana chat bookmark <id>`.
type bookmarkCmd struct{ deps Deps }

func (c *bookmarkCmd) Help() string {
	return "bookmark   Bookmark a chat.\n" +
		"Usage: ana chat bookmark <id>"
}

func (c *bookmarkCmd) Run(ctx context.Context, args []string, stdio cli.IO) error {
	return simpleAck(ctx, args, stdio, c.deps, "chat bookmark", "/BookmarkChat")
}

// unbookmarkCmd implements `ana chat unbookmark <id>`.
type unbookmarkCmd struct{ deps Deps }

func (c *unbookmarkCmd) Help() string {
	return "unbookmark   Remove a chat's bookmark.\n" +
		"Usage: ana chat unbookmark <id>"
}

func (c *unbookmarkCmd) Run(ctx context.Context, args []string, stdio cli.IO) error {
	return simpleAck(ctx, args, stdio, c.deps, "chat unbookmark", "/UnbookmarkChat")
}

// deleteCmd implements `ana chat delete <id>` and prints a distinct
// "deleted <id>" message to match the connector-delete convention.
type deleteCmd struct{ deps Deps }

func (c *deleteCmd) Help() string {
	return "delete   Delete a chat.\n" +
		"Usage: ana chat delete <id>"
}

func (c *deleteCmd) Run(ctx context.Context, args []string, stdio cli.IO) error {
	if err := cli.RequireMaxPositionals("chat delete", 1, args); err != nil {
		return err
	}
	id, err := cli.RequireStringID("chat delete", args)
	if err != nil {
		return err
	}
	var raw map[string]any
	if err := c.deps.Unary(ctx, chatServicePath+"/DeleteChat", chatIDReq{ChatID: id}, &raw); err != nil {
		return fmt.Errorf("chat delete: %w", err)
	}
	var typed struct{}
	if err := cli.RenderOutput(stdio.Stdout, raw, cli.GlobalFrom(ctx).JSON, &typed, func(w io.Writer, _ *struct{}) error {
		_, err := fmt.Fprintf(w, "deleted %s\n", id)
		return err
	}); err != nil {
		return fmt.Errorf("chat delete: %w", err)
	}
	return nil
}

// duplicateCmd implements `ana chat duplicate <id>` and prints the new id.
type duplicateCmd struct{ deps Deps }

func (c *duplicateCmd) Help() string {
	return "duplicate   Duplicate a chat and print the new id.\n" +
		"Usage: ana chat duplicate <id>"
}

type duplicateResp struct {
	Chat struct {
		ID string `json:"id"`
	} `json:"chat"`
}

func (c *duplicateCmd) Run(ctx context.Context, args []string, stdio cli.IO) error {
	if err := cli.RequireMaxPositionals("chat duplicate", 1, args); err != nil {
		return err
	}
	id, err := cli.RequireStringID("chat duplicate", args)
	if err != nil {
		return err
	}
	var raw map[string]any
	if err := c.deps.Unary(ctx, chatServicePath+"/DuplicateChat", chatIDReq{ChatID: id}, &raw); err != nil {
		return fmt.Errorf("chat duplicate: %w", err)
	}
	var typed duplicateResp
	if err := cli.RenderOutput(stdio.Stdout, raw, cli.GlobalFrom(ctx).JSON, &typed, func(w io.Writer, t *duplicateResp) error {
		_, err := fmt.Fprintln(w, t.Chat.ID)
		return err
	}); err != nil {
		return fmt.Errorf("chat duplicate: %w", err)
	}
	return nil
}

// simpleAck is the bookmark/unbookmark path — positional id, no-body
// response, prints `ok`. Extracted so the two verbs aren't literal copies.
func simpleAck(ctx context.Context, args []string, stdio cli.IO, deps Deps, verb, suffix string) error {
	if err := cli.RequireMaxPositionals(verb, 1, args); err != nil {
		return err
	}
	id, err := cli.RequireStringID(verb, args)
	if err != nil {
		return err
	}
	var raw map[string]any
	if err := deps.Unary(ctx, chatServicePath+suffix, chatIDReq{ChatID: id}, &raw); err != nil {
		return fmt.Errorf("%s: %w", verb, err)
	}
	var typed struct{}
	if err := cli.RenderOutput(stdio.Stdout, raw, cli.GlobalFrom(ctx).JSON, &typed, func(w io.Writer, _ *struct{}) error {
		_, err := fmt.Fprintln(w, "ok")
		return err
	}); err != nil {
		return fmt.Errorf("%s: %w", verb, err)
	}
	return nil
}
