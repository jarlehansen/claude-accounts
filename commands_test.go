package main

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// simulateLogin returns a fake runClaudeInteractive that, when called, writes
// the given token to the live Claude keychain entry and the given JSON to
// ~/.claude.json — i.e. mimics a successful Claude CLI login.
func simulateLogin(keychain map[string]string, token string, claudeJSON []byte) func() error {
	return func() error {
		keychain[keychainServiceClaude] = token
		return writeClaudeJSON(claudeJSON)
	}
}

func TestPerformLogin_RollsBackOnClaudeFailure(t *testing.T) {
	fakeHome(t)
	swallowStdout(t)
	keychain := fakeKeychain(t)

	// pre-existing live state
	keychain[keychainServiceClaude] = "old-token"
	prevJSON := []byte(`{"oauthAccount":{"emailAddress":"old@x.com"}}`)
	if err := writeClaudeJSON(prevJSON); err != nil {
		t.Fatal(err)
	}

	fakeClaudeRun(t, func() error { return fmt.Errorf("login failed") })

	store, _ := NewStore()
	err := performLogin(store, "alice", "create")
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	if got := keychain[keychainServiceClaude]; got != "old-token" {
		t.Errorf("live token = %q, want old-token (rollback failed)", got)
	}
	got, err := readClaudeJSON()
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, prevJSON) {
		t.Errorf("live ~/.claude.json not restored: got %s", got)
	}
	if store.Exists("alice") {
		t.Error("profile alice should not have been saved")
	}
}

func TestPerformLogin_RollsBackWhenNoTokenAfterLogin(t *testing.T) {
	fakeHome(t)
	swallowStdout(t)
	keychain := fakeKeychain(t)

	keychain[keychainServiceClaude] = "old-token"
	prevJSON := []byte(`{"x":1}`)
	_ = writeClaudeJSON(prevJSON)

	// claude "succeeds" but doesn't write a credential
	fakeClaudeRun(t, func() error {
		return writeClaudeJSON([]byte(`{"y":2}`))
	})

	store, _ := NewStore()
	err := performLogin(store, "alice", "create")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if got := keychain[keychainServiceClaude]; got != "old-token" {
		t.Errorf("live token = %q, want old-token", got)
	}
	got, _ := readClaudeJSON()
	if !bytes.Equal(got, prevJSON) {
		t.Errorf("~/.claude.json not restored: got %s", got)
	}
}

func TestPerformLogin_RollsBackWhenNoClaudeJSONAfterLogin(t *testing.T) {
	fakeHome(t)
	swallowStdout(t)
	keychain := fakeKeychain(t)

	keychain[keychainServiceClaude] = "old-token"
	prevJSON := []byte(`{"x":1}`)
	_ = writeClaudeJSON(prevJSON)

	// claude writes a token but no .claude.json
	fakeClaudeRun(t, func() error {
		keychain[keychainServiceClaude] = "new-token"
		return nil
	})

	store, _ := NewStore()
	err := performLogin(store, "alice", "create")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if got := keychain[keychainServiceClaude]; got != "old-token" {
		t.Errorf("live token = %q, want old-token", got)
	}
	got, _ := readClaudeJSON()
	if !bytes.Equal(got, prevJSON) {
		t.Errorf("~/.claude.json not restored: got %s", got)
	}
}

func TestPerformLogin_SuccessSavesAndSetsCurrent(t *testing.T) {
	fakeHome(t)
	swallowStdout(t)
	keychain := fakeKeychain(t)

	newJSON := []byte(`{"oauthAccount":{"emailAddress":"new@x.com"}}`)
	fakeClaudeRun(t, simulateLogin(keychain, "new-token", newJSON))

	store, _ := NewStore()
	if err := performLogin(store, "alice", "create"); err != nil {
		t.Fatal(err)
	}

	if !store.Exists("alice") {
		t.Fatal("profile alice not created")
	}
	cur, _ := store.Current()
	if cur != "alice" {
		t.Errorf("current = %q, want alice", cur)
	}
	if got := keychain[keychainServicePrefix+"alice"]; got != "new-token" {
		t.Errorf("profile keychain entry = %q, want new-token", got)
	}
	gotData, gotToken, _ := store.Load("alice")
	if gotToken != "new-token" {
		t.Errorf("loaded token = %q, want new-token", gotToken)
	}
	if !bytes.Equal(gotData, newJSON) {
		t.Errorf("loaded data mismatch")
	}
}

// performLogin documents that a Save failure is NOT rolled back: the live
// state is left as the freshly-logged-in identity, since the user really did
// log in — only the profile snapshot is missing. We simulate Save failure by
// making the profile's parent dir read-only after login, then verifying live
// state == new identity (not old).
func TestPerformLogin_DoesNotRollBackOnSaveFailure(t *testing.T) {
	fakeHome(t)
	swallowStdout(t)
	swallowStderr(t)
	keychain := fakeKeychain(t)

	keychain[keychainServiceClaude] = "old-token"
	prevJSON := []byte(`{"old":true}`)
	_ = writeClaudeJSON(prevJSON)

	newJSON := []byte(`{"new":true}`)
	fakeClaudeRun(t, simulateLogin(keychain, "new-token", newJSON))

	store, _ := NewStore()

	// Make the profiles dir read-only so store.Save's MkdirAll succeeds (the
	// dir already exists from NewStore) but atomicWrite into it fails.
	profilesDir := filepath.Join(store.root, "profiles")
	if err := os.Chmod(profilesDir, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(profilesDir, 0o700) })

	err := performLogin(store, "alice", "create")
	if err == nil {
		t.Fatal("expected Save failure, got nil error")
	}

	// Critical contract: live state stays as the NEW identity, not rolled
	// back. The user is logged in to the new account; only the snapshot
	// failed to save.
	if got := keychain[keychainServiceClaude]; got != "new-token" {
		t.Errorf("live token = %q, want new-token (Save failure should NOT rollback)", got)
	}
	live, _ := readClaudeJSON()
	if !bytes.Equal(live, newJSON) {
		t.Errorf("live ~/.claude.json = %s, want new identity", live)
	}
}

func TestPerformLogin_PropagatesMcpServersFromPreviousProfile(t *testing.T) {
	fakeHome(t)
	swallowStdout(t)
	keychain := fakeKeychain(t)

	// previous active profile had an iOS MCP server registered
	keychain[keychainServiceClaude] = "old-token"
	prevJSON := []byte(`{"mcpServers":{"ios":{"command":"x"}},"a":1}`)
	_ = writeClaudeJSON(prevJSON)

	// new login wipes the file and writes a fresh one with no mcpServers
	newJSON := []byte(`{"oauthAccount":{"emailAddress":"new@x.com"}}`)
	fakeClaudeRun(t, simulateLogin(keychain, "new-token", newJSON))

	store, _ := NewStore()
	if err := performLogin(store, "alice", "create"); err != nil {
		t.Fatal(err)
	}

	// the saved profile snapshot AND the live ~/.claude.json should both
	// have inherited the iOS MCP server.
	saved, _, _ := store.Load("alice")
	if !bytes.Contains(saved, []byte(`"ios"`)) {
		t.Errorf("saved snapshot missing mcpServers: %s", saved)
	}
	live, _ := readClaudeJSON()
	if !bytes.Contains(live, []byte(`"ios"`)) {
		t.Errorf("live ~/.claude.json missing mcpServers: %s", live)
	}
}

func TestCmdSwitch_HappyPath(t *testing.T) {
	fakeHome(t)
	swallowStdout(t)
	keychain := fakeKeychain(t)

	store, _ := NewStore()

	// create a stored profile "bob" directly
	bobJSON := []byte(`{"oauthAccount":{"emailAddress":"bob@x.com"}}`)
	if err := store.Save("bob", bobJSON, "bob-token"); err != nil {
		t.Fatal(err)
	}

	// live state belongs to some other identity
	keychain[keychainServiceClaude] = "alice-token"
	_ = writeClaudeJSON([]byte(`{"oauthAccount":{"emailAddress":"alice@x.com"}}`))

	if err := cmdSwitch("bob"); err != nil {
		t.Fatal(err)
	}

	if got := keychain[keychainServiceClaude]; got != "bob-token" {
		t.Errorf("live token = %q, want bob-token", got)
	}
	cur, _ := store.Current()
	if cur != "bob" {
		t.Errorf("current = %q, want bob", cur)
	}
	live, _ := readClaudeJSON()
	if !bytes.Contains(live, []byte("bob@x.com")) {
		t.Errorf("live ~/.claude.json not bob's: %s", live)
	}
}

func TestCmdSwitch_PropagatesMcpServers(t *testing.T) {
	fakeHome(t)
	swallowStdout(t)
	keychain := fakeKeychain(t)

	store, _ := NewStore()

	// stored profile bob has no mcpServers
	bobJSON := []byte(`{"oauthAccount":{"emailAddress":"bob@x.com"}}`)
	_ = store.Save("bob", bobJSON, "bob-token")

	// live state has an iOS MCP server (added under a different active profile)
	keychain[keychainServiceClaude] = "alice-token"
	liveJSON := []byte(`{"mcpServers":{"ios":{"command":"x"}}}`)
	_ = writeClaudeJSON(liveJSON)

	if err := cmdSwitch("bob"); err != nil {
		t.Fatal(err)
	}

	// the new live ~/.claude.json should carry forward the iOS server even
	// though bob's snapshot didn't have it.
	live, _ := readClaudeJSON()
	if !bytes.Contains(live, []byte(`"ios"`)) {
		t.Errorf("live ~/.claude.json lost mcpServers: %s", live)
	}
}

func TestCmdSwitch_UnknownProfileFails(t *testing.T) {
	fakeHome(t)
	fakeKeychain(t)
	if err := cmdSwitch("ghost"); err == nil {
		t.Fatal("expected error for unknown profile")
	}
}

func TestCmdReauth_RequiresExistingProfile(t *testing.T) {
	fakeHome(t)
	keychain := fakeKeychain(t)

	// pre-existing live state — must NOT be touched if reauth bails early
	keychain[keychainServiceClaude] = "live-token"
	_ = writeClaudeJSON([]byte(`{"live":true}`))

	err := cmdReauth("ghost")
	if err == nil {
		t.Fatal("expected error for nonexistent profile")
	}
	// the wipe-then-login flow must not have started: live state intact
	if got := keychain[keychainServiceClaude]; got != "live-token" {
		t.Errorf("live token clobbered: got %q, want live-token", got)
	}
}

func TestCmdReauth_RewritesExistingProfile(t *testing.T) {
	fakeHome(t)
	swallowStdout(t)
	keychain := fakeKeychain(t)
	store, _ := NewStore()

	// pre-existing profile with old credentials
	_ = store.Save("alice", []byte(`{"old":true}`), "old-token")

	// fresh login produces new credentials
	newJSON := []byte(`{"new":true}`)
	fakeClaudeRun(t, simulateLogin(keychain, "new-token", newJSON))

	if err := cmdReauth("alice"); err != nil {
		t.Fatal(err)
	}

	// snapshot updated
	gotData, gotToken, _ := store.Load("alice")
	if gotToken != "new-token" {
		t.Errorf("stored token = %q, want new-token", gotToken)
	}
	if !bytes.Equal(gotData, newJSON) {
		t.Errorf("stored data not updated: %s", gotData)
	}
	// reauth makes the target profile active
	cur, _ := store.Current()
	if cur != "alice" {
		t.Errorf("current = %q, want alice", cur)
	}
}

func TestCmdDelete_RemovesProfileAndKeychain(t *testing.T) {
	fakeHome(t)
	swallowStdout(t)
	keychain := fakeKeychain(t)
	store, _ := NewStore()
	_ = store.Save("alice", []byte("{}"), "tok")

	if err := cmdDelete("alice"); err != nil {
		t.Fatal(err)
	}
	if store.Exists("alice") {
		t.Error("profile dir not removed")
	}
	if _, ok := keychain[keychainServicePrefix+"alice"]; ok {
		t.Error("keychain entry not removed")
	}
}

func TestCmdDelete_RefusesActiveProfile(t *testing.T) {
	fakeHome(t)
	fakeKeychain(t)
	store, _ := NewStore()
	_ = store.Save("alice", []byte("{}"), "tok")
	_ = store.SetCurrent("alice")

	err := cmdDelete("alice")
	if err == nil {
		t.Fatal("expected error deleting active profile")
	}
	if !strings.Contains(err.Error(), "active profile") {
		t.Errorf("error = %v, want mention of 'active profile'", err)
	}
	if !store.Exists("alice") {
		t.Error("profile should still exist after refused delete")
	}
}

func TestCmdDelete_MissingProfileFails(t *testing.T) {
	fakeHome(t)
	fakeKeychain(t)
	if err := cmdDelete("ghost"); err == nil {
		t.Fatal("expected error for nonexistent profile")
	}
}

func TestCaptureLiveClaudeState_HandlesAbsentState(t *testing.T) {
	fakeHome(t)
	fakeKeychain(t)

	token, claudeJSON, err := captureLiveClaudeState()
	if err != nil {
		t.Fatal(err)
	}
	if token != "" {
		t.Errorf("token = %q, want empty", token)
	}
	if claudeJSON != nil {
		t.Errorf("claudeJSON = %s, want nil", claudeJSON)
	}
}

func TestRestoreSnapshot_DeletesWhenOriginallyAbsent(t *testing.T) {
	fakeHome(t)
	keychain := fakeKeychain(t)

	// some intermediate state was written
	keychain[keychainServiceClaude] = "intermediate"
	_ = writeClaudeJSON([]byte(`{"intermediate":true}`))

	// restoring a "nothing was there" snapshot should delete both.
	swallowStderr(t)
	restoreSnapshot("", nil)

	if _, ok := keychain[keychainServiceClaude]; ok {
		t.Error("keychain entry not deleted")
	}
	if data, _ := readClaudeJSON(); data != nil {
		t.Errorf("~/.claude.json not removed: %s", data)
	}
}

func TestRestoreSnapshot_RestoresWhenOriginallyPresent(t *testing.T) {
	fakeHome(t)
	keychain := fakeKeychain(t)

	// intermediate state — what would be present after a botched create
	keychain[keychainServiceClaude] = "intermediate"
	_ = writeClaudeJSON([]byte(`{"intermediate":true}`))

	// restore the original captured state
	origJSON := []byte(`{"original":true}`)
	restoreSnapshot("orig-token", origJSON)

	if got := keychain[keychainServiceClaude]; got != "orig-token" {
		t.Errorf("keychain = %q, want orig-token", got)
	}
	got, _ := readClaudeJSON()
	if !bytes.Equal(got, origJSON) {
		t.Errorf("~/.claude.json = %s, want %s", got, origJSON)
	}
}
