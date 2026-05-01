package main

import (
	"bytes"
	"errors"
	"fmt"
	"os/exec"
	"os/user"
	"strings"
)

// Service name used by Claude Code itself for its OAuth credentials on macOS.
const keychainServiceClaude = "Claude Code-credentials"

// Per-profile entries are stored under a namespaced service.
const keychainServicePrefix = "claude-accounts:"

var errKeychainNotFound = errors.New("keychain item not found")

func currentUsername() (string, error) {
	u, err := user.Current()
	if err != nil {
		return "", err
	}
	return u.Username, nil
}

// keychainGet, keychainSet, keychainDelete are vars (not direct funcs) so
// tests can swap them for an in-memory backend without touching the real
// macOS Keychain.
var (
	keychainGet    = realKeychainGet
	keychainSet    = realKeychainSet
	keychainDelete = realKeychainDelete
)

// realKeychainGet returns the password for service+current-user account, or
// errKeychainNotFound if the entry does not exist.
func realKeychainGet(service string) (string, error) {
	username, err := currentUsername()
	if err != nil {
		return "", err
	}
	cmd := exec.Command("security", "find-generic-password", "-s", service, "-a", username, "-w")
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) && exitErr.ExitCode() == 44 {
			return "", errKeychainNotFound
		}
		return "", fmt.Errorf("security find-generic-password: %w (%s)", err, strings.TrimSpace(stderr.String()))
	}
	// `security -w` includes a trailing newline.
	return strings.TrimRight(stdout.String(), "\n"), nil
}

// realKeychainSet creates or updates an entry. Note: the password is passed
// as an argv element to /usr/bin/security, so it is briefly visible in `ps`
// output. Acceptable on a single-user dev machine; not a server-grade design.
func realKeychainSet(service, password string) error {
	username, err := currentUsername()
	if err != nil {
		return err
	}
	cmd := exec.Command("security", "add-generic-password",
		"-U", "-s", service, "-a", username, "-w", password)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("security add-generic-password: %w (%s)", err, strings.TrimSpace(stderr.String()))
	}
	return nil
}

// realKeychainDelete removes an entry. Missing is not an error.
func realKeychainDelete(service string) error {
	username, err := currentUsername()
	if err != nil {
		return err
	}
	cmd := exec.Command("security", "delete-generic-password", "-s", service, "-a", username)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) && exitErr.ExitCode() == 44 {
			return nil
		}
		return fmt.Errorf("security delete-generic-password: %w (%s)", err, strings.TrimSpace(stderr.String()))
	}
	return nil
}
