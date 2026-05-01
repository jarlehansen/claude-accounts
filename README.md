# claude-accounts

`claude-accounts` lets you maintain multiple Claude Code logins on the same Mac and switch between them instantly. If you have separate Anthropic accounts for work and personal use — or manage accounts across several organisations — this tool handles the credential swap so you never have to log out and back in manually.

Switching takes under a second and works from either an interactive menu or a single CLI command.

## Getting started

### Prerequisites

- macOS (uses the macOS Keychain)
- Go 1.21 or later
- The `claude` CLI installed and on your `PATH`

### Install

Clone the repo and install the binary:

```sh
git clone https://github.com/your-username/claude-accounts
cd claude-accounts
go install .
```

Make sure `$(go env GOPATH)/bin` (usually `~/go/bin`) is on your `PATH`.

### Create your first profile

```sh
claude-accounts create work
```

This clears the current Claude login, launches `claude` so you can authenticate with the account you want to use for the `work` profile, then saves the credentials. Type `/exit` in Claude when done.

Repeat for any additional accounts:

```sh
claude-accounts create personal
```

### Switch between profiles

Run the interactive menu:

```sh
claude-accounts
```

Or switch directly from the command line:

```sh
claude-accounts work
claude-accounts personal
```

### Other commands

```sh
claude-accounts list             # list all profiles, with active one marked
claude-accounts current          # print the active profile name
claude-accounts delete <name>    # delete a profile and its stored credentials
```

### Re-authenticating a profile

If a token expires or is revoked, switch to the profile and re-run `create` with the same name:

```sh
claude-accounts create work
```

You will be asked to confirm the overwrite, then taken through the login flow again.

## How it works

Claude Code's identity is exactly two pieces of state:

1. A macOS Keychain entry (service `Claude Code-credentials`) — the OAuth token.
2. `~/.claude.json` — top-level config containing `oauthAccount`, `userID`, etc.

`claude-accounts` snapshots both per profile and swaps them on switch. Everything else under `~/.claude/` (projects, history, settings, plugins) stays shared across profiles.

Per-profile tokens are stored in the Keychain under `claude-accounts:<name>` — never on disk.

## Install

```sh
go install .
```

Requires Go 1.21+, macOS, and the `claude` CLI on your `PATH`.

## Usage

```sh
claude-accounts                  # interactive menu
claude-accounts <name>           # switch to profile
claude-accounts create <name>    # create a new profile (launches claude for login)
claude-accounts list             # list profiles
claude-accounts current          # print active profile
claude-accounts delete <name>    # delete a profile
```

When creating a profile, `claude-accounts` clears the current credentials, launches `claude` interactively so you can log in, then captures and stores the new token. If anything goes wrong, the previous credentials are restored.

## Storage

```
~/.config/claude-accounts/
  current                        # active profile name
  profiles/<name>/claude.json    # snapshot of ~/.claude.json
```

OAuth tokens live in the Keychain under `claude-accounts:<name>`, never on disk.

## Comparison with similar tools

Two other tools solve the same problem with a different approach:

| | [claude-switch](https://github.com/SaschaHeyer/claude-switch) | [claude-account-switcher](https://github.com/ukogan/claude-account-switcher) | claude-accounts |
|---|---|---|---|
| Strategy | Separate `CLAUDE_CONFIG_DIR` per profile | Separate `CLAUDE_CONFIG_DIR` per profile | Swap `~/.claude.json` + Keychain in place |
| Implementation | Bash script | Bash shell function | Go binary |
| Keychain management | None — tokens left as Claude's concern | None — tokens left as Claude's concern | Explicit — one Keychain entry per profile |
| Parallel sessions | Yes — different terminals can run different profiles | Yes | No — one active profile at a time |
| Shared config | Symlinked `settings.json`, `CLAUDE.md`, `plugins` | Symlinked `settings.json`, `mcp.json`, `hooks`, `commands`, etc. | Fully shared (`~/.claude/` untouched) |
| Rollback on failure | No | No | Yes — pre-login state restored on any error |
| Atomic writes | No | No | Yes |
| Orphaned Keychain entries on delete | Yes | Yes | No — cleaned up on `delete` |

### The core trade-off

**`CLAUDE_CONFIG_DIR` isolation** (the other two tools) creates a separate directory tree per profile and launches Claude pointing at it. This allows parallel sessions — two terminal windows can run different profiles simultaneously. The downside is that shared config (settings, MCP, hooks) is managed via symlinks, so a change in one profile affects all profiles, and OAuth tokens accumulate in the Keychain without being cleaned up on profile deletion.

**In-place swap** (this tool) keeps `~/.claude/` in its normal location and swaps only the identity state. `~/.claude/` stays shared without any symlink machinery. The trade-off is that switching is global — there is no concept of "profile A in this terminal, profile B in that terminal."

If you need parallel sessions, use one of the `CLAUDE_CONFIG_DIR`-based tools. If you want clean Keychain management and a minimal footprint, use this one.
