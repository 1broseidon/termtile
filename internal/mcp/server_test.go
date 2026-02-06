package mcp

import (
	"testing"

	"github.com/1broseidon/termtile/internal/config"
)

func TestShellQuote(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"", "''"},
		{"hello", "hello"},
		{"hello world", "'hello world'"},
		{"it's", `'it'\''s'`},
		{"$HOME", "'$HOME'"},
		{"foo;bar", "'foo;bar'"},
		{"a|b", "'a|b'"},
		{`back\slash`, `'back\slash'`},
		{"tab\there", "'tab\there'"},
		{"new\nline", "'new\nline'"},
		{"(parens)", "'(parens)'"},
		{`"quoted"`, `'"quoted"'`},
		{"simple-path/file.go", "simple-path/file.go"},
		{"no_special_chars123", "no_special_chars123"},
	}
	for _, tt := range tests {
		got := shellQuote(tt.input)
		if got != tt.want {
			t.Errorf("shellQuote(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func boolPtr(b bool) *bool { return &b }

func TestResolveSpawnMode(t *testing.T) {
	tests := []struct {
		name           string
		window         *bool
		agentSpawnMode string
		want           string
	}{
		{"nil window, empty config → pane", nil, "", "pane"},
		{"nil window, pane config → pane", nil, "pane", "pane"},
		{"nil window, window config → window", nil, "window", "window"},
		{"true window overrides pane config", boolPtr(true), "pane", "window"},
		{"true window overrides empty config", boolPtr(true), "", "window"},
		{"false window overrides window config", boolPtr(false), "window", "pane"},
		{"false window, empty config → pane", boolPtr(false), "", "pane"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := resolveSpawnMode(tt.window, tt.agentSpawnMode)
			if got != tt.want {
				t.Errorf("resolveSpawnMode(%v, %q) = %q, want %q", tt.window, tt.agentSpawnMode, got, tt.want)
			}
		})
	}
}

func TestPeekNextSlot(t *testing.T) {
	s := &Server{
		config:   config.DefaultConfig(),
		tracked:  make(map[string]map[int]trackedAgent),
		nextSlot: make(map[string]int),
	}

	// Fresh workspace starts at 0.
	if got := s.peekNextSlot("test-ws"); got != 0 {
		t.Errorf("peekNextSlot = %d, want 0", got)
	}

	// Peek should not increment.
	if got := s.peekNextSlot("test-ws"); got != 0 {
		t.Errorf("peekNextSlot after second peek = %d, want 0", got)
	}

	// After allocating, peek should return next.
	s.allocateSlot("test-ws", "claude", "%5", "pane", "", false)
	if got := s.peekNextSlot("test-ws"); got != 1 {
		t.Errorf("peekNextSlot after allocate = %d, want 1", got)
	}

	// Allocate in window mode.
	s.allocateSlot("test-ws", "codex", "termtile-test-ws-1:0.0", "window", "", false)
	if got := s.peekNextSlot("test-ws"); got != 2 {
		t.Errorf("peekNextSlot after window allocate = %d, want 2", got)
	}
}

func TestGetSpawnMode(t *testing.T) {
	s := &Server{
		config:   config.DefaultConfig(),
		tracked:  make(map[string]map[int]trackedAgent),
		nextSlot: make(map[string]int),
	}

	s.allocateSlot("ws", "claude", "%5", "pane", "", false)
	s.allocateSlot("ws", "codex", "termtile-ws-1:0.0", "window", "", false)

	if got := s.getSpawnMode("ws", 0); got != "pane" {
		t.Errorf("slot 0 spawn mode = %q, want %q", got, "pane")
	}
	if got := s.getSpawnMode("ws", 1); got != "window" {
		t.Errorf("slot 1 spawn mode = %q, want %q", got, "window")
	}
	if got := s.getSpawnMode("ws", 99); got != "" {
		t.Errorf("nonexistent slot spawn mode = %q, want empty", got)
	}
	if got := s.getSpawnMode("nonexistent", 0); got != "" {
		t.Errorf("nonexistent workspace spawn mode = %q, want empty", got)
	}
}
