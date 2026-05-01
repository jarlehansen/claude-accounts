# claude-accounts

`claude-accounts` lets you keep multiple Claude Code logins on one Mac and switch between them. Useful if you have separate Anthropic accounts for work and personal use, or across organisations.

Switching takes under a second. Use the interactive menu or a single CLI command.

## Getting started

### Create your first profile

```sh
claude-accounts create work
```

This clears the current Claude login, launches `claude` so you can sign in with the account for the `work` profile, then saves the credentials. Type `/exit` in Claude when done.

Repeat for other accounts:

```sh
claude-accounts create personal
```

### Switch between profiles

Run the interactive menu:

```sh
claude-accounts
```

Or switch from the command line:

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

If a token expires or is revoked, switch to the profile and run `create` with the same name:

```sh
claude-accounts create work
```

Confirm the overwrite, then sign in again.

## How it works

Claude Code's identity is two pieces of state:

1. A macOS Keychain entry (service `Claude Code-credentials`) holding the OAuth token.
2. `~/.claude.json` with `oauthAccount`, `userID`, and other top-level config.

`claude-accounts` snapshots both per profile and swaps them on switch. Everything else under `~/.claude/` (projects, history, settings, plugins) stays shared.

Per-profile tokens are stored in the Keychain under `claude-accounts:<name>`, never on disk.

## Usage

```sh
claude-accounts                  # interactive menu
claude-accounts <name>           # switch to profile
claude-accounts create <name>    # create a new profile (launches claude for login)
claude-accounts list             # list profiles
claude-accounts current          # print active profile
claude-accounts delete <name>    # delete a profile
```

When creating a profile, `claude-accounts` clears the current credentials, launches `claude` so you can log in, then captures and stores the new token. If anything fails, the previous credentials are restored.

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
| Keychain management | None | None | One Keychain entry per profile |
| Parallel sessions | Yes | Yes | No, one active profile at a time |
| Shared config | Symlinked `settings.json`, `CLAUDE.md`, `plugins` | Symlinked `settings.json`, `mcp.json`, `hooks`, `commands` | Fully shared (`~/.claude/` untouched) |
| Rollback on failure | No | No | Yes, pre-login state restored on any error |
| Atomic writes | No | No | Yes |
| Orphaned Keychain entries on delete | Yes | Yes | No, cleaned up on `delete` |

### The core trade-off

**`CLAUDE_CONFIG_DIR` isolation** (the other two tools) gives each profile its own directory tree and launches Claude pointing at it. This allows parallel sessions: two terminals can run different profiles at the same time. The downside is that shared config (settings, MCP, hooks) needs symlinks, so a change in one profile affects all of them, and OAuth tokens accumulate in the Keychain when profiles are deleted.

**In-place swap** (this tool) keeps `~/.claude/` where it is and swaps only the identity state. No symlinks, nothing extra to manage. The trade-off: switching is global, so you can't run profile A in one terminal and profile B in another.

If you need parallel sessions, use one of the `CLAUDE_CONFIG_DIR`-based tools. If you want clean Keychain management and a minimal footprint, use this one.
