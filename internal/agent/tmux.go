package agent

import (
	"bytes"
	_ "embed"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

//go:embed templates/tmux.conf.tmpl
var defaultTmuxConfig string

// ErrTmuxNotAvailable is returned when tmux is not installed
var ErrTmuxNotAvailable = errors.New("tmux is not available in PATH")

// SessionStatus represents the current state of a tmux session
type SessionStatus struct {
	Exists         bool   `json:"exists"`
	CurrentCommand string `json:"current_command,omitempty"`
	IsIdle         bool   `json:"is_idle"`
	PanePID        int    `json:"pane_pid,omitempty"`
}

// TmuxMultiplexer implements the Multiplexer interface for tmux
type TmuxMultiplexer struct {
	configPath string
}

// NewTmuxMultiplexer creates a new tmux multiplexer instance
func NewTmuxMultiplexer(opts ...MultiplexerOption) *TmuxMultiplexer {
	o := applyOptions(opts)
	t := &TmuxMultiplexer{}
	if o.configPath != "" {
		t.configPath = o.configPath
	}
	return t
}

// Name returns "tmux"
func (t *TmuxMultiplexer) Name() string {
	return "tmux"
}

// Available returns true if tmux is installed
func (t *TmuxMultiplexer) Available() bool {
	_, err := exec.LookPath("tmux")
	return err == nil
}

// HasSession checks if a tmux session exists
func (t *TmuxMultiplexer) HasSession(session string) (bool, error) {
	if !t.Available() {
		return false, ErrTmuxNotAvailable
	}
	cmd := exec.Command("tmux", "has-session", "-t", session)
	if err := cmd.Run(); err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) && exitErr.ExitCode() == 1 {
			return false, nil
		}
		return false, fmt.Errorf("tmux has-session failed: %w", err)
	}
	return true, nil
}

// GetSessionStatus queries the status of a tmux session
func (t *TmuxMultiplexer) GetSessionStatus(session string) (SessionStatus, error) {
	if !t.Available() {
		return SessionStatus{}, ErrTmuxNotAvailable
	}

	// Check if session exists
	exists, err := t.HasSession(session)
	if err != nil {
		return SessionStatus{}, err
	}
	if !exists {
		return SessionStatus{Exists: false}, nil
	}

	// Get pane info using tmux list-panes
	target := t.targetForSession(session)
	cmd := exec.Command("tmux", "list-panes", "-t", target,
		"-F", "#{pane_pid}:#{pane_current_command}")
	out, err := cmd.Output()
	if err != nil {
		return SessionStatus{Exists: true}, err
	}

	// Parse output "PID:COMMAND"
	parts := strings.SplitN(strings.TrimSpace(string(out)), ":", 2)
	if len(parts) != 2 {
		return SessionStatus{Exists: true}, nil
	}

	pid, _ := strconv.Atoi(parts[0])
	currentCmd := parts[1]

	// Determine if shell is idle (no child processes)
	isIdle := isShellIdle(pid, currentCmd)

	return SessionStatus{
		Exists:         true,
		CurrentCommand: currentCmd,
		IsIdle:         isIdle,
		PanePID:        pid,
	}, nil
}

// isShellIdle checks if a shell process has no running children
func isShellIdle(pid int, cmd string) bool {
	shells := map[string]bool{
		"bash": true, "zsh": true, "sh": true, "fish": true,
		"dash": true, "ksh": true, "tcsh": true, "csh": true,
	}
	if !shells[cmd] {
		return false // Running something other than a shell
	}
	// Check for child processes using pgrep -P
	out, err := exec.Command("pgrep", "-P", strconv.Itoa(pid)).Output()
	return err != nil || len(strings.TrimSpace(string(out))) == 0
}

// CreateSession creates a new tmux session and returns the attach command
func (t *TmuxMultiplexer) CreateSession(session string) (string, error) {
	if !t.Available() {
		return "", ErrTmuxNotAvailable
	}
	// Return the command that will create-or-attach to the session
	return t.SessionCommand(session), nil
}

// SendKeys sends text followed by Enter to a tmux session
func (t *TmuxMultiplexer) SendKeys(session, text string) error {
	if !t.Available() {
		return ErrTmuxNotAvailable
	}
	target := t.targetForSession(session)

	// Send text first
	cmd := exec.Command("tmux", "send-keys", "-t", target, text)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("tmux send-keys failed: %w (%s)", err, strings.TrimSpace(string(out)))
	}

	// Delay to allow terminal to process text before Enter
	// Scale based on text length: AI CLI tools convert long pastes to
	// '[pasted X chars]' format which takes time to process
	// Base: 50ms, add 1ms per 100 chars for long text, cap at 500ms
	delay := 50 * time.Millisecond
	if len(text) > 500 {
		extraDelay := time.Duration(len(text)/100) * time.Millisecond
		delay += extraDelay
		if delay > 500*time.Millisecond {
			delay = 500 * time.Millisecond
		}
	}
	time.Sleep(delay)

	// Send Enter separately
	cmd = exec.Command("tmux", "send-keys", "-t", target, "Enter")
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("tmux send-keys (Enter) failed: %w (%s)", err, strings.TrimSpace(string(out)))
	}
	return nil
}

// CapturePane captures output from a tmux pane
func (t *TmuxMultiplexer) CapturePane(session string, lines int) (string, error) {
	if !t.Available() {
		return "", ErrTmuxNotAvailable
	}
	target := t.targetForSession(session)
	args := []string{"capture-pane", "-p", "-t", target}
	if lines > 0 {
		args = append(args, "-S", fmt.Sprintf("-%d", lines))
	}
	cmd := exec.Command("tmux", args...)
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg != "" {
			return "", fmt.Errorf("tmux capture-pane failed: %w (%s)", err, msg)
		}
		return "", fmt.Errorf("tmux capture-pane failed: %w", err)
	}
	return stdout.String(), nil
}

// WaitFor polls session output until pattern is found or timeout
func (t *TmuxMultiplexer) WaitFor(session, pattern string, timeout time.Duration, lines int) (string, error) {
	if !t.Available() {
		return "", ErrTmuxNotAvailable
	}
	pattern = strings.TrimSpace(pattern)
	if pattern == "" {
		return "", fmt.Errorf("--wait-for pattern is required")
	}
	if timeout <= 0 {
		timeout = 10 * time.Second
	}

	deadline := time.Now().Add(timeout)
	for {
		out, err := t.CapturePane(session, lines)
		if err != nil {
			return "", err
		}
		if strings.Contains(out, pattern) {
			return out, nil
		}
		if time.Now().After(deadline) {
			return out, fmt.Errorf("timeout waiting for %q in slot output after %s", pattern, timeout)
		}
		time.Sleep(250 * time.Millisecond)
	}
}

// SessionCommand returns the tmux command to create-or-attach to a session
// Note: -f flag only applies when tmux server starts fresh. For existing servers,
// we also source the config to ensure settings are applied.
func (t *TmuxMultiplexer) SessionCommand(session string) string {
	configPath := t.ConfigPath()
	if configPath != "" {
		// Check if config file exists before using it
		if _, err := os.Stat(configPath); err == nil {
			// Use -f for a fresh server start and always source the config before attaching.
			// Note: when tmux is invoked via exec (no shell), `\;` must be unescaped to `;`.
			// The workspace loader uses a proper command parser to handle this.
			return fmt.Sprintf("tmux -f %s source-file %s \\; new-session -A -s %s", configPath, configPath, session)
		}
	}
	return fmt.Sprintf("tmux new-session -A -s %s", session)
}

// ConfigPath returns the path for termtile's tmux config
func (t *TmuxMultiplexer) ConfigPath() string {
	if t.configPath != "" {
		return t.configPath
	}
	configDir := os.Getenv("XDG_CONFIG_HOME")
	if configDir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return ""
		}
		configDir = filepath.Join(home, ".config")
	}
	return filepath.Join(configDir, "termtile", "tmux.conf")
}

// DefaultConfig returns the default tmux config optimized for agent workflows
// The config is embedded from templates/tmux.conf
func (t *TmuxMultiplexer) DefaultConfig() string {
	return defaultTmuxConfig
}

// targetForSession returns the tmux target string for a session
func (t *TmuxMultiplexer) targetForSession(session string) string {
	return session + ":0.0"
}

// RenameSession renames a tmux session.
func (t *TmuxMultiplexer) RenameSession(oldName, newName string) error {
	if !t.Available() {
		return ErrTmuxNotAvailable
	}
	cmd := exec.Command("tmux", "rename-session", "-t", oldName, newName)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("tmux rename-session failed: %s: %w", strings.TrimSpace(string(out)), err)
	}
	return nil
}

// ListSessions returns all tmux session names.
func (t *TmuxMultiplexer) ListSessions() ([]string, error) {
	if !t.Available() {
		return nil, ErrTmuxNotAvailable
	}
	cmd := exec.Command("tmux", "list-sessions", "-F", "#{session_name}")
	output, err := cmd.Output()
	if err != nil {
		// tmux returns exit code 1 if no sessions exist
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) && exitErr.ExitCode() == 1 {
			return nil, nil
		}
		return nil, fmt.Errorf("tmux list-sessions failed: %w", err)
	}

	lines := strings.Split(strings.TrimSpace(string(output)), "\n")
	var sessions []string
	for _, line := range lines {
		if line != "" {
			sessions = append(sessions, line)
		}
	}
	return sessions, nil
}

// KillSession kills a tmux session by name.
func (t *TmuxMultiplexer) KillSession(name string) error {
	if !t.Available() {
		return ErrTmuxNotAvailable
	}
	cmd := exec.Command("tmux", "kill-session", "-t", name)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("tmux kill-session failed: %s: %w", strings.TrimSpace(string(out)), err)
	}
	return nil
}

// Backward compatibility functions - these wrap the TmuxMultiplexer methods
// for code that hasn't been updated to use the interface yet

var defaultTmux = NewTmuxMultiplexer()

// TmuxAvailable returns true if tmux is installed (backward compat)
func TmuxAvailable() bool {
	return defaultTmux.Available()
}

// RequireTmux returns an error if tmux is not available (backward compat)
func RequireTmux() error {
	if defaultTmux.Available() {
		return nil
	}
	return ErrTmuxNotAvailable
}

// HasSession checks if a tmux session exists (backward compat)
func HasSession(session string) (bool, error) {
	return defaultTmux.HasSession(session)
}

// SendKeys sends text to a tmux session (backward compat)
func SendKeys(session, text string) error {
	return defaultTmux.SendKeys(session, text)
}

// CapturePane captures tmux pane output (backward compat)
func CapturePane(session string, lines int) (string, error) {
	return defaultTmux.CapturePane(session, lines)
}

// WaitFor waits for pattern in tmux output (backward compat)
func WaitFor(session, pattern string, timeout time.Duration, lines int) (string, error) {
	return defaultTmux.WaitFor(session, pattern, timeout, lines)
}

// GetSessionStatus queries the status of a tmux session (backward compat)
func GetSessionStatus(session string) (SessionStatus, error) {
	return defaultTmux.GetSessionStatus(session)
}

// ListSessions returns all tmux session names (backward compat)
func ListSessions() ([]string, error) {
	return defaultTmux.ListSessions()
}

// KillSession kills a tmux session by name (backward compat)
func KillSession(name string) error {
	return defaultTmux.KillSession(name)
}
