package config

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestDefaultConfig_ValidAndHasBuiltinLayouts(t *testing.T) {
	cfg := DefaultConfig()
	if err := cfg.Validate(); err != nil {
		t.Fatalf("expected defaults to validate, got %v", err)
	}
	if _, ok := cfg.Layouts[DefaultBuiltinLayout]; !ok {
		t.Fatalf("expected builtin %q to exist in layouts", DefaultBuiltinLayout)
	}
}

func TestLoadFromPath_EmptyFileUsesDefaults(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(path, []byte("# empty\n"), 0644); err != nil {
		t.Fatalf("write: %v", err)
	}

	res, err := LoadFromPath(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if res.Config.DefaultLayout != DefaultBuiltinLayout {
		t.Fatalf("expected default_layout %q, got %q", DefaultBuiltinLayout, res.Config.DefaultLayout)
	}
}

func TestLoadFromPath_StrictUnknownKeyErrors(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(path, []byte("unknown_key: 1\n"), 0644); err != nil {
		t.Fatalf("write: %v", err)
	}

	_, err := LoadFromPath(path)
	if err == nil {
		t.Fatalf("expected error for unknown key")
	}
	if !strings.Contains(err.Error(), "unknown_key") && !strings.Contains(err.Error(), "field") {
		t.Fatalf("expected unknown field error, got %v", err)
	}
	if !strings.Contains(err.Error(), path) {
		t.Fatalf("expected error to include file path, got %v", err)
	}
}

func TestLoadFromPath_IncludeDirectoryOrderAndMainOverrides(t *testing.T) {
	dir := t.TempDir()

	// config.d loaded first, in sorted order.
	configD := filepath.Join(dir, "config.d")
	if err := os.MkdirAll(configD, 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(configD, "10-base.yaml"), []byte("gap_size: 5\n"), 0644); err != nil {
		t.Fatalf("write: %v", err)
	}
	if err := os.WriteFile(filepath.Join(configD, "20-override.yaml"), []byte("gap_size: 6\n"), 0644); err != nil {
		t.Fatalf("write: %v", err)
	}

	// Main file overrides includes.
	path := filepath.Join(dir, "config.yaml")
	main := strings.Join([]string{
		"include:",
		"  - config.d",
		"gap_size: 7",
		"",
	}, "\n")
	if err := os.WriteFile(path, []byte(main), 0644); err != nil {
		t.Fatalf("write: %v", err)
	}

	res, err := LoadFromPath(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if res.Config.GapSize != 7 {
		t.Fatalf("expected gap_size to be 7, got %d", res.Config.GapSize)
	}
}

func TestLoadFromPath_IncludeMissingPathHasContext(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	data := "include:\n  - missing.yaml\n"
	if err := os.WriteFile(path, []byte(data), 0644); err != nil {
		t.Fatalf("write: %v", err)
	}

	_, err := LoadFromPath(path)
	if err == nil {
		t.Fatalf("expected error")
	}
	if !strings.Contains(err.Error(), "include") || !strings.Contains(err.Error(), "missing.yaml") {
		t.Fatalf("expected include error, got %v", err)
	}
	if !strings.Contains(err.Error(), path+":") {
		t.Fatalf("expected error to include file:line:col prefix, got %v", err)
	}
}

func TestLoadFromPath_IncludeCycleDetection(t *testing.T) {
	dir := t.TempDir()
	a := filepath.Join(dir, "a.yaml")
	b := filepath.Join(dir, "b.yaml")
	if err := os.WriteFile(a, []byte("include: b.yaml\n"), 0644); err != nil {
		t.Fatalf("write: %v", err)
	}
	if err := os.WriteFile(b, []byte("include: a.yaml\n"), 0644); err != nil {
		t.Fatalf("write: %v", err)
	}

	_, err := LoadFromPath(a)
	if err == nil {
		t.Fatalf("expected cycle error")
	}
	if !strings.Contains(err.Error(), "include cycle") {
		t.Fatalf("expected cycle error, got %v", err)
	}
}

func TestLoadFromPath_InheritsBuiltinAndExplainSource(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	data := `
layouts:
  dev:
    inherits: "builtin:fixed_2x2_full"
    tile_region:
      type: "left-half"
`
	if err := os.WriteFile(path, []byte(strings.TrimSpace(data)+"\n"), 0644); err != nil {
		t.Fatalf("write: %v", err)
	}

	res, err := LoadFromPath(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}

	layout, ok := res.Config.Layouts["dev"]
	if !ok {
		t.Fatalf("expected dev layout")
	}
	if layout.Mode != LayoutModeFixed {
		t.Fatalf("expected inherited mode %q, got %q", LayoutModeFixed, layout.Mode)
	}
	if res.LayoutBases["dev"] != "fixed_2x2_full" {
		t.Fatalf("expected base fixed_2x2_full, got %q", res.LayoutBases["dev"])
	}

	val, src, err := Explain(res, "layouts.dev.mode")
	if err != nil {
		t.Fatalf("explain: %v", err)
	}
	if val != LayoutModeFixed {
		t.Fatalf("expected explain value %q, got %#v", LayoutModeFixed, val)
	}
	if src.Kind != SourceBuiltin || src.Name != "fixed_2x2_full" {
		t.Fatalf("expected builtin source fixed_2x2_full, got %#v", src)
	}
}

func TestLoadFromPath_TerminalClassesSupportsStringsAndMappings(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	data := `
terminal_classes:
  - kitty
  - class: Alacritty
    default: true
`
	if err := os.WriteFile(path, []byte(strings.TrimSpace(data)+"\n"), 0644); err != nil {
		t.Fatalf("write: %v", err)
	}

	res, err := LoadFromPath(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}

	if got := len(res.Config.TerminalClasses); got != 2 {
		t.Fatalf("expected 2 terminal classes, got %d", got)
	}
	if res.Config.TerminalClasses[0].Class != "kitty" || res.Config.TerminalClasses[0].Default {
		t.Fatalf("unexpected first terminal class: %#v", res.Config.TerminalClasses[0])
	}
	if res.Config.TerminalClasses[1].Class != "Alacritty" || !res.Config.TerminalClasses[1].Default {
		t.Fatalf("unexpected second terminal class: %#v", res.Config.TerminalClasses[1])
	}
}

func TestResolveTerminal_PrefDefaultEnvSystemPriorityOrder(t *testing.T) {
	origLookPath := execLookPath
	origSysDetect := detectSystemTerminal
	t.Cleanup(func() {
		execLookPath = origLookPath
		detectSystemTerminal = origSysDetect
	})

	execLookPath = func(file string) (string, error) {
		switch file {
		case "kitty", "alacritty", "ghostty", "konsole", "x-terminal-emulator":
			return "/usr/bin/" + file, nil
		default:
			return "", exec.ErrNotFound
		}
	}
	detectSystemTerminal = func() string { return "konsole" }

	cfg := &Config{
		PreferredTerminal: "kitty",
		TerminalClasses: TerminalClassList{
			{Class: "Alacritty", Default: true},
			{Class: "kitty"},
			{Class: "ghostty"},
			{Class: "konsole"},
		},
		TerminalSpawnCommands: map[string]string{
			"Alacritty": "alacritty --working-directory {{dir}} -e {{cmd}}",
			"kitty":     "kitty --directory {{dir}} {{cmd}}",
			"ghostty":   "ghostty --working-directory={{dir}} -e {{cmd}}",
			"konsole":   "konsole --workdir {{dir}} -e {{cmd}}",
		},
	}

	t.Setenv("TERMINAL", "/usr/bin/ghostty")

	if got := cfg.ResolveTerminal(); got != "kitty" {
		t.Fatalf("expected preferred_terminal to win, got %q", got)
	}

	cfg.PreferredTerminal = ""
	if got := cfg.ResolveTerminal(); got != "Alacritty" {
		t.Fatalf("expected terminal_classes default marker to win, got %q", got)
	}

	cfg.TerminalClasses[0].Default = false
	if got := cfg.ResolveTerminal(); got != "ghostty" {
		t.Fatalf("expected $TERMINAL to win, got %q", got)
	}

	t.Setenv("TERMINAL", "")
	if got := cfg.ResolveTerminal(); got != "konsole" {
		t.Fatalf("expected system detection to win, got %q", got)
	}

	detectSystemTerminal = func() string { return "" }
	cfg.TerminalClasses = TerminalClassList{{Class: "Alacritty"}, {Class: "kitty"}}
	if got := cfg.ResolveTerminal(); got != "kitty" {
		t.Fatalf("expected priority list to prefer kitty, got %q", got)
	}
}

func TestResolveTerminal_PreferredTerminalNotInTerminalClassesFallsBack(t *testing.T) {
	origLookPath := execLookPath
	origSysDetect := detectSystemTerminal
	t.Cleanup(func() {
		execLookPath = origLookPath
		detectSystemTerminal = origSysDetect
	})

	execLookPath = func(file string) (string, error) {
		if file == "kitty" {
			return "/usr/bin/kitty", nil
		}
		return "", errors.New("not found")
	}
	detectSystemTerminal = func() string { return "" }

	cfg := &Config{
		PreferredTerminal: "does-not-exist",
		TerminalClasses:   TerminalClassList{{Class: "kitty"}},
		TerminalSpawnCommands: map[string]string{
			"kitty": "kitty --directory {{dir}} {{cmd}}",
		},
	}

	if got := cfg.ResolveTerminal(); got != "kitty" {
		t.Fatalf("expected fallback to kitty, got %q", got)
	}
}

func TestDefaultConfig_AgentsPromptAsArg(t *testing.T) {
	cfg := DefaultConfig()

	expectPromptAsArg := map[string]bool{
		"claude":       true,
		"codex":        true,
		"gemini":       true,
		"cursor-agent": true,
	}

	for name, wantPrompt := range expectPromptAsArg {
		agent, ok := cfg.Agents[name]
		if !ok {
			t.Errorf("expected default agent %q to exist", name)
			continue
		}
		if agent.PromptAsArg != wantPrompt {
			t.Errorf("agent %q: PromptAsArg = %v, want %v", name, agent.PromptAsArg, wantPrompt)
		}
	}

	if _, ok := cfg.Agents["aider"]; ok {
		t.Errorf("expected aider to be removed from default agents")
	}

	if got := len(cfg.Agents); got != 4 {
		t.Errorf("expected 4 default agents, got %d", got)
	}
}

func TestDefaultConfig_AgentsIdlePattern(t *testing.T) {
	cfg := DefaultConfig()

	expectIdlePattern := map[string]string{
		"claude":       "\u276f", // ❯
		"codex":        "\u203a", // ›
		"gemini":       ">",
		"cursor-agent": "\u2192", // →
	}

	for name, wantPattern := range expectIdlePattern {
		agent, ok := cfg.Agents[name]
		if !ok {
			t.Errorf("expected default agent %q to exist", name)
			continue
		}
		if agent.IdlePattern != wantPattern {
			t.Errorf("agent %q: IdlePattern = %q, want %q", name, agent.IdlePattern, wantPattern)
		}
	}
}

func TestLoadFromPath_LimitsOverrideAndDefaults(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	data := `
limits:
  max_workspaces: 2
  max_terminals_total: 0
  workspace_overrides:
    my-agents:
      max_terminals: 5
`
	if err := os.WriteFile(path, []byte(strings.TrimSpace(data)+"\n"), 0644); err != nil {
		t.Fatalf("write: %v", err)
	}

	res, err := LoadFromPath(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}

	if got := res.Config.GetMaxWorkspaces(); got != 2 {
		t.Fatalf("expected max workspaces 2, got %d", got)
	}
	if got := res.Config.GetMaxTerminalsTotal(); got != DefaultMaxTerminalsTotal {
		t.Fatalf("expected max terminals total to default (%d), got %d", DefaultMaxTerminalsTotal, got)
	}
	if got := res.Config.GetMaxTerminalsForWorkspace("my-agents"); got != 5 {
		t.Fatalf("expected workspace override 5, got %d", got)
	}
	if got := res.Config.GetMaxTerminalsForWorkspace("other"); got != DefaultMaxTerminalsPerWorkspace {
		t.Fatalf("expected default per-workspace limit (%d), got %d", DefaultMaxTerminalsPerWorkspace, got)
	}
}
