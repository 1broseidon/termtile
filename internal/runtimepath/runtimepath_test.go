package runtimepath

import (
	"fmt"
	"os"
	"strings"
	"testing"
)

func TestDir_UsesXDGRuntimeDirWhenSet(t *testing.T) {
	td := t.TempDir()
	t.Setenv("XDG_RUNTIME_DIR", td)

	got, err := Dir()
	if err != nil {
		t.Fatalf("Dir() error: %v", err)
	}
	if got != td {
		t.Fatalf("Dir() = %q, want %q", got, td)
	}
}

func TestDir_FallbacksWhenXDGRuntimeDirMissing(t *testing.T) {
	t.Setenv("XDG_RUNTIME_DIR", "")

	got, err := Dir()
	if err != nil {
		t.Fatalf("Dir() error: %v", err)
	}
	if got == "" {
		t.Fatal("Dir() returned empty path")
	}

	wantRun := fmt.Sprintf("/run/user/%d", os.Getuid())
	wantTmp := fmt.Sprintf("/tmp/termtile-runtime-%d", os.Getuid())
	if got != wantRun && got != wantTmp {
		t.Fatalf("Dir() = %q, want %q or %q", got, wantRun, wantTmp)
	}
}

func TestSocketPathAndWorkspaceRegistryPath(t *testing.T) {
	td := t.TempDir()
	t.Setenv("XDG_RUNTIME_DIR", td)

	socket, err := SocketPath()
	if err != nil {
		t.Fatalf("SocketPath() error: %v", err)
	}
	if !strings.HasSuffix(socket, "/termtile.sock") {
		t.Fatalf("SocketPath() = %q, missing suffix", socket)
	}

	reg, err := WorkspaceRegistryPath()
	if err != nil {
		t.Fatalf("WorkspaceRegistryPath() error: %v", err)
	}
	if !strings.HasSuffix(reg, "/termtile-workspace.json") {
		t.Fatalf("WorkspaceRegistryPath() = %q, missing suffix", reg)
	}
}
