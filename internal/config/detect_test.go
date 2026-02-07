package config

import (
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"testing"
)

func TestDetectAgents_NilAndEmptyExisting(t *testing.T) {
	dir := t.TempDir()
	codexPath := writeFakeAgentBinary(t, dir, "codex")
	aiderPath := writeFakeAgentBinary(t, dir, "aider")
	t.Setenv("PATH", dir)

	tests := []struct {
		name     string
		existing map[string]AgentConfig
	}{
		{name: "nil", existing: nil},
		{name: "empty", existing: map[string]AgentConfig{}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := DetectAgents(tt.existing)
			if len(got) != 2 {
				t.Fatalf("expected 2 detected agents, got %d", len(got))
			}

			if got[0].Name != "aider" || got[1].Name != "codex" {
				t.Fatalf("unexpected names/order: %#v", []string{got[0].Name, got[1].Name})
			}
			if got[0].Path != aiderPath {
				t.Fatalf("expected aider path %q, got %q", aiderPath, got[0].Path)
			}
			if got[1].Path != codexPath {
				t.Fatalf("expected codex path %q, got %q", codexPath, got[1].Path)
			}
			if got[0].Configured || got[1].Configured {
				t.Fatalf("expected both agents to be unconfigured")
			}
			if got[0].ProposedConfig.Command != "aider" || got[0].ProposedConfig.SpawnMode != "window" || !got[0].ProposedConfig.PromptAsArg {
				t.Fatalf("unexpected aider proposed config: %#v", got[0].ProposedConfig)
			}
			if got[1].ProposedConfig.Command != "codex" {
				t.Fatalf("unexpected codex proposed config: %#v", got[1].ProposedConfig)
			}
		})
	}
}

func TestDetectAgents_ConfiguredAgentSkipsProposal(t *testing.T) {
	dir := t.TempDir()
	writeFakeAgentBinary(t, dir, "claude")
	writeFakeAgentBinary(t, dir, "aider")
	t.Setenv("PATH", dir)

	got := DetectAgents(map[string]AgentConfig{
		"claude": {Command: "claude-custom"},
	})

	if len(got) != 2 {
		t.Fatalf("expected 2 detected agents, got %d", len(got))
	}

	var claude, aider *DetectedAgent
	for i := range got {
		switch got[i].Name {
		case "claude":
			claude = &got[i]
		case "aider":
			aider = &got[i]
		}
	}
	if claude == nil || aider == nil {
		t.Fatalf("expected claude and aider detections, got %#v", got)
	}

	if !claude.Configured {
		t.Fatalf("expected claude to be marked configured")
	}
	if !reflect.DeepEqual(claude.ProposedConfig, AgentConfig{}) {
		t.Fatalf("expected empty proposed config for configured agent, got %#v", claude.ProposedConfig)
	}

	if aider.Configured {
		t.Fatalf("expected aider to be unconfigured")
	}
	if aider.ProposedConfig.Command != "aider" {
		t.Fatalf("unexpected aider proposed config: %#v", aider.ProposedConfig)
	}
}

func TestDetectAgents_SortedByName(t *testing.T) {
	dir := t.TempDir()
	writeFakeAgentBinary(t, dir, "gemini")
	writeFakeAgentBinary(t, dir, "claude")
	writeFakeAgentBinary(t, dir, "codex")
	t.Setenv("PATH", dir)

	got := DetectAgents(nil)
	names := make([]string, 0, len(got))
	for _, a := range got {
		names = append(names, a.Name)
	}
	want := []string{"claude", "codex", "gemini"}
	if !reflect.DeepEqual(names, want) {
		t.Fatalf("expected sorted names %v, got %v", want, names)
	}
}

func writeFakeAgentBinary(t *testing.T, dir, name string) string {
	t.Helper()

	filename := name
	script := "#!/bin/sh\nexit 0\n"
	if runtime.GOOS == "windows" {
		filename += ".bat"
		script = "@echo off\r\nexit /B 0\r\n"
	}

	path := filepath.Join(dir, filename)
	if err := os.WriteFile(path, []byte(script), 0755); err != nil {
		t.Fatalf("write fake binary: %v", err)
	}
	return path
}
