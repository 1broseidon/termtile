package runtimepath

import (
	"fmt"
	"os"
	"path/filepath"
)

// Dir returns the runtime directory used by termtile state and IPC socket
// lookups. Priority:
// 1) XDG_RUNTIME_DIR (if set)
// 2) /run/user/<uid> (if present)
// 3) /tmp/termtile-runtime-<uid> (created)
func Dir() (string, error) {
	if runtimeDir := os.Getenv("XDG_RUNTIME_DIR"); runtimeDir != "" {
		return runtimeDir, nil
	}

	uid := os.Getuid()
	runUserDir := fmt.Sprintf("/run/user/%d", uid)
	if info, err := os.Stat(runUserDir); err == nil && info.IsDir() {
		return runUserDir, nil
	}

	tmpDir := fmt.Sprintf("/tmp/termtile-runtime-%d", uid)
	if err := os.MkdirAll(tmpDir, 0700); err != nil {
		return "", fmt.Errorf("failed to create runtime dir: %w", err)
	}
	return tmpDir, nil
}

// SocketPath returns the daemon IPC socket path.
func SocketPath() (string, error) {
	runtimeDir, err := Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(runtimeDir, "termtile.sock"), nil
}

// WorkspaceRegistryPath returns the active workspace registry path.
func WorkspaceRegistryPath() (string, error) {
	runtimeDir, err := Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(runtimeDir, "termtile-workspace.json"), nil
}
