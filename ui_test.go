package main

import "testing"

// The switch picker seeds its cursor with firstNonActive so the common
// two-profile case is a single enter press.
func TestFirstNonActive(t *testing.T) {
	cases := []struct {
		name    string
		names   []string
		current string
		want    string
	}{
		{"two profiles, first active", []string{"work", "personal"}, "work", "personal"},
		{"two profiles, second active", []string{"work", "personal"}, "personal", "work"},
		{"no active profile", []string{"work", "personal"}, "", "work"},
		{"only profile is active", []string{"work"}, "work", ""},
		{"only profile is inactive", []string{"work"}, "", "work"},
		{"three profiles skips active", []string{"work", "personal", "demo"}, "work", "personal"},
		{"unknown active falls through", []string{"work", "personal"}, "gone", "work"},
		{"empty list", nil, "work", ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := firstNonActive(c.names, c.current); got != c.want {
				t.Errorf("firstNonActive(%v, %q) = %q, want %q", c.names, c.current, got, c.want)
			}
		})
	}
}

// safeDisplay strips C0 control bytes (0x00–0x1F) and DEL (0x7F) so values
// read from disk can't inject ANSI escape sequences into the TUI. This test
// pins that policy: stripping ESC (0x1B) defangs the entire CSI/OSC family
// without needing to parse them.
func TestSafeDisplay(t *testing.T) {
	cases := []struct {
		name string
		in   string
		out  string
	}{
		{"plain ascii", "alice@example.com", "alice@example.com"},
		{"unicode passes through", "ålice@éxample.com", "ålice@éxample.com"},
		{"strips ESC", "before\x1b[31mAFTER", "before[31mAFTER"},
		{"strips full CSI sequence start", "x\x1b[2J\x1b[Hy", "x[2J[Hy"},
		{"strips DEL", "ab\x7fcd", "abcd"},
		{"strips NUL", "ab\x00cd", "abcd"},
		{"strips bell", "ab\x07cd", "abcd"},
		{"strips newline and tab", "a\nb\tc", "abc"},
		{"strips CR", "a\rb", "ab"},
		{"empty stays empty", "", ""},
		{"all-control becomes empty", "\x00\x07\x1b\x7f", ""},
		{"space (0x20) is preserved", "a b", "a b"},
		{"tilde (0x7E) is preserved", "a~b", "a~b"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := safeDisplay(c.in); got != c.out {
				t.Errorf("safeDisplay(%q) = %q, want %q", c.in, got, c.out)
			}
		})
	}
}
