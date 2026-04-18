#!/usr/bin/env python3
"""
Audits the directory tree for missing or stale CLAUDE.md files.
Outputs actionable diffs and instructions so the agent knows exactly what to do.

Leaf-only: if a parent dir has an issue AND a child dir also has an issue,
skip the parent — fix deepest problems first.

Usage: python audit.py [root_directory]
Exit code: 0 = clean, 1 = issues found
"""

import os
import shlex
import subprocess
import sys
from pathlib import Path


def run_git(root: Path, args: list[str], ok_codes: set[int] | None = None) -> subprocess.CompletedProcess:
    """Run a git command and fail loud on unexpected exit codes. Default
    `ok_codes` is {0}; callers that intentionally consume a non-zero status
    (e.g. `check-ignore` returning 1 for "not ignored") pass an explicit set."""
    ok = ok_codes or {0}
    result = subprocess.run(
        ["git", "-C", str(root), *args],
        capture_output=True,
        text=True,
    )
    if result.returncode not in ok:
        cmd = " ".join(shlex.quote(x) for x in ["git", "-C", str(root), *args])
        raise RuntimeError(
            f"git command failed (exit {result.returncode}): {cmd}\n"
            f"stderr: {result.stderr.strip()}\nstdout: {result.stdout.strip()}"
        )
    return result


def git_check_ignore(root: Path, path: Path) -> bool:
    """Return True if path is gitignored. Exit 0 = ignored, 1 = not ignored;
    anything else (e.g. missing .git) is a real failure that should raise."""
    result = run_git(root, ["check-ignore", "-q", str(path)], ok_codes={0, 1})
    return result.returncode == 0


def git_file_existed_before(root: Path, file: Path, mtime: float) -> bool:
    """Return True if file had a commit at or before mtime (i.e., it's not brand new)."""
    result = run_git(root, ["log", f"--until=@{int(mtime)}", "--format=%H", "--", str(file)])
    return bool(result.stdout.strip())


def git_diff_since(root: Path, file: Path, mtime: float) -> str:
    """Return diff between file at claude_mtime snapshot and current working tree."""
    ref = run_git(
        root,
        ["log", f"--until=@{int(mtime)}", "-n", "1", "--format=%H", "--", str(file)],
    ).stdout.strip()
    if not ref:
        return ""
    return run_git(root, ["diff", ref, "--", str(file)]).stdout.strip()


def git_diff_child_claude_md_since(root: Path, child_dir: Path, mtime: float) -> str:
    """Return diff of a child directory's CLAUDE.md since mtime, or '' if unchanged."""
    child_claude = child_dir / "CLAUDE.md"
    if not child_claude.exists():
        return ""
    return git_diff_since(root, child_claude, mtime)


def touch_file(claude_md: Path) -> None:
    """Update mtime of CLAUDE.md using the touch.sh script. Fails loud so the
    caller's AUTO-TOUCHED report stays trustworthy — a silent touch failure
    would leave the mtime stale and the next audit would rediscover the same
    "issue"."""
    touch_sh = Path(__file__).resolve().parent / "touch.sh"
    result = subprocess.run(
        ["bash", str(touch_sh), str(claude_md)],
        capture_output=True,
        text=True,
    )
    if result.returncode != 0:
        raise RuntimeError(
            result.stderr.strip()
            or result.stdout.strip()
            or f"touch.sh failed for {claude_md}"
        )


def get_mtime(path: Path) -> float:
    return path.stat().st_mtime


def entry_is_newer_than_claude(entry: Path, claude_mtime: float) -> bool:
    """Treat an entry as newer than the parent CLAUDE.md if either the entry
    itself or — for directories — its own CLAUDE.md is newer. Most Unix
    filesystems only bump directory mtime on add/remove/rename, not on content
    edits inside, so a pure `entry.stat().st_mtime` check misses the common
    case of someone editing a child's CLAUDE.md in place."""
    if get_mtime(entry) > claude_mtime:
        return True
    if entry.is_dir():
        child_claude = entry / "CLAUDE.md"
        if child_claude.exists() and get_mtime(child_claude) > claude_mtime:
            return True
    return False


def collect_dirs(root: Path) -> list[Path]:
    """Collect all non-hidden, non-pycache, non-gitignored directories sorted deepest first."""
    result = []
    for dirpath, dirnames, _ in os.walk(root):
        # Prune hidden dirs, __pycache__, and gitignored dirs in-place
        dirnames[:] = [
            d for d in dirnames
            if not d.startswith(".")
            and d != "__pycache__"
            and not git_check_ignore(root, Path(dirpath) / d)
        ]
        result.append(Path(dirpath))
    # Sort deepest first by counting path parts
    result.sort(key=lambda p: len(p.parts), reverse=True)
    return result


def has_issue(d: Path, root: Path) -> bool:
    """Return True if directory d has a missing or stale CLAUDE.md."""
    claude_md = d / "CLAUDE.md"
    if not claude_md.exists():
        return True
    claude_mtime = get_mtime(claude_md)
    for entry in d.iterdir():
        if entry.name == "CLAUDE.md":
            continue
        if entry.name.startswith("."):
            continue
        if entry.name == "__pycache__":
            continue
        if git_check_ignore(root, entry):
            continue
        if entry_is_newer_than_claude(entry, claude_mtime):
            return True
    return False


def immediate_children(d: Path, root: Path) -> list[Path]:
    """Return immediate non-hidden, non-gitignored children of d."""
    children = []
    for entry in sorted(d.iterdir()):
        if entry.name.startswith("."):
            continue
        if entry.name == "__pycache__":
            continue
        if git_check_ignore(root, entry):
            continue
        children.append(entry)
    return children


def main():
    root = Path(sys.argv[1] if len(sys.argv) > 1 else ".").resolve()
    all_dirs = collect_dirs(root)

    # Identify dirs with issues
    dirs_with_issues = {d for d in all_dirs if has_issue(d, root)}

    # Filter to leaf-only: skip a dir if any of its children also has an issue
    leaf_dirs = []
    skipped_non_leaf = []
    for d in all_dirs:
        if d not in dirs_with_issues:
            continue
        # Check if any immediate child directory also has an issue
        child_dirs_with_issues = [
            child for child in d.iterdir()
            if child.is_dir() and child in dirs_with_issues
        ]
        if child_dirs_with_issues:
            skipped_non_leaf.append(d)
        else:
            leaf_dirs.append(d)

    issues_found = False

    for d in leaf_dirs:
        claude_md = d / "CLAUDE.md"
        rel_claude = claude_md.relative_to(root)

        if not claude_md.exists():
            # MISSING case
            issues_found = True
            children = immediate_children(d, root)
            child_list = "\n".join(
                f"- {c.name}{'/' if c.is_dir() else ''}"
                for c in children
            )
            print(f"MISSING: {rel_claude} — create it with a three-sentence summary and a table of these children:")
            print(child_list)
            print()
            continue

        # STALE case
        claude_mtime = get_mtime(claude_md)
        newer_entries = [
            entry for entry in d.iterdir()
            if entry.name != "CLAUDE.md"
            and not entry.name.startswith(".")
            and entry.name != "__pycache__"
            and not git_check_ignore(root, entry)
            and entry_is_newer_than_claude(entry, claude_mtime)
        ]

        changed_files = []
        meaningful = False

        for entry in sorted(newer_entries):
            if entry.is_file():
                rel_entry = entry.relative_to(root)
                existed = git_file_existed_before(root, entry, claude_mtime)
                if not existed:
                    # New file
                    changed_files.append(("new", rel_entry))
                    meaningful = True
                else:
                    diff = git_diff_since(root, entry, claude_mtime)
                    if diff:
                        changed_files.append(("diff", rel_entry, diff))
                        meaningful = True
                    # else: file is newer on disk but has no git changes — ignore
            elif entry.is_dir():
                child_claude = entry / "CLAUDE.md"
                # A brand-new child CLAUDE.md won't produce a git diff (no
                # prior revision to diff against), but the parent's child
                # table is now stale by definition. Treat it as a "new file"
                # change so the parent gets flagged.
                if child_claude.exists() and not git_file_existed_before(root, child_claude, claude_mtime):
                    rel_child_claude = child_claude.relative_to(root)
                    changed_files.append(("new", rel_child_claude))
                    meaningful = True
                    continue
                child_diff = git_diff_child_claude_md_since(root, entry, claude_mtime)
                if child_diff:
                    rel_child_claude = child_claude.relative_to(root)
                    changed_files.append(("diff", rel_child_claude, child_diff))
                    meaningful = True
                # else: child dir's CLAUDE.md unchanged — skip, not meaningful

        if not meaningful:
            touch_file(claude_md)
            print(f"AUTO-TOUCHED: {rel_claude} (no meaningful child changes)")
            continue

        issues_found = True
        rel_dir = d.relative_to(root)
        print(f"@{rel_claude} may need updating because relevant files have changed. Review the diffs below and update CLAUDE.md if needed, then touch it:")
        print()
        print(f"  bash .claude/skills/claude-md-maintenance/scripts/touch.sh {rel_claude}")
        print()
        print("Changed files:")
        for item in changed_files:
            if item[0] == "new":
                print(f"- @{item[1]} (new file — read for full context)")
            elif item[0] == "diff":
                print(f"- ./{item[1]}:")
                print("```diff")
                print(item[2])
                print("```")
        print()

    if skipped_non_leaf:
        rerun_target = sys.argv[1] if len(sys.argv) > 1 else "."
        print("Some parent CLAUDE.md files were skipped because their children were stale first.")
        print("Re-run to check the next layer up:")
        print(f"  python3 .claude/skills/claude-md-maintenance/scripts/audit.py {rerun_target}")
        print()

    sys.exit(1 if issues_found else 0)


if __name__ == "__main__":
    main()
