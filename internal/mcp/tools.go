package mcp

import (
	"context"
	"fmt"
	"log"
	"os/exec"
	"sort"
	"strings"
	"time"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/1broseidon/termtile/internal/agent"
	"github.com/1broseidon/termtile/internal/config"
)

func (s *Server) handleSpawnAgent(_ context.Context, _ *mcpsdk.CallToolRequest, args SpawnAgentInput) (*mcpsdk.CallToolResult, SpawnAgentOutput, error) {
	agentCfg, ok := s.config.Agents[args.AgentType]
	if !ok {
		available := make([]string, 0, len(s.config.Agents))
		for k := range s.config.Agents {
			available = append(available, k)
		}
		sort.Strings(available)
		return nil, SpawnAgentOutput{}, fmt.Errorf("unknown agent type %q; available: %v", args.AgentType, available)
	}

	workspace := resolveWorkspace(args.Workspace)
	spawnMode := resolveSpawnMode(args.Window, agentCfg.SpawnMode)

	// Determine the task text to send to the agent.
	// When response_fence is enabled, prepend structured output instructions.
	// A unique marker is appended as the last line for reliable output trimming.
	responseFence := agentCfg.ResponseFence && args.Task != ""
	var taskMarker string
	taskToSend := args.Task
	if args.Task != "" {
		taskMarker = generateMarker()
		if responseFence {
			taskToSend = wrapTaskWithFence(args.Task)
		}
		taskToSend = appendMarker(taskToSend, taskMarker)
	}

	// Build the command string: "command arg1 arg2 ..."
	// When PromptAsArg is true and a task is provided, append the task
	// as a CLI argument instead of sending it via tmux send-keys later.
	cmdParts := []string{agentCfg.Command}
	cmdParts = append(cmdParts, agentCfg.Args...)
	promptInCmd := agentCfg.PromptAsArg && args.Task != ""
	if promptInCmd {
		cmdParts = append(cmdParts, shellQuote(taskToSend))
	}
	fullCmd := strings.Join(cmdParts, " ")

	var tmuxTarget string
	var slot int

	if spawnMode == "window" {
		target, s2, err := s.spawnWindow(workspace, args.AgentType, fullCmd, args.Cwd, taskMarker, responseFence, agentCfg)
		if err != nil {
			return nil, SpawnAgentOutput{}, err
		}
		tmuxTarget = target
		slot = s2
	} else {
		target, s2, err := s.spawnPane(workspace, args.AgentType, fullCmd, args.Cwd, taskMarker, responseFence, agentCfg)
		if err != nil {
			return nil, SpawnAgentOutput{}, err
		}
		tmuxTarget = target
		slot = s2
	}

	// If a task is provided and wasn't passed as a CLI argument,
	// wait until the agent is ready then send via tmux send-keys.
	if args.Task != "" && !promptInCmd {
		s.waitAndSendTask(tmuxTarget, args.AgentType, taskToSend, agentCfg)
	}

	return nil, SpawnAgentOutput{
		Slot:        slot,
		SessionName: tmuxTarget,
		AgentType:   args.AgentType,
		Workspace:   workspace,
		SpawnMode:   spawnMode,
	}, nil
}

// spawnPane creates a new tmux pane (existing behavior).
func (s *Server) spawnPane(workspace, agentType, fullCmd, cwd, taskMarker string, responseFence bool, agentCfg config.AgentConfig) (string, int, error) {
	// Determine where to create the pane.
	// If we already have pane-mode agents in this workspace, split from one of them.
	// Otherwise, split the active pane in the user's attached tmux session.
	existingPane := s.anyPaneModeTarget(workspace)

	var splitTarget string
	if existingPane != "" {
		splitTarget = existingPane
	} else {
		targetSession := findAttachedSession()
		if targetSession == "" {
			return "", 0, fmt.Errorf("no attached tmux session found; please open a tmux terminal first")
		}
		splitTarget = targetSession
	}
	tmuxArgs := []string{"split-window", "-t", splitTarget, "-P", "-F", "#{pane_id}"}
	if cwd != "" {
		tmuxArgs = append(tmuxArgs, "-c", cwd)
	}
	tmuxArgs = append(tmuxArgs, fullCmd)

	// Set environment variables if configured.
	cmd := exec.Command("tmux", tmuxArgs...)
	if len(agentCfg.Env) > 0 {
		cmd.Env = cmd.Environ()
		for k, v := range agentCfg.Env {
			cmd.Env = append(cmd.Env, k+"="+v)
		}
	}
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", 0, fmt.Errorf("failed to create tmux pane: %w (%s)", err, strings.TrimSpace(string(out)))
	}

	tmuxTarget := strings.TrimSpace(string(out))
	if tmuxTarget == "" {
		return "", 0, fmt.Errorf("tmux did not return a pane ID")
	}

	// Rebalance the layout so all panes are visible.
	_ = exec.Command("tmux", "select-layout", "-t", tmuxTarget, "tiled").Run()

	slot := s.allocateSlot(workspace, agentType, tmuxTarget, "pane", taskMarker, responseFence)
	return tmuxTarget, slot, nil
}

// spawnWindow creates a new terminal window with a detached tmux session inside.
func (s *Server) spawnWindow(workspace, agentType, fullCmd, cwd, taskMarker string, responseFence bool, agentCfg config.AgentConfig) (string, int, error) {
	// Resolve which terminal emulator to use.
	termClass := s.config.ResolveTerminal()
	if termClass == "" {
		return "", 0, fmt.Errorf("no terminal emulator found; configure preferred_terminal or install a supported terminal")
	}

	spawnTemplate, ok := lookupSpawnTemplate(s.config.TerminalSpawnCommands, termClass)
	if !ok {
		return "", 0, fmt.Errorf("no spawn template for terminal class %q; add it to terminal_spawn_commands", termClass)
	}

	// Peek at the next slot to build the session name before allocating.
	nextSlot := s.peekNextSlot(workspace)
	sessionName := agent.SessionName(workspace, nextSlot)
	sessionTarget := agent.TargetForSession(sessionName)

	if cwd == "" {
		cwd = "."
	}

	// Build the tmux command that will run inside the terminal window.
	// The terminal window attaches to this session (no -d flag).
	tmuxCmd := fmt.Sprintf("tmux new-session -s %s -c %s %s",
		shellQuote(sessionName), shellQuote(cwd), shellQuote(fullCmd))

	// Render the terminal spawn template with the tmux command.
	argv, err := renderSpawnTemplate(spawnTemplate, cwd, tmuxCmd)
	if err != nil {
		return "", 0, fmt.Errorf("failed to render spawn template: %w", err)
	}
	if len(argv) == 0 {
		return "", 0, fmt.Errorf("spawn template produced empty command")
	}

	// Set environment variables if configured.
	cmd := exec.Command(argv[0], argv[1:]...)
	if len(agentCfg.Env) > 0 {
		cmd.Env = cmd.Environ()
		for k, v := range agentCfg.Env {
			cmd.Env = append(cmd.Env, k+"="+v)
		}
	}

	// Fire and forget — the terminal window process runs independently.
	if err := cmd.Start(); err != nil {
		return "", 0, fmt.Errorf("failed to spawn terminal window: %w", err)
	}

	// Poll for the tmux session to appear (the terminal window needs time to start).
	deadline := time.Now().Add(15 * time.Second)
	for {
		if exec.Command("tmux", "has-session", "-t", sessionName).Run() == nil {
			break
		}
		if time.Now().After(deadline) {
			return "", 0, fmt.Errorf("timeout waiting for tmux session %q to appear", sessionName)
		}
		time.Sleep(250 * time.Millisecond)
	}

	slot := s.allocateSlot(workspace, agentType, sessionTarget, "window", taskMarker, responseFence)

	// Give the terminal window time to appear as an X11 window, then
	// ask the termtile daemon to re-tile all terminals.
	time.Sleep(500 * time.Millisecond)
	s.triggerRetile()

	return sessionTarget, slot, nil
}

// waitAndSendTask waits for an agent to become ready, then sends the task text.
func (s *Server) waitAndSendTask(tmuxTarget, agentType, task string, agentCfg config.AgentConfig) {
	readyPattern := agentCfg.ReadyPattern
	timeout := 30 * time.Second

	if readyPattern != "" {
		if _, err := tmuxWaitFor(tmuxTarget, readyPattern, timeout, 50); err != nil {
			log.Printf("Warning: agent %q (target %s) not ready after %s, sending task anyway", agentType, tmuxTarget, timeout)
		}
	} else {
		// No configured pattern. Wait for the TUI to render and become
		// interactive. We poll for content, then wait for the output to
		// stabilize (stop changing), which indicates the TUI has finished
		// its initial render and is likely ready for input.
		deadline := time.Now().Add(timeout)
		var lastOutput string
		stableCount := 0
		for time.Now().Before(deadline) {
			out, err := tmuxCapturePane(tmuxTarget, 30)
			if err != nil {
				time.Sleep(500 * time.Millisecond)
				continue
			}
			trimmed := strings.TrimSpace(out)
			if trimmed == "" {
				time.Sleep(500 * time.Millisecond)
				continue
			}
			// Content exists. Check if it has stabilized (same for 2 consecutive polls).
			if trimmed == lastOutput {
				stableCount++
				if stableCount >= 2 {
					break
				}
			} else {
				stableCount = 0
			}
			lastOutput = trimmed
			time.Sleep(500 * time.Millisecond)
		}
		// Extra settle time for TUI input handler to become interactive
		// after visual rendering completes.
		time.Sleep(2 * time.Second)
	}

	if err := tmuxSendKeys(tmuxTarget, task); err != nil {
		log.Printf("Warning: failed to send initial task to %s: %v", tmuxTarget, err)
	}
}

func (s *Server) handleSendToAgent(_ context.Context, _ *mcpsdk.CallToolRequest, args SendToAgentInput) (*mcpsdk.CallToolResult, any, error) {
	workspace := resolveWorkspace(args.Workspace)
	target, ok := s.getTmuxTarget(workspace, args.Slot)
	if !ok {
		return nil, nil, fmt.Errorf("no agent tracked in workspace %q slot %d", workspace, args.Slot)
	}

	if err := tmuxSendKeys(target, args.Text); err != nil {
		return nil, nil, fmt.Errorf("failed to send to slot %d (target %s): %w", args.Slot, target, err)
	}

	return &mcpsdk.CallToolResult{
		Content: []mcpsdk.Content{
			&mcpsdk.TextContent{Text: fmt.Sprintf("Sent to slot %d (target %s)", args.Slot, target)},
		},
	}, nil, nil
}

func (s *Server) handleReadFromAgent(_ context.Context, _ *mcpsdk.CallToolRequest, args ReadFromAgentInput) (*mcpsdk.CallToolResult, ReadFromAgentOutput, error) {
	workspace := resolveWorkspace(args.Workspace)
	target, ok := s.getTmuxTarget(workspace, args.Slot)
	if !ok {
		return nil, ReadFromAgentOutput{}, fmt.Errorf("no agent tracked in workspace %q slot %d", workspace, args.Slot)
	}

	lines := args.Lines
	if lines <= 0 {
		lines = 50
	}

	marker, fence := s.getOutputConfig(workspace, args.Slot)

	// When a pattern is provided, poll until it appears or timeout.
	if args.Pattern != "" {
		timeout := time.Duration(args.Timeout) * time.Second
		if timeout <= 0 {
			timeout = 30 * time.Second
		}

		output, err := tmuxWaitFor(target, args.Pattern, timeout, lines)
		if args.Clean {
			output = cleanOutput(output)
		}
		output = trimOutput(output, marker, fence)

		if err != nil {
			// Timeout: return found=false with whatever output we captured.
			found := false
			return nil, ReadFromAgentOutput{
				Output:      output,
				SessionName: target,
				Found:       &found,
			}, nil
		}

		found := true
		return nil, ReadFromAgentOutput{
			Output:      output,
			SessionName: target,
			Found:       &found,
		}, nil
	}

	// One-shot read (no pattern).
	output, err := tmuxCapturePane(target, lines)
	if err != nil {
		return nil, ReadFromAgentOutput{}, fmt.Errorf("failed to read from slot %d (target %s): %w", args.Slot, target, err)
	}

	if args.Clean {
		output = cleanOutput(output)
	}

	output = trimOutput(output, marker, fence)

	return nil, ReadFromAgentOutput{
		Output:      output,
		SessionName: target,
	}, nil
}

func (s *Server) handleListAgents(_ context.Context, _ *mcpsdk.CallToolRequest, args ListAgentsInput) (*mcpsdk.CallToolResult, ListAgentsOutput, error) {
	workspace := resolveWorkspace(args.Workspace)
	tracked := s.getTracked(workspace)

	agents := make([]AgentInfo, 0, len(tracked))
	for slot, ta := range tracked {
		info := AgentInfo{
			Slot:        slot,
			AgentType:   ta.agentType,
			SessionName: ta.tmuxTarget,
			Exists:      true,
			SpawnMode:   ta.spawnMode,
		}

		// Check if target still exists by trying to query it.
		cmd := exec.Command("tmux", "display-message", "-t", ta.tmuxTarget, "-p", "#{pane_current_command}")
		if out, err := cmd.Output(); err == nil {
			info.CurrentCommand = strings.TrimSpace(string(out))
			info.IsIdle = s.checkIdle(ta.tmuxTarget, ta.agentType)
		} else {
			info.Exists = false
		}

		agents = append(agents, info)
	}

	// Sort by slot for deterministic output.
	sort.Slice(agents, func(i, j int) bool {
		return agents[i].Slot < agents[j].Slot
	})

	return nil, ListAgentsOutput{
		Workspace: workspace,
		Agents:    agents,
	}, nil
}

func (s *Server) handleKillAgent(_ context.Context, _ *mcpsdk.CallToolRequest, args KillAgentInput) (*mcpsdk.CallToolResult, KillAgentOutput, error) {
	workspace := resolveWorkspace(args.Workspace)
	target, ok := s.getTmuxTarget(workspace, args.Slot)
	if !ok {
		return nil, KillAgentOutput{Killed: false}, fmt.Errorf("no agent tracked in workspace %q slot %d", workspace, args.Slot)
	}

	mode := s.getSpawnMode(workspace, args.Slot)

	if mode == "window" {
		// Window-mode: kill the entire tmux session. The terminal window
		// closes because its process (tmux attach) exits.
		// Extract session name from target (e.g., "termtile-ws-0:0.0" → "termtile-ws-0").
		sessionName := target
		if idx := strings.Index(target, ":"); idx >= 0 {
			sessionName = target[:idx]
		}
		_ = exec.Command("tmux", "kill-session", "-t", sessionName).Run()
	} else {
		// Pane-mode: kill just the pane.
		_ = exec.Command("tmux", "kill-pane", "-t", target).Run()
	}

	// Always remove tracking — the target may already be gone (killed externally).
	s.removeTracked(workspace, args.Slot)

	// Rebalance remaining panes only for pane-mode agents.
	// For window-mode agents, re-tile via the daemon to close the visual gap.
	if mode == "window" {
		// Give the terminal window time to close before re-tiling.
		time.Sleep(300 * time.Millisecond)
		s.triggerRetile()
	} else {
		if remainingPane := s.anyPaneModeTarget(workspace); remainingPane != "" {
			_ = exec.Command("tmux", "select-layout", "-t", remainingPane, "tiled").Run()
		}
	}

	return nil, KillAgentOutput{
		SessionName: target,
		Killed:      true,
	}, nil
}

func (s *Server) handleWaitForIdle(_ context.Context, _ *mcpsdk.CallToolRequest, args WaitForIdleInput) (*mcpsdk.CallToolResult, WaitForIdleOutput, error) {
	workspace := resolveWorkspace(args.Workspace)
	target, ok := s.getTmuxTarget(workspace, args.Slot)
	if !ok {
		return nil, WaitForIdleOutput{}, fmt.Errorf("no agent tracked in workspace %q slot %d", workspace, args.Slot)
	}

	timeout := time.Duration(args.Timeout) * time.Second
	if timeout <= 0 {
		timeout = 120 * time.Second
	}
	lines := args.Lines
	if lines <= 0 {
		lines = 100
	}

	agentType := s.getAgentType(workspace, args.Slot)
	marker, fence := s.getOutputConfig(workspace, args.Slot)
	deadline := time.Now().Add(timeout)

	for {
		if s.checkIdle(target, agentType) {
			output, err := tmuxCapturePane(target, lines)
			if err != nil {
				return nil, WaitForIdleOutput{}, fmt.Errorf("failed to capture output from slot %d (target %s): %w", args.Slot, target, err)
			}
			return nil, WaitForIdleOutput{
				IsIdle:      true,
				Output:      trimOutput(cleanOutput(output), marker, fence),
				SessionName: target,
			}, nil
		}

		if time.Now().After(deadline) {
			// Timeout: return whatever output is available.
			output, _ := tmuxCapturePane(target, lines)
			return nil, WaitForIdleOutput{
				IsIdle:      false,
				Output:      trimOutput(cleanOutput(output), marker, fence),
				SessionName: target,
			}, nil
		}

		time.Sleep(500 * time.Millisecond)
	}
}
