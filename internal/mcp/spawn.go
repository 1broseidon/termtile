package mcp

import (
	"fmt"
	"log"
	"strings"

	"github.com/1broseidon/termtile/internal/config"
)

// spawnAgentWithDependencies waits for depends_on slots (if provided) then
// spawns the agent exactly as current behavior. The optional preCommandFn is
// called after the window/session is created but before the agent command is
// sent (used for project_file hook injection).
func (s *Server) spawnAgentWithDependencies(workspaceName, agentType, cwd, agentCmd, spawnMode string, responseFence bool, agentCfg config.AgentConfig, dependsOn []int, dependsOnTimeout int, preCommandFn func(string, int) error) (string, int, error) {
	if len(dependsOn) > 0 {
		if err := s.waitForDependencies(workspaceName, dependsOn, dependsOnTimeout); err != nil {
			return "", 0, err
		}
	}

	if spawnMode == "window" {
		// Window mode: spawn a shell session, then send the agent command.
		target, slot, err := s.spawnWindow(workspaceName, agentType, cwd, responseFence, agentCfg)
		if err != nil {
			return "", 0, err
		}
		if _, err := EnsureArtifactDir(workspaceName, slot); err != nil {
			log.Printf("Warning: failed to create artifact directory for workspace %q slot %d: %v", workspaceName, slot, err)
		}
		// Clean stale output so wait_for_idle can detect fresh hook output.
		// Preserve context.md and checkpoint.json placed by the orchestrator.
		_ = CleanStaleOutput(workspaceName, slot)
		if preCommandFn != nil {
			if err := preCommandFn(workspaceName, slot); err != nil {
				log.Printf("Warning: preCommandFn failed for workspace %q slot %d: %v", workspaceName, slot, err)
			}
		}
		s.waitForShellAndSend(target, agentCmd)
		return target, slot, nil
	}

	// Pane mode: spawn directly running the agent command.
	target, slot, err := s.spawnPane(workspaceName, agentType, agentCmd, cwd, responseFence, agentCfg)
	if err != nil {
		return "", 0, err
	}
	if _, err := EnsureArtifactDir(workspaceName, slot); err != nil {
		log.Printf("Warning: failed to create artifact directory for workspace %q slot %d: %v", workspaceName, slot, err)
	}
	_ = CleanStaleOutput(workspaceName, slot)
	return target, slot, nil
}

// renderSpawnTemplate fills {{dir}} and {{cmd}} placeholders in a terminal
// spawn template and returns an exec-ready argv.
// Duplicated from internal/workspace/load.go (unexported there).
func renderSpawnTemplate(template, dir, cmd string) ([]string, error) {
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
		// {{cmd}} may expand to multiple words (e.g., "tmux new-session ...").
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

// splitCommand splits a shell-like command string into arguments,
// respecting single and double quotes and backslash escapes.
// Duplicated from internal/workspace/load.go (unexported there).
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

// lookupSpawnTemplate finds a spawn template by terminal class name,
// trying exact match first, then case-insensitive.
// Duplicated from internal/config/terminal.go (unexported there).
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
	lower := strings.ToLower(class)
	for k, v := range templates {
		if strings.ToLower(k) == lower {
			return v, true
		}
	}
	return "", false
}
