package workspace

import (
	"fmt"

	"github.com/1broseidon/termtile/internal/config"
)

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
