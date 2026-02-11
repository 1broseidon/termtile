package mcp

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/1broseidon/termtile/internal/config"
	"github.com/1broseidon/termtile/internal/runtimepath"
)

var (
	runCommandOutputFn        = runCommandOutput
	readFileFn                = os.ReadFile
	readDirFn                 = os.ReadDir
	detectSessionX11EnvFn     = detectSessionX11Env
	detectDisplayFromSocketFn = detectDisplayFromSockets
)

// ensureWindowSpawnEnv injects DISPLAY/XAUTHORITY into a window-mode spawn
// command when the MCP server process itself was launched without GUI env.
func ensureWindowSpawnEnv(cmd *exec.Cmd, cfg *config.Config) error {
	env := cmd.Environ()
	if strings.TrimSpace(envLookup(env, "XDG_RUNTIME_DIR")) == "" {
		if rd, err := runtimepath.Dir(); err == nil && strings.TrimSpace(rd) != "" {
			env = upsertEnv(env, "XDG_RUNTIME_DIR", rd)
		}
	}
	display := strings.TrimSpace(envLookup(env, "DISPLAY"))
	xauthority := strings.TrimSpace(envLookup(env, "XAUTHORITY"))

	if display == "" && cfg != nil {
		display = strings.TrimSpace(cfg.Display)
	}
	if xauthority == "" && cfg != nil {
		xauthority = strings.TrimSpace(cfg.XAuthority)
	}

	if display == "" || xauthority == "" {
		detectedDisplay, detectedXAuthority := detectSessionX11EnvFn()
		if display == "" {
			display = strings.TrimSpace(detectedDisplay)
		}
		if xauthority == "" {
			xauthority = strings.TrimSpace(detectedXAuthority)
		}
	}

	if display == "" {
		display = detectDisplayFromSocketFn("/tmp/.X11-unix")
	}
	if display == "" {
		return fmt.Errorf("window mode requires DISPLAY; set display in config (e.g. display: \":1\"), export DISPLAY for the MCP server, or use window: false")
	}

	if xauthority == "" {
		home := strings.TrimSpace(envLookup(env, "HOME"))
		if home == "" {
			if detectedHome, err := os.UserHomeDir(); err == nil {
				home = detectedHome
			}
		}
		if home != "" {
			candidate := filepath.Join(home, ".Xauthority")
			if _, err := os.Stat(candidate); err == nil {
				xauthority = candidate
			}
		}
	}

	env = upsertEnv(env, "DISPLAY", display)
	if xauthority != "" {
		env = upsertEnv(env, "XAUTHORITY", xauthority)
	}
	cmd.Env = env
	return nil
}

func runCommandOutput(name string, args ...string) (string, error) {
	out, err := exec.Command(name, args...).Output()
	if err != nil {
		return "", err
	}
	return string(out), nil
}

func detectSessionX11Env() (display string, xauthority string) {
	uid := strconv.Itoa(os.Getuid())
	out, err := runCommandOutputFn("loginctl", "list-sessions", "--no-legend")
	if err != nil {
		return "", ""
	}
	sessionIDs := parseLoginctlSessions(out, uid)
	for _, sessionID := range sessionIDs {
		d := strings.TrimSpace(loginctlShowSessionProp(sessionID, "Display"))
		if d == "" || strings.EqualFold(d, "n/a") {
			continue
		}

		xauth := ""
		leader := strings.TrimSpace(loginctlShowSessionProp(sessionID, "Leader"))
		if leader != "" && leader != "0" {
			if envMap, err := readProcEnviron(leader); err == nil {
				if ed := strings.TrimSpace(envMap["DISPLAY"]); ed != "" {
					d = ed
				}
				xauth = strings.TrimSpace(envMap["XAUTHORITY"])
			}
		}
		return d, xauth
	}
	return "", ""
}

func parseLoginctlSessions(output string, uid string) []string {
	var sessions []string
	for _, line := range strings.Split(output, "\n") {
		fields := strings.Fields(strings.TrimSpace(line))
		if len(fields) < 2 {
			continue
		}
		if fields[1] == uid {
			sessions = append(sessions, fields[0])
		}
	}
	return sessions
}

func loginctlShowSessionProp(sessionID string, prop string) string {
	out, err := runCommandOutputFn("loginctl", "show-session", sessionID, "-p", prop, "--value")
	if err != nil {
		return ""
	}
	return strings.TrimSpace(out)
}

func readProcEnviron(pid string) (map[string]string, error) {
	path := filepath.Join("/proc", pid, "environ")
	data, err := readFileFn(path)
	if err != nil {
		return nil, err
	}

	env := make(map[string]string)
	for _, part := range strings.Split(string(data), "\x00") {
		if part == "" {
			continue
		}
		kv := strings.SplitN(part, "=", 2)
		if len(kv) != 2 {
			continue
		}
		env[kv[0]] = kv[1]
	}
	return env, nil
}

func detectDisplayFromSockets(dir string) string {
	entries, err := readDirFn(dir)
	if err != nil {
		return ""
	}

	var displays []int
	for _, entry := range entries {
		name := entry.Name()
		if len(name) < 2 || name[0] != 'X' {
			continue
		}
		n, err := strconv.Atoi(name[1:])
		if err != nil {
			continue
		}
		displays = append(displays, n)
	}

	if len(displays) == 0 {
		return ""
	}
	sort.Ints(displays)
	return fmt.Sprintf(":%d", displays[len(displays)-1])
}

func envLookup(env []string, key string) string {
	prefix := key + "="
	for _, e := range env {
		if strings.HasPrefix(e, prefix) {
			return strings.TrimPrefix(e, prefix)
		}
	}
	return ""
}

func upsertEnv(env []string, key string, value string) []string {
	prefix := key + "="
	for i, e := range env {
		if strings.HasPrefix(e, prefix) {
			env[i] = prefix + value
			return env
		}
	}
	return append(env, prefix+value)
}
