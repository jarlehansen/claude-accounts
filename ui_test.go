package main

import "testing"

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
