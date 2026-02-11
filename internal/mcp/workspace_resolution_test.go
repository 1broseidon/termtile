package mcp

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	workspacepkg "github.com/1broseidon/termtile/internal/workspace"
)

func TestResolveWorkspaceForSpawn_ExplicitDefaultWorkspaceAllowed(t *testing.T) {
	t.Setenv("XDG_RUNTIME_DIR", t.TempDir())

	got, err := resolveWorkspaceForSpawn(DefaultWorkspace, "")
	if err != nil {
		t.Fatalf("resolveWorkspaceForSpawn returned error: %v", err)
	}
	if got != DefaultWorkspace {
		t.Fatalf("resolveWorkspaceForSpawn = %q, want %q", got, DefaultWorkspace)
	}
}

func TestResolveWorkspaceForSpawn_ExplicitDefaultWorkspaceRejectedWhenRegisteredWorkspacesExist(t *testing.T) {
	t.Setenv("XDG_RUNTIME_DIR", t.TempDir())

	if err := workspacepkg.SetActiveWorkspace("termtile", 1, true, 0, []int{0}); err != nil {
		t.Fatalf("SetActiveWorkspace termtile: %v", err)
	}
	if err := workspacepkg.SetActiveWorkspace("otto", 1, true, 1, []int{0}); err != nil {
		t.Fatalf("SetActiveWorkspace otto: %v", err)
	}

	_, err := resolveWorkspaceForSpawn(DefaultWorkspace, "")
	if err == nil {
		t.Fatal("expected mcp-agents rejection when registered workspaces exist")
	}
	if !strings.Contains(err.Error(), "legacy default") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestResolveWorkspaceForSpawn_ExplicitWorkspaceMustExist(t *testing.T) {
	t.Setenv("XDG_RUNTIME_DIR", t.TempDir())

	_, err := resolveWorkspaceForSpawn("does-not-exist", "")
	if err == nil {
		t.Fatal("expected error for unknown explicit workspace")
	}
	if !strings.Contains(err.Error(), "not found in registry") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestResolveWorkspaceForSpawn_ExplicitWorkspaceWins(t *testing.T) {
	t.Setenv("XDG_RUNTIME_DIR", t.TempDir())

	if err := workspacepkg.SetActiveWorkspace("ws-explicit", 1, true, 0, []int{0}); err != nil {
		t.Fatalf("SetActiveWorkspace ws-explicit: %v", err)
	}
	if err := workspacepkg.SetActiveWorkspace("ws-hint", 1, true, 1, []int{0}); err != nil {
		t.Fatalf("SetActiveWorkspace ws-hint: %v", err)
	}

	root := t.TempDir()
	writeProjectWorkspaceFile(t, root, "ws-project")
	t.Chdir(root)

	got, err := resolveWorkspaceForSpawn("ws-explicit", "ws-hint")
	if err != nil {
		t.Fatalf("resolveWorkspaceForSpawn returned error: %v", err)
	}
	if got != "ws-explicit" {
		t.Fatalf("resolveWorkspaceForSpawn = %q, want %q", got, "ws-explicit")
	}
}

func TestResolveWorkspaceForSpawn_SourceWorkspaceHintUsedWhenOmitted(t *testing.T) {
	t.Setenv("XDG_RUNTIME_DIR", t.TempDir())

	if err := workspacepkg.SetActiveWorkspace("ws-hint", 1, true, 0, []int{0}); err != nil {
		t.Fatalf("SetActiveWorkspace ws-hint: %v", err)
	}

	got, err := resolveWorkspaceForSpawn("", "ws-hint")
	if err != nil {
		t.Fatalf("resolveWorkspaceForSpawn returned error: %v", err)
	}
	if got != "ws-hint" {
		t.Fatalf("resolveWorkspaceForSpawn = %q, want %q", got, "ws-hint")
	}
}

func TestResolveWorkspaceForSpawn_UsesProjectMarkerWorkspaceWhenOmitted(t *testing.T) {
	t.Setenv("XDG_RUNTIME_DIR", t.TempDir())

	if err := workspacepkg.SetActiveWorkspace("ws-project", 1, true, 0, []int{0}); err != nil {
		t.Fatalf("SetActiveWorkspace ws-project: %v", err)
	}

	root := t.TempDir()
	writeProjectWorkspaceFile(t, root, "ws-project")
	nested := filepath.Join(root, "a", "b")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	t.Chdir(nested)

	got, err := resolveWorkspaceForSpawn("", "")
	if err != nil {
		t.Fatalf("resolveWorkspaceForSpawn returned error: %v", err)
	}
	if got != "ws-project" {
		t.Fatalf("resolveWorkspaceForSpawn = %q, want %q", got, "ws-project")
	}
}

func TestResolveWorkspaceForSpawn_ProjectMarkerMissingRootMarkerReturnsClearError(t *testing.T) {
	t.Setenv("XDG_RUNTIME_DIR", t.TempDir())

	root := t.TempDir()
	termtileDir := filepath.Join(root, ".termtile")
	if err := os.MkdirAll(termtileDir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	content := "workspace: ws-project\nproject:\n  root_marker: does-not-exist\n"
	if err := os.WriteFile(filepath.Join(termtileDir, "workspace.yaml"), []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile workspace.yaml: %v", err)
	}
	t.Chdir(root)

	_, err := resolveWorkspaceForSpawn("", "")
	if err == nil {
		t.Fatal("expected error for missing project root marker")
	}
	if !strings.Contains(err.Error(), "project.root_marker") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestResolveWorkspaceForSpawn_SingleRegisteredWorkspaceFallback(t *testing.T) {
	t.Setenv("XDG_RUNTIME_DIR", t.TempDir())
	t.Chdir(t.TempDir())

	if err := workspacepkg.SetActiveWorkspace("only-ws", 1, true, 0, []int{0}); err != nil {
		t.Fatalf("SetActiveWorkspace only-ws: %v", err)
	}

	got, err := resolveWorkspaceForSpawn("", "")
	if err != nil {
		t.Fatalf("resolveWorkspaceForSpawn returned error: %v", err)
	}
	if got != "only-ws" {
		t.Fatalf("resolveWorkspaceForSpawn = %q, want %q", got, "only-ws")
	}
}

func TestResolveWorkspaceForSpawn_MultipleAgentWorkspacesRequiresExplicit(t *testing.T) {
	t.Setenv("XDG_RUNTIME_DIR", t.TempDir())
	t.Chdir(t.TempDir())

	if err := workspacepkg.SetActiveWorkspace("ws-a", 1, true, 0, []int{0}); err != nil {
		t.Fatalf("SetActiveWorkspace ws-a: %v", err)
	}
	if err := workspacepkg.SetActiveWorkspace("ws-b", 1, true, 1, []int{0}); err != nil {
		t.Fatalf("SetActiveWorkspace ws-b: %v", err)
	}

	_, err := resolveWorkspaceForSpawn("", "")
	if err == nil {
		t.Fatal("expected explicit workspace error when multiple agent workspaces exist")
	}
	if !strings.Contains(err.Error(), "ambiguous workspace") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestResolveWorkspaceForSpawn_NoWorkspaceResolvedReturnsClearError(t *testing.T) {
	t.Setenv("XDG_RUNTIME_DIR", t.TempDir())
	t.Chdir(t.TempDir())

	_, err := resolveWorkspaceForSpawn("", "")
	if err == nil {
		t.Fatal("expected error with no resolvable workspace")
	}
	if !strings.Contains(err.Error(), "unable to resolve workspace") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestResolveWorkspaceForRead_SourceWorkspaceHintUsed(t *testing.T) {
	got, err := resolveWorkspaceForRead("", "hint-ws", "read_from_agent")
	if err != nil {
		t.Fatalf("resolveWorkspaceForRead returned error: %v", err)
	}
	if got != "hint-ws" {
		t.Fatalf("resolveWorkspaceForRead = %q, want %q", got, "hint-ws")
	}
}

func writeProjectWorkspaceFile(t *testing.T, root, workspace string) {
	t.Helper()

	gitDir := filepath.Join(root, ".git")
	if err := os.MkdirAll(gitDir, 0o755); err != nil {
		t.Fatalf("MkdirAll .git: %v", err)
	}
	termtileDir := filepath.Join(root, ".termtile")
	if err := os.MkdirAll(termtileDir, 0o755); err != nil {
		t.Fatalf("MkdirAll .termtile: %v", err)
	}
	content := "workspace: " + workspace + "\nproject:\n  root_marker: .git\n"
	if err := os.WriteFile(filepath.Join(termtileDir, "workspace.yaml"), []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile workspace.yaml: %v", err)
	}
}
