package workspace

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"sort"
	"strings"
	"time"

	"github.com/1broseidon/termtile/internal/agent"
	"github.com/1broseidon/termtile/internal/config"
)

const workspaceLoadDebugEnv = "TERMTILE_DEBUG_WORKSPACE_LOAD"

func workspaceLoadDebugEnabled() bool {
	v := strings.TrimSpace(os.Getenv(workspaceLoadDebugEnv))
	if v == "" {
		return false
	}
	switch strings.ToLower(v) {
	case "1", "true", "yes", "y", "on":
		return true
	default:
		return false
	}
}

func newWorkspaceLoadDebugf() func(format string, args ...any) {
	if !workspaceLoadDebugEnabled() {
		return nil
	}
	return func(format string, args ...any) {
		log.Printf("workspace: debug: "+format, args...)
	}
}

// notifyDesktop sends a desktop notification using notify-send (if available).
// Failures are silently ignored since notifications are non-critical.
func notifyDesktop(summary, body string) {
	cmd := exec.Command("notify-send", "-a", "termtile", "-i", "utilities-terminal", summary, body)
	_ = cmd.Start() // Fire and forget
}

// tmuxSessionExists checks if a tmux session with the given name exists.
func tmuxSessionExists(session string) bool {
	cmd := exec.Command("tmux", "has-session", "-t", session)
	return cmd.Run() == nil
}

func Load(cfg *WorkspaceConfig, spawnTemplates map[string]string, lister TerminalLister, minimizer WindowMinimizer, applier LayoutApplier, opts LoadOptions) error {
	if cfg == nil {
		return fmt.Errorf("workspace is nil")
	}
	if strings.TrimSpace(cfg.Name) == "" {
		return fmt.Errorf("workspace name is required")
	}
	if strings.TrimSpace(cfg.Layout) == "" {
		return fmt.Errorf("workspace layout is required")
	}
	if lister == nil {
		return fmt.Errorf("terminal lister is nil")
	}
	if applier == nil {
		return fmt.Errorf("layout applier is nil")
	}
	if opts.Timeout <= 0 {
		opts.Timeout = 10 * time.Second
	}

	debugf := newWorkspaceLoadDebugf()
	if debugf != nil {
		debugf(
			"Load start name=%q layout=%q agent_mode=%v terminals=%d timeout=%s rerun=%v no_replace=%v auto_save_layout=%q auto_save_sort=%q",
			cfg.Name,
			cfg.Layout,
			cfg.AgentMode,
			len(cfg.Terminals),
			opts.Timeout,
			opts.RerunCommand,
			opts.NoReplace,
			opts.AutoSaveLayout,
			opts.AutoSaveTerminalSort,
		)
	}

	// Show loading notification
	notifyDesktop("Loading workspace", fmt.Sprintf("Restoring %s (%d terminals)...", cfg.Name, len(cfg.Terminals)))

	before, err := lister.ListTerminals()
	if err != nil {
		return err
	}
	if debugf != nil {
		debugf("Existing terminals before load: %d", len(before))
		for _, w := range before {
			debugf("  existing window_id=%d wm_class=%q pid=%d pos=(%d,%d)", w.WindowID, w.WMClass, w.PID, w.X, w.Y)
		}
	}
	existing := make(map[uint32]struct{}, len(before))
	for _, w := range before {
		existing[w.WindowID] = struct{}{}
	}

	if !opts.NoReplace {
		if minimizer == nil {
			return fmt.Errorf("window minimizer is nil")
		}

		if cfg.Name != "_previous" {
			layout := strings.TrimSpace(opts.AutoSaveLayout)
			if layout == "" {
				return fmt.Errorf("auto-save layout is required")
			}

			if debugf != nil {
				debugf("Auto-saving previous workspace to %q layout=%q terminal_sort=%q", "_previous", layout, opts.AutoSaveTerminalSort)
			}
			prev, err := Save("_previous", layout, opts.AutoSaveTerminalSort, false, lister)
			if err != nil {
				return err
			}
			if err := Write(prev); err != nil {
				return err
			}
		}

		if debugf != nil {
			debugf("Minimizing %d existing terminal(s)", len(before))
		}
		for _, w := range before {
			if err := minimizer.MinimizeWindow(w.WindowID); err != nil {
				log.Printf("workspace: warning: failed to minimize window %d: %v", w.WindowID, err)
			}
		}
	}

	terms := make([]TerminalConfig, len(cfg.Terminals))
	copy(terms, cfg.Terminals)
	sort.Slice(terms, func(i, j int) bool { return terms[i].SlotIndex < terms[j].SlotIndex })
	if debugf != nil {
		debugf("Workspace terminals after sort (by slot_index):")
		for _, term := range terms {
			session := strings.TrimSpace(term.SessionName)
			if session == "" {
				session = agent.SessionName(cfg.Name, term.SlotIndex)
			}
			debugf("  slot=%d wm_class=%q cwd=%q session=%q cmd=%q", term.SlotIndex, term.WMClass, term.Cwd, session, shellJoin(term.Cmd))
		}
	}

	// Set up multiplexer for agent mode
	var configMgr *agent.ConfigManager
	if cfg.AgentMode {
		appCfg := opts.AppConfig
		if appCfg == nil {
			appCfg = config.DefaultConfig()
		}

		var err error
		configMgr, err = agent.NewConfigManager(appCfg)
		if err != nil {
			return fmt.Errorf("agent-mode workspace requires a multiplexer: %w", err)
		}
		if debugf != nil {
			debugf("Agent mode enabled: multiplexer=%q manage_config=%v", configMgr.Name(), appCfg.AgentMode.GetManageMultiplexerConfig())
		}

		// Ensure multiplexer config is generated (scroll UX, etc.)
		if err := configMgr.Initialize(); err != nil {
			log.Printf("workspace: warning: failed to initialize multiplexer config: %v", err)
		}
		if debugf != nil {
			debugf("Multiplexer config path: %q", configMgr.GetConfigPath())
		}
	}

	for _, term := range terms {
		cmdOverride := ""
		if cfg.AgentMode && configMgr != nil {
			cwd := strings.TrimSpace(term.Cwd)
			if cwd == "" {
				home, _ := os.UserHomeDir()
				cwd = home
			}

			session := strings.TrimSpace(term.SessionName)
			if session == "" {
				session = agent.SessionName(cfg.Name, term.SlotIndex)
			}

			// Check if session already exists - if so, attach instead of create
			var sessionCmd string
			if tmuxSessionExists(session) {
				if debugf != nil {
					debugf("Session %q exists, will attach", session)
				}
				// Attach to existing session
				configPath := configMgr.GetConfigPath()
				if configPath != "" {
					sessionCmd = fmt.Sprintf("tmux -f %s attach -t %s", configPath, session)
				} else {
					sessionCmd = fmt.Sprintf("tmux attach -t %s", session)
				}
			} else {
				if debugf != nil {
					debugf("Session %q does not exist, will create", session)
				}
				// Use the multiplexer's session command (includes config path if available)
				sessionCmd = configMgr.SessionCommand(session)
			}

			if debugf != nil {
				debugf("Building session command slot=%d session=%q base=%q", term.SlotIndex, session, sessionCmd)
			}
			// Build the command with cwd
			// The session command is like: "tmux -f /path/to/conf new-session -A -s session"
			// or "tmux attach -t session" for existing sessions
			// We need to add -c cwd and optionally the user command
			muxArgs := []string{}
			baseArgs, err := splitCommand(sessionCmd)
			if err != nil {
				return fmt.Errorf("failed to parse multiplexer session command: %w", err)
			}
			muxArgs = append(muxArgs, baseArgs...)
			muxArgs = append(muxArgs, "-c", cwd)
			if opts.RerunCommand && len(term.Cmd) > 0 {
				muxArgs = append(muxArgs, term.Cmd...)
			}
			cmdOverride = shellJoin(muxArgs)
		}

		if debugf != nil {
			debugf("Spawning slot=%d wm_class=%q cmd_override=%v", term.SlotIndex, term.WMClass, cmdOverride != "")
			if cmdOverride != "" {
				debugf("  cmd=%q", cmdOverride)
			}
		}
		if err := spawnTerminal(term, spawnTemplates, opts.RerunCommand, cmdOverride); err != nil {
			return err
		}
	}

	newWindowIDs, err := waitForNewTerminals(lister, existing, len(terms), opts.Timeout, debugf)
	if err != nil {
		return err
	}
	if debugf != nil {
		debugf("Spawned terminals detected: %d window(s) order=%v", len(newWindowIDs), newWindowIDs)
	}

	// Tile immediately with spawn order for instant visual feedback
	if debugf != nil {
		debugf("Applying initial layout=%q with spawn order", cfg.Layout)
	}
	if err := applier.ApplyLayoutWithOrder(cfg.Layout, newWindowIDs); err != nil {
		return err
	}
	if debugf != nil {
		debugf("Initial tiling applied")
	}

	// For agent mode, verify window titles match expected slots and re-tile if needed
	if cfg.AgentMode {
		type windowTitleLister interface {
			WindowTitle(windowID uint32) (string, error)
		}

		titleLister, ok := lister.(windowTitleLister)
		if !ok {
			log.Printf("workspace: warning: terminal lister does not support window title lookup; slot order may be incorrect")
		} else {
			// Use shorter timeout for title verification (500ms) since we already tiled
			titleTimeout := 500 * time.Millisecond
			if debugf != nil {
				debugf("Verifying window titles for agent-mode order (timeout=%s)", titleTimeout)
			}
			if matched, reason, ok := matchWindowsByTitle(cfg.Name, terms, newWindowIDs, titleLister.WindowTitle, titleTimeout, debugf); ok {
				if debugf != nil {
					debugf("Window title matching succeeded: order=%v", matched)
				}
				// Check if order differs from spawn order
				needsRetile := false
				for i, id := range matched {
					if newWindowIDs[i] != id {
						needsRetile = true
						break
					}
				}
				if needsRetile {
					if debugf != nil {
						debugf("Re-tiling required: spawn_order=%v matched_order=%v", newWindowIDs, matched)
					}
					log.Printf("workspace: re-tiling with verified slot order")
					if err := applier.ApplyLayoutWithOrder(cfg.Layout, matched); err != nil {
						log.Printf("workspace: warning: re-tile failed: %v", err)
					} else if debugf != nil {
						debugf("Re-tiling applied successfully")
					}
				} else if debugf != nil {
					debugf("No re-tiling required (spawn order already matches title order)")
				}
			} else {
				log.Printf("workspace: warning: %s; keeping spawn order", reason)
				if debugf != nil {
					debugf("Window title matching failed: %s", reason)
				}
			}
		}
	}

	// Show completion notification
	notifyDesktop("Workspace loaded", fmt.Sprintf("%s is ready (%d terminals)", cfg.Name, len(terms)))
	if debugf != nil {
		debugf("Load complete")
	}
	return nil
}

func spawnTerminal(term TerminalConfig, templates map[string]string, rerun bool, cmdOverride string) error {
	class := strings.TrimSpace(term.WMClass)
	if class == "" {
		return fmt.Errorf("workspace terminal WMClass is empty")
	}

	template, ok := lookupSpawnTemplate(templates, class)
	if !ok {
		return fmt.Errorf("no spawn template configured for terminal class %q (set terminal_spawn_commands.%s)", class, class)
	}
	if cmdOverride != "" && !strings.Contains(template, "{{cmd}}") {
		return fmt.Errorf("spawn template for %q must include {{cmd}} for agent-mode workspaces (set terminal_spawn_commands.%s)", class, class)
	}

	cwd := strings.TrimSpace(term.Cwd)
	if cwd == "" {
		home, _ := os.UserHomeDir()
		cwd = home
	}
	cmdStr := ""
	if cmdOverride != "" {
		cmdStr = cmdOverride
	} else if rerun && len(term.Cmd) > 0 {
		cmdStr = shellJoin(term.Cmd)
	}

	argv, err := renderCommandTemplate(template, cwd, cmdStr)
	if err != nil {
		return fmt.Errorf("failed to render spawn template for %q: %w", class, err)
	}
	if len(argv) == 0 {
		return fmt.Errorf("spawn template for %q produced empty command", class)
	}

	cmd := exec.Command(argv[0], argv[1:]...)
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to spawn %q: %w", class, err)
	}
	// Do not wait; terminals are long-lived.
	return nil
}

func lookupSpawnTemplate(templates map[string]string, class string) (string, bool) {
	if templates == nil {
		return "", false
	}
	if v, ok := templates[class]; ok {
		return v, true
	}
	if v, ok := templates[strings.ToLower(class)]; ok {
		return v, true
	}
	// Best-effort case-insensitive match.
	lower := strings.ToLower(class)
	for k, v := range templates {
		if strings.ToLower(k) == lower {
			return v, true
		}
	}
	return "", false
}

func waitForNewTerminals(lister TerminalLister, existing map[uint32]struct{}, want int, timeout time.Duration, debugf func(string, ...any)) ([]uint32, error) {
	if want <= 0 {
		return nil, nil
	}
	deadline := time.Now().Add(timeout)

	ticker := time.NewTicker(150 * time.Millisecond)
	defer ticker.Stop()

	// Track window IDs in the order they first appear
	seen := make(map[uint32]struct{})
	orderedIDs := make([]uint32, 0, want)

	for {
		windows, err := lister.ListTerminals()
		if err == nil {
			for _, w := range windows {
				if _, ok := existing[w.WindowID]; ok {
					continue // skip existing windows
				}
				if _, ok := seen[w.WindowID]; ok {
					continue // already tracked
				}
				seen[w.WindowID] = struct{}{}
				orderedIDs = append(orderedIDs, w.WindowID)
				if debugf != nil {
					debugf("Detected new terminal window_id=%d wm_class=%q pid=%d pos=(%d,%d) (%d/%d)", w.WindowID, w.WMClass, w.PID, w.X, w.Y, len(orderedIDs), want)
				}
			}
			if len(orderedIDs) >= want {
				return orderedIDs, nil
			}
		}

		if time.Now().After(deadline) {
			return orderedIDs, fmt.Errorf("timeout waiting for spawned terminals (%d/%d seen after %s)", len(orderedIDs), want, timeout)
		}

		<-ticker.C
	}
}

func matchWindowsByTitle(workspaceName string, terms []TerminalConfig, windowIDs []uint32, titleForWindow func(uint32) (string, error), timeout time.Duration, debugf func(string, ...any)) ([]uint32, string, bool) {
	if len(terms) == 0 {
		return nil, "", true
	}
	if len(windowIDs) == 0 {
		return nil, fmt.Sprintf("window title matching requires %d windows, got 0", len(terms)), false
	}
	if len(windowIDs) != len(terms) {
		return nil, fmt.Sprintf("window title matching requires %d windows, got %d", len(terms), len(windowIDs)), false
	}
	if titleForWindow == nil {
		return nil, "window title lookup is unavailable", false
	}

	expected := make([]string, 0, len(terms))
	for _, term := range terms {
		session := strings.TrimSpace(term.SessionName)
		if session == "" {
			session = agent.SessionName(workspaceName, term.SlotIndex)
		}
		expected = append(expected, session)
	}

	titleTimeout := timeout
	if titleTimeout <= 0 || titleTimeout > 2*time.Second {
		titleTimeout = 2 * time.Second
	}
	deadline := time.Now().Add(titleTimeout)

	var lastTitles map[uint32]string
	var lastReason string
	var lastErrors map[uint32]error
	attempt := 0

	if debugf != nil {
		debugf("Title matching start workspace=%q window_ids=%v expected_sessions=%v timeout=%s", workspaceName, windowIDs, expected, titleTimeout)
	}

	for {
		attempt++
		titles := make(map[uint32]string, len(windowIDs))
		errors := make(map[uint32]error, len(windowIDs))
		for _, id := range windowIDs {
			title, err := titleForWindow(id)
			if err != nil {
				errors[id] = err
			}
			titles[id] = strings.TrimSpace(title)
		}

		if debugf != nil {
			changed := lastTitles == nil || len(lastTitles) != len(titles)
			if !changed {
				for _, id := range windowIDs {
					if lastTitles[id] != titles[id] {
						changed = true
						break
					}
					prevErr := lastErrors[id]
					curErr := errors[id]
					if (prevErr == nil) != (curErr == nil) {
						changed = true
						break
					}
					if prevErr != nil && curErr != nil && prevErr.Error() != curErr.Error() {
						changed = true
						break
					}
				}
			}
			if changed {
				debugf("Title matching attempt=%d", attempt)
				for _, id := range windowIDs {
					if err := errors[id]; err != nil {
						debugf("  window_id=%d title=%q err=%v", id, titles[id], err)
					} else {
						debugf("  window_id=%d title=%q", id, titles[id])
					}
				}
			}
		}

		order := make([]uint32, len(expected))
		used := make(map[uint32]struct{}, len(expected))

		ok := true
		for i, session := range expected {
			var match uint32
			for _, id := range windowIDs {
				if _, already := used[id]; already {
					continue
				}
				title := titles[id]
				if !titleContainsSession(title, session) {
					continue
				}
				if match != 0 {
					ok = false
					lastReason = fmt.Sprintf("window title matching ambiguous for session %q", session)
					break
				}
				match = id
			}
			if !ok {
				break
			}
			if match == 0 {
				ok = false
				lastReason = fmt.Sprintf("window title matching incomplete (missing session %q)", session)
				break
			}
			order[i] = match
			used[match] = struct{}{}
			if debugf != nil {
				debugf("  matched session=%q -> window_id=%d title=%q", session, match, titles[match])
			}
		}

		if ok {
			if debugf != nil {
				debugf("Title matching success: order=%v", order)
			}
			return order, "", true
		}

		lastTitles = titles
		lastErrors = errors
		if time.Now().After(deadline) {
			if len(lastTitles) == 0 {
				return nil, lastReason, false
			}
			var parts []string
			for _, id := range windowIDs {
				titlePart := fmt.Sprintf("%d=%q", id, lastTitles[id])
				if err := lastErrors[id]; err != nil {
					titlePart += fmt.Sprintf(" (err=%v)", err)
				}
				parts = append(parts, titlePart)
			}
			return nil, fmt.Sprintf("%s; titles: %s", lastReason, strings.Join(parts, ", ")), false
		}
		time.Sleep(100 * time.Millisecond)
	}
}

func titleContainsSession(title, session string) bool {
	title = strings.TrimSpace(title)
	session = strings.TrimSpace(session)
	if title == "" || session == "" {
		return false
	}

	idx := strings.Index(title, session)
	for idx >= 0 {
		end := idx + len(session)
		if end >= len(title) || title[end] < '0' || title[end] > '9' {
			return true
		}
		next := strings.Index(title[end:], session)
		if next < 0 {
			break
		}
		idx = end + next
	}
	return false
}

func renderCommandTemplate(template, dir, cmd string) ([]string, error) {
	argv, err := splitCommand(template)
	if err != nil {
		return nil, err
	}

	argvOut := make([]string, 0, len(argv))
	for _, arg := range argv {
		hadCmdPlaceholder := strings.Contains(arg, "{{cmd}}")
		arg = strings.ReplaceAll(arg, "{{dir}}", dir)
		if cmd != "" {
			arg = strings.ReplaceAll(arg, "{{cmd}}", cmd)
		} else {
			arg = strings.ReplaceAll(arg, "{{cmd}}", "")
		}
		arg = strings.TrimSpace(arg)
		if arg == "" {
			// {{cmd}} expanded to empty: also remove the preceding flag that
			// introduces the command (e.g., "-e", "--").
			if hadCmdPlaceholder && cmd == "" && len(argvOut) > 0 {
				prev := argvOut[len(argvOut)-1]
				if strings.HasPrefix(prev, "-") {
					argvOut = argvOut[:len(argvOut)-1]
				}
			}
			continue
		}
		// {{cmd}} may expand to multiple words (e.g., "vim file.go").
		// Split them into separate exec args.
		if hadCmdPlaceholder && cmd != "" {
			parts, err := splitCommand(arg)
			if err == nil && len(parts) > 0 {
				argvOut = append(argvOut, parts...)
				continue
			}
		}
		argvOut = append(argvOut, arg)
	}

	return argvOut, nil
}

func shellJoin(argv []string) string {
	if len(argv) == 0 {
		return ""
	}
	parts := make([]string, 0, len(argv))
	for _, a := range argv {
		parts = append(parts, shellQuote(a))
	}
	return strings.Join(parts, " ")
}

func shellQuote(s string) string {
	if s == "" {
		return "''"
	}
	if !strings.ContainsAny(s, " \t\r\n'\"\\$`(){}[]*?!;|&<>") {
		return s
	}
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

func splitCommand(s string) ([]string, error) {
	var out []string

	var buf strings.Builder
	inSingle := false
	inDouble := false
	escaped := false

	flush := func() {
		if buf.Len() == 0 {
			return
		}
		out = append(out, buf.String())
		buf.Reset()
	}

	for _, r := range s {
		if escaped {
			buf.WriteRune(r)
			escaped = false
			continue
		}

		if !inSingle && r == '\\' {
			escaped = true
			continue
		}

		if !inDouble && r == '\'' {
			inSingle = !inSingle
			continue
		}
		if !inSingle && r == '"' {
			inDouble = !inDouble
			continue
		}

		if !inSingle && !inDouble {
			if r == ' ' || r == '\t' || r == '\n' || r == '\r' {
				flush()
				continue
			}
		}

		buf.WriteRune(r)
	}

	if escaped {
		return nil, fmt.Errorf("unfinished escape in command template")
	}
	if inSingle || inDouble {
		return nil, fmt.Errorf("unterminated quote in command template")
	}

	flush()
	return out, nil
}
