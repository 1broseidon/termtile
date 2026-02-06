package workspace

import (
	"fmt"
	"syscall"
)

// CloseTerminals closes all terminal windows by sending SIGKILL to their processes.
// This ensures a clean close without "are you sure" prompts from terminals.
// For agent-mode workspaces using tmux, the tmux sessions survive the terminal close.
func CloseTerminals(lister TerminalLister) error {
	if lister == nil {
		return fmt.Errorf("terminal lister is nil")
	}

	windows, err := lister.ListTerminals()
	if err != nil {
		return fmt.Errorf("failed to list terminals: %w", err)
	}

	if len(windows) == 0 {
		return nil
	}

	var lastErr error
	for _, win := range windows {
		if win.PID <= 0 {
			continue
		}

		// SIGKILL ensures the terminal closes without prompts.
		// This is safe because:
		// - Agent-mode workspaces have tmux sessions that survive
		// - Non-agent workspaces: user has chosen to close, accepting data loss
		if err := syscall.Kill(win.PID, syscall.SIGKILL); err != nil {
			lastErr = fmt.Errorf("failed to close terminal (PID %d): %w", win.PID, err)
		}
	}

	return lastErr
}
