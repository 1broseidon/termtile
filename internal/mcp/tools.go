package mcp

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"gopkg.in/yaml.v3"

	"github.com/1broseidon/termtile/internal/agent"
	"github.com/1broseidon/termtile/internal/config"
	"github.com/1broseidon/termtile/internal/platform"
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

	spawnMode := resolveSpawnMode(args.Window, agentCfg.SpawnMode)
	workspaceName, err := resolveWorkspaceForSpawn(args.Workspace, args.SourceWorkspace)
	if err != nil {
		if s.logger != nil {
			s.logger.Log(agent.ActionSpawnAgent, DefaultWorkspace, -1, map[string]interface{}{
				"agent_type": args.AgentType,
				"spawn_mode": spawnMode,
				"error":      err.Error(),
			})
		}
		return nil, SpawnAgentOutput{}, err
	}

	// If depends_on is set, wait now so we can substitute slot artifacts into the
	// task prompt BEFORE spawning (needed for prompt_as_arg agents).
	if len(args.DependsOn) > 0 {
		if err := s.waitForDependencies(workspaceName, args.DependsOn, args.DependsOnTimeout); err != nil {
			if s.logger != nil {
				details := map[string]interface{}{
					"agent_type":         args.AgentType,
					"spawn_mode":         spawnMode,
					"depends_on_count":   len(args.DependsOn),
					"depends_on_timeout": args.DependsOnTimeout,
					"error":              "depends_on_failed",
					"depends_on_message": err.Error(),
				}
				s.logger.Log(agent.ActionSpawnAgent, workspaceName, -1, details)
			}
			return nil, SpawnAgentOutput{}, err
		}

		// Ensure artifacts exist for dependency slots (best-effort capture after idle).
		seen := make(map[int]struct{}, len(args.DependsOn))
		for _, dep := range args.DependsOn {
			if _, ok := seen[dep]; ok {
				continue
			}
			seen[dep] = struct{}{}
			if s.artifacts != nil {
				if _, ok := s.artifacts.Get(workspaceName, dep); ok {
					continue
				}
			}
			if err := s.captureAndStoreArtifactForSlot(workspaceName, dep); err != nil {
				return nil, SpawnAgentOutput{}, fmt.Errorf("failed to capture artifact for dependency slot %d: %w", dep, err)
			}
		}
	}

	// Determine the task text to send to the agent.
	// When response_fence is enabled, prepend structured output instructions.
	taskTemplate := args.Task
	if taskTemplate != "" && len(args.DependsOn) > 0 {
		expanded, missing := substituteSlotOutputTemplates(taskTemplate, workspaceName, args.DependsOn, s.artifacts)
		taskTemplate = expanded
		if len(missing) > 0 {
			log.Printf("Warning: missing artifacts for workspace %q dependency slots %v", workspaceName, missing)
		}
	}
	responseFence := agentCfg.ResponseFence && taskTemplate != ""
	taskToSend := taskTemplate
	if taskTemplate != "" && responseFence {
		taskToSend = wrapTaskWithFence(taskTemplate)
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
	promptInCmd := agentCfg.PromptAsArg && taskTemplate != ""
	if promptInCmd && len(taskToSend) > 32*1024 {
		promptInCmd = false
	}
	if promptInCmd {
		cmdParts = append(cmdParts, shellQuote(taskToSend))
	}

	// When PipeTask is true and a task is provided, pipe the task via
	// stdin: printf '%s\n' 'TASK' | cmd args...
	// This avoids interactive prompt issues (e.g. fence instructions
	// leaking into aider/cecli's prompt).
	pipeInCmd := agentCfg.PipeTask && taskTemplate != "" && !promptInCmd
	if pipeInCmd && len(taskToSend) > 32*1024 {
		pipeInCmd = false
	}

	agentCmd := strings.Join(cmdParts, " ")
	if pipeInCmd {
		agentCmd = fmt.Sprintf("printf '%%s\\n' %s | %s", shellQuote(taskToSend), agentCmd)
	}

	tmuxTarget, slot, err := s.spawnAgentWithDependencies(
		workspaceName,
		args.AgentType,
		args.Cwd,
		agentCmd,
		spawnMode,
		responseFence,
		agentCfg,
		nil,
		0,
	)
	if err != nil {
		if s.logger != nil {
			details := map[string]interface{}{
				"agent_type": args.AgentType,
				"spawn_mode": spawnMode,
			}
			if len(args.DependsOn) > 0 {
				details["depends_on_count"] = len(args.DependsOn)
				details["depends_on_timeout"] = args.DependsOnTimeout
				details["error"] = "depends_on_failed"
			} else {
				details["error"] = "spawn_failed"
			}
			s.logger.Log(agent.ActionSpawnAgent, workspaceName, -1, details)
		}
		return nil, SpawnAgentOutput{}, err
	}

	// If a task is provided and wasn't passed as a CLI argument or piped,
	// wait until the agent is ready then send via tmux send-keys.
	if taskTemplate != "" && !promptInCmd && !pipeInCmd {
		s.waitAndSendTask(tmuxTarget, args.AgentType, taskToSend, agentCfg)
	}

	// Activate pipe-pane for fence-enabled agents to capture the raw byte
	// stream for reliable idle detection (avoids TUI artifacts in capture-pane).
	if responseFence {
		pipePath := pipeFilePath(workspaceName, slot)
		if f, err := os.Create(pipePath); err == nil {
			f.Close()
		}
		if err := startPipePane(tmuxTarget, pipePath); err != nil {
			log.Printf("Warning: pipe-pane failed for slot %d: %v", slot, err)
		} else {
			s.setPipeState(workspaceName, slot, pipePath)
			// Wait for the instruction echo to appear in the pipe file,
			// then snapshot the baseline close-tag count so the echo's
			// close tag is included in the baseline and not mistaken for
			// a real response.
			time.Sleep(3 * time.Second)
			if count, size, err := countCloseTagsInPipeFile(pipePath); err == nil {
				s.updateFenceState(workspaceName, slot, true, count)
				s.updateLastPipeSize(workspaceName, slot, size)
			}
		}
	}

	if s.logger != nil {
		details := map[string]interface{}{
			"agent_type":    args.AgentType,
			"spawn_mode":    spawnMode,
			"cwd":           args.Cwd,
			"prompt_as_arg": promptInCmd,
			"pipe_task":     pipeInCmd,
			"has_task":      taskTemplate != "",
		}
		if len(args.DependsOn) > 0 {
			details["depends_on_count"] = len(args.DependsOn)
			details["depends_on_timeout"] = args.DependsOnTimeout
		}
		if selectedModel != "" {
			details["model"] = selectedModel
		}
		s.addTextDetails(details, taskTemplate)
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
func (s *Server) spawnPane(workspace, agentType, fullCmd, cwd string, responseFence bool, agentCfg config.AgentConfig) (string, int, error) {
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

	slot := s.allocateSlot(workspace, agentType, tmuxTarget, "pane", responseFence)
	return tmuxTarget, slot, nil
}

// spawnWindow creates a new terminal window with a tmux session running the
// user's default shell. The agent command is NOT baked into the tmux session
// command — it is sent via send-keys afterward so that shell init files
// (.zshrc, .bashrc) are sourced and tool paths (proto, nvm, etc.) are available.
func (s *Server) spawnWindow(workspace, agentType, cwd string, responseFence bool, agentCfg config.AgentConfig) (string, int, error) {
	previousFocusID, _ := getActiveWindowID()

	// Resolve which terminal emulator to use.
	// Prefer the terminal class from the workspace config (matches what the
	// workspace was saved with), falling back to the global config.
	termClass := ""
	savedCwd := ""
	if savedWs, err := workspacepkg.Read(workspace); err == nil && len(savedWs.Terminals) > 0 {
		termClass = savedWs.Terminals[0].WMClass
		if c := strings.TrimSpace(savedWs.Terminals[0].Cwd); c != "" {
			savedCwd = c
		}
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

	slot := -1
	registryDesktop := -1
	registrySlot := -1
	if wsInfo, err := workspacepkg.GetWorkspaceByName(workspace); err == nil {
		registryDesktop = wsInfo.Desktop
		addedSlot, addErr := workspacepkg.AddTerminalToWorkspace(wsInfo.Desktop, true)
		if addErr != nil {
			return "", 0, fmt.Errorf("failed to update workspace terminal registry for %q: %w", workspace, addErr)
		}
		slot = addedSlot
		registrySlot = addedSlot
		if err := s.trackSpecificSlot(workspace, slot, agentType, "", "window", responseFence); err != nil {
			_ = workspacepkg.RemoveTerminalFromWorkspace(wsInfo.Desktop, addedSlot)
			return "", 0, fmt.Errorf("failed to track slot %d for workspace %q: %w", slot, workspace, err)
		}
	} else if workspace != DefaultWorkspace {
		return "", 0, fmt.Errorf("workspace %q not found in registry: %w", workspace, err)
	} else {
		slot = s.allocateSlot(workspace, agentType, "", "window", responseFence)
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
		cwd = savedCwd
	}
	if cwd == "" {
		if home, err := os.UserHomeDir(); err == nil {
			cwd = home
		} else {
			cwd = "/"
		}
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

	// Set environment variables (including DISPLAY/XAUTHORITY for window mode).
	cmd := exec.Command(argv[0], argv[1:]...)
	cmd.Env = cmd.Environ()
	for k, v := range agentCfg.Env {
		cmd.Env = upsertEnv(cmd.Env, k, v)
	}
	if err := ensureWindowSpawnEnv(cmd, s.config); err != nil {
		return "", 0, err
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
	// correct its desktop if the user switched desktops since the workspace
	// was created. This fixes the bug where resolveWorkspaceName() resolves
	// based on the currently visible desktop instead of the workspace's desktop.
	time.Sleep(500 * time.Millisecond)
	spawnedWindowID, _ := platform.FindWindowByTitleStandalone(sessionName)
	if registryDesktop >= 0 {
		currentDesktop, err := platform.GetCurrentDesktopStandalone()
		if err == nil && currentDesktop != registryDesktop {
			if spawnedWindowID != 0 {
				if err := platform.MoveWindowToDesktopStandalone(spawnedWindowID, registryDesktop); err != nil {
					log.Printf("Warning: failed to move window to desktop %d: %v", registryDesktop, err)
				}
			}
		}
	}

	// Best-effort focus restoration: if the spawned terminal took focus,
	// return focus to the previously active window so caller typing is not hijacked.
	if previousFocusID != 0 && previousFocusID != spawnedWindowID && spawnedWindowID != 0 {
		if currentFocusID, ok := getActiveWindowID(); ok && currentFocusID == spawnedWindowID {
			if err := platform.FocusWindowStandalone(previousFocusID); err != nil {
				log.Printf("Warning: failed to restore focus to window %d: %v", previousFocusID, err)
			}
		}
	}
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

	if err := tmuxClearInputLine(tmuxTarget); err != nil {
		log.Printf("Warning: failed to clear input line on %s: %v", tmuxTarget, err)
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

	if err := tmuxClearInputLine(tmuxTarget); err != nil {
		log.Printf("Warning: failed to clear input line on %s: %v", tmuxTarget, err)
	}
	if err := tmuxSendKeys(tmuxTarget, task); err != nil {
		log.Printf("Warning: failed to send initial task to %s: %v", tmuxTarget, err)
	}
}

func getActiveWindowID() (uint32, bool) {
	backend, err := platform.NewLinuxBackendFromDisplay()
	if err != nil {
		return 0, false
	}
	defer backend.Disconnect()

	active, err := backend.ActiveWindow()
	if err != nil {
		return 0, false
	}
	if active == 0 {
		return 0, false
	}
	return uint32(active), true
}

func (s *Server) handleSendToAgent(_ context.Context, _ *mcpsdk.CallToolRequest, args SendToAgentInput) (*mcpsdk.CallToolResult, any, error) {
	workspaceName, err := resolveWorkspaceForRead(args.Workspace, args.SourceWorkspace, "send_to_agent")
	if err != nil {
		if s.logger != nil {
			s.logger.Log(agent.ActionSend, DefaultWorkspace, args.Slot, map[string]interface{}{
				"error": err.Error(),
			})
		}
		return nil, nil, err
	}
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
	responseFence := false
	if args.Text != "" && agentType != "" {
		if agentCfg, ok := s.config.Agents[agentType]; ok && agentCfg.ResponseFence {
			responseFence = true
			// Snapshot current standalone close-tag count BEFORE sending so
			// checkIdle can detect the new response by comparing counts.
			// Prefer pipe file if available (more reliable than capture-pane).
			var baseline int
			pipePath, _ := s.getPipeState(workspaceName, args.Slot)
			if pipePath != "" {
				if count, size, err := countCloseTagsInPipeFile(pipePath); err == nil {
					baseline = count
					s.updateLastPipeSize(workspaceName, args.Slot, size)
				}
			}
			if pipePath == "" {
				if out, err := tmuxCapturePane(target, 100); err == nil {
					baseline = countCloseTags(out)
				}
			}
			s.updateFenceState(workspaceName, args.Slot, true, baseline)
			textToSend = wrapTaskWithFence(args.Text)
		}
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
	if s.logger != nil {
		details := map[string]interface{}{
			"agent_type":     agentType,
			"response_fence": responseFence,
			"sent_length":    len(textToSend),
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
	workspaceName, err := resolveWorkspaceForRead(args.Workspace, args.SourceWorkspace, "read_from_agent")
	if err != nil {
		if s.logger != nil {
			s.logger.Log(agent.ActionRead, DefaultWorkspace, args.Slot, map[string]interface{}{
				"error": err.Error(),
			})
		}
		return nil, ReadFromAgentOutput{}, err
	}
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

	linesRequested := args.Lines
	lines := normalizeReadLines(args.Lines)
	captureLines := lines
	if fence, _ := s.getFenceState(workspaceName, args.Slot); fence && captureLines < fenceReadCaptureLines {
		captureLines = fenceReadCaptureLines
	}
	fence, _ := s.getFenceState(workspaceName, args.Slot)

	postProcess := func(raw string) string {
		output := raw
		if args.Clean {
			output = cleanOutput(output)
		}
		output = trimOutput(output, fence)
		output = tailOutputLines(output, lines)
		if args.SinceLast {
			prev := s.getReadSnapshot(workspaceName, args.Slot)
			s.setReadSnapshot(workspaceName, args.Slot, output)
			return outputDelta(prev, output)
		}
		s.setReadSnapshot(workspaceName, args.Slot, output)
		return output
	}

	// When a pattern is provided, poll until it appears or timeout.
	if args.Pattern != "" {
		timeout := time.Duration(args.Timeout) * time.Second
		if timeout <= 0 {
			timeout = 30 * time.Second
		}

		output, err := tmuxWaitFor(target, args.Pattern, timeout, captureLines)
		output = postProcess(output)

		if err != nil {
			// Timeout: return found=false with whatever output we captured.
			found := false
			if s.logger != nil {
				details := map[string]interface{}{
					"agent_type":      agentType,
					"lines_requested": linesRequested,
					"lines_effective": lines,
					"lines":           lines,
					"clean":           args.Clean,
					"since_last":      args.SinceLast,
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
				"lines_requested": linesRequested,
				"lines_effective": lines,
				"lines":           lines,
				"clean":           args.Clean,
				"since_last":      args.SinceLast,
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

	// One-shot read (no pattern): return a bounded tail preview window.
	output, err := tmuxCapturePane(target, captureLines)
	if err != nil {
		if s.logger != nil {
			s.logger.Log(agent.ActionRead, workspaceName, args.Slot, map[string]interface{}{
				"agent_type":      agentType,
				"lines_requested": linesRequested,
				"lines_effective": lines,
				"lines":           lines,
				"clean":           args.Clean,
				"since_last":      args.SinceLast,
				"error":           "capture_failed",
			})
		}
		return nil, ReadFromAgentOutput{}, fmt.Errorf("failed to read from slot %d (target %s): %w", args.Slot, target, err)
	}

	output = postProcess(output)
	if s.logger != nil {
		details := map[string]interface{}{
			"agent_type":      agentType,
			"lines_requested": linesRequested,
			"lines_effective": lines,
			"lines":           lines,
			"clean":           args.Clean,
			"since_last":      args.SinceLast,
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
	workspaceName, err := resolveWorkspaceForRead(args.Workspace, args.SourceWorkspace, "list_agents")
	if err != nil {
		return nil, ListAgentsOutput{}, err
	}
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
	workspaceName, err := resolveWorkspaceForRead(args.Workspace, args.SourceWorkspace, "kill_agent")
	if err != nil {
		if s.logger != nil {
			s.logger.Log(agent.ActionKillAgent, DefaultWorkspace, args.Slot, map[string]interface{}{
				"error": err.Error(),
			})
		}
		return nil, KillAgentOutput{}, err
	}

	// Guard: prevent killing slot 0 in agent-mode workspaces (the orchestrator).
	if args.Slot == 0 && s.config.AgentMode.GetProtectSlotZero() && isAgentModeWorkspace(workspaceName) {
		if s.logger != nil {
			s.logger.Log(agent.ActionKillAgent, workspaceName, args.Slot, map[string]interface{}{
				"killed": false,
				"error":  "slot_zero_protected",
			})
		}
		return nil, KillAgentOutput{Killed: false}, fmt.Errorf(
			"slot 0 is protected in agent-mode workspace %q (this is typically the orchestrating agent); set agent_mode.protect_slot_zero: false in config to disable",
			workspaceName,
		)
	}

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

	// Stop pipe-pane and remove the pipe file before killing the session.
	pipePath, _ := s.getPipeState(workspaceName, args.Slot)
	if pipePath != "" {
		stopPipePane(target)
		removePipeFile(pipePath)
	}

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
			} else {
				if err := s.compactWindowSlots(workspaceName, args.Slot); err != nil {
					log.Printf("Warning: failed to compact slots for workspace %q after removing slot %d: %v", workspaceName, args.Slot, err)
				}
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

func (s *Server) handleGetArtifact(_ context.Context, _ *mcpsdk.CallToolRequest, args GetArtifactArgs) (*mcpsdk.CallToolResult, GetArtifactOutput, error) {
	workspaceName, err := resolveWorkspaceForRead(args.Workspace, args.SourceWorkspace, "get_artifact")
	if err != nil {
		return nil, GetArtifactOutput{}, err
	}
	if s == nil || s.artifacts == nil {
		return nil, GetArtifactOutput{}, fmt.Errorf("artifact store not initialized")
	}
	art, ok := s.artifacts.Get(workspaceName, args.Slot)
	if !ok {
		return nil, GetArtifactOutput{}, fmt.Errorf("no artifact for workspace %q slot %d", workspaceName, args.Slot)
	}
	// Ensure returned workspace is normalized to the request workspace.
	art.Workspace = workspaceName
	return nil, GetArtifactOutput{
		Workspace:      art.Workspace,
		Slot:           art.Slot,
		Output:         art.Output,
		Truncated:      art.Truncated,
		Warning:        art.Warning,
		OriginalBytes:  art.OriginalBytes,
		StoredBytes:    art.StoredBytes,
		LastUpdatedUTC: art.LastUpdatedUTC,
	}, nil
}

func (s *Server) handleWaitForIdle(_ context.Context, _ *mcpsdk.CallToolRequest, args WaitForIdleInput) (*mcpsdk.CallToolResult, WaitForIdleOutput, error) {
	workspaceName, err := resolveWorkspaceForRead(args.Workspace, args.SourceWorkspace, "wait_for_idle")
	if err != nil {
		if s.logger != nil {
			s.logger.Log(agent.ActionWaitIdle, DefaultWorkspace, args.Slot, map[string]interface{}{
				"error": err.Error(),
			})
		}
		return nil, WaitForIdleOutput{}, err
	}
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
	fence, _ := s.getFenceState(workspaceName, args.Slot)
	deadline := time.Now().Add(timeout)
	start := time.Now()

	for {
		if s.checkIdle(target, agentType, workspaceName, args.Slot) {
			// When fence is active, capture full scrollback so we can
			// find both the opening and closing tags for extraction,
			// even for long responses where the opening tag scrolled
			// past the default capture window.
			captureLines := lines
			if fence {
				captureLines = 0 // full scrollback
			}
			output, err := tmuxCapturePane(target, captureLines)
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
			cleanedOutput := trimOutput(cleanOutput(output), fence)
			if s.artifacts != nil {
				s.artifacts.Set(workspaceName, args.Slot, cleanedOutput)
			}
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
			cleanedOutput := trimOutput(cleanOutput(output), fence)
			if s.artifacts != nil {
				s.artifacts.Set(workspaceName, args.Slot, cleanedOutput)
			}
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

func (s *Server) handleMoveTerminal(_ context.Context, _ *mcpsdk.CallToolRequest, args MoveTerminalInput) (*mcpsdk.CallToolResult, MoveTerminalOutput, error) {
	srcWorkspace, err := resolveWorkspaceForRead(args.Workspace, args.SourceWorkspace, "move_terminal")
	if err != nil {
		return nil, MoveTerminalOutput{}, err
	}
	dstWorkspace := strings.TrimSpace(args.TargetWorkspace)
	if dstWorkspace == "" {
		return nil, MoveTerminalOutput{}, fmt.Errorf("target_workspace is required")
	}
	if srcWorkspace == dstWorkspace {
		return nil, MoveTerminalOutput{}, fmt.Errorf("source and target workspaces are the same (%q)", srcWorkspace)
	}

	// Validate source slot is tracked.
	target, ok := s.getTmuxTarget(srcWorkspace, args.Slot)
	if !ok {
		return nil, MoveTerminalOutput{}, fmt.Errorf("no agent tracked in workspace %q slot %d", srcWorkspace, args.Slot)
	}
	agentType := s.getAgentType(srcWorkspace, args.Slot)
	mode := s.getSpawnMode(srcWorkspace, args.Slot)

	// Look up both workspaces in the registry to get desktop numbers.
	srcWsInfo, err := workspacepkg.GetWorkspaceByName(srcWorkspace)
	if err != nil {
		return nil, MoveTerminalOutput{}, fmt.Errorf("source workspace %q not found in registry: %w", srcWorkspace, err)
	}
	dstWsInfo, err := workspacepkg.GetWorkspaceByName(dstWorkspace)
	if err != nil {
		return nil, MoveTerminalOutput{}, fmt.Errorf("target workspace %q not found in registry: %w", dstWorkspace, err)
	}

	// Find the X11 window for this terminal.
	oldSessionName := target
	if idx := strings.Index(target, ":"); idx >= 0 {
		oldSessionName = target[:idx]
	}

	// Move X11 window to the target desktop.
	if mode == "window" && srcWsInfo.Desktop != dstWsInfo.Desktop {
		if windowID, err := platform.FindWindowByTitleStandalone(oldSessionName); err == nil && windowID != 0 {
			if err := platform.MoveWindowToDesktopStandalone(windowID, dstWsInfo.Desktop); err != nil {
				log.Printf("Warning: failed to move window to desktop %d: %v", dstWsInfo.Desktop, err)
			}
		}
	}

	// Update workspace registry: move the slot between workspaces.
	newSlot, err := workspacepkg.MoveTerminalBetweenWorkspaces(srcWsInfo.Desktop, args.Slot, dstWsInfo.Desktop)
	if err != nil {
		return nil, MoveTerminalOutput{}, fmt.Errorf("failed to update workspace registry: %w", err)
	}

	// Rename tmux session from old workspace naming to new.
	newSessionName := agent.SessionName(dstWorkspace, newSlot)
	newTarget := agent.TargetForSession(newSessionName)
	if err := s.multiplexer.RenameSession(oldSessionName, newSessionName); err != nil {
		log.Printf("Warning: failed to rename tmux session %q to %q: %v", oldSessionName, newSessionName, err)
	}

	// Transfer MCP tracking state: copy tracked agent, remove from source, compact
	// shifted source slots, then add to destination.
	s.mu.Lock()
	var ta trackedAgent
	if srcMap, ok := s.tracked[srcWorkspace]; ok {
		ta = srcMap[args.Slot]
		delete(srcMap, args.Slot)
	}
	s.mu.Unlock()

	if mode == "window" {
		if err := s.compactWindowSlots(srcWorkspace, args.Slot); err != nil {
			log.Printf("Warning: failed to compact source workspace %q after moving slot %d: %v", srcWorkspace, args.Slot, err)
		}
	}

	ta.tmuxTarget = newTarget
	s.mu.Lock()
	if s.tracked[dstWorkspace] == nil {
		s.tracked[dstWorkspace] = make(map[int]trackedAgent)
	}
	s.tracked[dstWorkspace][newSlot] = ta
	s.mu.Unlock()

	// Transfer artifact if present.
	if s.artifacts != nil {
		if art, ok := s.artifacts.Get(srcWorkspace, args.Slot); ok {
			s.artifacts.Set(dstWorkspace, newSlot, art.Output)
			s.artifacts.Clear(srcWorkspace, args.Slot)
		}
	}
	// Transfer read snapshot if present.
	if snap := s.getReadSnapshot(srcWorkspace, args.Slot); snap != "" {
		s.setReadSnapshot(dstWorkspace, newSlot, snap)
	}
	s.clearReadSnapshot(srcWorkspace, args.Slot)

	// Retile the current desktop.
	time.Sleep(300 * time.Millisecond)
	s.triggerRetile()

	if s.logger != nil {
		s.logger.Log(agent.ActionMoveTerminal, srcWorkspace, args.Slot, map[string]interface{}{
			"agent_type":       agentType,
			"spawn_mode":       mode,
			"source_workspace": srcWorkspace,
			"target_workspace": dstWorkspace,
			"source_slot":      args.Slot,
			"target_slot":      newSlot,
			"old_session":      oldSessionName,
			"new_session":      newSessionName,
		})
	}

	return nil, MoveTerminalOutput{
		SourceWorkspace: srcWorkspace,
		TargetWorkspace: dstWorkspace,
		SourceSlot:      args.Slot,
		TargetSlot:      newSlot,
		SessionName:     newTarget,
		Moved:           true,
	}, nil
}

type sessionRename struct {
	old string
	new string
}

// compactWindowSlots shifts tracked window-mode slots down after removing a
// slot from the workspace registry (which compacts indices). It also migrates
// artifacts/read snapshots and renames tmux sessions to keep slot suffixes aligned.
func (s *Server) compactWindowSlots(workspace string, removedSlot int) error {
	if removedSlot < 0 {
		return nil
	}

	shifts := make([]sessionRename, 0)
	artifactMoves := make([][2]int, 0) // [from,to]

	s.mu.Lock()
	ws := s.tracked[workspace]
	if ws == nil {
		s.mu.Unlock()
		return nil
	}

	newWS := make(map[int]trackedAgent, len(ws))
	for slot, ta := range ws {
		if slot <= removedSlot {
			newWS[slot] = ta
			continue
		}

		newSlot := slot - 1
		if ta.spawnMode == "window" {
			oldSession := ta.tmuxTarget
			if idx := strings.Index(oldSession, ":"); idx >= 0 {
				oldSession = oldSession[:idx]
			}
			if oldSession == "" {
				oldSession = agent.SessionName(workspace, slot)
			}
			newSession := agent.SessionName(workspace, newSlot)
			if oldSession != newSession {
				shifts = append(shifts, sessionRename{old: oldSession, new: newSession})
			}
			ta.tmuxTarget = agent.TargetForSession(newSession)
		}
		newWS[newSlot] = ta
		artifactMoves = append(artifactMoves, [2]int{slot, newSlot})
	}
	s.tracked[workspace] = newWS

	if snaps := s.readSnapshots[workspace]; snaps != nil {
		newSnaps := make(map[int]string, len(snaps))
		for slot, out := range snaps {
			if slot <= removedSlot {
				newSnaps[slot] = out
				continue
			}
			newSnaps[slot-1] = out
		}
		s.readSnapshots[workspace] = newSnaps
	}
	s.mu.Unlock()

	sort.Slice(artifactMoves, func(i, j int) bool {
		return artifactMoves[i][0] < artifactMoves[j][0]
	})
	for _, mv := range artifactMoves {
		fromSlot := mv[0]
		toSlot := mv[1]
		if art, ok := s.artifacts.Get(workspace, fromSlot); ok {
			s.artifacts.Set(workspace, toSlot, art.Output)
			s.artifacts.Clear(workspace, fromSlot)
		}
	}

	for _, rename := range shifts {
		if err := s.multiplexer.RenameSession(rename.old, rename.new); err != nil {
			// Best effort: keep tracking in sync even if an external process already moved/killed it.
			log.Printf("Warning: failed to rename shifted session %q -> %q: %v", rename.old, rename.new, err)
		}
	}

	return nil
}

// isAgentModeWorkspace returns true if the given workspace name corresponds
// to an agent-mode workspace. Falls back to true for the default MCP workspace.
func isAgentModeWorkspace(name string) bool {
	wsInfo, err := workspacepkg.GetWorkspaceByName(name)
	if err != nil {
		return name == DefaultWorkspace
	}
	return wsInfo.AgentMode
}

func isKnownModel(model string, known []string) bool {
	for _, k := range known {
		if strings.TrimSpace(k) == model {
			return true
		}
	}
	return false
}

type projectWorkspaceFile struct {
	Workspace string `yaml:"workspace"`
	Project   struct {
		RootMarker string `yaml:"root_marker"`
	} `yaml:"project"`
}

type projectWorkspaceBinding struct {
	Workspace  string
	RootMarker string
	SourcePath string
}

// resolveWorkspaceForSpawn resolves workspace selection for spawn_agent using
// deterministic precedence:
// explicit_arg -> source_workspace_hint -> project_marker -> single_registered_agent_workspace -> error
func resolveWorkspaceForSpawn(ws, sourceWorkspace string) (string, error) {
	return resolveWorkspaceDeterministic(ws, sourceWorkspace, "spawn_agent", true)
}

// resolveWorkspaceForRead resolves workspace selection for read/list/send/kill/wait
// tools using deterministic precedence:
// explicit_arg -> source_workspace_hint -> project_marker -> single_registered_agent_workspace -> error
func resolveWorkspaceForRead(ws, sourceWorkspace, toolName string) (string, error) {
	return resolveWorkspaceDeterministic(ws, sourceWorkspace, toolName, false)
}

func resolveWorkspaceDeterministic(ws, sourceWorkspace, toolName string, requireRegistered bool) (string, error) {
	if explicit := strings.TrimSpace(ws); explicit != "" {
		return validateResolvedWorkspace(explicit, requireRegistered, "workspace")
	}

	if hint := strings.TrimSpace(sourceWorkspace); hint != "" {
		return validateResolvedWorkspace(hint, requireRegistered, "source_workspace")
	}

	projectWorkspace, projectSource, err := resolveWorkspaceFromProjectMarker()
	if err != nil {
		return "", fmt.Errorf("failed to resolve workspace for %s: %w", toolName, err)
	}
	if projectWorkspace != "" {
		resolved, err := validateResolvedWorkspace(projectWorkspace, requireRegistered, "project workspace")
		if err != nil {
			return "", fmt.Errorf(
				"failed to resolve workspace for %s: %w (project config: %s)",
				toolName,
				err,
				projectSource,
			)
		}
		return resolved, nil
	}

	candidates, err := listRegisteredAgentWorkspaces()
	if err != nil {
		return "", fmt.Errorf("failed to resolve workspace for %s: %w", toolName, err)
	}

	switch len(candidates) {
	case 1:
		return validateResolvedWorkspace(candidates[0], requireRegistered, "registered workspace")
	case 0:
		return "", fmt.Errorf(
			"unable to resolve workspace for %s: pass workspace explicitly or provide source_workspace (or set .termtile/workspace.yaml)",
			toolName,
		)
	default:
		return "", fmt.Errorf(
			"ambiguous workspace for %s: multiple registered agent-mode workspaces found (%s); pass workspace explicitly or provide source_workspace",
			toolName,
			strings.Join(candidates, ", "),
		)
	}
}

func validateResolvedWorkspace(name string, requireRegistered bool, source string) (string, error) {
	workspace := strings.TrimSpace(name)
	if workspace == "" {
		return "", fmt.Errorf("%s is empty", source)
	}
	if workspace == DefaultWorkspace && requireRegistered {
		// Legacy compatibility: allow mcp-agents only when no other registered
		// agent workspaces exist. This prevents accidental routing to the legacy
		// default when explicit project workspaces (e.g. otto/termtile) are active.
		candidates, err := listRegisteredAgentWorkspaces()
		if err != nil {
			return "", fmt.Errorf("failed to validate workspace %q: %w", workspace, err)
		}
		if len(candidates) > 0 {
			return "", fmt.Errorf(
				"%s %q is a legacy default and is not registered; use an explicit registered workspace (%s) or provide source_workspace",
				source,
				workspace,
				strings.Join(candidates, ", "),
			)
		}
		return workspace, nil
	}
	if !requireRegistered {
		return workspace, nil
	}
	if _, err := workspacepkg.GetWorkspaceByName(workspace); err != nil {
		return "", fmt.Errorf(
			"%s %q not found in registry; pass a valid workspace name or create/load one first",
			source,
			workspace,
		)
	}
	return workspace, nil
}

func listRegisteredAgentWorkspaces() ([]string, error) {
	all, err := workspacepkg.GetAllWorkspaces()
	if err != nil {
		return nil, err
	}

	candidates := make([]string, 0, len(all))
	seen := make(map[string]struct{}, len(all))
	for _, info := range all {
		name := strings.TrimSpace(info.Name)
		if name == "" || !info.AgentMode {
			continue
		}
		if _, ok := seen[name]; ok {
			continue
		}
		seen[name] = struct{}{}
		candidates = append(candidates, name)
	}
	sort.Strings(candidates)
	return candidates, nil
}

func resolveWorkspaceFromProjectMarker() (workspace string, sourcePath string, err error) {
	cwd, err := os.Getwd()
	if err != nil {
		return "", "", fmt.Errorf("failed to determine current working directory: %w", err)
	}

	for dir := cwd; ; dir = filepath.Dir(dir) {
		binding, found, err := loadProjectWorkspaceBinding(dir)
		if err != nil {
			return "", "", err
		}
		if found {
			marker := strings.TrimSpace(binding.RootMarker)
			if marker == "" {
				marker = ".git"
			}

			markerPath := marker
			if !filepath.IsAbs(markerPath) {
				markerPath = filepath.Join(dir, markerPath)
			}
			if _, err := os.Stat(markerPath); err != nil {
				if os.IsNotExist(err) {
					return "", "", fmt.Errorf(
						"project workspace config %q references missing project.root_marker %q; pass workspace explicitly or fix project config",
						binding.SourcePath,
						marker,
					)
				}
				return "", "", fmt.Errorf(
					"failed to stat project.root_marker %q from %q: %w",
					marker,
					binding.SourcePath,
					err,
				)
			}

			return strings.TrimSpace(binding.Workspace), binding.SourcePath, nil
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
	}

	return "", "", nil
}

func loadProjectWorkspaceBinding(dir string) (projectWorkspaceBinding, bool, error) {
	termtileDir := filepath.Join(dir, ".termtile")
	projectPath := filepath.Join(termtileDir, "workspace.yaml")
	localPath := filepath.Join(termtileDir, "local.yaml")

	projectCfg, projectExists, err := readProjectWorkspaceFile(projectPath)
	if err != nil {
		return projectWorkspaceBinding{}, false, err
	}
	localCfg, localExists, err := readProjectWorkspaceFile(localPath)
	if err != nil {
		return projectWorkspaceBinding{}, false, err
	}
	if !projectExists && !localExists {
		return projectWorkspaceBinding{}, false, nil
	}

	workspace := strings.TrimSpace(localCfg.Workspace)
	sourcePath := localPath
	if workspace == "" {
		workspace = strings.TrimSpace(projectCfg.Workspace)
		sourcePath = projectPath
	}
	if workspace == "" {
		return projectWorkspaceBinding{}, false, nil
	}

	rootMarker := strings.TrimSpace(localCfg.Project.RootMarker)
	if rootMarker == "" {
		rootMarker = strings.TrimSpace(projectCfg.Project.RootMarker)
	}
	if rootMarker == "" {
		rootMarker = ".git"
	}

	return projectWorkspaceBinding{
		Workspace:  workspace,
		RootMarker: rootMarker,
		SourcePath: sourcePath,
	}, true, nil
}

func readProjectWorkspaceFile(path string) (projectWorkspaceFile, bool, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return projectWorkspaceFile{}, false, nil
		}
		return projectWorkspaceFile{}, false, fmt.Errorf("failed to read project workspace file %q: %w", path, err)
	}

	var cfg projectWorkspaceFile
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return projectWorkspaceFile{}, false, fmt.Errorf("failed to parse project workspace file %q: %w", path, err)
	}
	return cfg, true, nil
}
