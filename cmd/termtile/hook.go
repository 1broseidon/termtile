package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/1broseidon/termtile/internal/config"
	"github.com/1broseidon/termtile/internal/mcp"
)

const hookArtifactFileName = "output.json"

type hookEmitPayload struct {
	Status    string    `json:"status"`
	Output    string    `json:"output"`
	Timestamp time.Time `json:"timestamp"`
}

type optionalStringFlag struct {
	value string
	set   bool
}

func (f *optionalStringFlag) String() string {
	return f.value
}

func (f *optionalStringFlag) Set(value string) error {
	f.value = value
	f.set = true
	return nil
}

const (
	contextFileName    = "context.md"
	checkpointFileName = "checkpoint.json"
)

func printHookUsage(w io.Writer) {
	fmt.Fprintln(w, "Usage:")
	fmt.Fprintln(w, "  termtile hook start --auto")
	fmt.Fprintln(w, "  termtile hook check --auto")
	fmt.Fprintln(w, "  termtile hook emit  --auto [--output TEXT]")
	fmt.Fprintln(w, "  termtile hook emit  --slot N [--workspace NAME] [--output TEXT]")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "Commands:")
	fmt.Fprintln(w, "  start   Return context for session start (reads context.md)")
	fmt.Fprintln(w, "  check   Return mid-conversation guidance (reads checkpoint.json)")
	fmt.Fprintln(w, "  emit    Write hook output JSON artifact for a workspace slot")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "Run 'termtile hook <command> --help' for command-specific options.")
}

func runHook(args []string) int {
	if len(args) == 0 {
		printHookUsage(os.Stderr)
		return 2
	}
	if args[0] == "help" || args[0] == "-h" || args[0] == "--help" {
		printHookUsage(os.Stdout)
		return 0
	}

	switch args[0] {
	case "start":
		return runHookStart(args[1:])
	case "check":
		return runHookCheck(args[1:])
	case "emit":
		return runHookEmit(args[1:])
	default:
		fmt.Fprintf(os.Stderr, "Unknown hook command: %s\n\n", args[0])
		printHookUsage(os.Stderr)
		return 2
	}
}

// runHookStart reads context.md from the artifact dir and prints it to stdout.
// The output format is driven by the agent's hook_output config template.
// Exits silently if no context file exists.
func runHookStart(args []string) int {
	fs := flag.NewFlagSet("start", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	autoDetect := fs.Bool("auto", false, "Auto-detect workspace/slot from tmux session name")
	workspaceName := fs.String("workspace", mcp.DefaultWorkspace, "Target workspace name")
	slot := fs.Int("slot", -1, "Target workspace slot index")

	if err := fs.Parse(args); err != nil {
		return 2
	}

	if *autoDetect {
		ws, sl, err := detectWorkspaceSlotFromTmux()
		if err != nil {
			return 0
		}
		*workspaceName = ws
		*slot = sl
	}
	if *slot < 0 {
		return 0
	}

	artifactDir, err := mcp.GetArtifactDir(*workspaceName, *slot)
	if err != nil {
		return 0
	}

	contextPath := filepath.Join(artifactDir, contextFileName)
	data, err := os.ReadFile(contextPath)
	if err != nil {
		return 0
	}

	content := strings.TrimSpace(string(data))
	if content == "" {
		return 0
	}

	fmt.Print(renderHookOutput(*workspaceName, *slot, content))
	return 0
}

// runHookCheck reads checkpoint.json from the artifact dir and returns it
// as context for the agent. Output format is driven by hook_output config.
// Exits silently if no checkpoint exists.
func runHookCheck(args []string) int {
	fs := flag.NewFlagSet("check", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	autoDetect := fs.Bool("auto", false, "Auto-detect workspace/slot from tmux session name")
	workspaceName := fs.String("workspace", mcp.DefaultWorkspace, "Target workspace name")
	slot := fs.Int("slot", -1, "Target workspace slot index")

	if err := fs.Parse(args); err != nil {
		return 2
	}

	if *autoDetect {
		ws, sl, err := detectWorkspaceSlotFromTmux()
		if err != nil {
			return 0
		}
		*workspaceName = ws
		*slot = sl
	}
	if *slot < 0 {
		return 0
	}

	artifactDir, err := mcp.GetArtifactDir(*workspaceName, *slot)
	if err != nil {
		return 0
	}

	checkpointPath := filepath.Join(artifactDir, checkpointFileName)
	data, err := os.ReadFile(checkpointPath)
	if err != nil {
		return 0
	}

	content := strings.TrimSpace(string(data))
	if content == "" {
		return 0
	}

	fmt.Print(renderHookOutput(*workspaceName, *slot, content))

	// Consume the checkpoint (one-shot steering).
	_ = os.Remove(checkpointPath)
	return 0
}

func runHookEmit(args []string) int {
	fs := flag.NewFlagSet("emit", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	fs.Usage = func() {
		fmt.Fprintln(os.Stderr, "Usage: termtile hook emit --auto [--output TEXT]")
		fmt.Fprintln(os.Stderr, "       termtile hook emit --slot N [--workspace NAME] [--output TEXT]")
		fmt.Fprintln(os.Stderr, "")
		fmt.Fprintln(os.Stderr, "Write structured output to the workspace slot artifact file.")
		fmt.Fprintln(os.Stderr, "")
		fmt.Fprintln(os.Stderr, "Flags:")
		fs.PrintDefaults()
	}

	autoDetect := fs.Bool("auto", false, "Auto-detect workspace/slot from tmux session name")
	workspaceName := fs.String("workspace", mcp.DefaultWorkspace, "Target workspace name")
	slot := fs.Int("slot", -1, "Target workspace slot index")
	var outputFlag optionalStringFlag
	fs.Var(&outputFlag, "output", "Output text (if omitted, read from stdin)")

	if err := fs.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return 0
		}
		return 2
	}
	if fs.NArg() != 0 {
		fmt.Fprintf(os.Stderr, "emit does not accept positional arguments: %s\n", strings.Join(fs.Args(), " "))
		fs.Usage()
		return 2
	}

	// Auto-detect workspace/slot from tmux session name if --auto is set.
	// Also extract output from transcript if stdin contains hook context.
	var output string
	if *autoDetect {
		ws, sl, err := detectWorkspaceSlotFromTmux()
		if err != nil {
			fmt.Fprintf(os.Stderr, "auto-detect failed: %v\n", err)
			return 1
		}
		*workspaceName = ws
		*slot = sl

		// In --auto mode, stdin contains hook context JSON.
		// Use hook_response_field from config to extract the response.
		agentCfg := loadAgentConfig(*workspaceName, *slot)
		extractedOutput, err := extractOutputFromHookContext(agentCfg.HookResponseField)
		if err != nil {
			fmt.Fprintf(os.Stderr, "failed to extract output from hook context: %v\n", err)
			return 1
		}
		output = extractedOutput
	} else {
		// Manual mode: use --output flag or read from stdin.
		var err error
		output, err = resolveHookEmitOutput(outputFlag)
		if err != nil {
			if errors.Is(err, errHookOutputRequired) {
				fmt.Fprintln(os.Stderr, err)
				fs.Usage()
				return 2
			}
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
	}

	if *slot < 0 {
		fmt.Fprintln(os.Stderr, "--slot must be >= 0 (or use --auto)")
		fs.Usage()
		return 2
	}

	payload := hookEmitPayload{
		Status:    "complete",
		Output:    output,
		Timestamp: time.Now().UTC(),
	}
	if err := writeHookArtifact(*workspaceName, *slot, payload); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}

	return 0
}

// transcriptEntry represents an entry in the Claude Code transcript JSONL.
type transcriptEntry struct {
	Type    string `json:"type"` // "assistant", "user", "system", etc.
	Message struct {
		Role    string `json:"role"`
		Content []struct {
			Type string `json:"type"` // "text", "thinking", "tool_use", etc.
			Text string `json:"text"`
		} `json:"content"`
	} `json:"message"`
}

// extractOutputFromHookContext reads the agent hook context from stdin and
// returns the agent's response text. The extraction strategy is config-driven:
//
//   - If hook_response_field is set (e.g. "prompt_response"), extract that field
//     directly from the stdin JSON.
//   - Otherwise, fall back to transcript_path parsing (Claude Code style).
func extractOutputFromHookContext(responseField string) (string, error) {
	// Read stdin (hook context JSON)
	stdinData, err := io.ReadAll(os.Stdin)
	if err != nil {
		return "", fmt.Errorf("failed to read stdin: %w", err)
	}
	if len(stdinData) == 0 {
		return "", errors.New("no hook context on stdin")
	}

	// Parse stdin as generic JSON.
	var ctx map[string]interface{}
	if err := json.Unmarshal(stdinData, &ctx); err != nil {
		return "", fmt.Errorf("failed to parse hook context: %w", err)
	}

	// Config-driven: extract response from the named field.
	if responseField != "" {
		if val, ok := ctx[responseField]; ok {
			if s, ok := val.(string); ok && s != "" {
				return s, nil
			}
		}
	}

	// Fallback: parse the transcript file for the last assistant response.
	tp, _ := ctx["transcript_path"].(string)
	if tp == "" {
		if responseField != "" {
			return "", fmt.Errorf("field %q is empty and no transcript_path in hook context", responseField)
		}
		return "", errors.New("no transcript_path in hook context")
	}

	// Retry reading the transcript — the Stop hook may fire before the
	// assistant message is flushed to the JSONL file on disk.
	const maxRetries = 10
	const retryDelay = 300 * time.Millisecond

	for attempt := 0; attempt < maxRetries; attempt++ {
		if attempt > 0 {
			time.Sleep(retryDelay)
		}

		text, err := parseTranscriptForAssistant(tp)
		if err != nil {
			if attempt < maxRetries-1 && strings.Contains(err.Error(), "no assistant response") {
				continue
			}
			return "", err
		}
		if strings.TrimSpace(text) == "" {
			if attempt < maxRetries-1 {
				continue
			}
			return "", errors.New("assistant response is empty after retries")
		}
		return text, nil
	}

	return "", errors.New("no assistant response found in transcript after retries")
}

// parseTranscriptForAssistant reads a Claude Code transcript JSONL file and
// returns the text content of the last assistant message.
func parseTranscriptForAssistant(path string) (string, error) {
	transcriptData, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("failed to read transcript %s: %w", path, err)
	}

	var lastAssistantText string
	lines := strings.Split(string(transcriptData), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var entry transcriptEntry
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			continue // Skip malformed lines
		}
		if entry.Type == "assistant" {
			for _, content := range entry.Message.Content {
				if content.Type == "text" && content.Text != "" {
					lastAssistantText = content.Text
				}
			}
		}
	}

	if lastAssistantText == "" {
		return "", errors.New("no assistant response found in transcript")
	}

	return lastAssistantText, nil
}

// detectWorkspaceSlotFromTmux parses the tmux session name to extract workspace and slot.
// Session name format: termtile-{workspace}-{slot}
func detectWorkspaceSlotFromTmux() (workspace string, slot int, err error) {
	// Get current tmux session name
	out, err := exec.Command("tmux", "display-message", "-p", "#S").Output()
	if err != nil {
		return "", -1, fmt.Errorf("not in a tmux session: %w", err)
	}
	sessionName := strings.TrimSpace(string(out))
	if sessionName == "" {
		return "", -1, errors.New("empty tmux session name")
	}

	// Parse termtile-{workspace}-{slot}
	// Workspace can contain hyphens, so we match from the end.
	re := regexp.MustCompile(`^termtile-(.+)-(\d+)$`)
	matches := re.FindStringSubmatch(sessionName)
	if len(matches) != 3 {
		return "", -1, fmt.Errorf("session name %q does not match termtile-{workspace}-{slot} format", sessionName)
	}

	workspace = matches[1]
	slot, err = strconv.Atoi(matches[2])
	if err != nil {
		return "", -1, fmt.Errorf("invalid slot number in session name: %w", err)
	}

	return workspace, slot, nil
}

// loadAgentConfig reads the agent type from the artifact dir's agent_meta.json,
// then looks up that agent's config from termtile's configuration. Returns a
// zero-value AgentConfig if anything fails (graceful degradation).
func loadAgentConfig(workspace string, slot int) config.AgentConfig {
	agentType, err := mcp.ReadAgentMeta(workspace, slot)
	if err != nil || agentType == "" {
		return config.AgentConfig{}
	}
	cfg := config.DefaultConfig()
	if loaded, err := config.Load(); err == nil {
		cfg = loaded
	}
	if ac, ok := cfg.Agents[agentType]; ok {
		return ac
	}
	return config.AgentConfig{}
}

// renderHookOutput formats content using the agent's hook_output template from
// config. Falls back to plain text if no template is configured.
func renderHookOutput(workspace string, slot int, content string) string {
	agentCfg := loadAgentConfig(workspace, slot)
	if agentCfg.HookOutput == nil {
		return content
	}
	rendered := deepCopyAndSubstituteContext(agentCfg.HookOutput, content)
	data, err := json.Marshal(rendered)
	if err != nil {
		return content
	}
	return string(data)
}

// deepCopyAndSubstituteContext recursively deep-copies a value, replacing the
// "{{context}}" string placeholder with the provided content.
func deepCopyAndSubstituteContext(v interface{}, content string) interface{} {
	switch val := v.(type) {
	case map[string]interface{}:
		out := make(map[string]interface{}, len(val))
		for k, v2 := range val {
			out[k] = deepCopyAndSubstituteContext(v2, content)
		}
		return out
	case []interface{}:
		out := make([]interface{}, len(val))
		for i, v2 := range val {
			out[i] = deepCopyAndSubstituteContext(v2, content)
		}
		return out
	case string:
		return strings.ReplaceAll(val, "{{context}}", content)
	default:
		return v
	}
}

var errHookOutputRequired = errors.New("emit requires --output or stdin input")

func resolveHookEmitOutput(outputFlag optionalStringFlag) (string, error) {
	if outputFlag.set {
		return outputFlag.value, nil
	}

	stdinInfo, err := os.Stdin.Stat()
	if err != nil {
		return "", fmt.Errorf("failed to inspect stdin: %w", err)
	}
	if stdinInfo.Mode()&os.ModeCharDevice != 0 {
		return "", errHookOutputRequired
	}

	raw, err := io.ReadAll(os.Stdin)
	if err != nil {
		return "", fmt.Errorf("failed to read stdin: %w", err)
	}
	return string(raw), nil
}

func writeHookArtifact(workspaceName string, slot int, payload hookEmitPayload) error {
	workspaceName = strings.TrimSpace(workspaceName)
	if workspaceName == "" {
		workspaceName = mcp.DefaultWorkspace
	}

	artifactDir, err := mcp.EnsureArtifactDir(workspaceName, slot)
	if err != nil {
		return fmt.Errorf("failed to ensure artifact directory: %w", err)
	}

	data, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal artifact payload: %w", err)
	}
	data = append(data, '\n')

	path := filepath.Join(artifactDir, hookArtifactFileName)
	tmpPath := path + ".tmp"

	if err := os.WriteFile(tmpPath, data, 0o644); err != nil {
		return fmt.Errorf("failed to write artifact %q: %w", tmpPath, err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("failed to finalize artifact %q: %w", path, err)
	}

	return nil
}
