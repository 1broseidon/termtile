package mcp

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func writeHookArtifactForTest(t *testing.T, workspace string, slot int, output string) {
	t.Helper()
	dir, err := EnsureArtifactDir(workspace, slot)
	if err != nil {
		t.Fatalf("EnsureArtifactDir: %v", err)
	}
	payload := fmt.Sprintf(`{"status":"complete","output":%q}`, output)
	if err := os.WriteFile(filepath.Join(dir, "output.json"), []byte(payload), 0o644); err != nil {
		t.Fatalf("write output.json: %v", err)
	}
}

func TestArtifactTemplateSubstitutionReplacesDependencyOnly(t *testing.T) {
	base := t.TempDir()
	t.Setenv("XDG_DATA_HOME", base)
	writeHookArtifactForTest(t, "ws", 1, "ONE")
	writeHookArtifactForTest(t, "ws", 2, "TWO")

	in := "a {{slot_1.output}} b {{slot_2.output}} c"
	out, missing := substituteSlotOutputTemplates(in, "ws", []int{1})

	if len(missing) != 0 {
		t.Fatalf("expected no missing, got %v", missing)
	}
	if out != "a ONE b {{slot_2.output}} c" {
		t.Fatalf("unexpected substitution output: %q", out)
	}
}

func TestArtifactTemplateSubstitutionMissingLeavesPlaceholder(t *testing.T) {
	base := t.TempDir()
	t.Setenv("XDG_DATA_HOME", base)
	in := "x {{slot_3.output}} y"
	out, missing := substituteSlotOutputTemplates(in, "ws", []int{3})

	if out != in {
		t.Fatalf("expected placeholder unchanged, got %q", out)
	}
	if len(missing) != 1 || missing[0] != 3 {
		t.Fatalf("expected missing [3], got %v", missing)
	}
}

func TestGetArtifactDirUsesXDGDataHome(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", "/tmp/xdg-data")

	dir, err := GetArtifactDir("workspace-a", 7)
	if err != nil {
		t.Fatalf("GetArtifactDir returned error: %v", err)
	}

	want := filepath.Join("/tmp/xdg-data", "termtile", "artifacts", "workspace-a", "7")
	if dir != want {
		t.Fatalf("artifact dir = %q, want %q", dir, want)
	}
}

func TestGetArtifactDirUsesHomeFallback(t *testing.T) {
	home := t.TempDir()
	t.Setenv("XDG_DATA_HOME", "")
	t.Setenv("HOME", home)

	dir, err := GetArtifactDir("workspace-b", 3)
	if err != nil {
		t.Fatalf("GetArtifactDir returned error: %v", err)
	}

	want := filepath.Join(home, ".local", "share", "termtile", "artifacts", "workspace-b", "3")
	if dir != want {
		t.Fatalf("artifact dir = %q, want %q", dir, want)
	}
}

func TestEnsureReadCleanupArtifact(t *testing.T) {
	base := t.TempDir()
	t.Setenv("XDG_DATA_HOME", base)

	dir, err := EnsureArtifactDir("workspace-c", 5)
	if err != nil {
		t.Fatalf("EnsureArtifactDir returned error: %v", err)
	}

	if info, err := os.Stat(dir); err != nil {
		t.Fatalf("artifact dir stat failed: %v", err)
	} else if !info.IsDir() {
		t.Fatalf("artifact path is not a directory: %s", dir)
	}

	artifactPath := filepath.Join(dir, "output.json")
	if err := os.WriteFile(artifactPath, []byte(`{"ok":true}`), 0o644); err != nil {
		t.Fatalf("failed to write artifact file: %v", err)
	}

	data, err := ReadArtifact("workspace-c", 5)
	if err != nil {
		t.Fatalf("ReadArtifact returned error: %v", err)
	}
	if string(data) != `{"ok":true}` {
		t.Fatalf("artifact content = %q, want %q", string(data), `{"ok":true}`)
	}

	if err := CleanupArtifact("workspace-c", 5); err != nil {
		t.Fatalf("CleanupArtifact returned error: %v", err)
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Fatalf("expected artifact dir to be removed, stat err=%v", err)
	}
}

func TestGetArtifactDirRejectsNegativeSlot(t *testing.T) {
	if _, err := GetArtifactDir("ws", -1); err == nil {
		t.Fatal("expected error for negative slot")
	}
}

func TestMoveArtifactDirMovesDirectory(t *testing.T) {
	base := t.TempDir()
	t.Setenv("XDG_DATA_HOME", base)
	writeHookArtifactForTest(t, "ws", 3, "artifact-3")

	if err := moveArtifactDir("ws", 3, "ws", 2); err != nil {
		t.Fatalf("moveArtifactDir: %v", err)
	}

	output, err := readArtifactOutputField("ws", 2)
	if err != nil {
		t.Fatalf("readArtifactOutputField slot 2: %v", err)
	}
	if output != "artifact-3" {
		t.Fatalf("slot 2 output = %q, want %q", output, "artifact-3")
	}
	if _, err := ReadArtifact("ws", 3); !os.IsNotExist(err) {
		t.Fatalf("expected slot 3 artifact missing after move, err=%v", err)
	}
}
