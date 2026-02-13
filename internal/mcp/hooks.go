package mcp

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/1broseidon/termtile/internal/config"
)

// Default hook commands used when output_mode is "hooks" and the user hasn't
// provided explicit hook overrides.
const (
	defaultOnStart = "termtile hook start --auto"
	defaultOnCheck = "termtile hook check --auto"
	defaultOnEnd   = "termtile hook emit --auto"
)

// resolveHooks returns the effective hooks for an agent, applying defaults
// when output_mode is "hooks" and no explicit command is set.
func resolveHooks(agentCfg config.AgentConfig) config.AgentHooks {
	h := agentCfg.Hooks
	mode := strings.ToLower(strings.TrimSpace(agentCfg.OutputMode))
	if mode == "" {
		mode = "hooks"
	}
	if mode != "hooks" {
		return h
	}
	if h.OnStart == "" {
		h.OnStart = defaultOnStart
	}
	if h.OnCheck == "" {
		h.OnCheck = defaultOnCheck
	}
	if h.OnEnd == "" {
		h.OnEnd = defaultOnEnd
	}
	return h
}

// renderHookSettings generates the native hook configuration JSON string for
// the given agent config using data-driven templates. Returns empty string if
// the agent doesn't support hook injection (no delivery, no templates, etc.).
func renderHookSettings(agentCfg config.AgentConfig, hooks config.AgentHooks) string {
	delivery := strings.ToLower(strings.TrimSpace(agentCfg.HookDelivery))
	if delivery == "" || delivery == "none" {
		return ""
	}
	if agentCfg.HookEvents == nil || agentCfg.HookEntry == nil || agentCfg.HookWrapper == nil {
		return ""
	}

	// Abstract hook names and their resolved commands.
	abstractHooks := map[string]string{
		"on_start": hooks.OnStart,
		"on_check": hooks.OnCheck,
		"on_end":   hooks.OnEnd,
	}

	// Build the events map: nativeEventName → [renderedEntry, ...]
	eventsMap := make(map[string]interface{})
	for abstract, command := range abstractHooks {
		if command == "" {
			continue
		}
		nativeEvent, ok := agentCfg.HookEvents[abstract]
		if !ok || nativeEvent == "" {
			continue
		}

		subs := map[string]string{
			"{{command}}": command,
			"{{event}}":   nativeEvent,
			"{{name}}":    abstract,
		}
		rendered := deepCopyAndSubstitute(agentCfg.HookEntry, subs)
		eventsMap[nativeEvent] = []interface{}{rendered}
	}

	if len(eventsMap) == 0 {
		return ""
	}

	// Render the wrapper, replacing the "{{events}}" sentinel with the events map.
	wrapper := deepCopyAndSubstituteEvents(agentCfg.HookWrapper, eventsMap)

	data, err := json.Marshal(wrapper)
	if err != nil {
		return ""
	}
	return string(data)
}

// deepCopyAndSubstitute recursively deep-copies a value, replacing string
// placeholders found in subs. Works on maps, slices, and strings.
func deepCopyAndSubstitute(v interface{}, subs map[string]string) interface{} {
	switch val := v.(type) {
	case map[string]interface{}:
		out := make(map[string]interface{}, len(val))
		for k, v2 := range val {
			out[k] = deepCopyAndSubstitute(v2, subs)
		}
		return out
	case []interface{}:
		out := make([]interface{}, len(val))
		for i, v2 := range val {
			out[i] = deepCopyAndSubstitute(v2, subs)
		}
		return out
	case string:
		s := val
		for placeholder, replacement := range subs {
			s = strings.ReplaceAll(s, placeholder, replacement)
		}
		return s
	default:
		return v
	}
}

// deepCopyAndSubstituteEvents recursively deep-copies a value, replacing the
// "{{events}}" string sentinel with the provided events map. This performs
// value replacement — the sentinel string is replaced with the map directly.
func deepCopyAndSubstituteEvents(v interface{}, eventsMap map[string]interface{}) interface{} {
	switch val := v.(type) {
	case map[string]interface{}:
		out := make(map[string]interface{}, len(val))
		for k, v2 := range val {
			out[k] = deepCopyAndSubstituteEvents(v2, eventsMap)
		}
		return out
	case []interface{}:
		out := make([]interface{}, len(val))
		for i, v2 := range val {
			out[i] = deepCopyAndSubstituteEvents(v2, eventsMap)
		}
		return out
	case string:
		if val == "{{events}}" {
			return eventsMap
		}
		return val
	default:
		return v
	}
}

// fileWriteInstructions returns a task suffix that instructs the agent to write
// its final response to the hook artifact file. Used for agents that don't
// support native hook injection (e.g. codex, cursor-agent).
func fileWriteInstructions(workspace string, slot int) string {
	path, err := artifactFilePath(workspace, slot)
	if err != nil {
		return ""
	}
	return fmt.Sprintf(
		"\n\nIMPORTANT — when you are completely finished, write your final summary as a JSON file to: %s\n"+
			"The file MUST contain exactly: {\"status\":\"complete\",\"output\":\"YOUR_SUMMARY_HERE\"}\n"+
			"Escape any quotes or newlines in your summary. This file signals completion to the orchestrator.",
		path,
	)
}
