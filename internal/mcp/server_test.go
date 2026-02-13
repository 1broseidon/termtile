package mcp

import (
	"os"
	"path/filepath"
	"strings"
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
	s.allocateSlot("test-ws", "claude", "%5", "pane", false)
	if got := s.peekNextSlot("test-ws"); got != 1 {
		t.Errorf("peekNextSlot after allocate = %d, want 1", got)
	}

	// Allocate in window mode.
	s.allocateSlot("test-ws", "codex", "termtile-test-ws-1:0.0", "window", false)
	if got := s.peekNextSlot("test-ws"); got != 2 {
		t.Errorf("peekNextSlot after window allocate = %d, want 2", got)
	}

	// After removing slot 0, the next available slot should be reused.
	s.removeTracked("test-ws", 0)
	if got := s.peekNextSlot("test-ws"); got != 0 {
		t.Errorf("peekNextSlot after removing slot 0 = %d, want 0", got)
	}
}

func TestGetSpawnMode(t *testing.T) {
	s := &Server{
		config:   config.DefaultConfig(),
		tracked:  make(map[string]map[int]trackedAgent),
		nextSlot: make(map[string]int),
	}

	s.allocateSlot("ws", "claude", "%5", "pane", false)
	s.allocateSlot("ws", "codex", "termtile-ws-1:0.0", "window", false)

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

func TestUpdateFenceState(t *testing.T) {
	s := &Server{
		config:   config.DefaultConfig(),
		tracked:  make(map[string]map[int]trackedAgent),
		nextSlot: make(map[string]int),
	}

	slot := s.allocateSlot("ws", "claude", "%5", "pane", false)
	s.updateFenceState("ws", slot, true, 3)

	fence, count := s.getFenceState("ws", slot)
	if !fence {
		t.Fatal("responseFence = false, want true")
	}
	if count != 3 {
		t.Fatalf("fencePairCount = %d, want 3", count)
	}

	// Non-existent slot should be a no-op.
	s.updateFenceState("ws", 999, false, 0)
	if fence2, count2 := s.getFenceState("ws", slot); !fence2 || count2 != 3 {
		t.Fatalf("after no-op update: fence=%v count=%d, want true/3", fence2, count2)
	}
}

func TestUpdateTmuxTarget(t *testing.T) {
	s := &Server{
		config:   config.DefaultConfig(),
		tracked:  make(map[string]map[int]trackedAgent),
		nextSlot: make(map[string]int),
	}

	slot := s.allocateSlot("ws", "claude", "", "window", false)
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

func TestPipeStateAccessors(t *testing.T) {
	s := &Server{
		config:   config.DefaultConfig(),
		tracked:  make(map[string]map[int]trackedAgent),
		nextSlot: make(map[string]int),
	}

	slot := s.allocateSlot("ws", "codex", "termtile-ws-0:0.0", "window", true)

	// Initially empty.
	path, size := s.getPipeState("ws", slot)
	if path != "" {
		t.Fatalf("initial pipe path = %q, want empty", path)
	}
	if size != 0 {
		t.Fatalf("initial pipe size = %d, want 0", size)
	}

	// Set pipe state.
	s.setPipeState("ws", slot, "/tmp/test-pipe.raw")
	path, size = s.getPipeState("ws", slot)
	if path != "/tmp/test-pipe.raw" {
		t.Fatalf("pipe path = %q, want %q", path, "/tmp/test-pipe.raw")
	}
	if size != 0 {
		t.Fatalf("pipe size after setPipeState = %d, want 0", size)
	}

	// Update last pipe size.
	s.updateLastPipeSize("ws", slot, 1024)
	_, size = s.getPipeState("ws", slot)
	if size != 1024 {
		t.Fatalf("pipe size after update = %d, want 1024", size)
	}

	// Non-existent slot/workspace should be no-ops.
	s.setPipeState("ws", 999, "/tmp/ignored.raw")
	s.updateLastPipeSize("ws", 999, 500)
	s.setPipeState("nonexistent", 0, "/tmp/ignored.raw")
	path2, size2 := s.getPipeState("nonexistent", 0)
	if path2 != "" || size2 != 0 {
		t.Fatalf("nonexistent workspace: path=%q size=%d, want empty/0", path2, size2)
	}
}

func TestMoveTerminalTracking(t *testing.T) {
	s := &Server{
		config:      config.DefaultConfig(),
		multiplexer: agent.NewTmuxMultiplexer(),
		tracked:     make(map[string]map[int]trackedAgent),
		nextSlot:    make(map[string]int),
	}

	// Allocate two slots in source workspace.
	s.allocateSlot("src-ws", "claude", "termtile-src-ws-0:0.0", "window", false)
	s.allocateSlot("src-ws", "codex", "termtile-src-ws-1:0.0", "window", true)

	// Set up some state on slot 1.
	s.updateFenceState("src-ws", 1, true, 5)
	s.setPipeState("src-ws", 1, "/tmp/test-pipe.raw")

	// Simulate moving slot 1 from src-ws to dst-ws at slot 0.
	// (This tests the tracking transfer logic without needing X11/tmux.)
	s.mu.Lock()
	var ta trackedAgent
	if srcMap, ok := s.tracked["src-ws"]; ok {
		ta = srcMap[1]
		delete(srcMap, 1)
	}
	ta.tmuxTarget = "termtile-dst-ws-0:0.0"
	if s.tracked["dst-ws"] == nil {
		s.tracked["dst-ws"] = make(map[int]trackedAgent)
	}
	s.tracked["dst-ws"][0] = ta
	s.nextSlot["dst-ws"] = 1
	s.mu.Unlock()

	// Verify source slot 1 is gone.
	if _, ok := s.getTmuxTarget("src-ws", 1); ok {
		t.Fatal("source slot 1 should be removed after move")
	}

	// Verify source slot 0 is still there.
	if target, ok := s.getTmuxTarget("src-ws", 0); !ok || target != "termtile-src-ws-0:0.0" {
		t.Fatalf("source slot 0 should still exist, got target=%q ok=%v", target, ok)
	}

	// Verify destination slot 0 has the moved agent.
	target, ok := s.getTmuxTarget("dst-ws", 0)
	if !ok {
		t.Fatal("destination slot 0 should exist after move")
	}
	if target != "termtile-dst-ws-0:0.0" {
		t.Fatalf("destination target = %q, want %q", target, "termtile-dst-ws-0:0.0")
	}

	// Verify agent type transferred.
	if agentType := s.getAgentType("dst-ws", 0); agentType != "codex" {
		t.Fatalf("destination agent type = %q, want %q", agentType, "codex")
	}

	// Verify spawn mode transferred.
	if mode := s.getSpawnMode("dst-ws", 0); mode != "window" {
		t.Fatalf("destination spawn mode = %q, want %q", mode, "window")
	}

	// Verify fence state transferred.
	hasFence, count := s.getFenceState("dst-ws", 0)
	if !hasFence {
		t.Fatal("destination fence should be true")
	}
	if count != 5 {
		t.Fatalf("destination fence count = %d, want 5", count)
	}

	// Verify pipe state transferred.
	pipePath, _ := s.getPipeState("dst-ws", 0)
	if pipePath != "/tmp/test-pipe.raw" {
		t.Fatalf("destination pipe path = %q, want %q", pipePath, "/tmp/test-pipe.raw")
	}
}

func TestHandleKillAgent_SlotZeroProtected(t *testing.T) {
	cfg := config.DefaultConfig()
	// Default config has protect_slot_zero = nil (defaults true via getter).

	s := &Server{
		config:   cfg,
		tracked:  make(map[string]map[int]trackedAgent),
		nextSlot: make(map[string]int),
	}

	// Allocate slot 0 in the default MCP workspace.
	s.allocateSlot(DefaultWorkspace, "claude", "termtile-mcp-agents-0:0.0", "window", false)

	// Try to kill slot 0 — should be rejected.
	_, _, err := s.handleKillAgent(nil, nil, KillAgentInput{Slot: 0, Workspace: DefaultWorkspace})
	if err == nil {
		t.Fatal("expected error killing protected slot 0")
	}
	if got := err.Error(); !containsAll(got, "slot 0", "protected") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestHandleKillAgent_SlotZeroAllowedWhenDisabled(t *testing.T) {
	cfg := config.DefaultConfig()
	f := false
	cfg.AgentMode.ProtectSlotZero = &f

	s := &Server{
		config:   cfg,
		tracked:  make(map[string]map[int]trackedAgent),
		nextSlot: make(map[string]int),
	}

	// Allocate slot 0.
	s.allocateSlot(DefaultWorkspace, "claude", "termtile-mcp-agents-0:0.0", "window", false)

	// With protection disabled, kill should proceed (will fail on tmux
	// command since we don't have a real session, but should NOT fail on
	// the slot-zero guard).
	_, out, err := s.handleKillAgent(nil, nil, KillAgentInput{Slot: 0, Workspace: DefaultWorkspace})
	// The kill succeeds in tracking terms (tmux kill-session runs best-effort).
	if err != nil {
		t.Fatalf("expected no error with protection disabled, got: %v", err)
	}
	if !out.Killed {
		t.Fatal("expected Killed=true with protection disabled")
	}
}

func TestHandleKillAgent_NonZeroSlotNotProtected(t *testing.T) {
	cfg := config.DefaultConfig()
	// Default protection is on, but only slot 0 is protected.

	s := &Server{
		config:   cfg,
		tracked:  make(map[string]map[int]trackedAgent),
		nextSlot: make(map[string]int),
	}

	// Allocate slots 0 and 1.
	s.allocateSlot(DefaultWorkspace, "claude", "termtile-mcp-agents-0:0.0", "window", false)
	s.allocateSlot(DefaultWorkspace, "codex", "termtile-mcp-agents-1:0.0", "window", false)

	// Killing slot 1 should work.
	_, out, err := s.handleKillAgent(nil, nil, KillAgentInput{Slot: 1, Workspace: DefaultWorkspace})
	if err != nil {
		t.Fatalf("expected no error killing slot 1, got: %v", err)
	}
	if !out.Killed {
		t.Fatal("expected Killed=true for slot 1")
	}
}

func TestHandleKillAgent_CleansArtifactDirectory(t *testing.T) {
	cfg := config.DefaultConfig()
	allow := false
	cfg.AgentMode.ProtectSlotZero = &allow

	s := &Server{
		config:   cfg,
		tracked:  make(map[string]map[int]trackedAgent),
		nextSlot: make(map[string]int),
	}

	base := t.TempDir()
	t.Setenv("XDG_DATA_HOME", base)

	slot := s.allocateSlot(DefaultWorkspace, "codex", "%42", "pane", false)
	dir, err := EnsureArtifactDir(DefaultWorkspace, slot)
	if err != nil {
		t.Fatalf("EnsureArtifactDir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "output.json"), []byte("artifact"), 0o644); err != nil {
		t.Fatalf("failed to write artifact file: %v", err)
	}

	_, out, err := s.handleKillAgent(nil, nil, KillAgentInput{Slot: slot, Workspace: DefaultWorkspace})
	if err != nil {
		t.Fatalf("handleKillAgent: %v", err)
	}
	if !out.Killed {
		t.Fatal("expected Killed=true")
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Fatalf("expected artifact dir to be removed, stat err=%v", err)
	}
}

// containsAll checks if s contains all the given substrings.
func containsAll(s string, subs ...string) bool {
	for _, sub := range subs {
		if !strings.Contains(s, sub) {
			return false
		}
	}
	return true
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
		if ta.responseFence {
			t.Fatalf("responseFence = %v, want false", ta.responseFence)
		}
	}

	checkTracked("mcp-agents", 0, "termtile-mcp-agents-0")
	checkTracked("mcp-agents", 2, "termtile-mcp-agents-2")
	checkTracked("other-ws", 1, "termtile-other-ws-1")
}

func TestTrackSpecificSlot_Collision(t *testing.T) {
	s := &Server{
		config:   config.DefaultConfig(),
		tracked:  make(map[string]map[int]trackedAgent),
		nextSlot: make(map[string]int),
	}

	if err := s.trackSpecificSlot("ws", 0, "claude", "termtile-ws-0:0.0", "window", false); err != nil {
		t.Fatalf("trackSpecificSlot initial: %v", err)
	}
	if err := s.trackSpecificSlot("ws", 0, "codex", "termtile-ws-0:0.0", "window", false); err == nil {
		t.Fatal("expected collision error for already tracked slot")
	}
}

func TestCompactWindowSlots_ShiftsTrackingState(t *testing.T) {
	s := &Server{
		config:        config.DefaultConfig(),
		multiplexer:   agent.NewTmuxMultiplexer(),
		tracked:       make(map[string]map[int]trackedAgent),
		nextSlot:      make(map[string]int),
		readSnapshots: make(map[string]map[int]string),
	}
	base := t.TempDir()
	t.Setenv("XDG_DATA_HOME", base)

	_ = s.trackSpecificSlot("ws", 0, "claude", "termtile-ws-0:0.0", "window", false)
	_ = s.trackSpecificSlot("ws", 2, "codex", "termtile-ws-2:0.0", "window", false)
	_ = s.trackSpecificSlot("ws", 3, "gemini", "termtile-ws-3:0.0", "window", false)
	writeHookArtifactForTest(t, "ws", 2, "artifact-2")
	writeHookArtifactForTest(t, "ws", 3, "artifact-3")
	s.setReadSnapshot("ws", 2, "snap-2")
	s.setReadSnapshot("ws", 3, "snap-3")

	if err := s.compactWindowSlots("ws", 1); err != nil {
		t.Fatalf("compactWindowSlots: %v", err)
	}

	if _, ok := s.getTmuxTarget("ws", 3); ok {
		t.Fatal("slot 3 should not exist after compaction")
	}
	if target, ok := s.getTmuxTarget("ws", 1); !ok || target != "termtile-ws-1:0.0" {
		t.Fatalf("slot 1 target = %q (ok=%v), want termtile-ws-1:0.0", target, ok)
	}
	if target, ok := s.getTmuxTarget("ws", 2); !ok || target != "termtile-ws-2:0.0" {
		t.Fatalf("slot 2 target = %q (ok=%v), want termtile-ws-2:0.0", target, ok)
	}
	if output, err := readArtifactOutputField("ws", 1); err != nil || output != "artifact-2" {
		t.Fatalf("artifact slot 1 = %q (err=%v), want artifact-2", output, err)
	}
	if output, err := readArtifactOutputField("ws", 2); err != nil || output != "artifact-3" {
		t.Fatalf("artifact slot 2 = %q (err=%v), want artifact-3", output, err)
	}
	if got := s.getReadSnapshot("ws", 1); got != "snap-2" {
		t.Fatalf("read snapshot slot 1 = %q, want snap-2", got)
	}
	if got := s.getReadSnapshot("ws", 2); got != "snap-3" {
		t.Fatalf("read snapshot slot 2 = %q, want snap-3", got)
	}
}
