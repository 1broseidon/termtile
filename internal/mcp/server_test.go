package mcp

import (
	"testing"

	"github.com/1broseidon/termtile/internal/agent"
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

func TestUpdateOutputConfig(t *testing.T) {
	s := &Server{
		config:   config.DefaultConfig(),
		tracked:  make(map[string]map[int]trackedAgent),
		nextSlot: make(map[string]int),
	}

	slot := s.allocateSlot("ws", "claude", "%5", "pane", "old-marker", false)
	s.updateOutputConfig("ws", slot, "new-marker", true)

	marker, fence := s.getOutputConfig("ws", slot)
	if marker != "new-marker" {
		t.Fatalf("marker = %q, want %q", marker, "new-marker")
	}
	if !fence {
		t.Fatal("responseFence = false, want true")
	}

	// Non-existent slot should be a no-op.
	s.updateOutputConfig("ws", 999, "ignored", false)
	if marker2, _ := s.getOutputConfig("ws", slot); marker2 != "new-marker" {
		t.Fatalf("marker after no-op update = %q, want %q", marker2, "new-marker")
	}
}

func TestUpdateTmuxTarget(t *testing.T) {
	s := &Server{
		config:   config.DefaultConfig(),
		tracked:  make(map[string]map[int]trackedAgent),
		nextSlot: make(map[string]int),
	}

	slot := s.allocateSlot("ws", "claude", "", "window", "", false)
	s.updateTmuxTarget("ws", slot, "termtile-ws-0:0.0")

	target, ok := s.getTmuxTarget("ws", slot)
	if !ok {
		t.Fatal("getTmuxTarget returned not found")
	}
	if target != "termtile-ws-0:0.0" {
		t.Fatalf("target = %q, want %q", target, "termtile-ws-0:0.0")
	}

	// Non-existent slot should be a no-op.
	s.updateTmuxTarget("ws", 999, "ignored")
	if target2, _ := s.getTmuxTarget("ws", slot); target2 != "termtile-ws-0:0.0" {
		t.Fatalf("target after no-op update = %q, want %q", target2, "termtile-ws-0:0.0")
	}
}

func TestReconcile(t *testing.T) {
	s := &Server{
		config:   config.DefaultConfig(),
		tracked:  make(map[string]map[int]trackedAgent),
		nextSlot: make(map[string]int),
	}

	s.reconcileSessionNames([]string{
		"termtile-mcp-agents-0",
		"termtile-mcp-agents-2",
		"termtile-other-ws-1",
		"unrelated-session",
	})

	if got := len(s.tracked["mcp-agents"]); got != 2 {
		t.Fatalf("mcp-agents tracked len = %d, want 2", got)
	}
	if got := len(s.tracked["other-ws"]); got != 1 {
		t.Fatalf("other-ws tracked len = %d, want 1", got)
	}
	if got := len(s.tracked); got != 2 {
		t.Fatalf("tracked workspaces len = %d, want 2", got)
	}

	checkTracked := func(workspace string, slot int, sessionName string) {
		t.Helper()
		ta, ok := s.tracked[workspace][slot]
		if !ok {
			t.Fatalf("missing tracked entry for workspace=%q slot=%d", workspace, slot)
		}
		if ta.agentType != "unknown" {
			t.Fatalf("agentType = %q, want %q", ta.agentType, "unknown")
		}
		if ta.tmuxTarget != agent.TargetForSession(sessionName) {
			t.Fatalf("tmuxTarget = %q, want %q", ta.tmuxTarget, agent.TargetForSession(sessionName))
		}
		if ta.spawnMode != "window" {
			t.Fatalf("spawnMode = %q, want %q", ta.spawnMode, "window")
		}
		if ta.taskMarker != "" {
			t.Fatalf("taskMarker = %q, want empty", ta.taskMarker)
		}
		if ta.responseFence {
			t.Fatalf("responseFence = %v, want false", ta.responseFence)
		}
	}

	checkTracked("mcp-agents", 0, "termtile-mcp-agents-0")
	checkTracked("mcp-agents", 2, "termtile-mcp-agents-2")
	checkTracked("other-ws", 1, "termtile-other-ws-1")

	if got := s.nextSlot["mcp-agents"]; got != 3 {
		t.Fatalf("nextSlot[mcp-agents] = %d, want 3", got)
	}
	if got := s.nextSlot["other-ws"]; got != 2 {
		t.Fatalf("nextSlot[other-ws] = %d, want 2", got)
	}
}
