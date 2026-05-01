package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestValidateProfileName(t *testing.T) {
	cases := []struct {
		in string
		ok bool
	}{
		{"work", true},
		{"a", true},
		{"a.b_c-d", true},
		{"WithCAPS", true},
		{"123numeric", true},
		{strings.Repeat("a", 64), true},

		{"", false},
		{".", false},
		{"..", false},
		{"../escape", false},
		{"with space", false},
		{"with/slash", false},
		{"with$dollar", false},
		{"with;semi", false},
		{"with\nnewline", false},
		{strings.Repeat("a", 65), false},
	}
	for _, c := range cases {
		err := validateProfileName(c.in)
		if (err == nil) != c.ok {
			t.Errorf("validateProfileName(%q) ok=%v, err=%v", c.in, c.ok, err)
		}
	}
}

func TestMergeUserMcpServers(t *testing.T) {
	cases := []struct {
		name           string
		target, source string
		wantChanged    bool
		wantHasKey     bool   // expect mcpServers in result
		wantKeyValue   string // expected mcpServers value (JSON), if wantHasKey
		wantErr        bool
	}{
		{
			name:        "neither has mcpServers",
			target:      `{"a":1}`,
			source:      `{"b":2}`,
			wantChanged: false,
			wantHasKey:  false,
		},
		{
			name:         "source has, target doesn't",
			target:       `{"a":1}`,
			source:       `{"mcpServers":{"ios":{"command":"x"}}}`,
			wantChanged:  true,
			wantHasKey:   true,
			wantKeyValue: `{"ios":{"command":"x"}}`,
		},
		{
			name:        "target has, source doesn't",
			target:      `{"mcpServers":{"ios":{"command":"x"}}}`,
			source:      `{"a":1}`,
			wantChanged: true,
			wantHasKey:  false,
		},
		{
			name:         "both have, identical",
			target:       `{"mcpServers":{"ios":{"command":"x"}}}`,
			source:       `{"mcpServers":{"ios":{"command":"x"}}}`,
			wantChanged:  false,
			wantHasKey:   true,
			wantKeyValue: `{"ios":{"command":"x"}}`,
		},
		{
			name:         "both have, source wins on conflict",
			target:       `{"mcpServers":{"ios":{"command":"old"}}}`,
			source:       `{"mcpServers":{"ios":{"command":"new"}}}`,
			wantChanged:  true,
			wantHasKey:   true,
			wantKeyValue: `{"ios":{"command":"new"}}`,
		},
		{
			name:         "nil source, target has key — preserves target unchanged",
			target:       `{"mcpServers":{"ios":{"command":"x"}}}`,
			source:       "", // sentinel for nil
			wantChanged:  false,
			wantHasKey:   true,
			wantKeyValue: `{"ios":{"command":"x"}}`,
		},
		{
			name:        "nil source, target has no key — passes through",
			target:      `{"a":1}`,
			source:      "",
			wantChanged: false,
			wantHasKey:  false,
		},
		{
			name:    "nil source, target malformed — still validated",
			target:  `{not json`,
			source:  "",
			wantErr: true,
		},
		{
			name:        "malformed target",
			target:      `{not json`,
			source:      `{}`,
			wantErr:     true,
			wantChanged: false,
		},
		{
			name:        "malformed source",
			target:      `{}`,
			source:      `{not json`,
			wantErr:     true,
			wantChanged: false,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var src []byte
			if c.source != "" {
				src = []byte(c.source)
			}
			out, changed, err := mergeUserMcpServers([]byte(c.target), src)
			if c.wantErr {
				if err == nil {
					t.Fatalf("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if changed != c.wantChanged {
				t.Errorf("changed=%v, want %v", changed, c.wantChanged)
			}

			var parsed map[string]json.RawMessage
			if err := json.Unmarshal(out, &parsed); err != nil {
				t.Fatalf("output not valid JSON: %v", err)
			}
			val, has := parsed["mcpServers"]
			if has != c.wantHasKey {
				t.Errorf("mcpServers present=%v, want %v", has, c.wantHasKey)
			}
			if c.wantHasKey && c.wantKeyValue != "" {
				// Compare semantically (key order doesn't matter).
				var got, want any
				if err := json.Unmarshal(val, &got); err != nil {
					t.Fatalf("parse got: %v", err)
				}
				if err := json.Unmarshal([]byte(c.wantKeyValue), &want); err != nil {
					t.Fatalf("parse want: %v", err)
				}
				gotJSON, _ := json.Marshal(got)
				wantJSON, _ := json.Marshal(want)
				if !bytes.Equal(gotJSON, wantJSON) {
					t.Errorf("mcpServers = %s, want %s", gotJSON, wantJSON)
				}
			}
		})
	}
}

func TestStore_SaveAndLoad(t *testing.T) {
	fakeHome(t)
	keychain := fakeKeychain(t)
	store, err := NewStore()
	if err != nil {
		t.Fatal(err)
	}

	data := []byte(`{"oauthAccount":{"emailAddress":"a@x.com"}}`)
	if err := store.Save("alice", data, "tok-alice"); err != nil {
		t.Fatal(err)
	}

	gotData, gotToken, err := store.Load("alice")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(gotData, data) {
		t.Errorf("data mismatch: got %s want %s", gotData, data)
	}
	if gotToken != "tok-alice" {
		t.Errorf("token = %q, want tok-alice", gotToken)
	}
	if keychain[keychainServicePrefix+"alice"] != "tok-alice" {
		t.Errorf("keychain entry not written")
	}
}

func TestStore_LoadMissing(t *testing.T) {
	fakeHome(t)
	fakeKeychain(t)
	store, err := NewStore()
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := store.Load("ghost"); err == nil {
		t.Fatal("expected error loading nonexistent profile")
	}
}

func TestStore_Delete(t *testing.T) {
	fakeHome(t)
	keychain := fakeKeychain(t)
	store, _ := NewStore()
	_ = store.Save("alice", []byte("{}"), "tok")

	if err := store.Delete("alice"); err != nil {
		t.Fatal(err)
	}
	if store.Exists("alice") {
		t.Error("profile dir not removed")
	}
	if _, ok := keychain[keychainServicePrefix+"alice"]; ok {
		t.Error("keychain entry not removed")
	}
}

func TestStore_DeleteMissing(t *testing.T) {
	fakeHome(t)
	fakeKeychain(t)
	store, _ := NewStore()
	if err := store.Delete("ghost"); err == nil {
		t.Fatal("expected error deleting nonexistent profile")
	}
}

func TestStore_List_SortedAndEmpty(t *testing.T) {
	fakeHome(t)
	fakeKeychain(t)
	store, _ := NewStore()

	names, err := store.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(names) != 0 {
		t.Errorf("empty store should list nothing, got %v", names)
	}

	for _, n := range []string{"charlie", "alice", "bob"} {
		_ = store.Save(n, []byte("{}"), "tok")
	}
	names, _ = store.List()
	want := []string{"alice", "bob", "charlie"}
	if !slices.Equal(names, want) {
		t.Errorf("List = %v, want %v", names, want)
	}
}

func TestStore_CurrentSetCurrent(t *testing.T) {
	fakeHome(t)
	fakeKeychain(t)
	store, _ := NewStore()

	cur, err := store.Current()
	if err != nil || cur != "" {
		t.Errorf("initial current = %q, %v; want \"\", nil", cur, err)
	}
	if err := store.SetCurrent("alice"); err != nil {
		t.Fatal(err)
	}
	cur, _ = store.Current()
	if cur != "alice" {
		t.Errorf("current = %q, want alice", cur)
	}
	_ = store.SetCurrent("")
	cur, _ = store.Current()
	if cur != "" {
		t.Errorf("after clear, current = %q, want \"\"", cur)
	}
}

func TestStore_Email(t *testing.T) {
	fakeHome(t)
	fakeKeychain(t)
	store, _ := NewStore()

	if got := store.Email("ghost"); got != "" {
		t.Errorf("missing profile email = %q, want \"\"", got)
	}

	_ = store.Save("alice", []byte(`{"oauthAccount":{"emailAddress":"a@x.com"}}`), "tok")
	if got := store.Email("alice"); got != "a@x.com" {
		t.Errorf("email = %q, want a@x.com", got)
	}

	_ = store.Save("broken", []byte(`{not json`), "tok")
	if got := store.Email("broken"); got != "" {
		t.Errorf("malformed claude.json email = %q, want \"\"", got)
	}
}

func TestStore_ReconcileCurrent(t *testing.T) {
	t.Run("clears when profile missing", func(t *testing.T) {
		fakeHome(t)
		fakeKeychain(t)
		store, _ := NewStore()
		_ = store.SetCurrent("ghost")
		if err := store.ReconcileCurrent(); err != nil {
			t.Fatal(err)
		}
		cur, _ := store.Current()
		if cur != "" {
			t.Errorf("current = %q, want \"\"", cur)
		}
	})

	t.Run("clears when live token differs from stored", func(t *testing.T) {
		fakeHome(t)
		keychain := fakeKeychain(t)
		store, _ := NewStore()
		_ = store.Save("alice", []byte("{}"), "tok-alice-stored")
		_ = store.SetCurrent("alice")
		keychain[keychainServiceClaude] = "different-live-token"

		if err := store.ReconcileCurrent(); err != nil {
			t.Fatal(err)
		}
		cur, _ := store.Current()
		if cur != "" {
			t.Errorf("current = %q, want \"\" after token mismatch", cur)
		}
	})

	t.Run("keeps when live token matches stored", func(t *testing.T) {
		fakeHome(t)
		keychain := fakeKeychain(t)
		store, _ := NewStore()
		_ = store.Save("alice", []byte("{}"), "shared-token")
		_ = store.SetCurrent("alice")
		keychain[keychainServiceClaude] = "shared-token"

		if err := store.ReconcileCurrent(); err != nil {
			t.Fatal(err)
		}
		cur, _ := store.Current()
		if cur != "alice" {
			t.Errorf("current = %q, want alice (matching tokens)", cur)
		}
	})
}

func TestAtomicWrite(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "file")
	if err := atomicWrite(path, []byte("hello"), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "hello" {
		t.Errorf("content = %q, want hello", got)
	}
	info, _ := os.Stat(path)
	if info.Mode().Perm() != 0o600 {
		t.Errorf("perm = %o, want 0600", info.Mode().Perm())
	}
}

func TestStore_SaveRollsBackKeychainOnDiskFailure(t *testing.T) {
	fakeHome(t)
	keychain := fakeKeychain(t)
	store, _ := NewStore()

	// pre-existing keychain entry (simulates a profile being overwritten)
	keychain[keychainServicePrefix+"alice"] = "old-token"
	// also create the profile dir so Save doesn't try to mkdir
	_ = os.MkdirAll(store.profileDir("alice"), 0o700)
	// make the profile dir read-only so atomicWrite of claude.json fails
	if err := os.Chmod(store.profileDir("alice"), 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(store.profileDir("alice"), 0o700) })

	err := store.Save("alice", []byte("{}"), "new-token")
	if err == nil {
		t.Fatal("expected Save to fail on read-only dir")
	}
	if !errors.Is(err, os.ErrPermission) && !strings.Contains(err.Error(), "permission") {
		// non-fatal: some filesystems report different errors
		t.Logf("note: error wasn't ErrPermission, was: %v", err)
	}
	if got := keychain[keychainServicePrefix+"alice"]; got != "old-token" {
		t.Errorf("keychain not rolled back: got %q, want old-token", got)
	}
}
