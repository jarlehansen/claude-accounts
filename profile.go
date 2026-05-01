package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

var profileNameRe = regexp.MustCompile(`^[A-Za-z0-9._-]{1,64}$`)

// validateProfileName rejects names that would escape the profiles dir,
// collide with shell metacharacters, or otherwise trip over filesystem rules.
// Apply at every trust boundary (CLI argv, TUI input). Names returned by
// Store.List come from the filesystem and are trusted.
func validateProfileName(name string) error {
	if name == "." || name == ".." {
		return fmt.Errorf("invalid profile name %q", name)
	}
	if !profileNameRe.MatchString(name) {
		return fmt.Errorf("invalid profile name %q: use letters, digits, '.', '_', '-' (max 64 chars)", name)
	}
	return nil
}

const claudeJSONRelative = ".claude.json"

type Store struct {
	root string // ~/.config/claude-accounts
}

func NewStore() (*Store, error) {
	configHome := os.Getenv("XDG_CONFIG_HOME")
	if configHome == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, err
		}
		configHome = filepath.Join(home, ".config")
	}
	root := filepath.Join(configHome, "claude-accounts")
	if err := os.MkdirAll(filepath.Join(root, "profiles"), 0o700); err != nil {
		return nil, err
	}
	return &Store{root: root}, nil
}

func (s *Store) profileDir(name string) string {
	return filepath.Join(s.root, "profiles", name)
}

func (s *Store) profileClaudeJSON(name string) string {
	return filepath.Join(s.profileDir(name), "claude.json")
}

func (s *Store) currentFile() string {
	return filepath.Join(s.root, "current")
}

func (s *Store) Current() (string, error) {
	data, err := os.ReadFile(s.currentFile())
	if os.IsNotExist(err) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(data)), nil
}

func (s *Store) SetCurrent(name string) error {
	return atomicWrite(s.currentFile(), []byte(name+"\n"), 0o600)
}

func (s *Store) List() ([]string, error) {
	entries, err := os.ReadDir(filepath.Join(s.root, "profiles"))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var names []string
	for _, e := range entries {
		if e.IsDir() {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)
	return names, nil
}

func (s *Store) Exists(name string) bool {
	_, err := os.Stat(s.profileDir(name))
	return err == nil
}

func (s *Store) Save(name string, claudeJSON []byte, token string) error {
	if err := os.MkdirAll(s.profileDir(name), 0o700); err != nil {
		return err
	}
	if err := atomicWrite(s.profileClaudeJSON(name), claudeJSON, 0o600); err != nil {
		return err
	}
	return keychainSet(keychainServicePrefix+name, token)
}

func (s *Store) Load(name string) ([]byte, string, error) {
	if !s.Exists(name) {
		return nil, "", fmt.Errorf("profile %q does not exist", name)
	}
	claudeJSON, err := os.ReadFile(s.profileClaudeJSON(name))
	if err != nil {
		return nil, "", err
	}
	token, err := keychainGet(keychainServicePrefix + name)
	if err != nil {
		return nil, "", fmt.Errorf("read keychain entry for %q: %w", name, err)
	}
	return claudeJSON, token, nil
}

// Email returns the OAuth account email stored in profiles/<name>/claude.json,
// or "" if it can't be read or parsed. Display-only, never an error.
func (s *Store) Email(name string) string {
	data, err := os.ReadFile(s.profileClaudeJSON(name))
	if err != nil {
		return ""
	}
	var j struct {
		OauthAccount struct {
			EmailAddress string `json:"emailAddress"`
		} `json:"oauthAccount"`
	}
	if err := json.Unmarshal(data, &j); err != nil {
		return ""
	}
	return j.OauthAccount.EmailAddress
}

// ReconcileCurrent clears the "current" pointer when reality has drifted:
// the named profile was deleted, or the live Keychain token no longer
// matches what we stored for it (e.g. user ran `claude /logout` directly).
func (s *Store) ReconcileCurrent() error {
	current, err := s.Current()
	if err != nil || current == "" {
		return err
	}
	if !s.Exists(current) {
		return s.SetCurrent("")
	}
	live, err := keychainGet(keychainServiceClaude)
	if err != nil && !errors.Is(err, errKeychainNotFound) {
		return err
	}
	stored, err := keychainGet(keychainServicePrefix + current)
	if err != nil && !errors.Is(err, errKeychainNotFound) {
		return err
	}
	if live != stored {
		return s.SetCurrent("")
	}
	return nil
}

func (s *Store) Delete(name string) error {
	if !s.Exists(name) {
		return fmt.Errorf("profile %q does not exist", name)
	}
	// Delete the keychain entry first. If it fails, the disk dir is untouched
	// and the user can retry. The reverse order would risk an orphan keychain
	// entry that's invisible to subsequent listings.
	if err := keychainDelete(keychainServicePrefix + name); err != nil {
		return fmt.Errorf("delete keychain entry for %q: %w", name, err)
	}
	if err := os.RemoveAll(s.profileDir(name)); err != nil {
		return fmt.Errorf("remove profile dir for %q (keychain entry already deleted): %w", name, err)
	}
	return nil
}

func atomicWrite(path string, data []byte, mode os.FileMode) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".tmp-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	cleanup := true
	defer func() {
		if cleanup {
			os.Remove(tmpName)
		}
	}()
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Chmod(mode); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmpName, path); err != nil {
		return err
	}
	cleanup = false
	return nil
}

func claudeJSONPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, claudeJSONRelative), nil
}

// readClaudeJSON returns (nil, nil) if ~/.claude.json is missing.
func readClaudeJSON() ([]byte, error) {
	p, err := claudeJSONPath()
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(p)
	if os.IsNotExist(err) {
		return nil, nil
	}
	return data, err
}

func writeClaudeJSON(data []byte) error {
	p, err := claudeJSONPath()
	if err != nil {
		return err
	}
	return atomicWrite(p, data, 0o600)
}

func removeClaudeJSON() error {
	p, err := claudeJSONPath()
	if err != nil {
		return err
	}
	if err := os.Remove(p); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}
