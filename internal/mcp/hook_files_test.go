package mcp

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/1broseidon/termtile/internal/config"
)

func TestInjectProjectFileHooks_NewFile(t *testing.T) {
	cwd := t.TempDir()
	// Override artifact base dir via XDG_DATA_HOME.
	artDir := t.TempDir()
	t.Setenv("XDG_DATA_HOME", artDir)

	agentCfg := config.AgentConfig{
		HookSettingsDir:  ".testdir",
		HookSettingsFile: "settings.json",
	}
	settings := `{"hooks":{"Start":[{"type":"command","command":"test-cmd"}]}}`

	state, err := injectProjectFileHooks("test-ws", 1, cwd, agentCfg, settings)
	if err != nil {
		t.Fatalf("inject failed: %v", err)
	}
	if state.HadOriginal {
		t.Error("expected HadOriginal=false for new file")
	}

	// Verify the settings file was written.
	settingsPath := filepath.Join(cwd, ".testdir", "settings.json")
	data, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatalf("failed to read settings file: %v", err)
	}
	var parsed map[string]interface{}
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("settings file is not valid JSON: %v", err)
	}
	if _, ok := parsed["hooks"]; !ok {
		t.Error("expected 'hooks' key in written settings")
	}

	// Verify state file was written.
	stateDir, _ := GetArtifactDir("test-ws", 1)
	statePath := filepath.Join(stateDir, hookStateFileName)
	if _, err := os.Stat(statePath); os.IsNotExist(err) {
		t.Error("expected hook state file to exist")
	}
}

func TestInjectProjectFileHooks_ExistingFile(t *testing.T) {
	cwd := t.TempDir()
	artDir := t.TempDir()
	t.Setenv("XDG_DATA_HOME", artDir)

	// Create existing settings file with extra config.
	settingsDir := filepath.Join(cwd, ".testdir")
	if err := os.MkdirAll(settingsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	original := `{"mcp":{"servers":{"my-server":{}}}}`
	if err := os.WriteFile(filepath.Join(settingsDir, "settings.json"), []byte(original), 0o644); err != nil {
		t.Fatal(err)
	}

	agentCfg := config.AgentConfig{
		HookSettingsDir:  ".testdir",
		HookSettingsFile: "settings.json",
	}
	settings := `{"hooks":{"Start":[{"type":"command","command":"test-cmd"}]}}`

	state, err := injectProjectFileHooks("test-ws", 2, cwd, agentCfg, settings)
	if err != nil {
		t.Fatalf("inject failed: %v", err)
	}
	if !state.HadOriginal {
		t.Error("expected HadOriginal=true for existing file")
	}

	// Verify the merged file preserves the original "mcp" key.
	data, err := os.ReadFile(filepath.Join(settingsDir, "settings.json"))
	if err != nil {
		t.Fatal(err)
	}
	var merged map[string]interface{}
	if err := json.Unmarshal(data, &merged); err != nil {
		t.Fatal(err)
	}
	if _, ok := merged["mcp"]; !ok {
		t.Error("expected original 'mcp' key to be preserved in merged settings")
	}
	if _, ok := merged["hooks"]; !ok {
		t.Error("expected 'hooks' key to be present in merged settings")
	}

	// Verify backup was created.
	stateDir, _ := GetArtifactDir("test-ws", 2)
	backupPath := filepath.Join(stateDir, hookBackupFileName)
	backupData, err := os.ReadFile(backupPath)
	if err != nil {
		t.Fatalf("failed to read backup: %v", err)
	}
	if string(backupData) != original {
		t.Errorf("backup doesn't match original: got %q, want %q", string(backupData), original)
	}
}

func TestRestoreProjectFileHooks_RestoreOriginal(t *testing.T) {
	cwd := t.TempDir()
	artDir := t.TempDir()
	t.Setenv("XDG_DATA_HOME", artDir)

	// Create existing settings file.
	settingsDir := filepath.Join(cwd, ".testdir")
	if err := os.MkdirAll(settingsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	original := `{"mcp":{"servers":{"my-server":{}}}}`
	settingsPath := filepath.Join(settingsDir, "settings.json")
	if err := os.WriteFile(settingsPath, []byte(original), 0o644); err != nil {
		t.Fatal(err)
	}

	agentCfg := config.AgentConfig{
		HookSettingsDir:  ".testdir",
		HookSettingsFile: "settings.json",
	}
	settings := `{"hooks":{"Start":[]}}`

	_, err := injectProjectFileHooks("test-ws", 3, cwd, agentCfg, settings)
	if err != nil {
		t.Fatal(err)
	}

	// Verify the file was modified.
	data, _ := os.ReadFile(settingsPath)
	if string(data) == original {
		t.Fatal("settings file should have been modified by injection")
	}

	// Restore.
	if err := restoreProjectFileHooks("test-ws", 3); err != nil {
		t.Fatalf("restore failed: %v", err)
	}

	// Verify the original was restored.
	restored, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatalf("failed to read restored file: %v", err)
	}
	if string(restored) != original {
		t.Errorf("restored file doesn't match original: got %q, want %q", string(restored), original)
	}
}

func TestRestoreProjectFileHooks_RemoveInjected(t *testing.T) {
	cwd := t.TempDir()
	artDir := t.TempDir()
	t.Setenv("XDG_DATA_HOME", artDir)

	agentCfg := config.AgentConfig{
		HookSettingsDir:  ".testdir",
		HookSettingsFile: "settings.json",
	}
	settings := `{"hooks":{}}`

	_, err := injectProjectFileHooks("test-ws", 4, cwd, agentCfg, settings)
	if err != nil {
		t.Fatal(err)
	}

	settingsPath := filepath.Join(cwd, ".testdir", "settings.json")
	if _, err := os.Stat(settingsPath); os.IsNotExist(err) {
		t.Fatal("injected file should exist before restore")
	}

	// Restore should remove the file since there was no original.
	if err := restoreProjectFileHooks("test-ws", 4); err != nil {
		t.Fatalf("restore failed: %v", err)
	}

	if _, err := os.Stat(settingsPath); !os.IsNotExist(err) {
		t.Error("expected injected file to be removed after restore")
	}
}

func TestRestoreProjectFileHooks_NoStateFile(t *testing.T) {
	artDir := t.TempDir()
	t.Setenv("XDG_DATA_HOME", artDir)

	// Restoring when there's no state file should be a no-op.
	if err := restoreProjectFileHooks("no-ws", 99); err != nil {
		t.Fatalf("expected no error for missing state file, got %v", err)
	}
}

func TestDeepMergeMap(t *testing.T) {
	base := map[string]interface{}{
		"a": "base-a",
		"b": map[string]interface{}{
			"x": "base-x",
			"y": "base-y",
		},
		"c": "base-c",
	}
	overlay := map[string]interface{}{
		"a": "overlay-a",
		"b": map[string]interface{}{
			"y": "overlay-y",
			"z": "overlay-z",
		},
		"d": "overlay-d",
	}

	result := deepMergeMap(base, overlay)

	if result["a"] != "overlay-a" {
		t.Errorf("expected a=overlay-a, got %v", result["a"])
	}
	if result["c"] != "base-c" {
		t.Errorf("expected c=base-c, got %v", result["c"])
	}
	if result["d"] != "overlay-d" {
		t.Errorf("expected d=overlay-d, got %v", result["d"])
	}

	b := result["b"].(map[string]interface{})
	if b["x"] != "base-x" {
		t.Errorf("expected b.x=base-x, got %v", b["x"])
	}
	if b["y"] != "overlay-y" {
		t.Errorf("expected b.y=overlay-y, got %v", b["y"])
	}
	if b["z"] != "overlay-z" {
		t.Errorf("expected b.z=overlay-z, got %v", b["z"])
	}
}

func TestParseSlotIndex(t *testing.T) {
	tests := []struct {
		name string
		want int
	}{
		{"0", 0},
		{"1", 1},
		{"42", 42},
		{"", -1},
		{"abc", -1},
		{"1a", -1},
	}
	for _, tt := range tests {
		got := parseSlotIndex(tt.name)
		if got != tt.want {
			t.Errorf("parseSlotIndex(%q) = %d, want %d", tt.name, got, tt.want)
		}
	}
}
