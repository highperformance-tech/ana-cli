---
name: claude-md-maintenance
description: Audits and maintains CLAUDE.md files across the project tree. Every directory should have a CLAUDE.md with a three-sentence summary and a table of immediate children. When a CLAUDE.md is older than something in its directory, it may be stale — read it, compare it to reality, and fix what's wrong.
user-invocable: true
context: fork
agent: Explore
allowed-tools:
  - Bash(python3 .claude/skills/claude-md-maintenance/scripts/audit.py *)
  - Bash(bash .claude/skills/claude-md-maintenance/scripts/touch.sh *)
---

# CLAUDE.md Maintenance

## Convention

Every directory in the project tree should have a `CLAUDE.md` that provides progressive disclosure:
1. A three-sentence summary of the directory's purpose
2. A table of immediate children (files and subdirectories) with brief descriptions
3. Pointers to where to look for specific concerns

The **mtime** of each `CLAUDE.md` is used by the pre-commit hook to detect potentially stale files. If any sibling in the directory is newer than the `CLAUDE.md`, the commit is blocked until the `CLAUDE.md` is reviewed and updated.

## Workflow

### 1. Run the audit script

```bash
python3 .claude/skills/claude-md-maintenance/scripts/audit.py .
```

The script outputs actionable instructions directly — diffs of what changed, which files are new, and the exact touch command to run after updating each file.

### 2. Follow the instructions

For each item the script reports:

- **`MISSING:`** — Create the CLAUDE.md with a three-sentence summary and a child table using the listed children.
- **`@path/to/CLAUDE.md may need updating`** — Review the diffs shown, update the CLAUDE.md if the changes affect what it documents, then run the touch command shown.
- **`AUTO-TOUCHED:`** — No action needed; the script handled it.

### 3. Re-run until clean

The script processes leaf directories first. Re-run it after each pass — parent directories may become actionable once their children are resolved.

```bash
python3 .claude/skills/claude-md-maintenance/scripts/audit.py .
```

Repeat until the script exits with code 0.
