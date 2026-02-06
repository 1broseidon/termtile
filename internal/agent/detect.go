package agent

import (
	"fmt"
	"os"
	"strings"
)

// MultiplexerType represents supported multiplexer types
type MultiplexerType string

const (
	MultiplexerAuto   MultiplexerType = "auto"
	MultiplexerTmux   MultiplexerType = "tmux"
	MultiplexerScreen MultiplexerType = "screen"
)

// AutoDetect returns the best available multiplexer
// Detection order: tmux > screen
// Returns nil and ErrMultiplexerNotAvailable if none are installed
func AutoDetect(opts ...MultiplexerOption) (Multiplexer, error) {
	// Try tmux first (preferred)
	tmux := NewTmuxMultiplexer(opts...)
	if tmux.Available() {
		return tmux, nil
	}

	// Screen support is deferred to backlog
	// When implemented, it would be checked here:
	// screen := NewScreenMultiplexer(opts...)
	// if screen.Available() {
	//     return screen, nil
	// }

	return nil, ErrMultiplexerNotAvailable
}

// GetMultiplexer returns a multiplexer by type
// Use MultiplexerAuto to auto-detect the best available option
func GetMultiplexer(mtype MultiplexerType, opts ...MultiplexerOption) (Multiplexer, error) {
	switch mtype {
	case MultiplexerAuto, "":
		return AutoDetect(opts...)

	case MultiplexerTmux:
		tmux := NewTmuxMultiplexer(opts...)
		if !tmux.Available() {
			return nil, ErrTmuxNotAvailable
		}
		return tmux, nil

	case MultiplexerScreen:
		// Screen support deferred to backlog
		return nil, fmt.Errorf("screen multiplexer not yet implemented (use tmux)")

	default:
		return nil, fmt.Errorf("unknown multiplexer type: %s (supported: auto, tmux)", mtype)
	}
}

// ParseMultiplexerType parses a string into MultiplexerType
func ParseMultiplexerType(s string) (MultiplexerType, error) {
	s = strings.ToLower(strings.TrimSpace(s))
	switch s {
	case "", "auto":
		return MultiplexerAuto, nil
	case "tmux":
		return MultiplexerTmux, nil
	case "screen":
		return MultiplexerScreen, nil
	default:
		return "", fmt.Errorf("unknown multiplexer: %s (supported: auto, tmux, screen)", s)
	}
}

// EnsureConfig creates the multiplexer config file if it doesn't exist
// Returns the config path (empty string if config management is disabled)
func EnsureConfig(m Multiplexer) (string, error) {
	configPath := m.ConfigPath()
	if configPath == "" {
		return "", nil
	}

	// Check if config already exists - never overwrite
	if _, err := os.Stat(configPath); err == nil {
		return configPath, nil
	}

	// Create parent directories
	configDir := configPath[:strings.LastIndex(configPath, "/")]
	if err := os.MkdirAll(configDir, 0755); err != nil {
		return "", fmt.Errorf("failed to create config directory: %w", err)
	}

	// Write default config
	content := m.DefaultConfig()
	if err := os.WriteFile(configPath, []byte(content), 0644); err != nil {
		return "", fmt.Errorf("failed to write multiplexer config: %w", err)
	}

	return configPath, nil
}

// ListAvailable returns a list of available multiplexer names
func ListAvailable() []string {
	var available []string

	if NewTmuxMultiplexer().Available() {
		available = append(available, "tmux")
	}

	// Screen check would go here when implemented

	return available
}
