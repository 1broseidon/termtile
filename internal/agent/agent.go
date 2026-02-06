package agent

import (
	"fmt"
	"strings"
	"unicode"
)

// SessionName returns the tmux session name for a workspace and slot.
func SessionName(workspaceName string, slot int) string {
	return fmt.Sprintf("termtile-%s-%d", sanitizeSessionComponent(workspaceName), slot)
}

// TargetForSession returns the tmux target for a session (session:window.pane).
func TargetForSession(session string) string {
	return session + ":0.0"
}

// WorkspaceInfo contains the information needed to resolve an agent session.
// This is passed in from the workspace package to avoid import cycles.
type WorkspaceInfo struct {
	Name       string
	AgentMode  bool
	AgentSlots []int
}

// ResolveSession resolves the tmux session name for a given workspace and slot.
// If workspaceOverride is provided, it uses that workspace name directly.
// Otherwise, it uses the provided workspace info (typically from the current desktop).
func ResolveSession(workspaceOverride string, slot int, wsInfo *WorkspaceInfo) (string, error) {
	if slot < 0 {
		return "", fmt.Errorf("--slot is required")
	}

	// If explicit workspace provided, use it directly
	workspaceOverride = strings.TrimSpace(workspaceOverride)
	if workspaceOverride != "" {
		return SessionName(workspaceOverride, slot), nil
	}

	// Use provided workspace info
	if wsInfo == nil {
		return "", fmt.Errorf("no workspace on current desktop")
	}
	if wsInfo.Name == "" {
		return "", fmt.Errorf("no workspace on current desktop")
	}
	if !wsInfo.AgentMode {
		return "", fmt.Errorf("workspace %q is not an agent-mode workspace", wsInfo.Name)
	}

	// Verify slot is valid for this workspace
	validSlot := false
	for _, s := range wsInfo.AgentSlots {
		if s == slot {
			validSlot = true
			break
		}
	}
	if !validSlot {
		return "", fmt.Errorf("slot %d not registered for workspace %q (valid slots: %v)",
			slot, wsInfo.Name, wsInfo.AgentSlots)
	}

	return SessionName(wsInfo.Name, slot), nil
}

// sanitizeSessionComponent cleans a string to be safe for use in a tmux session name.
func sanitizeSessionComponent(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return "workspace"
	}
	var b strings.Builder
	b.Grow(len(s))
	lastUnderscore := false
	for _, r := range s {
		ok := unicode.IsLetter(r) || unicode.IsDigit(r) || r == '-' || r == '_' || r == '.'
		if ok {
			b.WriteRune(r)
			lastUnderscore = false
			continue
		}
		if !lastUnderscore {
			b.WriteByte('_')
			lastUnderscore = true
		}
	}
	out := strings.Trim(b.String(), "_")
	if out == "" {
		return "workspace"
	}
	return out
}
