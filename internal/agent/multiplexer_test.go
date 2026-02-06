package agent

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestTmuxMultiplexer_Interface(t *testing.T) {
	// Verify TmuxMultiplexer implements Multiplexer interface
	var _ Multiplexer = (*TmuxMultiplexer)(nil)
}

func TestTmuxMultiplexer_Name(t *testing.T) {
	tmux := NewTmuxMultiplexer()
	if tmux.Name() != "tmux" {
		t.Errorf("Name() = %q, want %q", tmux.Name(), "tmux")
	}
}

func TestTmuxMultiplexer_ConfigPath(t *testing.T) {
	// Test default config path
	tmux := NewTmuxMultiplexer()
	configPath := tmux.ConfigPath()

	if configPath == "" {
		t.Error("ConfigPath() returned empty string")
	}

	if !strings.HasSuffix(configPath, "termtile/tmux.conf") {
		t.Errorf("ConfigPath() = %q, want suffix 'termtile/tmux.conf'", configPath)
	}

	// Test custom config path
	customPath := "/custom/path/tmux.conf"
	tmux2 := NewTmuxMultiplexer(WithConfigPath(customPath))
	if tmux2.ConfigPath() != customPath {
		t.Errorf("ConfigPath() with custom = %q, want %q", tmux2.ConfigPath(), customPath)
	}
}

func TestTmuxMultiplexer_DefaultConfig(t *testing.T) {
	tmux := NewTmuxMultiplexer()
	config := tmux.DefaultConfig()

	// Check essential config elements
	essentials := []string{
		"set -g mouse on",
		"history-limit 50000",
		"WheelUpPane",
		"copy-mode",
	}

	for _, essential := range essentials {
		if !strings.Contains(config, essential) {
			t.Errorf("DefaultConfig() missing %q", essential)
		}
	}
}

func TestTmuxMultiplexer_SessionCommand(t *testing.T) {
	// Without config file
	tmux := NewTmuxMultiplexer(WithConfigPath("/nonexistent/path"))
	cmd := tmux.SessionCommand("test-session")

	if !strings.Contains(cmd, "tmux") {
		t.Errorf("SessionCommand() missing 'tmux': %q", cmd)
	}
	if !strings.Contains(cmd, "new-session") {
		t.Errorf("SessionCommand() missing 'new-session': %q", cmd)
	}
	if !strings.Contains(cmd, "-A") {
		t.Errorf("SessionCommand() missing '-A' flag: %q", cmd)
	}
	if !strings.Contains(cmd, "test-session") {
		t.Errorf("SessionCommand() missing session name: %q", cmd)
	}
}

func TestTmuxMultiplexer_SessionCommand_WithConfig(t *testing.T) {
	// Create a temp config file
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "tmux.conf")
	if err := os.WriteFile(configPath, []byte("# test"), 0644); err != nil {
		t.Fatal(err)
	}

	tmux := NewTmuxMultiplexer(WithConfigPath(configPath))
	cmd := tmux.SessionCommand("test-session")

	if !strings.Contains(cmd, "-f") {
		t.Errorf("SessionCommand() with config missing '-f' flag: %q", cmd)
	}
	if !strings.Contains(cmd, configPath) {
		t.Errorf("SessionCommand() with config missing config path: %q", cmd)
	}
}

func TestParseMultiplexerType(t *testing.T) {
	tests := []struct {
		input    string
		expected MultiplexerType
		wantErr  bool
	}{
		{"", MultiplexerAuto, false},
		{"auto", MultiplexerAuto, false},
		{"AUTO", MultiplexerAuto, false},
		{"tmux", MultiplexerTmux, false},
		{"TMUX", MultiplexerTmux, false},
		{"screen", MultiplexerScreen, false},
		{"invalid", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got, err := ParseMultiplexerType(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseMultiplexerType(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
				return
			}
			if got != tt.expected {
				t.Errorf("ParseMultiplexerType(%q) = %v, want %v", tt.input, got, tt.expected)
			}
		})
	}
}

func TestGetMultiplexer_UnknownType(t *testing.T) {
	_, err := GetMultiplexer("unknown")
	if err == nil {
		t.Error("GetMultiplexer(unknown) should return error")
	}
}

func TestGetMultiplexer_Screen(t *testing.T) {
	// Screen is deferred - should return error
	_, err := GetMultiplexer(MultiplexerScreen)
	if err == nil {
		t.Error("GetMultiplexer(screen) should return 'not implemented' error")
	}
	if !strings.Contains(err.Error(), "not yet implemented") {
		t.Errorf("GetMultiplexer(screen) error = %v, want 'not yet implemented'", err)
	}
}

func TestEnsureConfig(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "termtile", "tmux.conf")

	tmux := NewTmuxMultiplexer(WithConfigPath(configPath))

	// First call should create the file
	path, err := EnsureConfig(tmux)
	if err != nil {
		t.Fatalf("EnsureConfig() error = %v", err)
	}
	if path != configPath {
		t.Errorf("EnsureConfig() path = %q, want %q", path, configPath)
	}

	// Verify file exists
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		t.Error("EnsureConfig() did not create config file")
	}

	// Verify content
	content, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(content), "mouse on") {
		t.Error("EnsureConfig() config missing expected content")
	}

	// Modify the file
	modified := "# user modified\n"
	if err := os.WriteFile(configPath, []byte(modified), 0644); err != nil {
		t.Fatal(err)
	}

	// Second call should NOT overwrite
	path2, err := EnsureConfig(tmux)
	if err != nil {
		t.Fatalf("EnsureConfig() second call error = %v", err)
	}
	if path2 != configPath {
		t.Errorf("EnsureConfig() second call path = %q, want %q", path2, configPath)
	}

	// Verify content was NOT overwritten
	content2, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(content2) != modified {
		t.Error("EnsureConfig() overwrote user-modified config")
	}
}

func TestListAvailable(t *testing.T) {
	available := ListAvailable()
	// We can't guarantee tmux is installed, but the function should not panic
	t.Logf("Available multiplexers: %v", available)
}

// Backward compatibility tests

func TestBackwardCompat_TmuxAvailable(t *testing.T) {
	// Just verify it doesn't panic
	_ = TmuxAvailable()
}

func TestBackwardCompat_RequireTmux(t *testing.T) {
	err := RequireTmux()
	// Either nil (tmux installed) or ErrTmuxNotAvailable
	if err != nil && err != ErrTmuxNotAvailable {
		t.Errorf("RequireTmux() unexpected error: %v", err)
	}
}
