# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Build & run

```
go build -o claude-accounts .   # local binary
go install .                    # install to GOBIN
go test ./...                   # run unit tests
go mod tidy                     # after dep changes
```

Tests use in-memory fakes for the keychain and `runClaudeInteractive`, plus a tempdir for `HOME`/`XDG_CONFIG_HOME`, so they don't touch the real macOS Keychain or spawn the real `claude` CLI. Helpers live in `testhelpers_test.go`. The TUI (`ui.go`) is not covered.

macOS-only at runtime: depends on `/usr/bin/security` and an installed `claude` CLI on `PATH`.

## What the tool does (and intentionally does not do)

The tool swaps Claude Code's *identity* — and only that — between named profiles. "Identity" is exactly two pieces of state:

1. macOS Keychain entry, service `Claude Code-credentials`, account = current macOS username (the OAuth token).
2. `~/.claude.json` (top-level config; contains `oauthAccount`, `userID`, etc., mixed with per-project state).

Everything else under `~/.claude/` (projects, todos, history, plugins) is deliberately **not** swapped — it stays shared across profiles. If you find yourself wanting to touch other paths, push back: the contract is "switch the login, nothing else."

One narrow exception inside `~/.claude.json`: the top-level `mcpServers` map (user-scope MCP servers) is propagated across profiles instead of being swapped. Without this, adding an MCP server under one profile would be invisible to the others, since Claude Code stores user-scope servers in the same file we replace. Local-scope servers (under `projects.<path>.mcpServers`) remain per-profile. See `mergeUserMcpServers` in `profile.go`.

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
- **Write-back before overwrite** (`commands.go` `syncActiveProfile`): Claude Code refreshes its OAuth token *in place*, rewriting `Claude Code-credentials` every few hours, so a profile snapshot goes stale within a day. Both `cmdSwitch` and `performLogin` must fold the live token + `~/.claude.json` back into the active profile *before* they overwrite or wipe it — otherwise switching away throws the refreshed token out and switching back restores a dead one, forcing a re-login. In `cmdSwitch` the write-back has to run before `store.Load`, or switching to the already-active profile reloads the stale snapshot it just replaced. The write-back is skipped unless the live account identity matches the profile's, so a manual `claude /logout` + login as someone else can't clobber a profile.
- **Identity, not token bytes** (`profile.go` `accountIdentity` / `ReconcileCurrent`): "is the live state still this profile?" is answered by comparing `oauthAccount.accountUuid` from `~/.claude.json`, never by comparing tokens. Tokens legitimately change on every refresh; byte comparison read that as a logout and silently cleared `current`.
- **Keychain not-found is a value, not an error** (`keychain.go`): `keychainGet` maps `security` exit code 44 to `errKeychainNotFound`. Callers distinguish "no entry" from "command failed" via `errors.Is`. Don't collapse this back into a generic error.
- **Atomic writes** (`profile.go` `atomicWrite`): used for `~/.claude.json` (~100KB) and the `current` file. A torn `~/.claude.json` can break the Claude CLI; always go through `atomicWrite`, never write in place.
- **Argv exposure** (`keychain.go` `keychainSet`): `security add-generic-password -w <pw>` puts the token in argv briefly — visible via `ps`. This is an accepted trade-off for a single-user dev tool; `security` has no stdin password mode. Don't "fix" it by writing the token to a temp file.

## UI

`huh` drives the interactive menu (`ui.go`). `huh.ErrUserAborted` (Ctrl+C / Esc) is treated as "go back" inside menus and "exit cleanly" at the top level — match that pattern when adding new prompts.
