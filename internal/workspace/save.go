package workspace

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/1broseidon/termtile/internal/agent"
)

func Save(name, layout, terminalSort string, includeCmd bool, lister TerminalLister) (*WorkspaceConfig, error) {
	if lister == nil {
		return nil, fmt.Errorf("terminal lister is nil")
	}
	if err := validateWorkspaceName(name); err != nil {
		return nil, err
	}
	if strings.TrimSpace(layout) == "" {
		return nil, fmt.Errorf("layout is required")
	}

	windows, err := lister.ListTerminals()
	if err != nil {
		return nil, err
	}
	activeWin, _ := lister.ActiveWindowID()
	sortTerminalWindows(windows, terminalSort, activeWin)

	out := &WorkspaceConfig{
		Name:      name,
		Layout:    layout,
		Terminals: make([]TerminalConfig, 0, len(windows)),
	}

	for idx, win := range windows {
		term := TerminalConfig{
			WMClass:     win.WMClass,
			SlotIndex:   idx,
			SessionName: agent.SessionName(name, idx),
		}

		if win.PID > 0 {
			// Walk the process tree to find the shell running inside the terminal.
			shellPID, _ := findShellForWindow(win.PID, win.WindowID)
			if shellPID == 0 {
				shellPID = win.PID // Fall back to terminal PID.
			}

			cwd, err := readProcCwd(shellPID)
			if err != nil {
				log.Printf("workspace: warning: %s (pid %d): %v", win.WMClass, win.PID, err)
			} else {
				term.Cwd = cwd
			}

			if includeCmd {
				cmd, err := readProcCmdline(shellPID)
				if err != nil {
					log.Printf("workspace: warning: %s (pid %d): %v", win.WMClass, win.PID, err)
				} else {
					term.Cmd = cmd
				}
			}
		}

		out.Terminals = append(out.Terminals, term)
	}

	return out, nil
}

func sortTerminalWindows(windows []TerminalWindow, mode string, activeWin uint32) {
	switch mode {
	case "client_list":
		return
	case "window_id":
		sort.Slice(windows, func(i, j int) bool {
			return windows[i].WindowID < windows[j].WindowID
		})
	case "active_first":
		sort.SliceStable(windows, func(i, j int) bool {
			wi, wj := windows[i], windows[j]
			if activeWin != 0 {
				if wi.WindowID == activeWin && wj.WindowID != activeWin {
					return true
				}
				if wj.WindowID == activeWin && wi.WindowID != activeWin {
					return false
				}
			}

			if wi.Y != wj.Y {
				return wi.Y < wj.Y
			}
			if wi.X != wj.X {
				return wi.X < wj.X
			}
			return wi.WindowID < wj.WindowID
		})
	default:
		sort.Slice(windows, func(i, j int) bool {
			wi, wj := windows[i], windows[j]
			if wi.Y != wj.Y {
				return wi.Y < wj.Y
			}
			if wi.X != wj.X {
				return wi.X < wj.X
			}
			return wi.WindowID < wj.WindowID
		})
	}
}

// findShellForWindow finds the shell process associated with a specific X11
// window. For single-instance terminals (e.g., Ghostty with --gtk-single-instance),
// all windows share the same _NET_WM_PID, so we match by reading the WINDOWID
// environment variable from each child process. For multi-process terminals,
// we simply walk down the single-child chain.
func findShellForWindow(terminalPID int, windowID uint32) (int, error) {
	children, err := readChildPIDs(terminalPID)
	if err != nil || len(children) == 0 {
		return 0, err
	}

	if len(children) == 1 {
		// Multi-process terminal (one PID per window): walk down to the shell.
		return walkToLeaf(children[0])
	}

	// Single-instance terminal (multiple children): match by WINDOWID env var.
	// Ghostty spawns sh → user_shell per window; the sh inherits WINDOWID.
	for _, childPID := range children {
		if matchesWindowID(childPID, windowID) {
			leafPID, _ := walkToLeaf(childPID)
			if leafPID == 0 {
				leafPID = childPID
			}
			return leafPID, nil
		}
	}

	return 0, fmt.Errorf("no shell found matching window %d", windowID)
}

// walkToLeaf follows single-child chains down the process tree, returning the
// deepest descendant. Stops at the first fork (multiple children) or leaf.
func walkToLeaf(pid int) (int, error) {
	for depth := 0; depth < 5; depth++ {
		children, err := readChildPIDs(pid)
		if err != nil || len(children) == 0 {
			return pid, nil
		}
		if len(children) > 1 {
			// Multiple children (e.g., shell running a pipeline): return the parent.
			return pid, nil
		}
		pid = children[0]
	}
	return pid, nil
}

// matchesWindowID checks if a process's environment contains WINDOWID matching
// the given X11 window ID.
func matchesWindowID(pid int, windowID uint32) bool {
	envPath := filepath.Join("/proc", fmt.Sprintf("%d", pid), "environ")
	data, err := os.ReadFile(envPath)
	if err != nil {
		return false
	}
	target := fmt.Sprintf("WINDOWID=%d", windowID)
	for _, entry := range strings.Split(string(data), "\x00") {
		if entry == target {
			return true
		}
	}
	return false
}

// readChildPIDs reads direct child PIDs from /proc/<pid>/task/<pid>/children.
// Falls back to scanning /proc if the children file is unavailable or empty.
func readChildPIDs(pid int) ([]int, error) {
	path := filepath.Join("/proc", fmt.Sprintf("%d", pid), "task", fmt.Sprintf("%d", pid), "children")
	data, err := os.ReadFile(path)
	if err == nil {
		if pids := parsePIDList(string(data)); len(pids) > 0 {
			return pids, nil
		}
	}
	// Fallback: scan /proc for processes with matching PPID.
	return scanChildPIDs(pid)
}

func parsePIDList(s string) []int {
	var pids []int
	for _, field := range strings.Fields(s) {
		var pid int
		if _, err := fmt.Sscanf(field, "%d", &pid); err == nil && pid > 0 {
			pids = append(pids, pid)
		}
	}
	return pids
}

func scanChildPIDs(parentPID int) ([]int, error) {
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return nil, err
	}
	var children []int
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		var pid int
		if _, err := fmt.Sscanf(entry.Name(), "%d", &pid); err != nil {
			continue
		}
		statPath := filepath.Join("/proc", entry.Name(), "stat")
		data, err := os.ReadFile(statPath)
		if err != nil {
			continue
		}
		ppid := parsePPID(string(data))
		if ppid == parentPID {
			children = append(children, pid)
		}
	}
	return children, nil
}

// parsePPID extracts the PPID (field 4) from /proc/<pid>/stat.
// Format: pid (comm) state ppid ...
func parsePPID(stat string) int {
	// Find closing paren of comm field (handles spaces/parens in name).
	idx := strings.LastIndex(stat, ") ")
	if idx < 0 {
		return 0
	}
	fields := strings.Fields(stat[idx+2:])
	if len(fields) < 2 {
		return 0
	}
	// fields[0] = state, fields[1] = ppid
	var ppid int
	fmt.Sscanf(fields[1], "%d", &ppid)
	return ppid
}

func readProcCwd(pid int) (string, error) {
	target := filepath.Join("/proc", fmt.Sprintf("%d", pid), "cwd")
	cwd, err := os.Readlink(target)
	if err != nil {
		return "", fmt.Errorf("failed to read cwd from %s: %w", target, err)
	}
	return cwd, nil
}

func readProcCmdline(pid int) ([]string, error) {
	path := filepath.Join("/proc", fmt.Sprintf("%d", pid), "cmdline")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read cmdline from %s: %w", path, err)
	}
	if len(data) == 0 {
		return nil, nil
	}

	parts := strings.Split(string(data), "\x00")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p == "" {
			continue
		}
		out = append(out, p)
	}
	return out, nil
}
