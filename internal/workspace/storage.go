package workspace

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

func workspacesDir() (string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("failed to get home directory: %w", err)
	}
	return filepath.Join(homeDir, ".config", "termtile", "workspaces"), nil
}

func validateWorkspaceName(name string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return fmt.Errorf("workspace name is required")
	}
	if strings.Contains(name, string(os.PathSeparator)) || name != filepath.Base(name) {
		return fmt.Errorf("invalid workspace name %q", name)
	}
	if name == "." || name == ".." || strings.Contains(name, "..") {
		return fmt.Errorf("invalid workspace name %q", name)
	}
	return nil
}

// ValidateWorkspaceName validates a workspace name (exported version).
func ValidateWorkspaceName(name string) error {
	return validateWorkspaceName(name)
}

// ConfigPath returns the path to a workspace config file.
func ConfigPath(name string) string {
	path, err := workspacePath(name)
	if err != nil {
		return ""
	}
	return path
}

func workspacePath(name string) (string, error) {
	if err := validateWorkspaceName(name); err != nil {
		return "", err
	}
	dir, err := workspacesDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, name+".json"), nil
}

func Write(cfg *WorkspaceConfig) error {
	if cfg == nil {
		return fmt.Errorf("workspace is nil")
	}
	if err := validateWorkspaceName(cfg.Name); err != nil {
		return err
	}
	dir, err := workspacesDir()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create workspace directory: %w", err)
	}
	path, err := workspacePath(cfg.Name)
	if err != nil {
		return err
	}

	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to encode workspace: %w", err)
	}
	if err := os.WriteFile(path, append(data, '\n'), 0644); err != nil {
		return fmt.Errorf("failed to write workspace %q: %w", cfg.Name, err)
	}
	return nil
}

func Read(name string) (*WorkspaceConfig, error) {
	path, err := workspacePath(name)
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read workspace %q: %w", name, err)
	}
	var cfg WorkspaceConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse workspace %q: %w", name, err)
	}
	if cfg.Name == "" {
		cfg.Name = name
	}
	return &cfg, nil
}

func Delete(name string) error {
	path, err := workspacePath(name)
	if err != nil {
		return err
	}
	if err := os.Remove(path); err != nil {
		return fmt.Errorf("failed to delete workspace %q: %w", name, err)
	}
	return nil
}

func List() ([]string, error) {
	dir, err := workspacesDir()
	if err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to list workspaces: %w", err)
	}

	var out []string
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !strings.HasSuffix(name, ".json") {
			continue
		}
		out = append(out, strings.TrimSuffix(name, ".json"))
	}
	sort.Strings(out)
	return out, nil
}

