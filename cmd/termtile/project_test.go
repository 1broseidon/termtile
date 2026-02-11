package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/1broseidon/termtile/internal/workspace"
)

func TestRunProjectInitCreatesProjectFiles(t *testing.T) {
	repo := t.TempDir()
	home := t.TempDir()
	t.Setenv("HOME", home)

	mustMkdir(t, filepath.Join(repo, ".git"))
	chdir(t, repo)

	if rc := runWorkspace([]string{"init", "--workspace", "dev"}); rc != 0 {
		t.Fatalf("runWorkspace init rc=%d, want 0", rc)
	}

	cfgPath := filepath.Join(repo, projectDirName, projectWorkspaceCfgFile)
	cfg, err := readProjectWorkspaceConfig(cfgPath)
	if err != nil {
		t.Fatalf("readProjectWorkspaceConfig: %v", err)
	}
	if cfg.Version != 1 {
		t.Fatalf("version=%d, want 1", cfg.Version)
	}
	if cfg.Workspace != "dev" {
		t.Fatalf("workspace=%q, want %q", cfg.Workspace, "dev")
	}
	if len(cfg.Sync.Include) != 3 {
		t.Fatalf("sync.include len=%d, want 3", len(cfg.Sync.Include))
	}

	gitignorePath := filepath.Join(repo, projectDirName, ".gitignore")
	data, err := os.ReadFile(gitignorePath)
	if err != nil {
		t.Fatalf("read .gitignore: %v", err)
	}
	if !strings.Contains(string(data), "local.yaml") {
		t.Fatalf(".gitignore missing local.yaml entry: %q", string(data))
	}
}

func TestRunProjectLinkUpdatesWorkspaceBindingOnly(t *testing.T) {
	repo := t.TempDir()
	home := t.TempDir()
	t.Setenv("HOME", home)

	mustMkdir(t, filepath.Join(repo, ".git"))
	chdir(t, repo)

	if rc := runWorkspace([]string{"init", "--workspace", "alpha"}); rc != 0 {
		t.Fatalf("runWorkspace init rc=%d, want 0", rc)
	}

	cfgPath := filepath.Join(repo, projectDirName, projectWorkspaceCfgFile)
	before, err := readProjectWorkspaceConfig(cfgPath)
	if err != nil {
		t.Fatalf("read before: %v", err)
	}

	if rc := runWorkspace([]string{"link", "--workspace", "beta"}); rc != 0 {
		t.Fatalf("runWorkspace link rc=%d, want 0", rc)
	}

	after, err := readProjectWorkspaceConfig(cfgPath)
	if err != nil {
		t.Fatalf("read after: %v", err)
	}

	if after.Workspace != "beta" {
		t.Fatalf("workspace=%q, want %q", after.Workspace, "beta")
	}
	if after.Project.RootMarker != before.Project.RootMarker {
		t.Fatalf("root_marker changed: %q -> %q", before.Project.RootMarker, after.Project.RootMarker)
	}
	if after.Sync.Mode != before.Sync.Mode {
		t.Fatalf("sync.mode changed: %q -> %q", before.Sync.Mode, after.Sync.Mode)
	}
}

func TestRunProjectSyncPullAndPush(t *testing.T) {
	repo := t.TempDir()
	home := t.TempDir()
	t.Setenv("HOME", home)

	mustMkdir(t, filepath.Join(repo, ".git"))
	chdir(t, repo)

	initial := &workspace.WorkspaceConfig{
		Name:      "dev",
		Layout:    "grid",
		AgentMode: false,
		Terminals: []workspace.TerminalConfig{
			{WMClass: "ghostty", Cwd: "/tmp/a", SlotIndex: 0},
		},
	}
	if err := workspace.Write(initial); err != nil {
		t.Fatalf("workspace.Write initial: %v", err)
	}

	if rc := runWorkspace([]string{"init", "--workspace", "dev"}); rc != 0 {
		t.Fatalf("runWorkspace init rc=%d, want 0", rc)
	}

	updated := &workspace.WorkspaceConfig{
		Name:      "dev",
		Layout:    "columns",
		AgentMode: true,
		Terminals: []workspace.TerminalConfig{
			{WMClass: "ghostty", Cwd: "/tmp/b", SlotIndex: 0},
			{WMClass: "ghostty", Cwd: "/tmp/c", SlotIndex: 1},
		},
	}
	if err := workspace.Write(updated); err != nil {
		t.Fatalf("workspace.Write updated: %v", err)
	}

	if rc := runWorkspace([]string{"sync", "pull"}); rc != 0 {
		t.Fatalf("runWorkspace sync pull rc=%d, want 0", rc)
	}

	localPath := filepath.Join(repo, projectDirName, projectLocalCfgFile)
	localCfg, err := readProjectLocalConfig(localPath)
	if err != nil {
		t.Fatalf("readProjectLocalConfig: %v", err)
	}
	if localCfg.Snapshot.Layout == nil || *localCfg.Snapshot.Layout != "columns" {
		t.Fatalf("snapshot.layout=%v, want %q", localCfg.Snapshot.Layout, "columns")
	}
	if localCfg.Snapshot.AgentMode == nil || !*localCfg.Snapshot.AgentMode {
		t.Fatalf("snapshot.agent_mode=%v, want true", localCfg.Snapshot.AgentMode)
	}
	if localCfg.Snapshot.Terminals == nil || len(*localCfg.Snapshot.Terminals) != 2 {
		t.Fatalf("snapshot.terminals=%v, want 2 terminals", localCfg.Snapshot.Terminals)
	}

	layout := "rows"
	agentMode := false
	terms := []workspace.TerminalConfig{
		{WMClass: "kitty", Cwd: "/tmp/new", SlotIndex: 0},
	}
	localCfg.Snapshot.Layout = &layout
	localCfg.Snapshot.AgentMode = &agentMode
	localCfg.Snapshot.Terminals = &terms
	if err := writeProjectLocalConfig(localPath, localCfg); err != nil {
		t.Fatalf("writeProjectLocalConfig: %v", err)
	}

	if rc := runWorkspace([]string{"sync", "push"}); rc != 0 {
		t.Fatalf("runWorkspace sync push rc=%d, want 0", rc)
	}

	got, err := workspace.Read("dev")
	if err != nil {
		t.Fatalf("workspace.Read: %v", err)
	}
	if got.Layout != "rows" {
		t.Fatalf("workspace layout=%q, want %q", got.Layout, "rows")
	}
	if got.AgentMode {
		t.Fatalf("workspace agent_mode=%v, want false", got.AgentMode)
	}
	if len(got.Terminals) != 1 || got.Terminals[0].WMClass != "kitty" {
		t.Fatalf("workspace terminals=%v, want single kitty terminal", got.Terminals)
	}
}

func TestRunProjectSyncPushFailsWhenSnapshotMissingRequiredFields(t *testing.T) {
	repo := t.TempDir()
	home := t.TempDir()
	t.Setenv("HOME", home)

	mustMkdir(t, filepath.Join(repo, ".git"))
	chdir(t, repo)

	if err := workspace.Write(&workspace.WorkspaceConfig{
		Name:      "dev",
		Layout:    "grid",
		AgentMode: false,
		Terminals: []workspace.TerminalConfig{{WMClass: "ghostty", SlotIndex: 0}},
	}); err != nil {
		t.Fatalf("workspace.Write: %v", err)
	}

	if rc := runWorkspace([]string{"init", "--workspace", "dev"}); rc != 0 {
		t.Fatalf("runWorkspace init rc=%d, want 0", rc)
	}

	layout := "grid"
	localCfg := &projectLocalConfig{
		Version:   1,
		Workspace: "dev",
		Snapshot: projectWorkspaceSnapshot{
			Layout: &layout,
		},
	}
	localPath := filepath.Join(repo, projectDirName, projectLocalCfgFile)
	if err := writeProjectLocalConfig(localPath, localCfg); err != nil {
		t.Fatalf("writeProjectLocalConfig: %v", err)
	}

	if rc := runWorkspace([]string{"sync", "push"}); rc == 0 {
		t.Fatalf("runWorkspace sync push rc=%d, want non-zero", rc)
	}
}

func mustMkdir(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(path, 0755); err != nil {
		t.Fatalf("mkdir %s: %v", path, err)
	}
}

func chdir(t *testing.T, dir string) {
	t.Helper()
	orig, err := os.Getwd()
	if err != nil {
		t.Fatalf("os.Getwd: %v", err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("chdir %s: %v", dir, err)
	}
	t.Cleanup(func() {
		_ = os.Chdir(orig)
	})
}
