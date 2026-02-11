package workspace

import (
	"strings"
	"testing"
	"time"

	"github.com/1broseidon/termtile/internal/config"
)

func TestWorkspaceLimits_Checks(t *testing.T) {
	t.Setenv("XDG_RUNTIME_DIR", t.TempDir())

	cfg := config.DefaultConfig()
	cfg.Limits.MaxWorkspaces = 2
	cfg.Limits.MaxTerminalsTotal = 2
	cfg.Limits.MaxTerminalsPerWorkspace = 10

	if err := CheckCanCreateWorkspace(cfg); err != nil {
		t.Fatalf("expected create workspace to pass, got %v", err)
	}

	if err := SetActiveWorkspace("ws1", 1, false, 0, nil); err != nil {
		t.Fatalf("set active workspace: %v", err)
	}
	if err := SetActiveWorkspace("ws2", 1, false, 1, nil); err != nil {
		t.Fatalf("set active workspace: %v", err)
	}

	// At max workspaces.
	if err := CheckCanCreateWorkspace(cfg); err == nil {
		t.Fatalf("expected workspace limit error")
	}

	// At max terminals total.
	if err := CheckCanAddTerminal("ws1", 1, cfg); err == nil {
		t.Fatalf("expected global terminal limit error")
	} else if !strings.Contains(err.Error(), "global terminal limit") {
		t.Fatalf("expected global limit error, got %v", err)
	}

	// Workspace-specific override.
	cfg.Limits.WorkspaceOverrides = map[string]config.WorkspaceLimit{
		"ws1": {MaxTerminals: 1},
	}
	if err := CheckCanAddTerminal("ws1", 1, cfg); err == nil {
		t.Fatalf("expected per-workspace terminal limit error")
	} else if !strings.Contains(err.Error(), "workspace \"ws1\" at terminal limit") {
		t.Fatalf("expected per-workspace limit error, got %v", err)
	}
}

func TestReconcileRegistry_RemovesStaleAgentWorkspace(t *testing.T) {
	t.Setenv("XDG_RUNTIME_DIR", t.TempDir())

	// Register an agent-mode workspace with 18 terminals — none of which
	// have live tmux sessions (we're in a test environment).
	slots := make([]int, 18)
	for i := range slots {
		slots[i] = i
	}
	if err := SetActiveWorkspace("stale-agents", 18, true, 0, slots); err != nil {
		t.Fatalf("set active workspace: %v", err)
	}

	// Before reconciliation the registry should have the workspace.
	all, err := GetAllWorkspaces()
	if err != nil {
		t.Fatalf("get all workspaces: %v", err)
	}
	if len(all) != 1 {
		t.Fatalf("expected 1 workspace before reconcile, got %d", len(all))
	}

	// Reconcile — should remove the stale workspace since no tmux sessions exist.
	if err := ReconcileRegistry(); err != nil {
		t.Fatalf("reconcile: %v", err)
	}

	all, err = GetAllWorkspaces()
	if err != nil {
		t.Fatalf("get all workspaces: %v", err)
	}
	if len(all) != 0 {
		t.Fatalf("expected 0 workspaces after reconcile, got %d", len(all))
	}
}

func TestReconcileRegistry_KeepsNonAgentWorkspace(t *testing.T) {
	t.Setenv("XDG_RUNTIME_DIR", t.TempDir())

	// Register a non-agent-mode workspace — reconciliation should leave it alone.
	if err := SetActiveWorkspace("regular-ws", 3, false, 0, nil); err != nil {
		t.Fatalf("set active workspace: %v", err)
	}

	if err := ReconcileRegistry(); err != nil {
		t.Fatalf("reconcile: %v", err)
	}

	all, err := GetAllWorkspaces()
	if err != nil {
		t.Fatalf("get all workspaces: %v", err)
	}
	if len(all) != 1 {
		t.Fatalf("expected 1 workspace after reconcile, got %d", len(all))
	}
	if all[0].TerminalCount != 3 {
		t.Fatalf("expected terminal count 3, got %d", all[0].TerminalCount)
	}
}

func TestReconcileRegistry_UnblocksLimitCheck(t *testing.T) {
	t.Setenv("XDG_RUNTIME_DIR", t.TempDir())

	// Simulate the exact bug: stale agent workspace with 18 terminals blocks new creation.
	slots := make([]int, 18)
	for i := range slots {
		slots[i] = i
	}
	if err := SetActiveWorkspace("my-agents", 18, true, 0, slots); err != nil {
		t.Fatalf("set active workspace: %v", err)
	}

	cfg := config.DefaultConfig()
	cfg.Limits.MaxTerminalsTotal = 20

	// Without reconciliation this would fail with "would exceed global terminal limit (18+3 > 20)".
	// With reconciliation, the stale workspace is removed and the check passes.
	if err := CheckCanCreateTerminals("new-ws", 3, cfg); err != nil {
		t.Fatalf("expected limit check to pass after reconciliation, got: %v", err)
	}
}

func TestReconcileRegistry_EmptyRegistry(t *testing.T) {
	t.Setenv("XDG_RUNTIME_DIR", t.TempDir())

	// Reconciling an empty registry should be a no-op.
	if err := ReconcileRegistry(); err != nil {
		t.Fatalf("reconcile empty registry: %v", err)
	}
}

func TestMoveTerminalBetweenWorkspaces(t *testing.T) {
	t.Setenv("XDG_RUNTIME_DIR", t.TempDir())

	// Set up two workspaces on different desktops.
	if err := SetActiveWorkspace("src-ws", 3, true, 0, []int{0, 1, 2}); err != nil {
		t.Fatalf("set source workspace: %v", err)
	}
	if err := SetActiveWorkspace("dst-ws", 1, true, 1, []int{0}); err != nil {
		t.Fatalf("set dest workspace: %v", err)
	}

	// Move slot 1 from src to dst.
	newSlot, err := MoveTerminalBetweenWorkspaces(0, 1, 1)
	if err != nil {
		t.Fatalf("MoveTerminalBetweenWorkspaces: %v", err)
	}
	if newSlot != 1 {
		t.Fatalf("expected new slot 1, got %d", newSlot)
	}

	// Verify source workspace updated.
	srcInfo, ok := GetWorkspaceByDesktop(0)
	if !ok {
		t.Fatal("source workspace not found after move")
	}
	if srcInfo.TerminalCount != 2 {
		t.Fatalf("source terminal count = %d, want 2", srcInfo.TerminalCount)
	}
	// Slot 1 removed, slot 2 shifted to 1: expect [0, 1]
	wantSrcSlots := []int{0, 1}
	if len(srcInfo.AgentSlots) != len(wantSrcSlots) {
		t.Fatalf("source agent slots = %v, want %v", srcInfo.AgentSlots, wantSrcSlots)
	}
	for i, s := range srcInfo.AgentSlots {
		if s != wantSrcSlots[i] {
			t.Fatalf("source agent slots = %v, want %v", srcInfo.AgentSlots, wantSrcSlots)
		}
	}

	// Verify destination workspace updated.
	dstInfo, ok := GetWorkspaceByDesktop(1)
	if !ok {
		t.Fatal("dest workspace not found after move")
	}
	if dstInfo.TerminalCount != 2 {
		t.Fatalf("dest terminal count = %d, want 2", dstInfo.TerminalCount)
	}
	wantDstSlots := []int{0, 1}
	if len(dstInfo.AgentSlots) != len(wantDstSlots) {
		t.Fatalf("dest agent slots = %v, want %v", dstInfo.AgentSlots, wantDstSlots)
	}
	for i, s := range dstInfo.AgentSlots {
		if s != wantDstSlots[i] {
			t.Fatalf("dest agent slots = %v, want %v", dstInfo.AgentSlots, wantDstSlots)
		}
	}
}

func TestMoveTerminalBetweenWorkspaces_InvalidSlot(t *testing.T) {
	t.Setenv("XDG_RUNTIME_DIR", t.TempDir())

	if err := SetActiveWorkspace("ws-a", 2, true, 0, []int{0, 1}); err != nil {
		t.Fatalf("set workspace: %v", err)
	}
	if err := SetActiveWorkspace("ws-b", 1, true, 1, []int{0}); err != nil {
		t.Fatalf("set workspace: %v", err)
	}

	// Out of range slot.
	if _, err := MoveTerminalBetweenWorkspaces(0, 5, 1); err == nil {
		t.Fatal("expected error for out-of-range slot")
	}

	// Non-existent source desktop.
	if _, err := MoveTerminalBetweenWorkspaces(99, 0, 1); err == nil {
		t.Fatal("expected error for non-existent source desktop")
	}

	// Non-existent destination desktop.
	if _, err := MoveTerminalBetweenWorkspaces(0, 0, 99); err == nil {
		t.Fatal("expected error for non-existent dest desktop")
	}
}

func TestReconcileRegistry_PreservesOpenedAt(t *testing.T) {
	t.Setenv("XDG_RUNTIME_DIR", t.TempDir())

	// Non-agent workspace should be untouched, including its OpenedAt.
	if err := SetActiveWorkspace("keep-me", 2, false, 1, nil); err != nil {
		t.Fatalf("set active workspace: %v", err)
	}

	all, _ := GetAllWorkspaces()
	before := all[1].OpenedAt

	time.Sleep(10 * time.Millisecond)
	if err := ReconcileRegistry(); err != nil {
		t.Fatalf("reconcile: %v", err)
	}

	all, _ = GetAllWorkspaces()
	if !all[1].OpenedAt.Equal(before) {
		t.Fatalf("OpenedAt changed: %v != %v", all[1].OpenedAt, before)
	}
}
