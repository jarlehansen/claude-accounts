# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Build & run

```
go build -o claude-accounts .   # local binary
go install .                    # install to GOBIN
go mod tidy                     # after dep changes
```

No test suite yet.

macOS-only: depends on `/usr/bin/security` and an installed `claude` CLI on `PATH`.

## What the tool does (and intentionally does not do)

The tool swaps Claude Code's *identity* — and only that — between named profiles. "Identity" is exactly two pieces of state:

1. macOS Keychain entry, service `Claude Code-credentials`, account = current macOS username (the OAuth token).
2. `~/.claude.json` (top-level config; contains `oauthAccount`, `userID`, etc., mixed with per-project state).

Everything else under `~/.claude/` (projects, todos, history, plugins) is deliberately **not** swapped — it stays shared across profiles. If you find yourself wanting to touch other paths, push back: the contract is "switch the login, nothing else."

## Storage layout

```
~/.config/claude-accounts/
  current                       # active profile name
  profiles/<name>/claude.json   # snapshot of ~/.claude.json
```

Per-profile OAuth tokens live in the Keychain under service `claude-accounts:<name>` — never on disk. Switching = read `claude-accounts:<name>` → write `Claude Code-credentials`, plus copy the stored `claude.json` to `~/.claude.json`.

## Architecture notes that span files

- **Arg dispatch** (`main.go`): a first arg that isn't a known subcommand is treated as a profile name to switch to. Adding a new subcommand means that name is no longer usable as a profile — update the `switch` and be aware of the shadow.
- **Create flow rollback** (`commands.go` `cmdCreate` + `restoreSnapshot`): before launching `claude` interactively for login, the existing Keychain entry + `~/.claude.json` are captured into local vars. Any failure between "wipe" and "save new profile" restores them. Preserve this invariant when editing `cmdCreate`: every error path after the wipe must call `restoreSnapshot`.
- **Keychain not-found is a value, not an error** (`keychain.go`): `keychainGet` maps `security` exit code 44 to `errKeychainNotFound`. Callers distinguish "no entry" from "command failed" via `errors.Is`. Don't collapse this back into a generic error.
- **Atomic writes** (`profile.go` `atomicWrite`): used for `~/.claude.json` (~100KB) and the `current` file. A torn `~/.claude.json` can break the Claude CLI; always go through `atomicWrite`, never write in place.
- **Argv exposure** (`keychain.go` `keychainSet`): `security add-generic-password -w <pw>` puts the token in argv briefly — visible via `ps`. This is an accepted trade-off for a single-user dev tool; `security` has no stdin password mode. Don't "fix" it by writing the token to a temp file.

## UI

`huh` drives the interactive menu (`ui.go`). `huh.ErrUserAborted` (Ctrl+C / Esc) is treated as "go back" inside menus and "exit cleanly" at the top level — match that pattern when adding new prompts.
