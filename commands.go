package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
)

func cmdList() error {
	store, err := NewStore()
	if err != nil {
		return err
	}
	current, _ := store.Current()
	names, err := store.List()
	if err != nil {
		return err
	}
	if len(names) == 0 {
		fmt.Println("(no profiles yet — run `claude-accounts create <name>`)")
		return nil
	}
	for _, n := range names {
		marker := "  "
		if n == current {
			marker = "* "
		}
		fmt.Println(marker + n)
	}
	return nil
}

func cmdCurrent() error {
	store, err := NewStore()
	if err != nil {
		return err
	}
	current, err := store.Current()
	if err != nil {
		return err
	}
	if current == "" {
		fmt.Println("(no active profile)")
		return nil
	}
	fmt.Println(current)
	return nil
}

func cmdSwitch(name string) error {
	store, err := NewStore()
	if err != nil {
		return err
	}
	if !store.Exists(name) {
		return fmt.Errorf("profile %q does not exist; run `claude-accounts create %s` first", name, name)
	}
	claudeJSON, token, err := store.Load(name)
	if err != nil {
		return err
	}

	prevToken, prevClaudeJSON, err := captureLiveClaudeState()
	if err != nil {
		return err
	}

	if merged, changed, err := mergeUserMcpServers(claudeJSON, prevClaudeJSON); err != nil {
		return fmt.Errorf("merge user-scope MCP servers: %w", err)
	} else if changed {
		claudeJSON = merged
	}

	if err := keychainSet(keychainServiceClaude, token); err != nil {
		return fmt.Errorf("write Claude credentials: %w", err)
	}
	if err := writeClaudeJSON(claudeJSON); err != nil {
		restoreSnapshot(prevToken, prevClaudeJSON)
		return fmt.Errorf("write ~/.claude.json: %w", err)
	}
	if err := store.SetCurrent(name); err != nil {
		restoreSnapshot(prevToken, prevClaudeJSON)
		return err
	}
	fmt.Printf("switched to %s\n", name)
	return nil
}

func cmdDelete(name string) error {
	store, err := NewStore()
	if err != nil {
		return err
	}
	if !store.Exists(name) {
		return fmt.Errorf("profile %q does not exist", name)
	}
	current, _ := store.Current()
	if name == current {
		return fmt.Errorf("cannot delete %q: it is the active profile (switch to another first)", name)
	}
	if err := store.Delete(name); err != nil {
		return err
	}
	fmt.Printf("deleted %s\n", name)
	return nil
}

func cmdCreate(name string) error {
	store, err := NewStore()
	if err != nil {
		return err
	}

	if store.Exists(name) {
		ok, err := confirmOverwrite(name)
		if err != nil {
			return err
		}
		if !ok {
			fmt.Println("aborted")
			return nil
		}
	}

	if err := performLogin(store, name, "create"); err != nil {
		return err
	}
	fmt.Printf("\ncreated profile %q and set it as the active profile\n", name)
	return nil
}

func cmdReauth(name string) error {
	store, err := NewStore()
	if err != nil {
		return err
	}
	if !store.Exists(name) {
		return fmt.Errorf("profile %q does not exist; run `claude-accounts create %s` first", name, name)
	}
	if err := performLogin(store, name, "reauth"); err != nil {
		return err
	}
	fmt.Printf("\nreauthenticated profile %q and set it as the active profile\n", name)
	return nil
}

// performLogin captures the live state, wipes it so Claude prompts for a fresh
// login, runs `claude` interactively, then saves the new state under name. Any
// failure between wipe and save restores the snapshot — preserve this when
// editing.
func performLogin(store *Store, name string, subcommand string) error {
	existingToken, existingClaudeJSON, err := captureLiveClaudeState()
	if err != nil {
		return err
	}

	_ = keychainDelete(keychainServiceClaude)
	_ = removeClaudeJSON()

	fmt.Printf("\nLaunching Claude — log in for the %q profile, then /exit to return here.\n\n", name)
	if err := runClaudeInteractive(); err != nil {
		restoreSnapshot(existingToken, existingClaudeJSON)
		return fmt.Errorf("running claude: %w", err)
	}

	newToken, err := keychainGet(keychainServiceClaude)
	if err != nil {
		restoreSnapshot(existingToken, existingClaudeJSON)
		return fmt.Errorf("no Claude credentials found after login — was login completed?")
	}
	newClaudeJSON, err := readClaudeJSON()
	if err != nil {
		restoreSnapshot(existingToken, existingClaudeJSON)
		return fmt.Errorf("read new ~/.claude.json: %w", err)
	}
	if newClaudeJSON == nil {
		restoreSnapshot(existingToken, existingClaudeJSON)
		return fmt.Errorf("no ~/.claude.json found after login — was login completed?")
	}

	if merged, changed, err := mergeUserMcpServers(newClaudeJSON, existingClaudeJSON); err != nil {
		restoreSnapshot(existingToken, existingClaudeJSON)
		return fmt.Errorf("merge user-scope MCP servers: %w", err)
	} else if changed {
		newClaudeJSON = merged
		if err := writeClaudeJSON(newClaudeJSON); err != nil {
			restoreSnapshot(existingToken, existingClaudeJSON)
			return fmt.Errorf("write merged ~/.claude.json: %w", err)
		}
	}

	if err := store.Save(name, newClaudeJSON, newToken); err != nil {
		fmt.Fprintf(os.Stderr, "\nthe new account is logged in to Claude, but the profile snapshot was not saved.\n")
		fmt.Fprintf(os.Stderr, "fix the underlying error and re-run `claude-accounts %s %s` to retry.\n", subcommand, name)
		return fmt.Errorf("save profile: %w", err)
	}
	if err := store.SetCurrent(name); err != nil {
		fmt.Fprintf(os.Stderr, "\nprofile %q saved, but couldn't be marked as active.\n", name)
		fmt.Fprintf(os.Stderr, "run `claude-accounts %s` to fix.\n", name)
		return fmt.Errorf("set current: %w", err)
	}
	return nil
}

func runClaudeInteractive() error {
	cmd := exec.Command("claude")
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// captureLiveClaudeState reads the currently active Claude identity (Keychain
// token + ~/.claude.json) so a later step can roll back to it on failure.
// Either piece may legitimately be absent.
func captureLiveClaudeState() (string, []byte, error) {
	token, err := keychainGet(keychainServiceClaude)
	if err != nil && !errors.Is(err, errKeychainNotFound) {
		return "", nil, fmt.Errorf("read existing Claude credentials: %w", err)
	}
	claudeJSON, err := readClaudeJSON()
	if err != nil {
		return "", nil, fmt.Errorf("read existing ~/.claude.json: %w", err)
	}
	return token, claudeJSON, nil
}

// restoreSnapshot puts the live Claude identity back to a captured state.
// Empty token / nil JSON mean "originally absent" — delete rather than skip,
// otherwise we'd leave behind whatever was written in between.
func restoreSnapshot(token string, claudeJSON []byte) {
	if token != "" {
		_ = keychainSet(keychainServiceClaude, token)
	} else {
		_ = keychainDelete(keychainServiceClaude)
	}
	if claudeJSON != nil {
		_ = writeClaudeJSON(claudeJSON)
	} else {
		_ = removeClaudeJSON()
	}
}
