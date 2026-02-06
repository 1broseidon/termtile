package agent

import (
	"fmt"

	"github.com/1broseidon/termtile/internal/config"
)

// ConfigManager handles multiplexer configuration and initialization
type ConfigManager struct {
	multiplexer Multiplexer
	cfg         *config.Config
}

// NewConfigManager creates a new ConfigManager from the application config
func NewConfigManager(cfg *config.Config) (*ConfigManager, error) {
	if cfg == nil {
		cfg = config.DefaultConfig()
	}

	// Parse multiplexer type from config
	mtype, err := ParseMultiplexerType(cfg.AgentMode.Multiplexer)
	if err != nil {
		return nil, fmt.Errorf("invalid agent_mode.multiplexer: %w", err)
	}

	// Get the multiplexer implementation
	m, err := GetMultiplexer(mtype)
	if err != nil {
		return nil, err
	}

	return &ConfigManager{
		multiplexer: m,
		cfg:         cfg,
	}, nil
}

// Multiplexer returns the configured multiplexer
func (cm *ConfigManager) Multiplexer() Multiplexer {
	return cm.multiplexer
}

// EnsureConfigIfEnabled creates the multiplexer config file if:
// 1. manage_multiplexer_config is enabled (default: true)
// 2. The config file doesn't already exist
// Returns the config path, or empty string if config management is disabled
func (cm *ConfigManager) EnsureConfigIfEnabled() (string, error) {
	if !cm.cfg.AgentMode.GetManageMultiplexerConfig() {
		return "", nil
	}
	return EnsureConfig(cm.multiplexer)
}

// SessionCommand returns the command to create/attach to a session
// This will include the -f flag for the config file if it exists
func (cm *ConfigManager) SessionCommand(session string) string {
	return cm.multiplexer.SessionCommand(session)
}

// GetConfigPath returns the path where the multiplexer config is stored
func (cm *ConfigManager) GetConfigPath() string {
	return cm.multiplexer.ConfigPath()
}

// Initialize sets up the multiplexer environment
// Call this before creating agent mode sessions
func (cm *ConfigManager) Initialize() error {
	_, err := cm.EnsureConfigIfEnabled()
	return err
}

// Available returns true if the configured multiplexer is available
func (cm *ConfigManager) Available() bool {
	return cm.multiplexer.Available()
}

// Name returns the name of the configured multiplexer
func (cm *ConfigManager) Name() string {
	return cm.multiplexer.Name()
}
