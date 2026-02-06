package config

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

type TerminalClass struct {
	Class   string `yaml:"class"`
	Default bool   `yaml:"default,omitempty"`
}

// TerminalClassList supports either:
//
//	terminal_classes:
//	  - "kitty"
//	  - "Alacritty"
//
// or:
//
//	terminal_classes:
//	  - class: kitty
//	    default: true
//	  - class: Alacritty
type TerminalClassList []TerminalClass

func (l *TerminalClassList) UnmarshalYAML(value *yaml.Node) error {
	switch value.Kind {
	case 0:
		*l = nil
		return nil
	case yaml.SequenceNode:
		out := make([]TerminalClass, 0, len(value.Content))
		for _, item := range value.Content {
			switch item.Kind {
			case yaml.ScalarNode:
				if item.Tag != "!!str" {
					return fmt.Errorf("terminal_classes entries must be strings or mappings")
				}
				class := strings.TrimSpace(item.Value)
				if class == "" {
					return fmt.Errorf("terminal_classes entries must not be empty")
				}
				out = append(out, TerminalClass{Class: class})

			case yaml.MappingNode:
				tc, err := decodeTerminalClassMapping(item)
				if err != nil {
					return err
				}
				out = append(out, tc)

			default:
				return fmt.Errorf("terminal_classes entries must be strings or mappings")
			}
		}
		*l = out
		return nil
	default:
		return fmt.Errorf("terminal_classes must be a list")
	}
}

func decodeTerminalClassMapping(node *yaml.Node) (TerminalClass, error) {
	if node == nil || node.Kind != yaml.MappingNode {
		return TerminalClass{}, fmt.Errorf("terminal_classes entries must be strings or mappings")
	}

	var tc TerminalClass
	for i := 0; i < len(node.Content); i += 2 {
		key := node.Content[i]
		val := node.Content[i+1]
		if key.Kind != yaml.ScalarNode || key.Tag != "!!str" {
			return TerminalClass{}, fmt.Errorf("terminal_classes mapping keys must be strings")
		}
		switch key.Value {
		case "class":
			if val.Kind != yaml.ScalarNode || val.Tag != "!!str" {
				return TerminalClass{}, fmt.Errorf("terminal_classes[].class must be a string")
			}
			tc.Class = strings.TrimSpace(val.Value)
			if tc.Class == "" {
				return TerminalClass{}, fmt.Errorf("terminal_classes[].class must not be empty")
			}
		case "default":
			var b bool
			if err := val.Decode(&b); err != nil {
				return TerminalClass{}, fmt.Errorf("terminal_classes[].default must be a boolean")
			}
			tc.Default = b
		default:
			return TerminalClass{}, fmt.Errorf("unknown terminal_classes field %q", key.Value)
		}
	}

	if tc.Class == "" {
		return TerminalClass{}, fmt.Errorf("terminal_classes[].class is required")
	}

	return tc, nil
}

func (l TerminalClassList) MarshalYAML() (any, error) {
	hasDefault := false
	for _, tc := range l {
		if tc.Default {
			hasDefault = true
			break
		}
	}
	if !hasDefault {
		out := make([]string, 0, len(l))
		for _, tc := range l {
			out = append(out, tc.Class)
		}
		return out, nil
	}
	return []TerminalClass(l), nil
}

func (c *Config) TerminalClassNames() []string {
	if c == nil {
		return nil
	}
	out := make([]string, 0, len(c.TerminalClasses))
	for _, tc := range c.TerminalClasses {
		out = append(out, tc.Class)
	}
	return out
}

var (
	execLookPath         = exec.LookPath
	execCommandOutput    = func(name string, args ...string) ([]byte, error) { return exec.Command(name, args...).Output() }
	evalSymlinks         = filepath.EvalSymlinks
	detectSystemTerminal = defaultDetectSystemTerminal
)

func (c *Config) ResolveTerminal() string {
	if c == nil {
		return ""
	}

	if pref := strings.TrimSpace(c.PreferredTerminal); pref != "" {
		if class, ok := c.matchTerminalClass(pref); ok && c.canSpawnTerminal(class) {
			return class
		}
	}

	defaultClass := ""
	for _, tc := range c.TerminalClasses {
		if tc.Default {
			defaultClass = tc.Class
			break
		}
	}
	if defaultClass != "" && c.canSpawnTerminal(defaultClass) {
		return defaultClass
	}

	if env := normalizeTerminalRef(os.Getenv("TERMINAL")); env != "" {
		if class, ok := c.matchTerminalClass(env); ok && c.canSpawnTerminal(class) {
			return class
		}
	}

	if sys := normalizeTerminalRef(detectSystemTerminal()); sys != "" {
		if class, ok := c.matchTerminalClass(sys); ok && c.canSpawnTerminal(class) {
			return class
		}
	}

	for _, exe := range []string{"kitty", "ghostty", "wezterm", "alacritty", "gnome-terminal", "konsole", "xterm"} {
		if _, err := execLookPath(exe); err != nil {
			continue
		}
		if class, ok := c.matchTerminalClass(exe); ok && c.canSpawnTerminal(class) {
			return class
		}
	}

	if len(c.TerminalClasses) > 0 {
		for _, tc := range c.TerminalClasses {
			if c.canSpawnTerminal(tc.Class) {
				return tc.Class
			}
		}
		return c.TerminalClasses[0].Class
	}

	return ""
}

func normalizeTerminalRef(ref string) string {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return ""
	}
	ref = strings.Trim(ref, "\"'")
	if fields := strings.Fields(ref); len(fields) > 0 {
		ref = fields[0]
	}
	ref = strings.Trim(ref, "\"'")

	if strings.Contains(ref, "/") {
		ref = filepath.Base(ref)
	}
	ref = strings.TrimSpace(ref)
	ref = strings.TrimSuffix(ref, ".desktop")

	if ref == "x-terminal-emulator" {
		if resolved := resolveXTerminalEmulator(); resolved != "" {
			ref = resolved
		}
	}
	if strings.HasSuffix(ref, ".wrapper") {
		ref = strings.TrimSuffix(ref, ".wrapper")
	}

	return strings.TrimSpace(ref)
}

func resolveXTerminalEmulator() string {
	path, err := execLookPath("x-terminal-emulator")
	if err != nil {
		return ""
	}
	resolved, err := evalSymlinks(path)
	if err == nil && resolved != "" {
		return filepath.Base(resolved)
	}
	return filepath.Base(path)
}

func defaultDetectSystemTerminal() string {
	if resolved := resolveXTerminalEmulator(); resolved != "" && resolved != "x-terminal-emulator" {
		return resolved
	}

	out, err := execCommandOutput("gsettings", "get",
		"org.gnome.desktop.default-applications.terminal", "exec")
	if err == nil {
		term := strings.TrimSpace(string(out))
		term = strings.Trim(term, "\"'")
		term = strings.TrimSpace(term)
		if term != "" && term != "''" {
			return term
		}
	}

	kread := "kreadconfig5"
	if _, err := execLookPath(kread); err != nil {
		if _, err := execLookPath("kreadconfig6"); err == nil {
			kread = "kreadconfig6"
		}
	}
	out, err = execCommandOutput(kread, "--group", "General", "--key", "TerminalApplication")
	if err == nil {
		term := strings.TrimSpace(string(out))
		term = strings.Trim(term, "\"'")
		term = strings.TrimSpace(term)
		if term != "" {
			return term
		}
	}

	return ""
}

func (c *Config) matchTerminalClass(candidate string) (string, bool) {
	candidate = normalizeTerminalRef(candidate)
	if candidate == "" || c == nil {
		return "", false
	}

	for _, tc := range c.TerminalClasses {
		if strings.EqualFold(tc.Class, candidate) {
			return tc.Class, true
		}
	}

	if strings.Contains(candidate, ".") {
		if base := candidate[strings.LastIndex(candidate, ".")+1:]; base != "" {
			for _, tc := range c.TerminalClasses {
				if strings.EqualFold(tc.Class, base) {
					return tc.Class, true
				}
			}
		}
	}

	for _, alias := range terminalClassAliases(candidate) {
		for _, tc := range c.TerminalClasses {
			if strings.EqualFold(tc.Class, alias) {
				return tc.Class, true
			}
		}
	}

	return "", false
}

func terminalClassAliases(candidate string) []string {
	switch strings.ToLower(candidate) {
	case "ghostty":
		return []string{"com.mitchellh.ghostty"}
	case "tilix":
		return []string{"com.gexperts.Tilix"}
	case "gnome-terminal":
		return []string{"Gnome-terminal", "gnome-terminal-server"}
	case "xterm":
		return []string{"XTerm", "UXTerm"}
	default:
		return nil
	}
}

func (c *Config) canSpawnTerminal(class string) bool {
	template, ok := lookupSpawnTemplate(c.TerminalSpawnCommands, class)
	if !ok {
		return false
	}
	argv, err := splitCommand(template)
	if err != nil || len(argv) == 0 {
		return false
	}
	if _, err := execLookPath(argv[0]); err != nil {
		return false
	}
	return true
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
	lower := strings.ToLower(class)
	for k, v := range templates {
		if strings.ToLower(k) == lower {
			return v, true
		}
	}
	return "", false
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
