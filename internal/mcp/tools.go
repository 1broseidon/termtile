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
	workspacepkg "github.com/1broseidon/termtile/internal/workspace"
)

func (s *Server) logTextOptions() (includeContent bool, previewLen int) {
	previewLen = 50
	if s == nil || s.config == nil {
		return false, previewLen
	}
	logCfg := s.config.GetLoggingConfig()
	if logCfg.PreviewLength > 0 {
		previewLen = logCfg.PreviewLength
	}
	return logCfg.IncludeContent, previewLen
}

func (s *Server) addTextDetails(details map[string]interface{}, text string) {
	includeContent, previewLen := s.logTextOptions()
	details["text_length"] = len(text)
	details["text_preview"] = agent.Truncate(text, previewLen)
	if includeContent {
		details["text"] = text
	}
}

func (s *Server) addOutputDetails(details map[string]interface{}, output string) {
	includeContent, previewLen := s.logTextOptions()
	details["output_length"] = len(output)
	details["output_preview"] = agent.Truncate(output, previewLen)
	if includeContent {
		details["output"] = output
	}
}

func (s *Server) handleSpawnAgent(_ context.Context, _ *mcpsdk.CallToolRequest, args SpawnAgentInput) (*mcpsdk.CallToolResult, SpawnAgentOutput, error) {
	agentCfg, ok := s.config.Agents[args.AgentType]
	if !ok {
		available := make([]string, 0, len(s.config.Agents))
		for k := range s.config.Agents {
			available = append(available, k)
		}
		sort.Strings(available)
		if s.logger != nil {
			workspaceForLog := strings.TrimSpace(args.Workspace)
			if workspaceForLog == "" {
				workspaceForLog = DefaultWorkspace
			}
			s.logger.Log(agent.ActionSpawnAgent, workspaceForLog, -1, map[string]interface{}{
				"agent_type":      args.AgentType,
				"available_count": len(available),
				"error":           "unknown_agent_type",
			})
		}
		return nil, SpawnAgentOutput{}, fmt.Errorf("unknown agent type %q; available: %v", args.AgentType, available)
	}

	workspaceName := resolveWorkspaceName(args.Workspace)
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

	// Build the agent command string: "command arg1 arg2 ..."
	cmdParts := []string{agentCfg.Command}
	cmdParts = append(cmdParts, agentCfg.Args...)
	var selectedModel string
	if args.Model != nil {
		selectedModel = strings.TrimSpace(*args.Model)
	}
	if selectedModel == "" {
		selectedModel = strings.TrimSpace(agentCfg.DefaultModel)
	}
	if selectedModel != "" {
		if len(agentCfg.Models) > 0 && !isKnownModel(selectedModel, agentCfg.Models) {
			log.Printf("Warning: unknown model %q for agent %q (configured models: %v)", selectedModel, args.AgentType, agentCfg.Models)
		}
		modelFlag := strings.TrimSpace(agentCfg.ModelFlag)
		if modelFlag == "" {
			modelFlag = "--model"
		}
		cmdParts = append(cmdParts, modelFlag, shellQuote(selectedModel))
	}

	// When PromptAsArg is true and a task is provided, append the task
	// as a CLI argument instead of sending it via tmux send-keys later.
	promptInCmd := agentCfg.PromptAsArg && args.Task != ""
	if promptInCmd && len(taskToSend) > 32*1024 {
		promptInCmd = false
	}
	if promptInCmd {
		cmdParts = append(cmdParts, shellQuote(taskToSend))
	}
	agentCmd := strings.Join(cmdParts, " ")

	var tmuxTarget string
	var slot int

	if spawnMode == "window" {
		// Window mode: start tmux with the default shell (so that
		// .zshrc/.bashrc are sourced and tool paths like proto/nvm are
		// available), then send the agent command via send-keys.
		target, s2, err := s.spawnWindow(workspaceName, args.AgentType, args.Cwd, taskMarker, responseFence, agentCfg)
		if err != nil {
			if s.logger != nil {
				s.logger.Log(agent.ActionSpawnAgent, workspaceName, -1, map[string]interface{}{
					"agent_type": args.AgentType,
					"spawn_mode": spawnMode,
					"error":      "spawn_failed",
				})
			}
			return nil, SpawnAgentOutput{}, err
		}
		tmuxTarget = target
		slot = s2

		// Wait for the shell to become ready, then send the agent command.
		s.waitForShellAndSend(tmuxTarget, agentCmd)

		// If the task was baked into the agent command (promptInCmd),
		// we're done. Otherwise fall through to waitAndSendTask below.
	} else {
		target, s2, err := s.spawnPane(workspaceName, args.AgentType, agentCmd, args.Cwd, taskMarker, responseFence, agentCfg)
		if err != nil {
			if s.logger != nil {
				s.logger.Log(agent.ActionSpawnAgent, workspaceName, -1, map[string]interface{}{
					"agent_type": args.AgentType,
					"spawn_mode": spawnMode,
					"error":      "spawn_failed",
				})
			}
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

	if s.logger != nil {
		details := map[string]interface{}{
			"agent_type":    args.AgentType,
			"spawn_mode":    spawnMode,
			"cwd":           args.Cwd,
			"prompt_as_arg": promptInCmd,
			"has_task":      args.Task != "",
		}
		if selectedModel != "" {
			details["model"] = selectedModel
		}
		s.addTextDetails(details, args.Task)
		s.logger.Log(agent.ActionSpawnAgent, workspaceName, slot, details)
	}

	return nil, SpawnAgentOutput{
		Slot:        slot,
		SessionName: tmuxTarget,
		AgentType:   args.AgentType,
		Workspace:   workspaceName,
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

// spawnWindow creates a new terminal window with a tmux session running the
// user's default shell. The agent command is NOT baked into the tmux session
// command — it is sent via send-keys afterward so that shell init files
// (.zshrc, .bashrc) are sourced and tool paths (proto, nvm, etc.) are available.
func (s *Server) spawnWindow(workspace, agentType, cwd, taskMarker string, responseFence bool, agentCfg config.AgentConfig) (string, int, error) {
	// Resolve which terminal emulator to use.
	// Prefer the terminal class from the workspace config (matches what the
	// workspace was saved with), falling back to the global config.
	termClass := ""
	if savedWs, err := workspacepkg.Read(workspace); err == nil && len(savedWs.Terminals) > 0 {
		termClass = savedWs.Terminals[0].WMClass
	}
	if termClass == "" {
		termClass = s.config.ResolveTerminal()
	}
	if termClass == "" {
		return "", 0, fmt.Errorf("no terminal emulator found; configure preferred_terminal or install a supported terminal")
	}

	spawnTemplate, ok := lookupSpawnTemplate(s.config.TerminalSpawnCommands, termClass)
	if !ok {
		return "", 0, fmt.Errorf("no spawn template for terminal class %q; add it to terminal_spawn_commands", termClass)
	}

	slot := s.allocateSlot(workspace, agentType, "", "window", taskMarker, responseFence)
	registryDesktop := -1
	registrySlot := -1
	if wsInfo, err := workspacepkg.GetWorkspaceByName(workspace); err == nil {
		registryDesktop = wsInfo.Desktop
		addedSlot, addErr := workspacepkg.AddTerminalToWorkspace(wsInfo.Desktop, true)
		if addErr != nil {
			s.removeTracked(workspace, slot)
			return "", 0, fmt.Errorf("failed to update workspace terminal registry for %q: %w", workspace, addErr)
		}
		registrySlot = addedSlot
		if addedSlot != slot {
			log.Printf("Warning: MCP slot (%d) differs from workspace slot (%d) for workspace %q", slot, addedSlot, workspace)
		}
	} else if workspace != DefaultWorkspace {
		s.removeTracked(workspace, slot)
		return "", 0, fmt.Errorf("workspace %q not found in registry: %w", workspace, err)
	}

	sessionName := agent.SessionName(workspace, slot)
	sessionTarget := agent.TargetForSession(sessionName)
	s.updateTmuxTarget(workspace, slot, sessionTarget)
	success := false
	defer func() {
		if !success {
			s.removeTracked(workspace, slot)
			if registryDesktop >= 0 && registrySlot >= 0 {
				if err := workspacepkg.RemoveTerminalFromWorkspace(registryDesktop, registrySlot); err != nil {
					log.Printf("Warning: failed to roll back workspace terminal registry for workspace %q slot %d: %v", workspace, registrySlot, err)
				}
			}
		}
	}()

	if cwd == "" {
		cwd = "."
	}

	// Build the tmux command that will run inside the terminal window.
	// Start with the default shell so that init files are sourced.
	tmuxCmd := fmt.Sprintf("tmux new-session -s %s -c %s",
		shellQuote(sessionName), shellQuote(cwd))

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
	success = true

	// Give the terminal window time to appear as an X11 window, then
	// ask the termtile daemon to re-tile all terminals.
	time.Sleep(500 * time.Millisecond)
	s.triggerRetile()

	return sessionTarget, slot, nil
}

// waitForShellAndSend waits for the default shell to become ready in a new
// tmux session, then sends the agent command via send-keys. This ensures
// shell init files (.zshrc/.bashrc) are sourced before the agent starts,
// making tool paths (proto, nvm, pyenv, etc.) available.
func (s *Server) waitForShellAndSend(tmuxTarget, agentCmd string) {
	// Wait for the shell prompt to appear (content stabilizes).
	deadline := time.Now().Add(10 * time.Second)
	var lastOutput string
	stableCount := 0
	for time.Now().Before(deadline) {
		out, err := tmuxCapturePane(tmuxTarget, 10)
		if err != nil {
			time.Sleep(300 * time.Millisecond)
			continue
		}
		trimmed := strings.TrimSpace(out)
		if trimmed == "" {
			time.Sleep(300 * time.Millisecond)
			continue
		}
		if trimmed == lastOutput {
			stableCount++
			if stableCount >= 2 {
				break
			}
		} else {
			stableCount = 0
		}
		lastOutput = trimmed
		time.Sleep(300 * time.Millisecond)
	}

	if err := tmuxSendKeys(tmuxTarget, agentCmd); err != nil {
		log.Printf("Warning: failed to send agent command to %s: %v", tmuxTarget, err)
	}
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
	workspaceName := resolveWorkspaceName(args.Workspace)
	target, ok := s.getTmuxTarget(workspaceName, args.Slot)
	if !ok {
		if s.logger != nil {
			s.logger.Log(agent.ActionSend, workspaceName, args.Slot, map[string]interface{}{
				"error": "agent_not_tracked",
			})
		}
		return nil, nil, fmt.Errorf("no agent tracked in workspace %q slot %d", workspaceName, args.Slot)
	}

	textToSend := args.Text
	agentType := s.getAgentType(workspaceName, args.Slot)
	taskMarker := ""
	responseFence := false
	if args.Text != "" {
		taskMarker = generateMarker()
		if agentType != "" {
			if agentCfg, ok := s.config.Agents[agentType]; ok && agentCfg.ResponseFence {
				responseFence = true
				textToSend = wrapTaskWithFence(args.Text)
			}
		}
		textToSend = appendMarker(textToSend, taskMarker)
	}

	if err := tmuxSendKeys(target, textToSend); err != nil {
		if s.logger != nil {
			details := map[string]interface{}{
				"agent_type":     agentType,
				"response_fence": responseFence,
				"sent_length":    len(textToSend),
				"error":          "send_failed",
			}
			s.addTextDetails(details, args.Text)
			s.logger.Log(agent.ActionSend, workspaceName, args.Slot, details)
		}
		return nil, nil, fmt.Errorf("failed to send to slot %d (target %s): %w", args.Slot, target, err)
	}
	if taskMarker != "" {
		s.updateOutputConfig(workspaceName, args.Slot, taskMarker, responseFence)
	}
	if s.logger != nil {
		details := map[string]interface{}{
			"agent_type":     agentType,
			"response_fence": responseFence,
			"sent_length":    len(textToSend),
		}
		if taskMarker != "" {
			details["task_marker"] = taskMarker
		}
		s.addTextDetails(details, args.Text)
		s.logger.Log(agent.ActionSend, workspaceName, args.Slot, details)
	}

	return &mcpsdk.CallToolResult{
		Content: []mcpsdk.Content{
			&mcpsdk.TextContent{Text: fmt.Sprintf("Sent to slot %d (target %s)", args.Slot, target)},
		},
	}, nil, nil
}

func (s *Server) handleReadFromAgent(_ context.Context, _ *mcpsdk.CallToolRequest, args ReadFromAgentInput) (*mcpsdk.CallToolResult, ReadFromAgentOutput, error) {
	workspaceName := resolveWorkspaceName(args.Workspace)
	target, ok := s.getTmuxTarget(workspaceName, args.Slot)
	if !ok {
		if s.logger != nil {
			s.logger.Log(agent.ActionRead, workspaceName, args.Slot, map[string]interface{}{
				"error": "agent_not_tracked",
			})
		}
		return nil, ReadFromAgentOutput{}, fmt.Errorf("no agent tracked in workspace %q slot %d", workspaceName, args.Slot)
	}
	agentType := s.getAgentType(workspaceName, args.Slot)

	lines := args.Lines
	if lines <= 0 {
		lines = 50
	}

	marker, fence := s.getOutputConfig(workspaceName, args.Slot)

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
			if s.logger != nil {
				details := map[string]interface{}{
					"agent_type":      agentType,
					"lines":           lines,
					"clean":           args.Clean,
					"pattern":         args.Pattern,
					"timeout_seconds": int(timeout / time.Second),
					"found":           found,
				}
				s.addOutputDetails(details, output)
				s.logger.Log(agent.ActionRead, workspaceName, args.Slot, details)
			}
			return nil, ReadFromAgentOutput{
				Output:      output,
				SessionName: target,
				Found:       &found,
			}, nil
		}

		found := true
		if s.logger != nil {
			details := map[string]interface{}{
				"agent_type":      agentType,
				"lines":           lines,
				"clean":           args.Clean,
				"pattern":         args.Pattern,
				"timeout_seconds": int(timeout / time.Second),
				"found":           found,
			}
			s.addOutputDetails(details, output)
			s.logger.Log(agent.ActionRead, workspaceName, args.Slot, details)
		}
		return nil, ReadFromAgentOutput{
			Output:      output,
			SessionName: target,
			Found:       &found,
		}, nil
	}

	// One-shot read (no pattern).
	output, err := tmuxCapturePane(target, lines)
	if err != nil {
		if s.logger != nil {
			s.logger.Log(agent.ActionRead, workspaceName, args.Slot, map[string]interface{}{
				"agent_type": agentType,
				"lines":      lines,
				"clean":      args.Clean,
				"error":      "capture_failed",
			})
		}
		return nil, ReadFromAgentOutput{}, fmt.Errorf("failed to read from slot %d (target %s): %w", args.Slot, target, err)
	}

	if args.Clean {
		output = cleanOutput(output)
	}

	output = trimOutput(output, marker, fence)
	if s.logger != nil {
		details := map[string]interface{}{
			"agent_type": agentType,
			"lines":      lines,
			"clean":      args.Clean,
		}
		s.addOutputDetails(details, output)
		s.logger.Log(agent.ActionRead, workspaceName, args.Slot, details)
	}

	return nil, ReadFromAgentOutput{
		Output:      output,
		SessionName: target,
	}, nil
}

func (s *Server) handleListAgents(_ context.Context, _ *mcpsdk.CallToolRequest, args ListAgentsInput) (*mcpsdk.CallToolResult, ListAgentsOutput, error) {
	workspaceName := resolveWorkspaceName(args.Workspace)
	tracked := s.getTracked(workspaceName)

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
			info.IsIdle = s.checkIdle(ta.tmuxTarget, ta.agentType, workspaceName, slot)
		} else {
			info.Exists = false
		}

		agents = append(agents, info)
	}

	// Sort by slot for deterministic output.
	sort.Slice(agents, func(i, j int) bool {
		return agents[i].Slot < agents[j].Slot
	})
	if s.logger != nil {
		idleCount := 0
		missingCount := 0
		for _, a := range agents {
			if a.Exists && a.IsIdle {
				idleCount++
			}
			if !a.Exists {
				missingCount++
			}
		}
		s.logger.Log(agent.ActionListAgents, workspaceName, -1, map[string]interface{}{
			"agent_count":   len(agents),
			"idle_count":    idleCount,
			"missing_count": missingCount,
		})
	}

	return nil, ListAgentsOutput{
		Workspace: workspaceName,
		Agents:    agents,
	}, nil
}

func (s *Server) handleKillAgent(_ context.Context, _ *mcpsdk.CallToolRequest, args KillAgentInput) (*mcpsdk.CallToolResult, KillAgentOutput, error) {
	workspaceName := resolveWorkspaceName(args.Workspace)
	target, ok := s.getTmuxTarget(workspaceName, args.Slot)
	if !ok {
		if s.logger != nil {
			s.logger.Log(agent.ActionKillAgent, workspaceName, args.Slot, map[string]interface{}{
				"killed": false,
				"error":  "agent_not_tracked",
			})
		}
		return nil, KillAgentOutput{Killed: false}, fmt.Errorf("no agent tracked in workspace %q slot %d", workspaceName, args.Slot)
	}

	mode := s.getSpawnMode(workspaceName, args.Slot)
	agentType := s.getAgentType(workspaceName, args.Slot)

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
	s.removeTracked(workspaceName, args.Slot)

	if mode == "window" {
		if wsInfo, err := workspacepkg.GetWorkspaceByName(workspaceName); err == nil {
			if err := workspacepkg.RemoveTerminalFromWorkspace(wsInfo.Desktop, args.Slot); err != nil {
				log.Printf("Warning: failed to remove slot %d from workspace registry %q: %v", args.Slot, workspaceName, err)
			}
		}
	}

	// Rebalance remaining panes only for pane-mode agents.
	// For window-mode agents, re-tile via the daemon to close the visual gap.
	if mode == "window" {
		// Give the terminal window time to close before re-tiling.
		time.Sleep(300 * time.Millisecond)
		s.triggerRetile()
	} else {
		if remainingPane := s.anyPaneModeTarget(workspaceName); remainingPane != "" {
			_ = exec.Command("tmux", "select-layout", "-t", remainingPane, "tiled").Run()
		}
	}
	if s.logger != nil {
		s.logger.Log(agent.ActionKillAgent, workspaceName, args.Slot, map[string]interface{}{
			"agent_type":   agentType,
			"spawn_mode":   mode,
			"session_name": target,
			"killed":       true,
		})
	}

	return nil, KillAgentOutput{
		SessionName: target,
		Killed:      true,
	}, nil
}

func (s *Server) handleWaitForIdle(_ context.Context, _ *mcpsdk.CallToolRequest, args WaitForIdleInput) (*mcpsdk.CallToolResult, WaitForIdleOutput, error) {
	workspaceName := resolveWorkspaceName(args.Workspace)
	target, ok := s.getTmuxTarget(workspaceName, args.Slot)
	if !ok {
		if s.logger != nil {
			s.logger.Log(agent.ActionWaitIdle, workspaceName, args.Slot, map[string]interface{}{
				"error": "agent_not_tracked",
			})
		}
		return nil, WaitForIdleOutput{}, fmt.Errorf("no agent tracked in workspace %q slot %d", workspaceName, args.Slot)
	}

	timeout := time.Duration(args.Timeout) * time.Second
	if timeout <= 0 {
		timeout = 120 * time.Second
	}
	lines := args.Lines
	if lines <= 0 {
		lines = 100
	}

	agentType := s.getAgentType(workspaceName, args.Slot)
	marker, fence := s.getOutputConfig(workspaceName, args.Slot)
	deadline := time.Now().Add(timeout)
	start := time.Now()

	for {
		if s.checkIdle(target, agentType, workspaceName, args.Slot) {
			output, err := tmuxCapturePane(target, lines)
			if err != nil {
				if s.logger != nil {
					s.logger.Log(agent.ActionWaitIdle, workspaceName, args.Slot, map[string]interface{}{
						"agent_type":      agentType,
						"is_idle":         false,
						"lines":           lines,
						"timeout_seconds": int(timeout / time.Second),
						"elapsed_ms":      time.Since(start).Milliseconds(),
						"error":           "capture_failed",
					})
				}
				return nil, WaitForIdleOutput{}, fmt.Errorf("failed to capture output from slot %d (target %s): %w", args.Slot, target, err)
			}
			cleanedOutput := trimOutput(cleanOutput(output), marker, fence)
			if s.logger != nil {
				details := map[string]interface{}{
					"agent_type":      agentType,
					"is_idle":         true,
					"lines":           lines,
					"timeout_seconds": int(timeout / time.Second),
					"elapsed_ms":      time.Since(start).Milliseconds(),
				}
				s.addOutputDetails(details, cleanedOutput)
				s.logger.Log(agent.ActionWaitIdle, workspaceName, args.Slot, details)
			}
			return nil, WaitForIdleOutput{
				IsIdle:      true,
				Output:      cleanedOutput,
				SessionName: target,
			}, nil
		}

		if time.Now().After(deadline) {
			// Timeout: return whatever output is available.
			output, _ := tmuxCapturePane(target, lines)
			cleanedOutput := trimOutput(cleanOutput(output), marker, fence)
			if s.logger != nil {
				details := map[string]interface{}{
					"agent_type":      agentType,
					"is_idle":         false,
					"lines":           lines,
					"timeout_seconds": int(timeout / time.Second),
					"elapsed_ms":      time.Since(start).Milliseconds(),
				}
				s.addOutputDetails(details, cleanedOutput)
				s.logger.Log(agent.ActionWaitIdle, workspaceName, args.Slot, details)
			}
			return nil, WaitForIdleOutput{
				IsIdle:      false,
				Output:      cleanedOutput,
				SessionName: target,
			}, nil
		}

		time.Sleep(500 * time.Millisecond)
	}
}

func isKnownModel(model string, known []string) bool {
	for _, k := range known {
		if strings.TrimSpace(k) == model {
			return true
		}
	}
	return false
}

// resolveWorkspaceName returns the provided workspace, or if omitted, the active
// workspace on the current desktop. Falls back to DefaultWorkspace only when no
// active workspace is available.
func resolveWorkspaceName(ws string) string {
	if strings.TrimSpace(ws) != "" {
		return ws
	}

	active, err := workspacepkg.GetActiveWorkspace()
	if err == nil && strings.TrimSpace(active.Name) != "" {
		return active.Name
	}

	return DefaultWorkspace
}
