package agent

import (
	"testing"

	"github.com/1broseidon/termtile/internal/config"
)

func TestNewConfigManager(t *testing.T) {
	// Test with default config
	cfg := config.DefaultConfig()
	cm, err := NewConfigManager(cfg)
	if err != nil {
		// Skip if tmux not available
		if err == ErrMultiplexerNotAvailable {
			t.Skip("tmux not available")
		}
		t.Fatalf("NewConfigManager() error = %v", err)
	}

	if cm.Name() != "tmux" {
		t.Errorf("Name() = %q, want %q", cm.Name(), "tmux")
	}

	if !cm.Available() {
		t.Error("Available() = false, want true")
	}
}

func TestNewConfigManager_InvalidMultiplexer(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.AgentMode.Multiplexer = "invalid"

	_, err := NewConfigManager(cfg)
	if err == nil {
		t.Error("NewConfigManager() with invalid multiplexer should return error")
	}
}

func TestConfigManager_EnsureConfigIfEnabled(t *testing.T) {
	cfg := config.DefaultConfig()

	// Set to disabled
	disabled := false
	cfg.AgentMode.ManageMultiplexerConfig = &disabled

	cm, err := NewConfigManager(cfg)
	if err != nil {
		if err == ErrMultiplexerNotAvailable {
			t.Skip("tmux not available")
		}
		t.Fatal(err)
	}

	// Should return empty path when disabled
	path, err := cm.EnsureConfigIfEnabled()
	if err != nil {
		t.Fatalf("EnsureConfigIfEnabled() error = %v", err)
	}
	if path != "" {
		t.Errorf("EnsureConfigIfEnabled() with disabled = %q, want empty", path)
	}
}

func TestAgentMode_GetManageMultiplexerConfig(t *testing.T) {
	// Test nil AgentMode
	var am *config.AgentMode
	if !am.GetManageMultiplexerConfig() {
		t.Error("nil AgentMode.GetManageMultiplexerConfig() = false, want true")
	}

	// Test nil pointer
	am = &config.AgentMode{}
	if !am.GetManageMultiplexerConfig() {
		t.Error("AgentMode with nil pointer.GetManageMultiplexerConfig() = false, want true")
	}

	// Test explicit false
	disabled := false
	am.ManageMultiplexerConfig = &disabled
	if am.GetManageMultiplexerConfig() {
		t.Error("AgentMode with false.GetManageMultiplexerConfig() = true, want false")
	}

	// Test explicit true
	enabled := true
	am.ManageMultiplexerConfig = &enabled
	if !am.GetManageMultiplexerConfig() {
		t.Error("AgentMode with true.GetManageMultiplexerConfig() = false, want true")
	}
}
