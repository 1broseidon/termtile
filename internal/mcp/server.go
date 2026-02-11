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
	agentType      string
	tmuxTarget     string // pane ID ("%5") or session target ("termtile-ws-0:0.0")
	spawnMode      string // "pane" or "window"
	responseFence  bool   // true if fence instructions were prepended to the task
	fencePairCount int    // baseline count of standalone close tags at last task send
	// lastArtifactCloseCount tracks the last close-tag count for which we captured
	// and stored an artifact. Used to avoid repeatedly capturing full scrollback
	// on every checkIdle poll once the agent is idle.
	lastArtifactCloseCount int
	pipeFilePath           string // path to pipe-pane output file; empty = not active
	lastPipeSize           int64  // last stat'd file size for cheap change detection
}

// Server is the MCP server for termtile agent orchestration.
type Server struct {
	mcpServer   *mcpsdk.Server
	config      *config.Config
	multiplexer *agent.TmuxMultiplexer
	logger      *agent.Logger
	artifacts   *ArtifactStore

	mu       sync.Mutex
	tracked  map[string]map[int]trackedAgent // workspace -> slot -> info
	nextSlot map[string]int                  // legacy; slot allocation now uses lowest free tracked slot
	// readSnapshots stores the most recent read_from_agent output per workspace/slot.
	readSnapshots map[string]map[int]string // workspace -> slot -> output snapshot

	// Dependency waiting hooks (primarily for tests).
	idleCheckFn     func(target, agentType, workspace string, slot int) bool
	targetExistsFn  func(target string) bool
	depPollInterval time.Duration
}

// NewServer creates a new MCP server backed by tmux.
func NewServer(cfg *config.Config) (*Server, error) {
	mux := agent.NewTmuxMultiplexer()
	if !mux.Available() {
		return nil, fmt.Errorf("tmux is required for MCP server but not found in PATH")
	}

	logCfg := cfg.GetLoggingConfig()
	var logger *agent.Logger
	if logCfg.Enabled {
		var err error
		logger, err = agent.NewLogger(agent.LogConfig{
			Enabled:        logCfg.Enabled,
			Level:          agent.ParseLogLevel(logCfg.Level),
			FilePath:       logCfg.File,
			MaxSizeMB:      logCfg.MaxSizeMB,
			MaxFiles:       logCfg.MaxFiles,
			IncludeContent: logCfg.IncludeContent,
			PreviewLength:  logCfg.PreviewLength,
		})
		if err != nil {
			log.Printf("Warning: failed to initialize MCP logger: %v", err)
			logger = nil
		}
	}

	s := &Server{
		config:          cfg,
		multiplexer:     mux,
		logger:          logger,
		artifacts:       NewArtifactStore(DefaultArtifactCapBytes),
		tracked:         make(map[string]map[int]trackedAgent),
		nextSlot:        make(map[string]int),
		readSnapshots:   make(map[string]map[int]string),
		targetExistsFn:  tmuxTargetExists,
		depPollInterval: 2 * time.Second,
	}
	s.idleCheckFn = s.checkIdle
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

// Close releases server resources.
func (s *Server) Close() error {
	if s == nil || s.logger == nil {
		return nil
	}
	return s.logger.Close()
}

func (s *Server) registerTools() {
	mcpsdk.AddTool(s.mcpServer, &mcpsdk.Tool{
		Name:        "spawn_agent",
		Description: "Spawn a new AI agent in a terminal slot. The agent type must be configured in termtile's agents config. Uses the active workspace by default; pass workspace explicitly when no active workspace is available. Optionally wait for other slots to become idle first via depends_on (polling every 2s up to depends_on_timeout, default 300s). Returns the slot number for future reference.",
	}, s.handleSpawnAgent)

	mcpsdk.AddTool(s.mcpServer, &mcpsdk.Tool{
		Name:        "send_to_agent",
		Description: "Send text input to an agent running in a specific terminal slot. The text is sent followed by Enter.",
	}, s.handleSendToAgent)

	mcpsdk.AddTool(s.mcpServer, &mcpsdk.Tool{
		Name:        "read_from_agent",
		Description: "Read the current terminal output from an agent's slot. Returns a bounded tail window (default 50 lines, max 100). Optionally wait for a specific text pattern or return only output since the previous read via since_last.",
	}, s.handleReadFromAgent)

	mcpsdk.AddTool(s.mcpServer, &mcpsdk.Tool{
		Name:        "wait_for_idle",
		Description: "Wait for an agent to become idle (finished processing). Polls until the agent's prompt reappears or timeout. Returns cleaned output suitable for parsing.",
	}, s.handleWaitForIdle)

	mcpsdk.AddTool(s.mcpServer, &mcpsdk.Tool{
		Name:        "get_artifact",
		Description: "Fetch the last captured output artifact for a workspace slot. Artifacts are stored in memory (not persisted) and are cleared when the slot is killed or pruned.",
	}, s.handleGetArtifact)

	mcpsdk.AddTool(s.mcpServer, &mcpsdk.Tool{
		Name:        "list_agents",
		Description: "List all running agents in a workspace with their status (idle/busy, current command).",
	}, s.handleListAgents)

	mcpsdk.AddTool(s.mcpServer, &mcpsdk.Tool{
		Name:        "kill_agent",
		Description: "Kill an agent running in a specific terminal slot by destroying its tmux session.",
	}, s.handleKillAgent)

	mcpsdk.AddTool(s.mcpServer, &mcpsdk.Tool{
		Name:        "move_terminal",
		Description: "Move a terminal from one workspace to another. Moves the X11 window to the target desktop, renames the tmux session, and updates workspace state.",
	}, s.handleMoveTerminal)
}

func (s *Server) waitForDependencies(workspace string, slots []int, timeoutSeconds int) error {
	if len(slots) == 0 {
		return nil
	}

	// Normalize: validate and de-dupe while preserving first-seen order.
	unique := make([]int, 0, len(slots))
	seen := make(map[int]struct{}, len(slots))
	for _, slot := range slots {
		if slot < 0 {
			return fmt.Errorf("depends_on contains negative slot %d", slot)
		}
		if _, ok := seen[slot]; ok {
			continue
		}
		seen[slot] = struct{}{}
		unique = append(unique, slot)
	}
	if len(unique) == 0 {
		return nil
	}

	timeout := time.Duration(timeoutSeconds) * time.Second
	if timeout <= 0 {
		timeout = 300 * time.Second
	}

	poll := s.depPollInterval
	if poll <= 0 {
		poll = 2 * time.Second
	}

	checkIdle := s.idleCheckFn
	if checkIdle == nil {
		checkIdle = s.checkIdle
	}
	targetExists := s.targetExistsFn
	if targetExists == nil {
		targetExists = tmuxTargetExists
	}

	checkAll := func() (bool, error) {
		for _, slot := range unique {
			target, ok := s.getTmuxTarget(workspace, slot)
			if !ok {
				return false, fmt.Errorf("dependency slot %d not tracked in workspace %q", slot, workspace)
			}
			if strings.TrimSpace(target) == "" {
				return false, fmt.Errorf("dependency slot %d has empty tmux target in workspace %q", slot, workspace)
			}
			if !targetExists(target) {
				return false, fmt.Errorf("dependency slot %d (target %s) is not alive (killed)", slot, target)
			}

			agentType := s.getAgentType(workspace, slot)
			if !checkIdle(target, agentType, workspace, slot) {
				return false, nil
			}
		}
		return true, nil
	}

	// Fast path: all deps are already idle.
	if ok, err := checkAll(); err != nil {
		return err
	} else if ok {
		return nil
	}

	ticker := time.NewTicker(poll)
	defer ticker.Stop()
	timer := time.NewTimer(timeout)
	defer timer.Stop()

	for {
		select {
		case <-ticker.C:
			if ok, err := checkAll(); err != nil {
				return err
			} else if ok {
				return nil
			}
		case <-timer.C:
			return fmt.Errorf("timeout waiting for dependency slots %v to become idle after %s", unique, timeout)
		}
	}
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

	// Clean up stale pipe files from previous runs.
	cleanStalePipeFiles(s.tracked)
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
		if s.artifacts != nil {
			s.artifacts.Clear(workspace, slot)
		}
		s.clearReadSnapshot(workspace, slot)
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
	return s.nextAvailableSlotLocked(workspace)
}

// allocateSlot returns the next available slot for a workspace and tracks the agent.
func (s *Server) allocateSlot(workspace, agentType, tmuxTarget, spawnMode string, responseFence bool) int {
	s.mu.Lock()
	defer s.mu.Unlock()

	slot := s.nextAvailableSlotLocked(workspace)
	s.trackSlotLocked(workspace, slot, agentType, tmuxTarget, spawnMode, responseFence)

	return slot
}

func (s *Server) nextAvailableSlotLocked(workspace string) int {
	ws := s.tracked[workspace]
	for slot := 0; ; slot++ {
		if _, ok := ws[slot]; !ok {
			return slot
		}
	}
}

func (s *Server) trackSpecificSlot(workspace string, slot int, agentType, tmuxTarget, spawnMode string, responseFence bool) error {
	if slot < 0 {
		return fmt.Errorf("invalid slot %d", slot)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.tracked[workspace] == nil {
		s.tracked[workspace] = make(map[int]trackedAgent)
	}
	if _, exists := s.tracked[workspace][slot]; exists {
		return fmt.Errorf("slot %d already tracked in workspace %q", slot, workspace)
	}
	s.trackSlotLocked(workspace, slot, agentType, tmuxTarget, spawnMode, responseFence)
	return nil
}

func (s *Server) trackSlotLocked(workspace string, slot int, agentType, tmuxTarget, spawnMode string, responseFence bool) {
	if s.tracked[workspace] == nil {
		s.tracked[workspace] = make(map[int]trackedAgent)
	}
	s.tracked[workspace][slot] = trackedAgent{
		agentType:              agentType,
		tmuxTarget:             tmuxTarget,
		spawnMode:              spawnMode,
		responseFence:          responseFence,
		fencePairCount:         0,
		lastArtifactCloseCount: 0,
	}
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
	if ws, ok := s.tracked[workspace]; ok {
		delete(ws, slot)
	}
	if rs, ok := s.readSnapshots[workspace]; ok {
		delete(rs, slot)
	}
	s.mu.Unlock()
	if s.artifacts != nil {
		s.artifacts.Clear(workspace, slot)
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

func (s *Server) getReadSnapshot(workspace string, slot int) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	rs := s.readSnapshots[workspace]
	if rs == nil {
		return ""
	}
	return rs[slot]
}

func (s *Server) setReadSnapshot(workspace string, slot int, output string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.readSnapshots[workspace] == nil {
		s.readSnapshots[workspace] = make(map[int]string)
	}
	s.readSnapshots[workspace][slot] = output
}

func (s *Server) clearReadSnapshot(workspace string, slot int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if rs := s.readSnapshots[workspace]; rs != nil {
		delete(rs, slot)
	}
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
// It uses a tiered strategy:
//
//	Tier 0a (pipe-pane): If a pipe file exists, stat for size change, then
//	    read and count close tags against the baseline.
//	Tier 0b (capture-pane fallback): Existing close-tag counting via capture-pane,
//	    used when no pipe file is active or pipe read fails.
//	Tier 1: Content-based detection via IdlePattern.
//	Tier 2: Process-based fallback (pane child process check).
func (s *Server) checkIdle(target, agentType, workspace string, slot int) bool {
	hasFence, baselineCount, lastArtifactCount := s.getFenceCaptureCounts(workspace, slot)

	if hasFence {
		// Tier 0a: pipe-pane based detection.
		pipePath, lastSize := s.getPipeState(workspace, slot)
		if pipePath != "" {
			currentSize := pipeFileSize(pipePath)
			if currentSize <= lastSize {
				// File size unchanged — agent still working.
				return false
			}
			// Size changed — read and count close tags.
			count, size, err := countCloseTagsInPipeFile(pipePath)
			if err == nil {
				s.updateLastPipeSize(workspace, slot, size)
				if count > baselineCount {
					if s.artifacts != nil && count > lastArtifactCount {
						s.captureAndStoreFenceArtifact(target, workspace, slot, count)
					}
					return true
				}
				// No new close tags yet — still working.
				return false
			}
			// Pipe read failed — fall through to capture-pane fallback.
		}

		// Tier 0b: capture-pane fallback for fence detection.
		out, err := tmuxCapturePane(target, 30)
		if err != nil {
			return false
		}
		currentCount := countCloseTags(out)
		if currentCount > baselineCount {
			if s.artifacts != nil && currentCount > lastArtifactCount {
				s.captureAndStoreFenceArtifact(target, workspace, slot, currentCount)
			}
			return true
		}
		// Fence expected but no new response yet — the agent is still working.
		// Do NOT fall through to Tier 1/2 which can false-positive on
		// startup tips, rate limit prompts, etc. that match the idle pattern.
		return false
	}

	// No fence — use capture-pane for Tier 1/2.
	out, err := tmuxCapturePane(target, 30)
	if err != nil {
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

// captureAndStoreFenceArtifact captures full scrollback for a fence-enabled slot,
// extracts the last response content, and stores it as an artifact. Best-effort:
// capture failures do not change the idle decision.
func (s *Server) captureAndStoreFenceArtifact(target, workspace string, slot int, closeTagCount int) {
	// Full scrollback required to reliably find the opening tag for long responses.
	full, err := tmuxCapturePane(target, 0)
	if err != nil {
		return
	}
	cleaned := trimOutput(cleanOutput(full), true)
	s.artifacts.Set(workspace, slot, cleaned)
	s.setLastArtifactCloseCount(workspace, slot, closeTagCount)
}

// captureAndStoreArtifactForSlot captures the most recent output for a slot and
// stores it as an artifact. For fence-enabled slots, it captures full scrollback
// so the opening tag can be found for long responses.
func (s *Server) captureAndStoreArtifactForSlot(workspace string, slot int) error {
	if s == nil || s.artifacts == nil {
		return fmt.Errorf("artifact store not initialized")
	}
	target, ok := s.getTmuxTarget(workspace, slot)
	if !ok {
		return fmt.Errorf("no agent tracked in workspace %q slot %d", workspace, slot)
	}
	if strings.TrimSpace(target) == "" {
		return fmt.Errorf("slot %d has empty tmux target in workspace %q", slot, workspace)
	}
	if !tmuxTargetExists(target) {
		return fmt.Errorf("slot %d (target %s) is not alive (killed)", slot, target)
	}

	fence, _ := s.getFenceState(workspace, slot)
	captureLines := 200
	if fence {
		captureLines = 0 // full scrollback for reliable fence extraction
	}
	raw, err := tmuxCapturePane(target, captureLines)
	if err != nil {
		return err
	}
	cleaned := trimOutput(cleanOutput(raw), fence)
	s.artifacts.Set(workspace, slot, cleaned)
	if fence {
		s.setLastArtifactCloseCount(workspace, slot, countCloseTags(raw))
	}
	return nil
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

// getFenceState returns the fence detection state for a tracked slot.
func (s *Server) getFenceState(workspace string, slot int) (hasFence bool, pairCount int) {
	s.mu.Lock()
	defer s.mu.Unlock()

	ws := s.tracked[workspace]
	if ws == nil {
		return false, 0
	}
	ta, ok := ws[slot]
	if !ok {
		return false, 0
	}
	return ta.responseFence, ta.fencePairCount
}

// getFenceCaptureCounts returns the fence baseline close-tag count and the last
// close-tag count for which we captured an artifact.
func (s *Server) getFenceCaptureCounts(workspace string, slot int) (hasFence bool, baseline int, lastArtifact int) {
	s.mu.Lock()
	defer s.mu.Unlock()

	ws := s.tracked[workspace]
	if ws == nil {
		return false, 0, 0
	}
	ta, ok := ws[slot]
	if !ok {
		return false, 0, 0
	}
	return ta.responseFence, ta.fencePairCount, ta.lastArtifactCloseCount
}

// updateFenceState updates the fence detection state for a tracked slot.
func (s *Server) updateFenceState(workspace string, slot int, responseFence bool, pairCount int) {
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
	ta.responseFence = responseFence
	ta.fencePairCount = pairCount
	if responseFence {
		// Reset artifact capture counter to the baseline at send time.
		ta.lastArtifactCloseCount = pairCount
	} else {
		ta.lastArtifactCloseCount = 0
	}
	ws[slot] = ta
}

func (s *Server) setLastArtifactCloseCount(workspace string, slot int, count int) {
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
	ta.lastArtifactCloseCount = count
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

// getPipeState returns the pipe file path and last recorded size for a tracked slot.
func (s *Server) getPipeState(workspace string, slot int) (filePath string, lastSize int64) {
	s.mu.Lock()
	defer s.mu.Unlock()

	ws := s.tracked[workspace]
	if ws == nil {
		return "", 0
	}
	ta, ok := ws[slot]
	if !ok {
		return "", 0
	}
	return ta.pipeFilePath, ta.lastPipeSize
}

// setPipeState sets the pipe file path for a tracked slot and resets lastPipeSize to 0.
func (s *Server) setPipeState(workspace string, slot int, filePath string) {
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
	ta.pipeFilePath = filePath
	ta.lastPipeSize = 0
	ws[slot] = ta
}

// updateLastPipeSize updates the last recorded pipe file size for a tracked slot.
func (s *Server) updateLastPipeSize(workspace string, slot int, size int64) {
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
	ta.lastPipeSize = size
	ws[slot] = ta
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

// tmuxClearInputLine best-effort clears any partially typed input in the
// focused prompt before automation sends a command/task. This mitigates race
// conditions where user keystrokes land in a newly spawned terminal window.
func tmuxClearInputLine(target string) error {
	// Escape first to dismiss transient UI modes in prompt UIs.
	cmd := exec.Command("tmux", "send-keys", "-t", target, "Escape")
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("tmux send-keys (Escape) failed: %w (%s)", err, strings.TrimSpace(string(out)))
	}
	time.Sleep(75 * time.Millisecond)

	// Ctrl-U clears the current input line in shells/readline-style prompts.
	cmd = exec.Command("tmux", "send-keys", "-t", target, "C-u")
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("tmux send-keys (C-u) failed: %w (%s)", err, strings.TrimSpace(string(out)))
	}
	time.Sleep(75 * time.Millisecond)
	return nil
}

// tmuxCapturePane captures output from a specific tmux target.
// If lines > 0, captures the last N lines. If lines <= 0, captures the
// full scrollback history (using -S -). The -J flag joins wrapped lines
// so that fence tags split across visual lines are reassembled.
func tmuxCapturePane(target string, lines int) (string, error) {
	args := []string{"capture-pane", "-p", "-J", "-t", target}
	if lines > 0 {
		args = append(args, "-S", fmt.Sprintf("-%d", lines))
	} else {
		args = append(args, "-S", "-")
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
