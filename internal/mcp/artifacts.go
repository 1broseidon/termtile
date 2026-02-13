package mcp

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

const (
	artifactFileName = "output.json"
)

type hookArtifactPayload struct {
	Status string `json:"status"`
	Output string `json:"output"`
}

func parseHookArtifactPayload(data []byte) (hookArtifactPayload, error) {
	if strings.TrimSpace(string(data)) == "" {
		return hookArtifactPayload{}, errors.New("artifact is empty")
	}
	var payload hookArtifactPayload
	if err := json.Unmarshal(data, &payload); err != nil {
		return hookArtifactPayload{}, err
	}
	return payload, nil
}

func readArtifactOutputField(workspace string, slot int) (string, error) {
	data, err := ReadArtifact(workspace, slot)
	if err != nil {
		return "", err
	}
	payload, err := parseHookArtifactPayload(data)
	if err != nil {
		return "", err
	}
	return payload.Output, nil
}

func normalizeArtifactWorkspace(workspace string) string {
	workspace = strings.TrimSpace(workspace)
	if workspace == "" {
		return DefaultWorkspace
	}
	return workspace
}

func artifactBaseDir() (string, error) {
	if xdg := strings.TrimSpace(os.Getenv("XDG_DATA_HOME")); xdg != "" {
		return filepath.Join(xdg, "termtile", "artifacts"), nil
	}

	home, err := os.UserHomeDir()
	if err != nil || strings.TrimSpace(home) == "" {
		home = strings.TrimSpace(os.Getenv("HOME"))
	}
	if home == "" {
		return "", fmt.Errorf("failed to resolve artifact directory: home directory is not set")
	}

	return filepath.Join(home, ".local", "share", "termtile", "artifacts"), nil
}

// GetArtifactDir returns the filesystem directory for workspace+slot artifacts:
// {base}/artifacts/{workspace}/{slot}.
func GetArtifactDir(workspace string, slot int) (string, error) {
	if slot < 0 {
		return "", fmt.Errorf("invalid slot %d", slot)
	}
	baseDir, err := artifactBaseDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(baseDir, normalizeArtifactWorkspace(workspace), strconv.Itoa(slot)), nil
}

func artifactFilePath(workspace string, slot int) (string, error) {
	artifactDir, err := GetArtifactDir(workspace, slot)
	if err != nil {
		return "", err
	}
	return filepath.Join(artifactDir, artifactFileName), nil
}

// EnsureArtifactDir creates the artifact directory for workspace+slot with 0755
// permissions. Returns the directory path on success.
func EnsureArtifactDir(workspace string, slot int) (string, error) {
	artifactDir, err := GetArtifactDir(workspace, slot)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(artifactDir, 0o755); err != nil {
		return "", err
	}
	return artifactDir, nil
}

// ReadArtifact reads output.json from the workspace+slot artifact directory.
func ReadArtifact(workspace string, slot int) ([]byte, error) {
	path, err := artifactFilePath(workspace, slot)
	if err != nil {
		return nil, err
	}
	return os.ReadFile(path)
}

// CleanupArtifact removes the workspace+slot artifact directory and its
// contents. It is safe to call even if the directory does not exist.
func CleanupArtifact(workspace string, slot int) error {
	artifactDir, err := GetArtifactDir(workspace, slot)
	if err != nil {
		return err
	}
	return os.RemoveAll(artifactDir)
}

// CleanStaleOutput removes only the output.json artifact file from a
// workspace+slot directory, preserving context.md and checkpoint.json
// which may have been placed by the orchestrator for the next spawn.
func CleanStaleOutput(workspace string, slot int) error {
	path, err := artifactFilePath(workspace, slot)
	if err != nil {
		return err
	}
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

func moveArtifactDir(srcWorkspace string, srcSlot int, dstWorkspace string, dstSlot int) error {
	srcDir, err := GetArtifactDir(srcWorkspace, srcSlot)
	if err != nil {
		return err
	}
	if _, err := os.Stat(srcDir); err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	dstDir, err := GetArtifactDir(dstWorkspace, dstSlot)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(dstDir), 0o755); err != nil {
		return err
	}
	if err := os.RemoveAll(dstDir); err != nil {
		return err
	}
	if err := os.Rename(srcDir, dstDir); err == nil {
		return nil
	}

	if err := copyArtifactDir(srcDir, dstDir); err != nil {
		return err
	}
	return os.RemoveAll(srcDir)
}

func copyArtifactDir(srcDir, dstDir string) error {
	return filepath.WalkDir(srcDir, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel, err := filepath.Rel(srcDir, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dstDir, rel)
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return os.MkdirAll(target, info.Mode().Perm())
		}
		if !entry.Type().IsRegular() {
			return fmt.Errorf("unsupported artifact entry type %q in %s", entry.Type().String(), path)
		}

		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		srcFile, err := os.Open(path)
		if err != nil {
			return err
		}

		dstFile, err := os.OpenFile(target, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, info.Mode().Perm())
		if err != nil {
			_ = srcFile.Close()
			return err
		}
		if _, err := io.Copy(dstFile, srcFile); err != nil {
			_ = dstFile.Close()
			_ = srcFile.Close()
			return err
		}
		if err := dstFile.Close(); err != nil {
			_ = srcFile.Close()
			return err
		}
		return srcFile.Close()
	})
}

var slotOutputTemplateRE = regexp.MustCompile(`\{\{\s*slot_(\d+)\.output\s*\}\}`)

// substituteSlotOutputTemplates replaces {{slot_N.output}} placeholders using
// artifacts from dependency slots only. Placeholders for non-dependency slots
// or missing artifacts are left unchanged.
func substituteSlotOutputTemplates(task, workspace string, dependsOn []int) (string, []int) {
	if strings.TrimSpace(task) == "" || len(dependsOn) == 0 {
		return task, nil
	}

	depSet := make(map[int]struct{}, len(dependsOn))
	for _, s := range dependsOn {
		depSet[s] = struct{}{}
	}

	missingSet := make(map[int]struct{})
	out := slotOutputTemplateRE.ReplaceAllStringFunc(task, func(m string) string {
		sub := slotOutputTemplateRE.FindStringSubmatch(m)
		if len(sub) != 2 {
			return m
		}
		n, err := strconv.Atoi(sub[1])
		if err != nil {
			return m
		}
		if _, ok := depSet[n]; !ok {
			return m
		}
		output, err := readArtifactOutputField(workspace, n)
		if err != nil {
			missingSet[n] = struct{}{}
			return m
		}
		return output
	})

	missing := make([]int, 0, len(missingSet))
	for slot := range missingSet {
		missing = append(missing, slot)
	}
	sort.Ints(missing)
	return out, missing
}
