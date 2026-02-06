package workspace

import (
	"strings"
	"testing"

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
