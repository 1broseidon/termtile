package mcp

import "time"

// SpawnAgentInput is the input for the spawn_agent tool.
type SpawnAgentInput struct {
	AgentType string  `json:"agent_type" jsonschema:"required,The agent type from config (e.g. claude, codex, aider)"`
	Workspace string  `json:"workspace,omitempty" jsonschema:"Workspace name (default: active workspace on current desktop; fallback: mcp-agents)"`
	Cwd       string  `json:"cwd,omitempty" jsonschema:"Working directory for the agent"`
	Task      string  `json:"task,omitempty" jsonschema:"Initial task/prompt to send after agent starts. When prompt_as_arg is true for the agent, the task is passed as a CLI argument for instant delivery; otherwise it is sent via tmux send-keys after the agent is ready."`
	Model     *string `json:"model,omitempty" jsonschema:"Optional model name to pass to the agent CLI. If omitted, the agent config default_model is used when configured."`
	Window    *bool   `json:"window,omitempty" jsonschema:"When true, spawn the agent in a new terminal window instead of a tmux pane. Overrides the agent's configured spawn_mode."`
	DependsOn []int   `json:"depends_on,omitempty" jsonschema:"Optional list of slot numbers that must be idle before spawning this agent. If any dependency slot is missing or killed, spawn fails."`
	// DependsOnTimeout is only used when DependsOn is set.
	// Value is seconds; default is 300.
	DependsOnTimeout int `json:"depends_on_timeout,omitempty" jsonschema:"Timeout in seconds to wait for depends_on slots to become idle (default: 300). Only used when depends_on is set."`
}

// SpawnAgentOutput is the output for the spawn_agent tool.
type SpawnAgentOutput struct {
	Slot        int    `json:"slot"`
	SessionName string `json:"session_name"`
	AgentType   string `json:"agent_type"`
	Workspace   string `json:"workspace"`
	SpawnMode   string `json:"spawn_mode"`
}

// SendToAgentInput is the input for the send_to_agent tool.
type SendToAgentInput struct {
	Slot      int    `json:"slot" jsonschema:"required,Slot index of the target agent"`
	Text      string `json:"text" jsonschema:"required,Text to send to the agent"`
	Workspace string `json:"workspace,omitempty" jsonschema:"Workspace name (default: active workspace on current desktop; fallback: mcp-agents)"`
}

// ReadFromAgentInput is the input for the read_from_agent tool.
type ReadFromAgentInput struct {
	Slot      int    `json:"slot" jsonschema:"required,Slot index to read from"`
	Lines     int    `json:"lines,omitempty" jsonschema:"Number of lines to capture (default: 50)"`
	Clean     bool   `json:"clean,omitempty" jsonschema:"When true, strip TUI chrome and control characters from output (default: false)"`
	Workspace string `json:"workspace,omitempty" jsonschema:"Workspace name (default: active workspace on current desktop; fallback: mcp-agents)"`
	Pattern   string `json:"pattern,omitempty" jsonschema:"Optional text pattern to wait for. When set, polls until pattern appears or timeout."`
	Timeout   int    `json:"timeout,omitempty" jsonschema:"Timeout in seconds when waiting for pattern (default: 30). Only used when pattern is set."`
}

// ReadFromAgentOutput is the output for the read_from_agent tool.
type ReadFromAgentOutput struct {
	Output      string `json:"output"`
	SessionName string `json:"session_name"`
	Found       *bool  `json:"found,omitempty"`
}

// ListAgentsInput is the input for the list_agents tool.
type ListAgentsInput struct {
	Workspace string `json:"workspace,omitempty" jsonschema:"Workspace name (default: active workspace on current desktop; fallback: mcp-agents)"`
}

// AgentInfo describes a single running agent.
type AgentInfo struct {
	Slot           int    `json:"slot"`
	AgentType      string `json:"agent_type"`
	SessionName    string `json:"session_name"`
	CurrentCommand string `json:"current_command"`
	IsIdle         bool   `json:"is_idle"`
	Exists         bool   `json:"exists"`
	SpawnMode      string `json:"spawn_mode"`
}

// ListAgentsOutput is the output for the list_agents tool.
type ListAgentsOutput struct {
	Workspace string      `json:"workspace"`
	Agents    []AgentInfo `json:"agents"`
}

// KillAgentInput is the input for the kill_agent tool.
type KillAgentInput struct {
	Slot      int    `json:"slot" jsonschema:"required,Slot index of agent to kill"`
	Workspace string `json:"workspace,omitempty" jsonschema:"Workspace name (default: active workspace on current desktop; fallback: mcp-agents)"`
}

// KillAgentOutput is the output for the kill_agent tool.
type KillAgentOutput struct {
	SessionName string `json:"session_name"`
	Killed      bool   `json:"killed"`
}

// WaitForIdleInput is the input for the wait_for_idle tool.
type WaitForIdleInput struct {
	Slot      int    `json:"slot" jsonschema:"required,Slot index to monitor"`
	Timeout   int    `json:"timeout,omitempty" jsonschema:"Timeout in seconds (default: 120)"`
	Lines     int    `json:"lines,omitempty" jsonschema:"Number of lines to capture when idle (default: 100)"`
	Workspace string `json:"workspace,omitempty" jsonschema:"Workspace name (default: active workspace on current desktop; fallback: mcp-agents)"`
}

// WaitForIdleOutput is the output for the wait_for_idle tool.
type WaitForIdleOutput struct {
	IsIdle      bool   `json:"is_idle"`
	Output      string `json:"output"`
	SessionName string `json:"session_name"`
}

// GetArtifactArgs is the input for the get_artifact tool.
type GetArtifactArgs struct {
	Slot      int    `json:"slot" jsonschema:"required,Slot index to fetch artifact from"`
	Workspace string `json:"workspace,omitempty" jsonschema:"Workspace name (default: active workspace on current desktop; fallback: mcp-agents)"`
}

// GetArtifactOutput is the output for the get_artifact tool.
type GetArtifactOutput struct {
	Workspace      string    `json:"workspace"`
	Slot           int       `json:"slot"`
	Output         string    `json:"output"`
	Truncated      bool      `json:"truncated"`
	Warning        string    `json:"warning,omitempty"`
	OriginalBytes  int       `json:"original_bytes"`
	StoredBytes    int       `json:"stored_bytes"`
	LastUpdatedUTC time.Time `json:"last_updated_utc"`
}
