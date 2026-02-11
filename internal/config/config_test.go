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

func TestLoadFromPath_PaletteFuzzyMatching(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(path, []byte("palette_fuzzy_matching: true\n"), 0644); err != nil {
		t.Fatalf("write: %v", err)
	}

	res, err := LoadFromPath(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if !res.Config.PaletteFuzzyMatching {
		t.Fatalf("expected palette_fuzzy_matching to be true")
	}
}

func TestLoadFromPath_DisplayAndXAuthority(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	data := strings.Join([]string{
		"display: \":1\"",
		"xauthority: \"/tmp/test-xauth\"",
		"",
	}, "\n")
	if err := os.WriteFile(path, []byte(data), 0644); err != nil {
		t.Fatalf("write: %v", err)
	}

	res, err := LoadFromPath(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if res.Config.Display != ":1" {
		t.Fatalf("expected display :1, got %q", res.Config.Display)
	}
	if res.Config.XAuthority != "/tmp/test-xauth" {
		t.Fatalf("expected xauthority /tmp/test-xauth, got %q", res.Config.XAuthority)
	}

	val, src, err := Explain(res, "display")
	if err != nil {
		t.Fatalf("explain display: %v", err)
	}
	if val != ":1" {
		t.Fatalf("expected explain display :1, got %#v", val)
	}
	if src.Kind != SourceFile {
		t.Fatalf("expected display source kind file, got %#v", src)
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
    inherits: "builtin:grid"
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
	if layout.Mode != LayoutModeAuto {
		t.Fatalf("expected inherited mode %q, got %q", LayoutModeAuto, layout.Mode)
	}
	if res.LayoutBases["dev"] != "grid" {
		t.Fatalf("expected base grid, got %q", res.LayoutBases["dev"])
	}

	val, src, err := Explain(res, "layouts.dev.mode")
	if err != nil {
		t.Fatalf("explain: %v", err)
	}
	if val != LayoutModeAuto {
		t.Fatalf("expected explain value %q, got %#v", LayoutModeAuto, val)
	}
	if src.Kind != SourceBuiltin || src.Name != "grid" {
		t.Fatalf("expected builtin source grid, got %#v", src)
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

func TestDefaultConfig_CodexHasNoAltScreen(t *testing.T) {
	cfg := DefaultConfig()
	codex, ok := cfg.Agents["codex"]
	if !ok {
		t.Fatal("expected codex agent in default config")
	}
	found := false
	for _, arg := range codex.Args {
		if arg == "--no-alt-screen" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("codex args %v missing --no-alt-screen", codex.Args)
	}
}

func TestDefaultConfig_AgentsPromptAsArg(t *testing.T) {
	cfg := DefaultConfig()

	expectPromptAsArg := map[string]bool{
		"claude":       true,
		"codex":        true,
		"gemini":       true,
		"cursor-agent": true,
		"pi":           true,
		"cecli":        false,
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

	if got := len(cfg.Agents); got != 6 {
		t.Errorf("expected 6 default agents, got %d", got)
	}
}

func TestDefaultConfig_AgentsIdlePattern(t *testing.T) {
	cfg := DefaultConfig()

	expectIdlePattern := map[string]string{
		"claude":       "\u276f", // ❯
		"codex":        "\u203a", // ›
		"gemini":       ">",
		"cursor-agent": "\u2192", // →
		"pi":           "\u2500", // ─
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

func TestGetProtectSlotZero(t *testing.T) {
	// nil AgentMode pointer defaults to true.
	var nilMode *AgentMode
	if !nilMode.GetProtectSlotZero() {
		t.Fatal("nil AgentMode should default to true")
	}

	// Zero-value AgentMode (nil ProtectSlotZero) defaults to true.
	zeroMode := &AgentMode{}
	if !zeroMode.GetProtectSlotZero() {
		t.Fatal("zero-value AgentMode should default to true")
	}

	// Explicit true.
	trueVal := true
	explicitTrue := &AgentMode{ProtectSlotZero: &trueVal}
	if !explicitTrue.GetProtectSlotZero() {
		t.Fatal("explicit true should return true")
	}

	// Explicit false.
	falseVal := false
	explicitFalse := &AgentMode{ProtectSlotZero: &falseVal}
	if explicitFalse.GetProtectSlotZero() {
		t.Fatal("explicit false should return false")
	}
}

func TestLoadFromPath_ProtectSlotZeroFromYAML(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	data := "agent_mode:\n  protect_slot_zero: false\n"
	if err := os.WriteFile(path, []byte(data), 0644); err != nil {
		t.Fatalf("write: %v", err)
	}

	res, err := LoadFromPath(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if res.Config.AgentMode.GetProtectSlotZero() {
		t.Fatal("expected protect_slot_zero to be false from YAML")
	}
}

func TestLoadFromPath_ProtectSlotZeroDefaultTrue(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(path, []byte("# empty\n"), 0644); err != nil {
		t.Fatalf("write: %v", err)
	}

	res, err := LoadFromPath(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if !res.Config.AgentMode.GetProtectSlotZero() {
		t.Fatal("expected protect_slot_zero to default to true")
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

func TestLoadFromPathWithProject_MergesWorkspaceAndLocal(t *testing.T) {
	root := t.TempDir()
	globalPath := filepath.Join(root, "config.yaml")
	if err := os.WriteFile(globalPath, []byte("default_layout: grid\n"), 0644); err != nil {
		t.Fatalf("write global config: %v", err)
	}

	projectRoot := filepath.Join(root, "project")
	projectConfigDir := filepath.Join(projectRoot, ".termtile")
	if err := os.MkdirAll(projectConfigDir, 0755); err != nil {
		t.Fatalf("mkdir project config: %v", err)
	}

	workspacePath := filepath.Join(projectConfigDir, "workspace.yaml")
	workspaceData := `
version: 1
workspace: alpha
mcp:
  read:
    default_lines: 40
workspace_overrides:
  layout: columns
  terminal: kitty
agents:
  defaults:
    spawn_mode: pane
    model: gpt-5
    env:
      TERM: xterm-256color
  overrides:
    codex:
      model: gpt-5.2-codex
`
	if err := os.WriteFile(workspacePath, []byte(strings.TrimSpace(workspaceData)+"\n"), 0644); err != nil {
		t.Fatalf("write workspace config: %v", err)
	}

	localPath := filepath.Join(projectConfigDir, "local.yaml")
	localData := `
workspace: beta
project:
  cwd_mode: explicit
  cwd: /tmp/project
workspace_overrides:
  terminal_spawn_command: "kitty --directory {{dir}} -- {{cmd}}"
agents:
  overrides:
    codex:
      spawn_mode: window
      env:
        FOO: bar
sync:
  mode: detached
`
	if err := os.WriteFile(localPath, []byte(strings.TrimSpace(localData)+"\n"), 0644); err != nil {
		t.Fatalf("write local config: %v", err)
	}

	res, err := LoadFromPathWithProject(globalPath, projectRoot)
	if err != nil {
		t.Fatalf("load with project: %v", err)
	}

	if res.Config.ProjectWorkspace == nil {
		t.Fatalf("expected project workspace config to be loaded")
	}
	if got := res.Config.ProjectWorkspace.Workspace; got != "beta" {
		t.Fatalf("expected local workspace override 'beta', got %q", got)
	}
	if got := res.Config.ProjectWorkspace.Project.CWDMode; got != ProjectCWDModeExplicit {
		t.Fatalf("expected cwd_mode explicit, got %q", got)
	}
	if got := res.Config.ProjectWorkspace.Project.CWD; got != "/tmp/project" {
		t.Fatalf("expected explicit cwd /tmp/project, got %q", got)
	}
	if got := res.Config.ProjectWorkspace.MCP.Read.DefaultLines; got != 40 {
		t.Fatalf("expected mcp.read.default_lines 40, got %d", got)
	}
	if got := res.Config.ProjectWorkspace.MCP.Read.MaxLines; got != 100 {
		t.Fatalf("expected mcp.read.max_lines default 100, got %d", got)
	}
	if got := res.Config.DefaultLayout; got != "columns" {
		t.Fatalf("expected default_layout override columns, got %q", got)
	}
	if got := res.Config.PreferredTerminal; got != "kitty" {
		t.Fatalf("expected preferred terminal override kitty, got %q", got)
	}
	if got := res.Config.TerminalSpawnCommands["kitty"]; got != "kitty --directory {{dir}} -- {{cmd}}" {
		t.Fatalf("expected terminal spawn command override, got %q", got)
	}

	claude := res.Config.Agents["claude"]
	if got := claude.SpawnMode; got != "pane" {
		t.Fatalf("expected project default spawn_mode pane for claude, got %q", got)
	}
	if got := claude.DefaultModel; got != "gpt-5" {
		t.Fatalf("expected project default model gpt-5 for claude, got %q", got)
	}
	if got := claude.Env["TERM"]; got != "xterm-256color" {
		t.Fatalf("expected project default env TERM for claude, got %q", got)
	}

	codex := res.Config.Agents["codex"]
	if got := codex.SpawnMode; got != "window" {
		t.Fatalf("expected codex spawn_mode override window, got %q", got)
	}
	if got := codex.DefaultModel; got != "gpt-5.2-codex" {
		t.Fatalf("expected codex model override gpt-5.2-codex, got %q", got)
	}
	if got := codex.Env["FOO"]; got != "bar" {
		t.Fatalf("expected codex env override FOO=bar, got %q", got)
	}
	if got := codex.Env["TERM"]; got != "xterm-256color" {
		t.Fatalf("expected codex to inherit defaults env TERM, got %q", got)
	}

	src, ok := res.Sources["project_workspace.workspace"]
	if !ok {
		t.Fatalf("expected project source for workspace")
	}
	if src.Kind != SourceFile || src.File != localPath {
		t.Fatalf("expected workspace source local file %q, got %#v", localPath, src)
	}

	if len(res.Files) != 3 {
		t.Fatalf("expected 3 loaded files, got %d (%v)", len(res.Files), res.Files)
	}
	if res.Files[1] != workspacePath || res.Files[2] != localPath {
		t.Fatalf("expected project files order [workspace, local], got %v", res.Files)
	}
}

func TestLoadFromPathWithProject_MinimalProjectConfigGetsDefaults(t *testing.T) {
	root := t.TempDir()
	projectConfigDir := filepath.Join(root, ".termtile")
	if err := os.MkdirAll(projectConfigDir, 0755); err != nil {
		t.Fatalf("mkdir project config: %v", err)
	}

	workspacePath := filepath.Join(projectConfigDir, "workspace.yaml")
	data := `
version: 1
workspace: ws-main
`
	if err := os.WriteFile(workspacePath, []byte(strings.TrimSpace(data)+"\n"), 0644); err != nil {
		t.Fatalf("write workspace config: %v", err)
	}

	globalPath := filepath.Join(root, "missing-global.yaml")
	res, err := LoadFromPathWithProject(globalPath, root)
	if err != nil {
		t.Fatalf("load with project defaults: %v", err)
	}

	if res.Config.ProjectWorkspace == nil {
		t.Fatalf("expected project workspace config")
	}
	if got := res.Config.ProjectWorkspace.Project.RootMarker; got != ".git" {
		t.Fatalf("expected root_marker default .git, got %q", got)
	}
	if got := res.Config.ProjectWorkspace.Project.CWDMode; got != ProjectCWDModeProjectRoot {
		t.Fatalf("expected cwd_mode default project_root, got %q", got)
	}
	if got := res.Config.ProjectWorkspace.Sync.Mode; got != ProjectSyncModeLinked {
		t.Fatalf("expected sync mode default linked, got %q", got)
	}
	if got := res.Config.ProjectWorkspace.MCP.Read.DefaultLines; got != 50 {
		t.Fatalf("expected default_lines default 50, got %d", got)
	}
	if got := res.Config.ProjectWorkspace.MCP.Read.MaxLines; got != 100 {
		t.Fatalf("expected max_lines default 100, got %d", got)
	}
}

func TestLoadFromPathWithProject_InvalidProjectVersionHasSourceContext(t *testing.T) {
	root := t.TempDir()
	globalPath := filepath.Join(root, "config.yaml")
	if err := os.WriteFile(globalPath, []byte("# global\n"), 0644); err != nil {
		t.Fatalf("write global config: %v", err)
	}

	projectConfigDir := filepath.Join(root, ".termtile")
	if err := os.MkdirAll(projectConfigDir, 0755); err != nil {
		t.Fatalf("mkdir project config: %v", err)
	}
	workspacePath := filepath.Join(projectConfigDir, "workspace.yaml")
	data := `
version: 2
workspace: ws-main
`
	if err := os.WriteFile(workspacePath, []byte(strings.TrimSpace(data)+"\n"), 0644); err != nil {
		t.Fatalf("write workspace config: %v", err)
	}

	_, err := LoadFromPathWithProject(globalPath, root)
	if err == nil {
		t.Fatalf("expected validation error for project workspace version")
	}
	if !strings.Contains(err.Error(), "project_workspace.version") {
		t.Fatalf("expected project_workspace.version in error, got %v", err)
	}
	if !strings.Contains(err.Error(), workspacePath+":") {
		t.Fatalf("expected source context for workspace file, got %v", err)
	}
}
