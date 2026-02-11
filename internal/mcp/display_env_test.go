package mcp

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/1broseidon/termtile/internal/config"
)

func TestEnsureWindowSpawnEnv_UsesExistingEnv(t *testing.T) {
	restore := stubDetectFns(
		func() (string, string) { return ":99", "/tmp/should-not-be-used" },
		func(string) string { return ":88" },
	)
	defer restore()

	cmd := exec.Command("sh", "-lc", "true")
	cmd.Env = []string{
		"HOME=" + t.TempDir(),
		"DISPLAY=:7",
		"XAUTHORITY=/tmp/xauth-existing",
	}

	if err := ensureWindowSpawnEnv(cmd, &config.Config{Display: ":1", XAuthority: "/tmp/cfg"}); err != nil {
		t.Fatalf("ensureWindowSpawnEnv returned error: %v", err)
	}

	if got := envLookup(cmd.Env, "DISPLAY"); got != ":7" {
		t.Fatalf("DISPLAY = %q, want %q", got, ":7")
	}
	if got := envLookup(cmd.Env, "XAUTHORITY"); got != "/tmp/xauth-existing" {
		t.Fatalf("XAUTHORITY = %q, want %q", got, "/tmp/xauth-existing")
	}
}

func TestEnsureWindowSpawnEnv_UsesConfigAndFallsBackToHomeXAuthority(t *testing.T) {
	restore := stubDetectFns(
		func() (string, string) { return "", "" },
		func(string) string { return "" },
	)
	defer restore()

	home := t.TempDir()
	xauth := filepath.Join(home, ".Xauthority")
	if err := os.WriteFile(xauth, []byte("cookie"), 0600); err != nil {
		t.Fatalf("write xauthority: %v", err)
	}

	cmd := exec.Command("sh", "-lc", "true")
	cmd.Env = []string{"HOME=" + home}

	if err := ensureWindowSpawnEnv(cmd, &config.Config{Display: ":1"}); err != nil {
		t.Fatalf("ensureWindowSpawnEnv returned error: %v", err)
	}

	if got := envLookup(cmd.Env, "DISPLAY"); got != ":1" {
		t.Fatalf("DISPLAY = %q, want %q", got, ":1")
	}
	if got := envLookup(cmd.Env, "XAUTHORITY"); got != xauth {
		t.Fatalf("XAUTHORITY = %q, want %q", got, xauth)
	}
}

func TestEnsureWindowSpawnEnv_UsesDetectedValues(t *testing.T) {
	restore := stubDetectFns(
		func() (string, string) { return ":5", "/tmp/xauth-detected" },
		func(string) string { return "" },
	)
	defer restore()

	cmd := exec.Command("sh", "-lc", "true")
	cmd.Env = []string{"HOME=" + t.TempDir()}

	if err := ensureWindowSpawnEnv(cmd, &config.Config{}); err != nil {
		t.Fatalf("ensureWindowSpawnEnv returned error: %v", err)
	}

	if got := envLookup(cmd.Env, "DISPLAY"); got != ":5" {
		t.Fatalf("DISPLAY = %q, want %q", got, ":5")
	}
	if got := envLookup(cmd.Env, "XAUTHORITY"); got != "/tmp/xauth-detected" {
		t.Fatalf("XAUTHORITY = %q, want %q", got, "/tmp/xauth-detected")
	}
}

func TestEnsureWindowSpawnEnv_SetsXdgRuntimeDirWhenMissingOnCmdEnv(t *testing.T) {
	restore := stubDetectFns(
		func() (string, string) { return "", "" },
		func(string) string { return "" },
	)
	defer restore()

	xdg := t.TempDir()
	t.Setenv("XDG_RUNTIME_DIR", xdg)

	cmd := exec.Command("sh", "-lc", "true")
	cmd.Env = []string{"HOME=" + t.TempDir()}

	if err := ensureWindowSpawnEnv(cmd, &config.Config{Display: ":1"}); err != nil {
		t.Fatalf("ensureWindowSpawnEnv returned error: %v", err)
	}
	if got := envLookup(cmd.Env, "XDG_RUNTIME_DIR"); got != xdg {
		t.Fatalf("XDG_RUNTIME_DIR = %q, want %q", got, xdg)
	}
}

func TestEnsureWindowSpawnEnv_ReturnsClearErrorWhenDisplayUnavailable(t *testing.T) {
	restore := stubDetectFns(
		func() (string, string) { return "", "" },
		func(string) string { return "" },
	)
	defer restore()

	cmd := exec.Command("sh", "-lc", "true")
	cmd.Env = []string{"HOME=" + t.TempDir()}

	err := ensureWindowSpawnEnv(cmd, &config.Config{})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "window mode requires DISPLAY") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestDetectDisplayFromSockets(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "X0"), []byte{}, 0600); err != nil {
		t.Fatalf("write X0: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "X2"), []byte{}, 0600); err != nil {
		t.Fatalf("write X2: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "not-a-display"), []byte{}, 0600); err != nil {
		t.Fatalf("write junk: %v", err)
	}

	if got := detectDisplayFromSockets(dir); got != ":2" {
		t.Fatalf("detectDisplayFromSockets = %q, want %q", got, ":2")
	}
}

func TestParseLoginctlSessions(t *testing.T) {
	out := strings.Join([]string{
		"1 1000 george seat0",
		"2 1001 alice seat0",
		"3 1000 george seat1",
		"",
	}, "\n")
	got := parseLoginctlSessions(out, "1000")
	if len(got) != 2 || got[0] != "1" || got[1] != "3" {
		t.Fatalf("parseLoginctlSessions = %v, want [1 3]", got)
	}
}

func stubDetectFns(
	detectSession func() (string, string),
	detectSocket func(string) string,
) func() {
	origSession := detectSessionX11EnvFn
	origSocket := detectDisplayFromSocketFn
	detectSessionX11EnvFn = detectSession
	detectDisplayFromSocketFn = detectSocket
	return func() {
		detectSessionX11EnvFn = origSession
		detectDisplayFromSocketFn = origSocket
	}
}
