package agent

import (
	"errors"
	"time"
)

// ErrMultiplexerNotAvailable is returned when no multiplexer is installed
var ErrMultiplexerNotAvailable = errors.New("no terminal multiplexer available (install tmux)")

// Multiplexer defines the interface for terminal multiplexer implementations.
// This abstraction allows termtile to work with tmux, screen, zellij, etc.
type Multiplexer interface {
	// Name returns the multiplexer name (e.g., "tmux", "screen")
	Name() string

	// Available returns true if this multiplexer is installed and usable
	Available() bool

	// HasSession checks if a session with the given name exists
	HasSession(session string) (bool, error)

	// CreateSession creates a new session with the given name
	// Returns the command string to spawn a terminal attached to this session
	CreateSession(session string) (string, error)

	// SendKeys sends text followed by Enter to the session
	SendKeys(session, text string) error

	// CapturePane captures the last N lines of output from the session
	// If lines is 0, captures the visible pane content
	CapturePane(session string, lines int) (string, error)

	// WaitFor polls the session output until pattern is found or timeout
	WaitFor(session, pattern string, timeout time.Duration, lines int) (string, error)

	// SessionCommand returns the command to attach to an existing session
	// or create it if it doesn't exist (e.g., "tmux new-session -A -s <name>")
	SessionCommand(session string) string

	// ConfigPath returns the path where this multiplexer's config should be stored
	// Returns empty string if config management is not supported
	ConfigPath() string

	// DefaultConfig returns the default config content for agent mode
	// This config optimizes for the agentic workflow (scroll UX, history, etc.)
	DefaultConfig() string
}

// MultiplexerOption configures multiplexer behavior
type MultiplexerOption func(*multiplexerOptions)

type multiplexerOptions struct {
	configPath string // Override default config path
}

// WithConfigPath overrides the default config file path
func WithConfigPath(path string) MultiplexerOption {
	return func(o *multiplexerOptions) {
		o.configPath = path
	}
}

func applyOptions(opts []MultiplexerOption) *multiplexerOptions {
	o := &multiplexerOptions{}
	for _, opt := range opts {
		opt(o)
	}
	return o
}
