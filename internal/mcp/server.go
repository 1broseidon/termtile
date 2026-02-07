package mcp

import (
	"bytes"
	"context"
	"fmt"
	"log"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/1broseidon/termtile/internal/agent"
	"github.com/1broseidon/termtile/internal/config"
	"github.com/1broseidon/termtile/internal/ipc"
	workspacepkg "github.com/1broseidon/termtile/internal/workspace"
)

const (
	DefaultWorkspace = "mcp-agents"
	ServerName       = "termtile"
	ServerVersion    = "0.1.0"
)

// trackedAgent records which agent type occupies a workspace slot.
type trackedAgent struct {
	agentType     string
	tmuxTarget    string // pane ID ("%5") or session target ("termtile-ws-0:0.0")
	spawnMode     string // "pane" or "window"
	taskMarker    string // unique short marker appended to task, used as output delimiter
	responseFence bool   // true if fence instructions were prepended to the task
}

// Server is the MCP server for termtile agent orchestration.
type Server struct {
	mcpServer   *mcpsdk.Server
	config      *config.Config
	multiplexer *agent.TmuxMultiplexer

	mu       sync.Mutex
	tracked  map[string]map[int]trackedAgent // workspace -> slot -> info
	nextSlot map[string]int                  // workspace -> next slot
}

// NewServer creates a new MCP server backed by tmux.
func NewServer(cfg *config.Config) (*Server, error) {
	mux := agent.NewTmuxMultiplexer()
	if !mux.Available() {
		return nil, fmt.Errorf("tmux is required for MCP server but not found in PATH")
	}

	s := &Server{
		config:      cfg,
		multiplexer: mux,
		tracked:     make(map[string]map[int]trackedAgent),
		nextSlot:    make(map[string]int),
	}
	s.reconcile()

	s.mcpServer = mcpsdk.NewServer(
		&mcpsdk.Implementation{
			Name:    ServerName,
			Version: ServerVersion,
		},
		nil,
	)

	s.registerTools()
	return s, nil
}

// Run starts the MCP server on stdio transport, blocking until done.
func (s *Server) Run(ctx context.Context) error {
	return s.mcpServer.Run(ctx, &mcpsdk.StdioTransport{})
}

func (s *Server) registerTools() {
	mcpsdk.AddTool(s.mcpServer, &mcpsdk.Tool{
		Name:        "spawn_agent",
		Description: "Spawn a new AI agent in a terminal slot. The agent type must be configured in termtile's agents config. Returns the slot number for future reference.",
	}, s.handleSpawnAgent)

	mcpsdk.AddTool(s.mcpServer, &mcpsdk.Tool{
		Name:        "send_to_agent",
		Description: "Send text input to an agent running in a specific terminal slot. The text is sent followed by Enter.",
	}, s.handleSendToAgent)

	mcpsdk.AddTool(s.mcpServer, &mcpsdk.Tool{
		Name:        "read_from_agent",
		Description: "Read the current terminal output from an agent's slot. Returns the last N lines of visible output. Optionally wait for a specific text pattern by setting pattern and timeout.",
	}, s.handleReadFromAgent)

	mcpsdk.AddTool(s.mcpServer, &mcpsdk.Tool{
		Name:        "wait_for_idle",
		Description: "Wait for an agent to become idle (finished processing). Polls until the agent's prompt reappears or timeout. Returns cleaned output suitable for parsing.",
	}, s.handleWaitForIdle)

	mcpsdk.AddTool(s.mcpServer, &mcpsdk.Tool{
		Name:        "list_agents",
		Description: "List all running agents in a workspace with their status (idle/busy, current command).",
	}, s.handleListAgents)

	mcpsdk.AddTool(s.mcpServer, &mcpsdk.Tool{
		Name:        "kill_agent",
		Description: "Kill an agent running in a specific terminal slot by destroying its tmux session.",
	}, s.handleKillAgent)
}

// reconcile rebuilds startup tracking state from existing termtile tmux sessions.
func (s *Server) reconcile() {
	out, err := exec.Command("tmux", "list-sessions", "-F", "#{session_name}").Output()
	if err != nil {
		return
	}
	s.reconcileSessionNames(strings.Split(strings.TrimSpace(string(out)), "\n"))
}

// reconcileSessionNames applies reconcile logic over a provided session list.
func (s *Server) reconcileSessionNames(sessionNames []string) {
	maxSlots := make(map[string]int)

	s.mu.Lock()
	defer s.mu.Unlock()

	for _, sessionName := range sessionNames {
		sessionName = strings.TrimSpace(sessionName)
		if !strings.HasPrefix(sessionName, "termtile-") {
			continue
		}

		trimmed := strings.TrimPrefix(sessionName, "termtile-")
		lastDash := strings.LastIndex(trimmed, "-")
		if lastDash <= 0 || lastDash == len(trimmed)-1 {
			continue
		}

		workspace := trimmed[:lastDash]
		slot, err := strconv.Atoi(trimmed[lastDash+1:])
		if err != nil || slot < 0 {
			continue
		}
		isInRegistry := workspacepkg.HasSessionInRegistry(sessionName)

		if max, ok := maxSlots[workspace]; !ok || slot > max {
			maxSlots[workspace] = slot
		}

		// Registry-backed sessions are managed by workspace state and are not
		// orphan MCP sessions to recover into the in-memory tracked map.
		if isInRegistry {
			continue
		}

		if s.tracked[workspace] == nil {
			s.tracked[workspace] = make(map[int]trackedAgent)
		}
		s.tracked[workspace][slot] = trackedAgent{
			agentType:  "unknown",
			tmuxTarget: agent.TargetForSession(sessionName),
			spawnMode:  "window",
		}

	}

	for workspace, max := range maxSlots {
		next := max + 1
		if next > s.nextSlot[workspace] {
			s.nextSlot[workspace] = next
		}
	}
}

// resolveSpawnMode determines the spawn mode from the request and agent config.
// Priority: explicit Window param > agent's SpawnMode config > default "pane".
func resolveSpawnMode(window *bool, agentSpawnMode string) string {
	if window != nil {
		if *window {
			return "window"
		}
		return "pane"
	}
	if agentSpawnMode == "window" {
		return "window"
	}
	return "pane"
}

// anyPaneModeTarget returns the tmux target of a live pane-mode agent in the
// workspace, or empty string if none exist. Only considers pane-mode agents
// (needed for split-window source). Prunes dead targets encountered along the way.
func (s *Server) anyPaneModeTarget(workspace string) string {
	s.mu.Lock()
	defer s.mu.Unlock()

	ws := s.tracked[workspace]
	for slot, ta := range ws {
		if ta.spawnMode == "window" {
			continue
		}
		if tmuxTargetExists(ta.tmuxTarget) {
			return ta.tmuxTarget
		}
		// Target was killed externally — prune it.
		delete(ws, slot)
	}
	return ""
}

// tmuxTargetExists checks whether a tmux target (pane ID or session) is still alive.
func tmuxTargetExists(target string) bool {
	return exec.Command("tmux", "display-message", "-t", target, "-p", "").Run() == nil
}

// findAttachedSession returns the name of the most recently active attached
// tmux session, or empty string if none found.
func findAttachedSession() string {
	// List attached clients sorted by activity (most recent first).
	cmd := exec.Command("tmux", "list-clients", "-F", "#{session_name}")
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	for _, line := range lines {
		name := strings.TrimSpace(line)
		if name != "" {
			return name
		}
	}
	return ""
}

// peekNextSlot returns the next slot number for a workspace without incrementing.
func (s *Server) peekNextSlot(workspace string) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.nextSlot[workspace]
}

// allocateSlot returns the next available slot for a workspace and tracks the agent.
func (s *Server) allocateSlot(workspace, agentType, tmuxTarget, spawnMode, taskMarker string, responseFence bool) int {
	s.mu.Lock()
	defer s.mu.Unlock()

	slot := s.nextSlot[workspace]
	s.nextSlot[workspace] = slot + 1

	if s.tracked[workspace] == nil {
		s.tracked[workspace] = make(map[int]trackedAgent)
	}
	s.tracked[workspace][slot] = trackedAgent{
		agentType:     agentType,
		tmuxTarget:    tmuxTarget,
		spawnMode:     spawnMode,
		taskMarker:    taskMarker,
		responseFence: responseFence,
	}

	return slot
}

// updateTmuxTarget updates the tmux target for a tracked slot.
func (s *Server) updateTmuxTarget(workspace string, slot int, tmuxTarget string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	ws := s.tracked[workspace]
	if ws == nil {
		return
	}
	ta, ok := ws[slot]
	if !ok {
		return
	}
	ta.tmuxTarget = tmuxTarget
	ws[slot] = ta
}

// removeTracked removes a slot from the tracking map.
func (s *Server) removeTracked(workspace string, slot int) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if ws, ok := s.tracked[workspace]; ok {
		delete(ws, slot)
	}
}

// getTracked returns all tracked agents for a workspace.
func (s *Server) getTracked(workspace string) map[int]trackedAgent {
	s.mu.Lock()
	defer s.mu.Unlock()

	ws := s.tracked[workspace]
	if ws == nil {
		return nil
	}
	// Return a copy to avoid races.
	out := make(map[int]trackedAgent, len(ws))
	for k, v := range ws {
		out[k] = v
	}
	return out
}

// getTmuxTarget returns the tmux target for a tracked slot.
func (s *Server) getTmuxTarget(workspace string, slot int) (string, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	ws := s.tracked[workspace]
	if ws == nil {
		return "", false
	}
	ta, ok := ws[slot]
	if !ok {
		return "", false
	}
	return ta.tmuxTarget, true
}

// getSpawnMode returns the spawn mode for a tracked slot.
func (s *Server) getSpawnMode(workspace string, slot int) string {
	s.mu.Lock()
	defer s.mu.Unlock()

	ws := s.tracked[workspace]
	if ws == nil {
		return ""
	}
	ta, ok := ws[slot]
	if !ok {
		return ""
	}
	return ta.spawnMode
}

// shellQuote wraps s in single quotes for safe shell embedding.
func shellQuote(s string) string {
	if s == "" {
		return "''"
	}
	if !strings.ContainsAny(s, " \t\r\n'\"\\$`(){}[]*?!;|&<>") {
		return s
	}
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// checkIdle determines whether an agent in a tmux target is idle.
// It uses a two-tier strategy:
//  1. Content-based: if the agent has an IdlePattern configured, scan the last
//     few non-empty lines for the pattern. We check multiple lines because TUI
//     agents (e.g., Claude Code) render status bars below the input prompt.
//  2. Process-based fallback: if the pane's current command is a shell,
//     check whether it has any child processes.
func (s *Server) checkIdle(target, agentType, workspace string, slot int) bool {
	out, err := tmuxCapturePane(target, 30)
	if err != nil {
		return false
	}

	// Tier 0: response fence detection. When a task was sent with
	// response_fence enabled, the closing [/termtile-response] tag is
	// the most reliable "done" signal — it only appears when the agent
	// has finished and wrapped its final answer.
	//
	// However, the fence instruction text itself contains the literal
	// tags ("...inside [termtile-response] and [/termtile-response] tags...").
	// We must only match the close tag if it appears AFTER the marker
	// delimiter, which separates the instruction/task from the response.
	marker, hasFence := s.getOutputConfig(workspace, slot)
	if hasFence {
		if marker != "" {
			markerIdx := strings.LastIndex(out, marker)
			if markerIdx >= 0 && strings.Contains(out[markerIdx:], fenceClose) {
				return true
			}
		} else if strings.Contains(out, fenceClose) {
			return true
		}
		// Fence expected but not found — the agent is still working.
		// Do NOT fall through to Tier 1/2 which can false-positive on
		// startup tips, rate limit prompts, etc. that match the idle pattern.
		return false
	}

	// Tier 1: content-based detection via IdlePattern.
	if agentCfg, ok := s.config.Agents[agentType]; ok && agentCfg.IdlePattern != "" {
		return containsIdlePattern(out, agentCfg.IdlePattern)
	}

	// Tier 2: process-based detection for shell agents.
	cmd := exec.Command("tmux", "display-message", "-t", target, "-p", "#{pane_pid}")
	pidOut, err := cmd.Output()
	if err != nil {
		return false
	}
	pid := strings.TrimSpace(string(pidOut))
	if pid == "" {
		return false
	}

	// Check if the pane's process has any children.
	pgrepCmd := exec.Command("pgrep", "-P", pid)
	if err := pgrepCmd.Run(); err != nil {
		// pgrep exits non-zero when no children found → idle.
		return true
	}
	return false
}

// lastNonEmptyLine returns the last non-blank line from text.
func lastNonEmptyLine(text string) string {
	lines := strings.Split(text, "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		if trimmed := strings.TrimSpace(lines[i]); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

// containsIdlePattern scans the last few non-empty lines of text for the
// idle pattern. The pattern must appear at the START of a short line
// (under 40 chars) to avoid false positives from hint/help text that
// contains the same character (e.g. codex shows "› Use /skills..." while
// actively working, but the actual idle prompt is just "›" on its own).
func containsIdlePattern(text, pattern string) bool {
	lines := strings.Split(text, "\n")
	checked := 0
	for i := len(lines) - 1; i >= 0 && checked < 5; i-- {
		trimmed := strings.TrimSpace(lines[i])
		if trimmed == "" {
			continue
		}
		// The idle prompt is typically just the pattern character (possibly
		// followed by a short user-typed prefix). Hint lines are long
		// sentences that happen to start with the same character.
		if strings.HasPrefix(trimmed, pattern) && len(trimmed) < 40 {
			return true
		}
		checked++
	}
	return false
}

// getOutputConfig returns the task marker and responseFence flag for a tracked slot.
func (s *Server) getOutputConfig(workspace string, slot int) (taskMarker string, responseFence bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	ws := s.tracked[workspace]
	if ws == nil {
		return "", false
	}
	ta, ok := ws[slot]
	if !ok {
		return "", false
	}
	return ta.taskMarker, ta.responseFence
}

// updateOutputConfig updates the output parsing config for a tracked slot.
func (s *Server) updateOutputConfig(workspace string, slot int, taskMarker string, responseFence bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	ws := s.tracked[workspace]
	if ws == nil {
		return
	}
	ta, ok := ws[slot]
	if !ok {
		return
	}
	ta.taskMarker = taskMarker
	ta.responseFence = responseFence
	ws[slot] = ta
}

// getAgentType returns the agent type for a tracked slot.
func (s *Server) getAgentType(workspace string, slot int) string {
	s.mu.Lock()
	defer s.mu.Unlock()

	ws := s.tracked[workspace]
	if ws == nil {
		return ""
	}
	ta, ok := ws[slot]
	if !ok {
		return ""
	}
	return ta.agentType
}

// triggerRetile asks the termtile daemon to re-tile all terminal windows using
// the currently active layout. This is best-effort: if the daemon is not
// running the error is logged and silently ignored.
func (s *Server) triggerRetile() {
	client := ipc.NewClient()

	// Determine layout: prefer daemon's active layout, fall back to config default.
	layoutName := s.config.DefaultLayout
	if status, err := client.GetStatus(); err == nil && status.ActiveLayout != "" {
		layoutName = status.ActiveLayout
	}

	if err := client.ApplyLayout(layoutName, true); err != nil {
		log.Printf("auto-tile: failed to re-tile (%s): %v", layoutName, err)
	}
}

// --- tmux target helpers ---
// These bypass the multiplexer (which targets sessions) and operate on tmux targets directly.

// tmuxSendKeys sends text followed by Enter to a specific tmux target.
func tmuxSendKeys(target, text string) error {
	// Send text with -l (literal) flag to avoid key name interpretation.
	cmd := exec.Command("tmux", "send-keys", "-l", "-t", target, text)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("tmux send-keys failed: %w (%s)", err, strings.TrimSpace(string(out)))
	}

	// Delay to allow terminal/TUI to process and render the text before Enter.
	// TUI apps (Claude Code, etc.) need time to update their input state.
	delay := 200 * time.Millisecond
	if len(text) > 500 {
		extra := time.Duration(len(text)/100) * time.Millisecond
		delay += extra
		if delay > 1*time.Second {
			delay = 1 * time.Second
		}
	}
	time.Sleep(delay)

	// Send Enter as a key name (without -l).
	cmd = exec.Command("tmux", "send-keys", "-t", target, "Enter")
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("tmux send-keys (Enter) failed: %w (%s)", err, strings.TrimSpace(string(out)))
	}
	return nil
}

// tmuxCapturePane captures the last N lines from a specific tmux target.
func tmuxCapturePane(target string, lines int) (string, error) {
	args := []string{"capture-pane", "-p", "-t", target}
	if lines > 0 {
		args = append(args, "-S", fmt.Sprintf("-%d", lines))
	}
	cmd := exec.Command("tmux", args...)
	var stdout, stderr bytes.Buffer
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

// tmuxWaitFor polls a tmux target's output until pattern is found or timeout.
func tmuxWaitFor(target, pattern string, timeout time.Duration, lines int) (string, error) {
	if strings.TrimSpace(pattern) == "" {
		return "", fmt.Errorf("pattern is required")
	}
	if timeout <= 0 {
		timeout = 10 * time.Second
	}

	deadline := time.Now().Add(timeout)
	for {
		out, err := tmuxCapturePane(target, lines)
		if err != nil {
			return "", err
		}
		if strings.Contains(out, pattern) {
			return out, nil
		}
		if time.Now().After(deadline) {
			return out, fmt.Errorf("timeout waiting for %q after %s", pattern, timeout)
		}
		time.Sleep(250 * time.Millisecond)
	}
}
