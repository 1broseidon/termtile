package agent

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// LogLevel defines the logging verbosity.
type LogLevel int

const (
	LevelDebug LogLevel = iota
	LevelInfo
	LevelWarn
	LevelError
)

// ActionType represents the type of agent action being logged.
type ActionType string

const (
	ActionSend            ActionType = "SEND"
	ActionRead            ActionType = "READ"
	ActionAddTerminal     ActionType = "ADD-TERMINAL"
	ActionRemoveTerminal  ActionType = "REMOVE-TERMINAL"
	ActionWorkspaceNew    ActionType = "WORKSPACE-NEW"
	ActionWorkspaceClose  ActionType = "WORKSPACE-CLOSE"
)

// actionLevel returns the log level for an action type.
func actionLevel(action ActionType) LogLevel {
	switch action {
	case ActionSend, ActionRead:
		return LevelDebug
	case ActionAddTerminal, ActionRemoveTerminal, ActionWorkspaceNew, ActionWorkspaceClose:
		return LevelInfo
	default:
		return LevelInfo
	}
}

// LogConfig holds configuration for the agent logger.
type LogConfig struct {
	Enabled        bool
	Level          LogLevel
	FilePath       string
	MaxSizeMB      int
	MaxFiles       int
	IncludeContent bool
	PreviewLength  int
}

// Logger handles agent action logging with file rotation.
type Logger struct {
	mu          sync.Mutex
	file        *os.File
	config      LogConfig
	currentSize int64
}

// NewLogger creates a new logger with the given configuration.
func NewLogger(cfg LogConfig) (*Logger, error) {
	if !cfg.Enabled {
		return &Logger{config: cfg}, nil
	}

	// Ensure directory exists
	dir := filepath.Dir(cfg.FilePath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create log directory %s: %w", dir, err)
	}

	// Open or create log file with secure permissions
	f, err := os.OpenFile(cfg.FilePath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		return nil, fmt.Errorf("failed to open log file %s: %w", cfg.FilePath, err)
	}

	// Get current file size
	stat, err := f.Stat()
	if err != nil {
		f.Close()
		return nil, fmt.Errorf("failed to stat log file: %w", err)
	}

	return &Logger{
		file:        f,
		config:      cfg,
		currentSize: stat.Size(),
	}, nil
}

// Log records an agent action to the log file.
func (l *Logger) Log(action ActionType, workspace string, slot int, details map[string]interface{}) {
	if l == nil || !l.config.Enabled {
		return
	}

	// Check log level filtering
	if actionLevel(action) < l.config.Level {
		return
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	// Check if file is available
	if l.file == nil {
		return
	}

	// Check if rotation is needed
	maxBytes := int64(l.config.MaxSizeMB) * 1024 * 1024
	if l.currentSize >= maxBytes {
		if err := l.rotate(); err != nil {
			// Log rotation failed, but continue logging
			fmt.Fprintf(os.Stderr, "log rotation failed: %v\n", err)
		}
		// After rotation, check if file is still available
		if l.file == nil {
			return
		}
	}

	// Build log entry
	timestamp := time.Now().Format("2006-01-02 15:04:05")
	var sb strings.Builder
	sb.WriteString(timestamp)
	sb.WriteString(" [")
	sb.WriteString(string(action))
	sb.WriteString("]")

	if workspace != "" {
		sb.WriteString(" workspace=")
		sb.WriteString(workspace)
	}
	if slot >= 0 {
		sb.WriteString(fmt.Sprintf(" slot=%d", slot))
	}

	// Add details in sorted order for consistent output
	if len(details) > 0 {
		keys := make([]string, 0, len(details))
		for k := range details {
			keys = append(keys, k)
		}
		sort.Strings(keys)

		for _, k := range keys {
			v := details[k]
			switch val := v.(type) {
			case string:
				// Quote string values
				sb.WriteString(fmt.Sprintf(" %s=%q", k, val))
			default:
				sb.WriteString(fmt.Sprintf(" %s=%v", k, val))
			}
		}
	}

	sb.WriteString("\n")
	entry := sb.String()

	// Write to file
	n, err := l.file.WriteString(entry)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to write log entry: %v\n", err)
		return
	}
	l.currentSize += int64(n)
}

// Close closes the logger and releases resources.
func (l *Logger) Close() error {
	if l == nil || l.file == nil {
		return nil
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	err := l.file.Close()
	l.file = nil
	return err
}

// rotate performs log file rotation.
func (l *Logger) rotate() error {
	// Close current file
	if l.file != nil {
		l.file.Close()
		l.file = nil
	}

	// Rotate existing files
	// agent-actions.log -> agent-actions.log.1
	// agent-actions.log.1 -> agent-actions.log.2
	// etc.
	// With MaxFiles=3, we keep .1, .2, .3 (3 rotated files)
	basePath := l.config.FilePath
	for i := l.config.MaxFiles; i >= 1; i-- {
		oldPath := fmt.Sprintf("%s.%d", basePath, i)
		newPath := fmt.Sprintf("%s.%d", basePath, i+1)
		if i == l.config.MaxFiles {
			// Delete the oldest file (e.g., .3 when MaxFiles=3)
			os.Remove(oldPath)
		} else {
			// Rename to next number (e.g., .2 -> .3, .1 -> .2)
			os.Rename(oldPath, newPath)
		}
	}

	// Rename current log to .1
	if err := os.Rename(basePath, basePath+".1"); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to rotate log file: %w", err)
	}

	// Open new log file
	f, err := os.OpenFile(basePath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		return fmt.Errorf("failed to open new log file: %w", err)
	}

	l.file = f
	l.currentSize = 0
	return nil
}

// ParseLogLevel converts a string to LogLevel.
func ParseLogLevel(s string) LogLevel {
	switch strings.ToLower(s) {
	case "debug":
		return LevelDebug
	case "info":
		return LevelInfo
	case "warn", "warning":
		return LevelWarn
	case "error":
		return LevelError
	default:
		return LevelInfo
	}
}

// Truncate returns a preview of a string, truncating if necessary.
func Truncate(s string, maxLen int) string {
	if maxLen <= 0 || len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
