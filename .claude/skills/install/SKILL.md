---
name: install
description: Build and install the claude-accounts binary so the `claude-accounts` command is available on PATH. Use when the user asks to "install", "build and install", "reinstall after my changes", or similar.
---

Run `go install .` from the project root. This compiles and places the binary in `$(go env GOBIN)`, falling back to `$(go env GOPATH)/bin` (typically `~/go/bin`).

Steps:

1. `go install .` — builds and installs in one shot.
2. `command -v claude-accounts` — verify it resolved on PATH.
3. `claude-accounts help` — sanity-check the installed binary runs.

If step 2 fails, the install directory isn't on PATH. Tell the user the resolved path (`go env GOPATH`/bin or `go env GOBIN`) and suggest adding it to their shell rc — do not modify their shell config without asking.

Prefer `go install .` over `go build -o claude-accounts .`. The latter leaves an untracked binary in the working tree; only use it if the user specifically wants a local build artifact.
