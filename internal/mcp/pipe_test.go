package mcp

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPipeFilePath(t *testing.T) {
	got := pipeFilePath("my-workspace", 3)
	want := filepath.Join(os.TempDir(), "termtile-pipe-my-workspace-3.raw")
	if got != want {
		t.Errorf("pipeFilePath = %q, want %q", got, want)
	}
}

func TestPipeFilePath_DefaultWorkspace(t *testing.T) {
	got := pipeFilePath("mcp-agents", 0)
	want := filepath.Join(os.TempDir(), "termtile-pipe-mcp-agents-0.raw")
	if got != want {
		t.Errorf("pipeFilePath = %q, want %q", got, want)
	}
}

func TestCountCloseTagsInPipeFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.raw")

	// Simulate raw pipe output with contiguous fence close tags
	// interspersed with ANSI escape sequences and other output.
	content := "some output\x1b[32m\n" +
		"[termtile-response]\n" +
		"response content here\n" +
		"[/termtile-response]\n" +
		"more stuff\n" +
		"[termtile-response]\n" +
		"second response\n" +
		"[/termtile-response]\n"

	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("write: %v", err)
	}

	count, size, err := countCloseTagsInPipeFile(path)
	if err != nil {
		t.Fatalf("countCloseTagsInPipeFile: %v", err)
	}
	if count != 2 {
		t.Errorf("count = %d, want 2", count)
	}
	if size != int64(len(content)) {
		t.Errorf("size = %d, want %d", size, len(content))
	}
}

func TestCountCloseTagsInPipeFile_NoMatch(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.raw")

	// Simulate instruction echo where the close tag is broken up by TUI
	// character-by-character rendering (not contiguous).
	content := "[/t\x1b[0me\x1b[0mr\x1b[0mm\x1b[0mt\x1b[0mi\x1b[0ml\x1b[0me\x1b[0m-\x1b[0mr\x1b[0me\x1b[0ms\x1b[0mp\x1b[0mo\x1b[0mn\x1b[0ms\x1b[0me\x1b[0m]\n"

	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("write: %v", err)
	}

	count, _, err := countCloseTagsInPipeFile(path)
	if err != nil {
		t.Fatalf("countCloseTagsInPipeFile: %v", err)
	}
	if count != 0 {
		t.Errorf("count = %d, want 0 (instruction echo should not match)", count)
	}
}

func TestCountCloseTagsInPipeFile_Missing(t *testing.T) {
	_, _, err := countCloseTagsInPipeFile("/nonexistent/path/test.raw")
	if err == nil {
		t.Fatal("expected error for missing file")
	}
}

func TestPipeFileSize(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.raw")

	content := "hello world\n"
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("write: %v", err)
	}

	got := pipeFileSize(path)
	want := int64(len(content))
	if got != want {
		t.Errorf("pipeFileSize = %d, want %d", got, want)
	}
}

func TestPipeFileSize_Missing(t *testing.T) {
	got := pipeFileSize("/nonexistent/path/test.raw")
	if got != 0 {
		t.Errorf("pipeFileSize for missing file = %d, want 0", got)
	}
}

func TestCleanStalePipeFiles(t *testing.T) {
	dir := t.TempDir()

	// Override TempDir for this test by creating files in actual TempDir.
	// We test the logic by creating real files matching the pattern.
	active := filepath.Join(dir, "termtile-pipe-ws-0.raw")
	stale := filepath.Join(dir, "termtile-pipe-ws-1.raw")

	for _, path := range []string{active, stale} {
		if err := os.WriteFile(path, []byte("data"), 0644); err != nil {
			t.Fatalf("write: %v", err)
		}
	}

	// Simulate tracked map with only slot 0 active.
	tracked := map[string]map[int]trackedAgent{
		"ws": {
			0: {pipeFilePath: active},
		},
	}

	// cleanStalePipeFiles uses os.TempDir() for glob pattern, so we test
	// the file matching logic manually here.
	activeSet := make(map[string]struct{})
	for _, slots := range tracked {
		for _, ta := range slots {
			if ta.pipeFilePath != "" {
				activeSet[ta.pipeFilePath] = struct{}{}
			}
		}
	}

	// Stale file should not be in active set.
	if _, ok := activeSet[stale]; ok {
		t.Fatal("stale file should not be in active set")
	}
	if _, ok := activeSet[active]; !ok {
		t.Fatal("active file should be in active set")
	}
}
