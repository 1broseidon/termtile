package mcp

import (
	"encoding/json"
	"testing"

	"github.com/1broseidon/termtile/internal/config"
)

func TestRenderHookSettings_ClaudeParity(t *testing.T) {
	// Verify the data-driven renderer with Claude defaults produces the
	// expected JSON structure (parity with old hardcoded claudeCodeHookSettings).
	cfg := config.DefaultConfig()
	agentCfg := cfg.Agents["claude"]
	hooks := resolveHooks(agentCfg)

	settings := renderHookSettings(agentCfg, hooks)
	if settings == "" {
		t.Fatal("expected non-empty settings for claude agent")
	}

	var parsed map[string]interface{}
	if err := json.Unmarshal([]byte(settings), &parsed); err != nil {
		t.Fatalf("failed to parse settings JSON: %v", err)
	}

	// Verify top-level structure: {"hooks": {...}}
	hooksVal, ok := parsed["hooks"]
	if !ok {
		t.Fatal("expected 'hooks' key in settings")
	}
	hooksMap, ok := hooksVal.(map[string]interface{})
	if !ok {
		t.Fatalf("expected hooks to be a map, got %T", hooksVal)
	}

	// Verify all three Claude events are present.
	expectedEvents := map[string]string{
		"SessionStart": defaultOnStart,
		"PostToolUse":  defaultOnCheck,
		"Stop":         defaultOnEnd,
	}
	for event, expectedCmd := range expectedEvents {
		entryList, ok := hooksMap[event]
		if !ok {
			t.Errorf("missing event %q in hooks map", event)
			continue
		}
		entries, ok := entryList.([]interface{})
		if !ok || len(entries) == 0 {
			t.Errorf("expected non-empty entry list for event %q", event)
			continue
		}
		entry, ok := entries[0].(map[string]interface{})
		if !ok {
			t.Errorf("expected map entry for event %q, got %T", event, entries[0])
			continue
		}
		// Verify the entry has {"hooks": [{"type":"command","command":"..."}]}
		entryHooks, ok := entry["hooks"]
		if !ok {
			t.Errorf("missing 'hooks' in entry for event %q", event)
			continue
		}
		hooksList, ok := entryHooks.([]interface{})
		if !ok || len(hooksList) == 0 {
			t.Errorf("expected non-empty hooks list in entry for event %q", event)
			continue
		}
		hook, ok := hooksList[0].(map[string]interface{})
		if !ok {
			t.Errorf("expected map hook for event %q, got %T", event, hooksList[0])
			continue
		}
		if hook["type"] != "command" {
			t.Errorf("expected hook type 'command' for event %q, got %q", event, hook["type"])
		}
		if hook["command"] != expectedCmd {
			t.Errorf("expected hook command %q for event %q, got %q", expectedCmd, event, hook["command"])
		}
	}
}

func TestRenderHookSettings_GeminiParity(t *testing.T) {
	// Verify the data-driven renderer with Gemini defaults produces the
	// expected JSON structure (parity with old hardcoded geminiHookSettings).
	cfg := config.DefaultConfig()
	agentCfg := cfg.Agents["gemini"]
	hooks := resolveHooks(agentCfg)

	settings := renderHookSettings(agentCfg, hooks)
	if settings == "" {
		t.Fatal("expected non-empty settings for gemini agent")
	}

	var parsed map[string]interface{}
	if err := json.Unmarshal([]byte(settings), &parsed); err != nil {
		t.Fatalf("failed to parse settings JSON: %v", err)
	}

	// Verify top-level structure: {"hooks": {...}}
	hooksVal, ok := parsed["hooks"]
	if !ok {
		t.Fatal("expected 'hooks' key in settings")
	}
	hooksMap, ok := hooksVal.(map[string]interface{})
	if !ok {
		t.Fatalf("expected hooks to be a map, got %T", hooksVal)
	}

	// Verify all three Gemini events are present.
	expectedEvents := map[string]string{
		"BeforeAgent": defaultOnStart,
		"AfterTool":   defaultOnCheck,
		"AfterAgent":  defaultOnEnd,
	}
	for event, expectedCmd := range expectedEvents {
		entryList, ok := hooksMap[event]
		if !ok {
			t.Errorf("missing event %q in hooks map", event)
			continue
		}
		entries, ok := entryList.([]interface{})
		if !ok || len(entries) == 0 {
			t.Errorf("expected non-empty entry list for event %q", event)
			continue
		}
		entry, ok := entries[0].(map[string]interface{})
		if !ok {
			t.Errorf("expected map entry for event %q, got %T", event, entries[0])
			continue
		}
		// Gemini entries should have "matcher" field.
		if entry["matcher"] != "*" {
			t.Errorf("expected matcher '*' for event %q, got %v", event, entry["matcher"])
		}
		// Verify the entry has {"hooks": [{"type":"command","command":"..."}]}
		entryHooks, ok := entry["hooks"]
		if !ok {
			t.Errorf("missing 'hooks' in entry for event %q", event)
			continue
		}
		hooksList, ok := entryHooks.([]interface{})
		if !ok || len(hooksList) == 0 {
			t.Errorf("expected non-empty hooks list in entry for event %q", event)
			continue
		}
		hook, ok := hooksList[0].(map[string]interface{})
		if !ok {
			t.Errorf("expected map hook for event %q, got %T", event, hooksList[0])
			continue
		}
		if hook["type"] != "command" {
			t.Errorf("expected hook type 'command' for event %q, got %q", event, hook["type"])
		}
		if hook["command"] != expectedCmd {
			t.Errorf("expected hook command %q for event %q, got %q", expectedCmd, event, hook["command"])
		}
	}
}

func TestRenderHookSettings_NoDelivery(t *testing.T) {
	agentCfg := config.AgentConfig{
		Command: "test-agent",
	}
	hooks := config.AgentHooks{OnStart: "cmd1"}
	if got := renderHookSettings(agentCfg, hooks); got != "" {
		t.Errorf("expected empty string for agent with no delivery, got %q", got)
	}
}

func TestRenderHookSettings_NoneDelivery(t *testing.T) {
	agentCfg := config.AgentConfig{
		Command:      "test-agent",
		HookDelivery: "none",
	}
	hooks := config.AgentHooks{OnStart: "cmd1"}
	if got := renderHookSettings(agentCfg, hooks); got != "" {
		t.Errorf("expected empty string for delivery=none, got %q", got)
	}
}

func TestRenderHookSettings_NilTemplates(t *testing.T) {
	agentCfg := config.AgentConfig{
		Command:      "test-agent",
		HookDelivery: "cli_flag",
		HookEvents:   nil,
	}
	hooks := config.AgentHooks{OnStart: "cmd1"}
	if got := renderHookSettings(agentCfg, hooks); got != "" {
		t.Errorf("expected empty string for nil templates, got %q", got)
	}
}

func TestRenderHookSettings_EmptyHookCommands(t *testing.T) {
	agentCfg := config.AgentConfig{
		Command:      "test-agent",
		HookDelivery: "cli_flag",
		HookEvents:   map[string]string{"on_start": "Start"},
		HookEntry:    map[string]interface{}{"command": "{{command}}"},
		HookWrapper:  map[string]interface{}{"hooks": "{{events}}"},
	}
	// All hook commands are empty.
	hooks := config.AgentHooks{}
	if got := renderHookSettings(agentCfg, hooks); got != "" {
		t.Errorf("expected empty string when all hook commands are empty, got %q", got)
	}
}

func TestRenderHookSettings_PartialHooks(t *testing.T) {
	// Only on_start is set; on_check and on_end are empty.
	agentCfg := config.AgentConfig{
		Command:      "test-agent",
		HookDelivery: "cli_flag",
		HookEvents: map[string]string{
			"on_start": "MyStartEvent",
			"on_check": "MyCheckEvent",
			"on_end":   "MyEndEvent",
		},
		HookEntry: map[string]interface{}{
			"hooks": []interface{}{
				map[string]interface{}{"type": "command", "command": "{{command}}"},
			},
		},
		HookWrapper: map[string]interface{}{
			"hooks": "{{events}}",
		},
	}
	hooks := config.AgentHooks{OnStart: "my-start-cmd"}

	settings := renderHookSettings(agentCfg, hooks)
	if settings == "" {
		t.Fatal("expected non-empty settings for partial hooks")
	}

	var parsed map[string]interface{}
	if err := json.Unmarshal([]byte(settings), &parsed); err != nil {
		t.Fatalf("failed to parse settings JSON: %v", err)
	}

	hooksMap := parsed["hooks"].(map[string]interface{})
	// Only MyStartEvent should be present.
	if _, ok := hooksMap["MyStartEvent"]; !ok {
		t.Error("expected MyStartEvent in hooks map")
	}
	if _, ok := hooksMap["MyCheckEvent"]; ok {
		t.Error("unexpected MyCheckEvent in hooks map (command was empty)")
	}
	if _, ok := hooksMap["MyEndEvent"]; ok {
		t.Error("unexpected MyEndEvent in hooks map (command was empty)")
	}
}

func TestDeepCopyAndSubstitute(t *testing.T) {
	input := map[string]interface{}{
		"name": "{{name}}",
		"nested": map[string]interface{}{
			"cmd": "{{command}}",
		},
		"list": []interface{}{"{{event}}", "literal"},
	}
	subs := map[string]string{
		"{{name}}":    "on_start",
		"{{command}}": "my-cmd",
		"{{event}}":   "StartEvent",
	}
	result := deepCopyAndSubstitute(input, subs)
	resultMap := result.(map[string]interface{})

	if resultMap["name"] != "on_start" {
		t.Errorf("expected name=on_start, got %v", resultMap["name"])
	}
	nested := resultMap["nested"].(map[string]interface{})
	if nested["cmd"] != "my-cmd" {
		t.Errorf("expected nested.cmd=my-cmd, got %v", nested["cmd"])
	}
	list := resultMap["list"].([]interface{})
	if list[0] != "StartEvent" {
		t.Errorf("expected list[0]=StartEvent, got %v", list[0])
	}
	if list[1] != "literal" {
		t.Errorf("expected list[1]=literal, got %v", list[1])
	}

	// Verify deep copy: modifying result doesn't affect input.
	resultMap["name"] = "modified"
	if input["name"] != "{{name}}" {
		t.Error("deep copy violated: original was mutated")
	}
}

func TestDeepCopyAndSubstituteEvents(t *testing.T) {
	input := map[string]interface{}{
		"hooks": "{{events}}",
		"other": "kept",
	}
	eventsMap := map[string]interface{}{
		"Start": []interface{}{"entry1"},
		"Stop":  []interface{}{"entry2"},
	}
	result := deepCopyAndSubstituteEvents(input, eventsMap)
	resultMap := result.(map[string]interface{})

	if resultMap["other"] != "kept" {
		t.Errorf("expected other=kept, got %v", resultMap["other"])
	}
	replaced, ok := resultMap["hooks"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected hooks to be replaced with events map, got %T", resultMap["hooks"])
	}
	if _, ok := replaced["Start"]; !ok {
		t.Error("expected Start in replaced events")
	}
	if _, ok := replaced["Stop"]; !ok {
		t.Error("expected Stop in replaced events")
	}
}

func TestResolveHooks_DefaultsApplied(t *testing.T) {
	agentCfg := config.AgentConfig{
		OutputMode: "hooks",
	}
	hooks := resolveHooks(agentCfg)
	if hooks.OnStart != defaultOnStart {
		t.Errorf("expected default on_start, got %q", hooks.OnStart)
	}
	if hooks.OnCheck != defaultOnCheck {
		t.Errorf("expected default on_check, got %q", hooks.OnCheck)
	}
	if hooks.OnEnd != defaultOnEnd {
		t.Errorf("expected default on_end, got %q", hooks.OnEnd)
	}
}

func TestResolveHooks_NonHooksModeNoDefaults(t *testing.T) {
	agentCfg := config.AgentConfig{
		OutputMode: "terminal",
	}
	hooks := resolveHooks(agentCfg)
	if hooks.OnStart != "" {
		t.Errorf("expected empty on_start for terminal mode, got %q", hooks.OnStart)
	}
}

func TestResolveHooks_CustomOverride(t *testing.T) {
	agentCfg := config.AgentConfig{
		OutputMode: "hooks",
		Hooks: config.AgentHooks{
			OnStart: "custom-start",
		},
	}
	hooks := resolveHooks(agentCfg)
	if hooks.OnStart != "custom-start" {
		t.Errorf("expected custom on_start, got %q", hooks.OnStart)
	}
	// on_check and on_end should still get defaults.
	if hooks.OnCheck != defaultOnCheck {
		t.Errorf("expected default on_check, got %q", hooks.OnCheck)
	}
}
