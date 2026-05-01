package main

import (
	"os"
	"path/filepath"
	"testing"
)

// fakeKeychain swaps the package-level keychain* vars for an in-memory map
// scoped to a single test, restoring the originals via t.Cleanup. The
// returned map is the live backing store: tests can read it to assert state
// or write to it to seed an entry.
func fakeKeychain(t *testing.T) map[string]string {
	t.Helper()
	store := map[string]string{}

	origGet, origSet, origDelete := keychainGet, keychainSet, keychainDelete

	keychainGet = func(service string) (string, error) {
		v, ok := store[service]
		if !ok {
			return "", errKeychainNotFound
		}
		return v, nil
	}
	keychainSet = func(service, password string) error {
		store[service] = password
		return nil
	}
	keychainDelete = func(service string) error {
		delete(store, service)
		return nil
	}

	t.Cleanup(func() {
		keychainGet, keychainSet, keychainDelete = origGet, origSet, origDelete
	})
	return store
}

// fakeHome redirects HOME and XDG_CONFIG_HOME to a fresh tempdir so file IO
// (~/.claude.json, ~/.config/claude-accounts) is sandboxed. Returns the home
// path. t.Setenv handles cleanup automatically.
func fakeHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	return home
}

// fakeClaudeRun replaces runClaudeInteractive with fn for the test. The fn
// typically writes to the fake keychain and ~/.claude.json to simulate a
// successful login, or returns an error to simulate a failed one.
func fakeClaudeRun(t *testing.T, fn func() error) {
	t.Helper()
	orig := runClaudeInteractive
	runClaudeInteractive = fn
	t.Cleanup(func() { runClaudeInteractive = orig })
}

// swallowStdout redirects os.Stdout to /dev/null for the test. Several
// production paths (cmdSwitch, performLogin, cmdDelete) print human-facing
// progress messages that just clutter `go test -v` output.
func swallowStdout(t *testing.T) {
	t.Helper()
	orig := os.Stdout
	devnull, err := os.OpenFile(os.DevNull, os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = devnull
	t.Cleanup(func() {
		os.Stdout = orig
		_ = devnull.Close()
	})
}

// swallowStderr redirects os.Stderr to /dev/null for the test, since
// restoreSnapshot writes warnings there in some paths and we don't want
// noisy test output.
func swallowStderr(t *testing.T) {
	t.Helper()
	orig := os.Stderr
	devnull, err := os.OpenFile(os.DevNull, os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	os.Stderr = devnull
	t.Cleanup(func() {
		os.Stderr = orig
		_ = devnull.Close()
	})
}
