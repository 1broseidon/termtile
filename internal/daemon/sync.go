package daemon

import (
	"fmt"
	"log/slog"
	"sort"
	"strings"

	"github.com/1broseidon/termtile/internal/agent"
	"github.com/1broseidon/termtile/internal/workspace"
)

// StateSynchronizer handles cleanup when windows close or state drifts.
type StateSynchronizer struct {
	tmux   *agent.TmuxMultiplexer
	logger *slog.Logger
}

// NewStateSynchronizer creates a new state synchronizer.
func NewStateSynchronizer(logger *slog.Logger) *StateSynchronizer {
	return &StateSynchronizer{
		tmux:   agent.NewTmuxMultiplexer(),
		logger: logger,
	}
}

// HandleWindowClosed is called when a tracked window is destroyed.
// It cleans up the orphaned tmux session and updates the registry.
func (s *StateSynchronizer) HandleWindowClosed(windowID uint32) {
	slot, ok := workspace.GetSlotByWindowID(windowID)
	if !ok {
		return // Window not in registry, nothing to do
	}

	s.logger.Info("window closed, cleaning up",
		"window_id", windowID,
		"slot", slot.SlotIndex,
		"session", slot.SessionName,
		"desktop", slot.Desktop)

	// Kill orphaned tmux session if it exists
	if slot.SessionName != "" {
		if exists, _ := s.tmux.HasSession(slot.SessionName); exists {
			if err := s.tmux.KillSession(slot.SessionName); err != nil {
				s.logger.Warn("failed to kill session",
					"session", slot.SessionName,
					"error", err)
			} else {
				s.logger.Debug("killed orphaned session", "session", slot.SessionName)
			}
		}
	}

	// Remove from slot registry
	if err := workspace.RemoveSlotByWindowID(windowID); err != nil {
		s.logger.Warn("failed to remove slot from registry",
			"window_id", windowID,
			"error", err)
	}

	// Renumber remaining slots to close gaps
	if err := s.RenumberSlots(slot.Desktop); err != nil {
		s.logger.Warn("failed to renumber slots",
			"desktop", slot.Desktop,
			"error", err)
	}

	// Update workspace terminal count
	wsInfo, ok := workspace.GetWorkspaceByDesktop(slot.Desktop)
	if ok && wsInfo.TerminalCount > 0 {
		if err := workspace.RemoveTerminalFromWorkspace(slot.Desktop, slot.SlotIndex); err != nil {
			s.logger.Warn("failed to update workspace terminal count",
				"desktop", slot.Desktop,
				"error", err)
		}
	}
}

// RenumberSlots renumbers slots on a desktop to close gaps after removal.
// For example: [0, 2, 3] becomes [0, 1, 2]
func (s *StateSynchronizer) RenumberSlots(desktop int) error {
	slots, err := workspace.GetSlotsByDesktop(desktop)
	if err != nil {
		return err
	}

	if len(slots) == 0 {
		return nil
	}

	// Get workspace info to know the session prefix
	wsInfo, ok := workspace.GetWorkspaceByDesktop(desktop)
	if !ok || !wsInfo.AgentMode {
		// Not agent mode, nothing to rename
		return nil
	}

	// Sort by current slot index
	sort.Slice(slots, func(i, j int) bool {
		return slots[i].SlotIndex < slots[j].SlotIndex
	})

	// Renumber to close gaps
	for newIndex, slot := range slots {
		if slot.SlotIndex == newIndex {
			continue // Already correct
		}

		oldName := slot.SessionName
		newName := agent.SessionName(wsInfo.Name, newIndex)

		s.logger.Debug("renumbering slot",
			"window_id", slot.WindowID,
			"old_index", slot.SlotIndex,
			"new_index", newIndex,
			"old_session", oldName,
			"new_session", newName)

		// Rename tmux session if it exists
		if oldName != "" && oldName != newName {
			if exists, _ := s.tmux.HasSession(oldName); exists {
				if err := s.tmux.RenameSession(oldName, newName); err != nil {
					return fmt.Errorf("rename session %s -> %s: %w", oldName, newName, err)
				}
			}
		}

		// Update slot registry
		if err := workspace.UpdateSlotIndex(slot.WindowID, newIndex, newName); err != nil {
			return fmt.Errorf("update slot index for window %d: %w", slot.WindowID, err)
		}
	}

	return nil
}

// CleanupOrphanedSessions removes tmux sessions that don't have corresponding windows.
// Only performs cleanup if there are slots registered - otherwise we have no tracking
// data and would incorrectly kill all sessions.
func (s *StateSynchronizer) CleanupOrphanedSessions() error {
	// Check if we have any slots registered - if not, skip cleanup
	// since we don't have tracking data yet
	allSlots, err := workspace.GetAllSlots()
	if err != nil {
		return fmt.Errorf("get slots: %w", err)
	}
	if len(allSlots) == 0 {
		// No slots tracked, skip orphan cleanup to avoid killing valid sessions
		return nil
	}

	sessions, err := s.tmux.ListSessions()
	if err != nil {
		return fmt.Errorf("list sessions: %w", err)
	}

	for _, session := range sessions {
		// Only process termtile sessions
		if !strings.HasPrefix(session, "termtile-") {
			continue
		}

		if !workspace.HasSessionInRegistry(session) {
			s.logger.Info("killing orphaned tmux session", "session", session)
			if err := s.tmux.KillSession(session); err != nil {
				s.logger.Warn("failed to kill orphaned session",
					"session", session,
					"error", err)
			}
		}
	}

	return nil
}
