package mcp

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/1broseidon/termtile/internal/agent"
	"github.com/1broseidon/termtile/internal/config"
)

const (
	hookBackupFileName = "hook_backup.json"
	hookStateFileName  = "hook_state.json"
	agentMetaFileName  = "agent_meta.json"
	contextFileName    = "context.md"
)

// agentMeta is written to the artifact dir at spawn time so the hook CLI
// can look up agent-specific config (hook_output template, response field, etc.).
type agentMeta struct {
	AgentType string `json:"agent_type"`
}

// writeAgentMeta persists the agent type to the artifact directory.
func writeAgentMeta(workspace string, slot int, agentType string) error {
	artifactDir, err := EnsureArtifactDir(workspace, slot)
	if err != nil {
		return fmt.Errorf("failed to ensure artifact dir for agent meta: %w", err)
	}
	meta := agentMeta{AgentType: agentType}
	data, err := json.Marshal(meta)
	if err != nil {
		return fmt.Errorf("failed to marshal agent meta: %w", err)
	}
	metaPath := filepath.Join(artifactDir, agentMetaFileName)
	if err := os.WriteFile(metaPath, data, 0644); err != nil {
		return fmt.Errorf("failed to write agent meta: %w", err)
	}
	return nil
}

// ReadAgentMeta reads the agent type from the artifact directory.
// Exported so the hook CLI (cmd/termtile) can use it.
func ReadAgentMeta(workspace string, slot int) (string, error) {
	artifactDir, err := GetArtifactDir(workspace, slot)
	if err != nil {
		return "", err
	}
	data, err := os.ReadFile(filepath.Join(artifactDir, agentMetaFileName))
	if err != nil {
		return "", err
	}
	var meta agentMeta
	if err := json.Unmarshal(data, &meta); err != nil {
		return "", err
	}
	return meta.AgentType, nil
}

// writeTaskContext writes the task to context.md in the artifact directory so
// the on_start hook can inject it as context when the agent starts.
func writeTaskContext(workspace string, slot int, task string) error {
	artifactDir, err := EnsureArtifactDir(workspace, slot)
	if err != nil {
		return fmt.Errorf("failed to ensure artifact dir for context: %w", err)
	}
	contextPath := filepath.Join(artifactDir, contextFileName)
	if err := os.WriteFile(contextPath, []byte(task), 0644); err != nil {
		return fmt.Errorf("failed to write task context: %w", err)
	}
	return nil
}

// hookFileState records the state of a project file injection so it can be
// reversed when the agent is killed or the server reconciles.
type hookFileState struct {
	OriginalPath string `json:"original_path"` // path to the project settings file
	BackupPath   string `json:"backup_path"`   // path to the backup in artifact dir
	HadOriginal  bool   `json:"had_original"`  // true if the file existed before injection
}

// injectProjectFileHooks writes the rendered hook settings into the agent's
// project config file (e.g. .gemini/settings.json). It backs up any existing
// file, deep-merges the hooks, and writes a state file for crash recovery.
func injectProjectFileHooks(workspace string, slot int, cwd string, agentCfg config.AgentConfig, settings string) (*hookFileState, error) {
	dir := strings.TrimSpace(agentCfg.HookSettingsDir)
	file := strings.TrimSpace(agentCfg.HookSettingsFile)
	if dir == "" || file == "" {
		return nil, fmt.Errorf("hook_settings_dir and hook_settings_file must be set for project_file delivery")
	}

	settingsDir := filepath.Join(cwd, dir)
	settingsPath := filepath.Join(settingsDir, file)

	// Ensure artifact directory exists for backup/state storage.
	artifactDir, err := EnsureArtifactDir(workspace, slot)
	if err != nil {
		return nil, fmt.Errorf("failed to ensure artifact dir: %w", err)
	}

	// Read existing settings file if present.
	var existingData []byte
	hadOriginal := false
	if data, err := os.ReadFile(settingsPath); err == nil {
		existingData = data
		hadOriginal = true
	} else if !os.IsNotExist(err) {
		return nil, fmt.Errorf("failed to read existing settings file %q: %w", settingsPath, err)
	}

	// Back up the original.
	backupPath := filepath.Join(artifactDir, hookBackupFileName)
	if hadOriginal {
		if err := os.WriteFile(backupPath, existingData, 0o644); err != nil {
			return nil, fmt.Errorf("failed to back up settings file: %w", err)
		}
	}

	// Parse rendered hook settings.
	var overlay map[string]interface{}
	if err := json.Unmarshal([]byte(settings), &overlay); err != nil {
		return nil, fmt.Errorf("failed to parse rendered hook settings: %w", err)
	}

	// Deep-merge hooks into existing config (or use overlay directly).
	var merged map[string]interface{}
	if hadOriginal && len(existingData) > 0 {
		var existing map[string]interface{}
		if err := json.Unmarshal(existingData, &existing); err != nil {
			// Existing file is not valid JSON — overwrite entirely.
			merged = overlay
		} else {
			merged = deepMergeMap(existing, overlay)
		}
	} else {
		merged = overlay
	}

	// Write merged config.
	mergedData, err := json.MarshalIndent(merged, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("failed to marshal merged settings: %w", err)
	}
	if err := os.MkdirAll(settingsDir, 0o755); err != nil {
		return nil, fmt.Errorf("failed to create settings dir %q: %w", settingsDir, err)
	}
	if err := os.WriteFile(settingsPath, mergedData, 0o644); err != nil {
		return nil, fmt.Errorf("failed to write merged settings file %q: %w", settingsPath, err)
	}

	// Write state file for crash recovery.
	state := hookFileState{
		OriginalPath: settingsPath,
		BackupPath:   backupPath,
		HadOriginal:  hadOriginal,
	}
	stateData, err := json.Marshal(state)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal hook state: %w", err)
	}
	statePath := filepath.Join(artifactDir, hookStateFileName)
	if err := os.WriteFile(statePath, stateData, 0o644); err != nil {
		return nil, fmt.Errorf("failed to write hook state file: %w", err)
	}

	return &state, nil
}

// restoreProjectFileHooks reverses a project file injection by reading the
// hook_state.json from the artifact directory and restoring the original file.
func restoreProjectFileHooks(workspace string, slot int) error {
	artifactDir, err := GetArtifactDir(workspace, slot)
	if err != nil {
		return err
	}

	statePath := filepath.Join(artifactDir, hookStateFileName)
	stateData, err := os.ReadFile(statePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil // no state file — nothing to restore
		}
		return fmt.Errorf("failed to read hook state file: %w", err)
	}

	var state hookFileState
	if err := json.Unmarshal(stateData, &state); err != nil {
		return fmt.Errorf("failed to parse hook state file: %w", err)
	}

	if state.HadOriginal {
		// Restore from backup.
		backupData, err := os.ReadFile(state.BackupPath)
		if err != nil {
			return fmt.Errorf("failed to read backup file %q: %w", state.BackupPath, err)
		}
		if err := os.WriteFile(state.OriginalPath, backupData, 0o644); err != nil {
			return fmt.Errorf("failed to restore settings file %q: %w", state.OriginalPath, err)
		}
	} else {
		// No original existed — remove the injected file.
		if err := os.Remove(state.OriginalPath); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("failed to remove injected settings file %q: %w", state.OriginalPath, err)
		}
		// Try to remove the empty directory (best-effort).
		dir := filepath.Dir(state.OriginalPath)
		_ = os.Remove(dir)
	}

	// Clean up state and backup files.
	_ = os.Remove(statePath)
	_ = os.Remove(state.BackupPath)

	return nil
}

// reconcileHookFileState scans artifact directories for orphaned hook_state.json
// files whose tmux sessions no longer exist, and restores the original files.
func reconcileHookFileState(liveSessions map[string]bool) {
	baseDir, err := artifactBaseDir()
	if err != nil {
		return
	}

	// Walk workspace directories.
	workspaceDirs, err := os.ReadDir(baseDir)
	if err != nil {
		return
	}

	for _, wsDir := range workspaceDirs {
		if !wsDir.IsDir() {
			continue
		}
		workspace := wsDir.Name()
		wsPath := filepath.Join(baseDir, workspace)
		slotDirs, err := os.ReadDir(wsPath)
		if err != nil {
			continue
		}

		for _, slotDir := range slotDirs {
			if !slotDir.IsDir() {
				continue
			}
			statePath := filepath.Join(wsPath, slotDir.Name(), hookStateFileName)
			if _, err := os.Stat(statePath); os.IsNotExist(err) {
				continue
			}

			// Check if the tmux session is still alive.
			slotName := slotDir.Name()
			sessionName := agent.SessionName(workspace, parseSlotIndex(slotName))
			if liveSessions[sessionName] {
				continue
			}

			// Session is dead — restore the project file.
			slot := parseSlotIndex(slotName)
			if slot < 0 {
				continue
			}
			log.Printf("reconcile: restoring hook file state for workspace %q slot %d (session %q is dead)", workspace, slot, sessionName)
			if err := restoreProjectFileHooks(workspace, slot); err != nil {
				log.Printf("reconcile: failed to restore hook file state for workspace %q slot %d: %v", workspace, slot, err)
			}
		}
	}
}

// parseSlotIndex converts a slot directory name (e.g. "3") to an int.
// Returns -1 if the name is not a valid slot index.
func parseSlotIndex(name string) int {
	n := 0
	for _, c := range name {
		if c < '0' || c > '9' {
			return -1
		}
		n = n*10 + int(c-'0')
	}
	if name == "" {
		return -1
	}
	return n
}

// deepMergeMap recursively merges overlay into base. Overlay values win for
// non-map types; nested maps are merged recursively.
func deepMergeMap(base, overlay map[string]interface{}) map[string]interface{} {
	result := make(map[string]interface{}, len(base)+len(overlay))
	for k, v := range base {
		result[k] = v
	}
	for k, v := range overlay {
		if baseVal, ok := result[k]; ok {
			if baseMap, ok := baseVal.(map[string]interface{}); ok {
				if overlayMap, ok := v.(map[string]interface{}); ok {
					result[k] = deepMergeMap(baseMap, overlayMap)
					continue
				}
			}
		}
		result[k] = v
	}
	return result
}
