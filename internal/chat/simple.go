package chat

import (
	"context"
	"fmt"

	"github.com/textql/ana-cli/internal/cli"
)

// This file gathers the small single-RPC verbs — rename / bookmark /
// unbookmark / duplicate / delete. Each follows the same shape: parse flags,
// read one positional id, build a typed request, Unary, print a one-liner.
// They live together so the package has one file per conceptual area instead
// of a dozen near-identical files.

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
	fs := newFlagSet("chat rename")
	if err := parseFlags(fs, args); err != nil {
		return err
	}
	rest := fs.Args()
	id, err := requirePositionalID("chat rename", rest)
	if err != nil {
		return err
	}
	if len(rest) < 2 || rest[1] == "" {
		return usageErrf("chat rename: <title> positional argument required")
	}
	title := rest[1]
	global := cli.GlobalFrom(ctx)
	var raw map[string]any
	if err := c.deps.Unary(ctx, chatServicePath+"/UpdateChat",
		renameReq{ChatID: id, Summary: title}, &raw); err != nil {
		return fmt.Errorf("chat rename: %w", err)
	}
	if global.JSON {
		return writeJSON(stdio.Stdout, raw)
	}
	fmt.Fprintln(stdio.Stdout, "ok")
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
	fs := newFlagSet("chat delete")
	if err := parseFlags(fs, args); err != nil {
		return err
	}
	id, err := requirePositionalID("chat delete", fs.Args())
	if err != nil {
		return err
	}
	global := cli.GlobalFrom(ctx)
	var raw map[string]any
	if err := c.deps.Unary(ctx, chatServicePath+"/DeleteChat", chatIDReq{ChatID: id}, &raw); err != nil {
		return fmt.Errorf("chat delete: %w", err)
	}
	if global.JSON {
		return writeJSON(stdio.Stdout, raw)
	}
	fmt.Fprintf(stdio.Stdout, "deleted %s\n", id)
	return nil
}

// duplicateCmd implements `ana chat duplicate <id>` and prints the new id.
type duplicateCmd struct{ deps Deps }

func (c *duplicateCmd) Help() string {
	return "duplicate   Duplicate a chat and print the new id.\n" +
		"Usage: ana chat duplicate <id>"
}

// duplicateResp carries only what we print. Full chat envelope available via
// --json for anyone who wants to script against it.
type duplicateResp struct {
	Chat struct {
		ID string `json:"id"`
	} `json:"chat"`
}

func (c *duplicateCmd) Run(ctx context.Context, args []string, stdio cli.IO) error {
	fs := newFlagSet("chat duplicate")
	if err := parseFlags(fs, args); err != nil {
		return err
	}
	id, err := requirePositionalID("chat duplicate", fs.Args())
	if err != nil {
		return err
	}
	global := cli.GlobalFrom(ctx)
	var raw map[string]any
	if err := c.deps.Unary(ctx, chatServicePath+"/DuplicateChat", chatIDReq{ChatID: id}, &raw); err != nil {
		return fmt.Errorf("chat duplicate: %w", err)
	}
	if global.JSON {
		return writeJSON(stdio.Stdout, raw)
	}
	var typed duplicateResp
	if err := remarshal(raw, &typed); err != nil {
		return fmt.Errorf("chat duplicate: decode response: %w", err)
	}
	fmt.Fprintln(stdio.Stdout, typed.Chat.ID)
	return nil
}

// simpleAck is the bookmark/unbookmark path — positional id, no-body
// response, prints `ok`. Extracted so the two verbs aren't literal copies.
func simpleAck(ctx context.Context, args []string, stdio cli.IO, deps Deps, verb, suffix string) error {
	fs := newFlagSet(verb)
	if err := parseFlags(fs, args); err != nil {
		return err
	}
	id, err := requirePositionalID(verb, fs.Args())
	if err != nil {
		return err
	}
	global := cli.GlobalFrom(ctx)
	var raw map[string]any
	if err := deps.Unary(ctx, chatServicePath+suffix, chatIDReq{ChatID: id}, &raw); err != nil {
		return fmt.Errorf("%s: %w", verb, err)
	}
	if global.JSON {
		return writeJSON(stdio.Stdout, raw)
	}
	fmt.Fprintln(stdio.Stdout, "ok")
	return nil
}
