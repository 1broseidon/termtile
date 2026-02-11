package mcp

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// pipeFilePath returns the deterministic path for a pipe-pane output file.
func pipeFilePath(workspace string, slot int) string {
	return filepath.Join(os.TempDir(), fmt.Sprintf("termtile-pipe-%s-%d.raw", workspace, slot))
}

// startPipePane activates tmux pipe-pane to append raw output to filepath.
func startPipePane(target, filepath string) error {
	cmd := exec.Command("tmux", "pipe-pane", "-o", "-t", target, "cat >> "+filepath)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("tmux pipe-pane failed: %w (%s)", err, strings.TrimSpace(string(out)))
	}
	return nil
}

// stopPipePane deactivates pipe-pane for a tmux target (no command = stop).
func stopPipePane(target string) {
	_ = exec.Command("tmux", "pipe-pane", "-t", target).Run()
}

// removePipeFile removes a pipe output file (best-effort).
func removePipeFile(filepath string) {
	_ = os.Remove(filepath)
}

// countCloseTagsInPipeFile reads the raw pipe file and counts contiguous
// occurrences of the fence close tag. Returns the count, file size, and any error.
func countCloseTagsInPipeFile(filepath string) (count int, size int64, err error) {
	data, err := os.ReadFile(filepath)
	if err != nil {
		return 0, 0, err
	}
	size = int64(len(data))
	count = strings.Count(string(data), fenceClose)
	return count, size, nil
}

// pipeFileSize returns the file size via os.Stat, or 0 on error.
func pipeFileSize(filepath string) int64 {
	info, err := os.Stat(filepath)
	if err != nil {
		return 0
	}
	return info.Size()
}

// cleanStalePipeFiles removes /tmp/termtile-pipe-*.raw files that don't belong
// to any currently tracked agent.
func cleanStalePipeFiles(tracked map[string]map[int]trackedAgent) {
	pattern := filepath.Join(os.TempDir(), "termtile-pipe-*.raw")
	matches, err := filepath.Glob(pattern)
	if err != nil || len(matches) == 0 {
		return
	}

	// Build set of active pipe file paths.
	active := make(map[string]struct{})
	for ws, slots := range tracked {
		for slot, ta := range slots {
			if ta.pipeFilePath != "" {
				active[ta.pipeFilePath] = struct{}{}
			} else {
				// Include the expected path even if not yet set,
				// to avoid removing files for slots being set up.
				active[pipeFilePath(ws, slot)] = struct{}{}
			}
		}
	}

	for _, path := range matches {
		if _, ok := active[path]; !ok {
			_ = os.Remove(path)
		}
	}
}
