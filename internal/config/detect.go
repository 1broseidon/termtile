package config

import (
	"os/exec"
	"sort"
)

// DetectedAgent represents a known agent CLI found on PATH.
type DetectedAgent struct {
	Name           string
	Path           string
	Configured     bool
	ProposedConfig AgentConfig
}

// DetectAgents scans PATH for known agent CLIs and returns detected entries.
// If an agent already exists in existing, it is marked configured and no
// proposed defaults are returned for that entry.
func DetectAgents(existing map[string]AgentConfig) []DetectedAgent {
	known := knownAgentDefaults()
	detected := make([]DetectedAgent, 0, len(known))

	for name, cfg := range known {
		path, err := exec.LookPath(cfg.Command)
		if err != nil {
			continue
		}

		entry := DetectedAgent{
			Name: name,
			Path: path,
		}
		if _, ok := existing[name]; ok {
			entry.Configured = true
		} else {
			entry.ProposedConfig = cloneAgentConfig(cfg)
		}
		detected = append(detected, entry)
	}

	sort.Slice(detected, func(i, j int) bool {
		return detected[i].Name < detected[j].Name
	})
	return detected
}

func knownAgentDefaults() map[string]AgentConfig {
	defaults := DefaultConfig().Agents
	known := make(map[string]AgentConfig, len(defaults)+1)
	for name, cfg := range defaults {
		known[name] = cloneAgentConfig(cfg)
	}
	known["aider"] = AgentConfig{
		Command:     "aider",
		SpawnMode:   "window",
		PromptAsArg: true,
		Description: "Aider CLI agent",
	}
	return known
}

func cloneAgentConfig(cfg AgentConfig) AgentConfig {
	out := cfg
	if cfg.Args != nil {
		out.Args = append([]string(nil), cfg.Args...)
	}
	if cfg.Env != nil {
		out.Env = make(map[string]string, len(cfg.Env))
		for k, v := range cfg.Env {
			out.Env[k] = v
		}
	}
	if cfg.Models != nil {
		out.Models = append([]string(nil), cfg.Models...)
	}
	return out
}
