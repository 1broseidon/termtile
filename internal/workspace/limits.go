package workspace

import (
	"fmt"
	"strings"

	"github.com/1broseidon/termtile/internal/agent"
	"github.com/1broseidon/termtile/internal/config"
)

// ReconcileRegistry removes stale workspace entries from the registry.
// For agent-mode workspaces, it checks if tmux sessions still exist.
// Workspaces with no live sessions are removed; workspaces with fewer
// live sessions than recorded get their count updated.
func ReconcileRegistry() error {
	registry, err := loadRegistry()
	if err != nil {
		return err
	}
	if len(registry.Workspaces) == 0 {
		return nil
	}

	// Get all live tmux sessions in one call
	liveSessions, err := agent.ListSessions()
	if err != nil {
		// tmux not available — can't reconcile agent-mode workspaces
		return nil
	}
	liveSet := make(map[string]struct{}, len(liveSessions))
	for _, s := range liveSessions {
		liveSet[s] = struct{}{}
	}

	changed := false
	for desktop, ws := range registry.Workspaces {
		if !ws.AgentMode {
			continue
		}

		// Count how many of this workspace's tmux sessions are still alive
		liveCount := 0
		var liveSlots []int
		for _, slot := range ws.AgentSlots {
			sessionName := agent.SessionName(ws.Name, slot)
			if _, ok := liveSet[sessionName]; ok {
				liveCount++
				liveSlots = append(liveSlots, slot)
			}
		}

		// Also check for sessions matching the prefix but not in AgentSlots
		// (handles cases where AgentSlots wasn't fully recorded)
		if len(ws.AgentSlots) == 0 {
			prefix := fmt.Sprintf("termtile-%s-", ws.Name)
			for s := range liveSet {
				if strings.HasPrefix(s, prefix) {
					liveCount++
				}
			}
		}

		if liveCount == 0 {
			delete(registry.Workspaces, desktop)
			changed = true
		} else if liveCount != ws.TerminalCount {
			ws.TerminalCount = liveCount
			ws.AgentSlots = liveSlots
			registry.Workspaces[desktop] = ws
			changed = true
		}
	}

	if changed {
		return saveRegistry(registry)
	}
	return nil
}

// CheckCanAddTerminal verifies limits allow adding a terminal to an existing workspace.
func CheckCanAddTerminal(wsName string, currentCount int, cfg *config.Config) error {
	maxForWorkspace := cfg.GetMaxTerminalsForWorkspace(wsName)
	if currentCount >= maxForWorkspace {
		return fmt.Errorf("workspace %q at terminal limit (%d/%d)", wsName, currentCount, maxForWorkspace)
	}

	totalTerminals, err := countAllWorkspaceTerminals()
	if err != nil {
		return err
	}
	maxTotal := cfg.GetMaxTerminalsTotal()
	if totalTerminals >= maxTotal {
		return fmt.Errorf("global terminal limit reached (%d/%d)", totalTerminals, maxTotal)
	}

	return nil
}

// CheckCanCreateWorkspace verifies limits allow creating a new workspace.
func CheckCanCreateWorkspace(cfg *config.Config) error {
	// Reconcile first to remove stale entries
	_ = ReconcileRegistry()

	allWorkspaces, err := GetAllWorkspaces()
	if err != nil {
		return err
	}
	maxWorkspaces := cfg.GetMaxWorkspaces()
	if len(allWorkspaces) >= maxWorkspaces {
		return fmt.Errorf("workspace limit reached (%d/%d)", len(allWorkspaces), maxWorkspaces)
	}
	return nil
}

// CheckCanCreateTerminals verifies bulk terminal creation is allowed.
func CheckCanCreateTerminals(wsName string, count int, cfg *config.Config) error {
	maxForWorkspace := cfg.GetMaxTerminalsForWorkspace(wsName)
	if count > maxForWorkspace {
		return fmt.Errorf("requested %d terminals exceeds workspace limit (%d)", count, maxForWorkspace)
	}

	totalTerminals, err := countAllWorkspaceTerminals()
	if err != nil {
		return err
	}
	maxTotal := cfg.GetMaxTerminalsTotal()
	if totalTerminals+count > maxTotal {
		return fmt.Errorf("would exceed global terminal limit (%d+%d > %d)", totalTerminals, count, maxTotal)
	}

	return nil
}

func countAllWorkspaceTerminals() (int, error) {
	// Reconcile first to remove stale entries
	if err := ReconcileRegistry(); err != nil {
		return 0, fmt.Errorf("registry reconciliation failed: %w", err)
	}

	allWorkspaces, err := GetAllWorkspaces()
	if err != nil {
		return 0, err
	}
	total := 0
	for _, ws := range allWorkspaces {
		total += ws.TerminalCount
	}
	return total, nil
}
