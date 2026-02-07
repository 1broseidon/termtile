package palette

import (
	"bytes"
	"errors"
	"fmt"
	"html"
	"os/exec"
	"strconv"
	"strings"
)

// ErrCancelled is returned when the user closes the palette without selecting an item.
var ErrCancelled = errors.New("palette cancelled")

// Exit codes for rofi kb-custom keybindings
const (
	ExitNormal    = 0  // Normal selection
	ExitCancelled = 1  // User cancelled (Escape)
	ExitCustom1   = 10 // kb-custom-1 (Alt+Return by default)
	ExitCustom2   = 11 // kb-custom-2 (Alt+d by default)
	ExitCustom3   = 12 // kb-custom-3
)

type backendKind int

const (
	kindRofi backendKind = iota
	kindFuzzel
	kindWofi
	kindDmenu
)

type dmenuLikeBackend struct {
	command string
	kind    backendKind
	caps    Capabilities

	fuzzyMatching bool
}

type rowStates struct {
	active         []int
	urgent         []int
	selectedRow    int
	hasSelectedRow bool
}

func NewRofiBackend() Backend {
	return &dmenuLikeBackend{
		command: "rofi",
		kind:    kindRofi,
		caps: Capabilities{
			Icons:         true,
			Markup:        true,
			NonSelectable: true,
			CustomKeys:    true,
			IndexOutput:   true,
			MessageBar:    true,
			RowStates:     true,
		},
	}
}

func NewDmenuBackend() Backend {
	return &dmenuLikeBackend{
		command: "dmenu",
		kind:    kindDmenu,
		caps: Capabilities{
			// dmenu has minimal features
			Icons:         false,
			Markup:        false,
			NonSelectable: false,
			CustomKeys:    false,
			IndexOutput:   false,
			MessageBar:    false,
			RowStates:     false,
		},
	}
}

func NewWofiBackend() Backend {
	return &dmenuLikeBackend{
		command: "wofi",
		kind:    kindWofi,
		caps: Capabilities{
			Icons:         true,
			Markup:        true,
			NonSelectable: false,
			CustomKeys:    false,
			IndexOutput:   false,
			MessageBar:    false,
			RowStates:     false,
		},
	}
}

func NewFuzzelBackend() Backend {
	return &dmenuLikeBackend{
		command: "fuzzel",
		kind:    kindFuzzel,
		caps: Capabilities{
			Icons:         true,
			Markup:        false,
			NonSelectable: false,
			CustomKeys:    false,
			IndexOutput:   true,
			MessageBar:    false,
			RowStates:     false,
		},
	}
}

func (b *dmenuLikeBackend) Capabilities() Capabilities {
	return b.caps
}

// SetFuzzyMatching enables rofi's fuzzy matching mode when supported.
func (b *dmenuLikeBackend) SetFuzzyMatching(enabled bool) {
	b.fuzzyMatching = enabled
}

func (b *dmenuLikeBackend) Show(prompt string, items []Item, message string) (SelectResult, error) {
	if len(items) == 0 {
		return SelectResult{}, fmt.Errorf("palette: no items to show")
	}

	displayItems := make([]Item, len(items))
	copy(displayItems, items)

	input, states := b.formatInput(displayItems)
	args := b.buildArgs(prompt, message, states)

	cmd := exec.Command(b.command, args...)
	cmd.Stdin = strings.NewReader(input)

	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	out, err := cmd.Output()
	selection := strings.TrimSpace(string(out))

	exitCode := 0
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			exitCode = exitErr.ExitCode()
		}

		// Check for cancel (exit code 1 or 130 for Ctrl+C)
		if selection == "" && isCancelExit(err) {
			return SelectResult{}, ErrCancelled
		}

		// Custom keybinding exits (10, 11, 12) are not errors
		if exitCode < ExitCustom1 || exitCode > ExitCustom3 {
			if msg := strings.TrimSpace(stderr.String()); msg != "" {
				return SelectResult{}, fmt.Errorf("%s failed: %s", b.command, msg)
			}
			return SelectResult{}, fmt.Errorf("%s failed: %w", b.command, err)
		}
	}

	if selection == "" {
		return SelectResult{}, ErrCancelled
	}

	// Parse the selection based on backend capabilities
	item, err := b.parseSelection(selection, displayItems)
	if err != nil {
		return SelectResult{}, err
	}

	return SelectResult{
		Item:     item,
		ExitCode: exitCode,
	}, nil
}

func (b *dmenuLikeBackend) buildArgs(prompt string, message string, states rowStates) []string {
	var args []string

	switch b.kind {
	case kindRofi:
		args = []string{"-dmenu", "-i"}
		if prompt != "" {
			args = append(args, "-p", prompt)
		}
		// Output only the index for robust selection parsing (labels may contain ':' or markup).
		args = append(args, "-format", "i")
		// Don't allow arbitrary typed entries; this menu is always a fixed set of actions.
		args = append(args, "-no-custom")
		if b.fuzzyMatching {
			args = append(args, "-matching", "fuzzy")
		}
		// Enable features
		if b.caps.Markup {
			args = append(args, "-markup-rows")
		}
		if b.caps.Icons {
			args = append(args, "-show-icons")
		}
		// Active/urgent row highlighting (more reliable than per-row properties in dmenu mode).
		if len(states.active) > 0 {
			args = append(args, "-a", formatIndices(states.active))
		}
		if len(states.urgent) > 0 {
			args = append(args, "-u", formatIndices(states.urgent))
		}
		if states.hasSelectedRow {
			args = append(args, "-selected-row", strconv.Itoa(states.selectedRow))
		}
		// Custom keybindings
		args = append(args, "-kb-custom-1", "Alt+Return")
		args = append(args, "-kb-custom-2", "Alt+d")
		// Message bar
		if message != "" {
			args = append(args, "-mesg", message)
		}

	case kindFuzzel:
		args = []string{"--dmenu"}
		if prompt != "" {
			args = append(args, "--prompt", prompt)
		}
		// Output index
		args = append(args, "--index")

	case kindWofi:
		args = []string{"--dmenu"}
		if prompt != "" {
			args = append(args, "--prompt", prompt)
		}
		args = append(args, "--allow-markup")
		args = append(args, "--allow-images")

	case kindDmenu:
		args = []string{"-i"}
		if prompt != "" {
			args = append(args, "-p", prompt)
		}
	}

	return args
}

func (b *dmenuLikeBackend) formatInput(items []Item) (string, rowStates) {
	lines := make([]string, 0, len(items))
	var states rowStates
	firstSelectable := -1
	firstActiveSelectable := -1

	// Backends that match by visible text (dmenu/wofi) need label disambiguation.
	// Index-output backends (rofi/fuzzel) select by row index and do not.
	if !b.caps.IndexOutput {
		seen := make(map[string]int)
		for i := range items {
			if items[i].IsHeader || items[i].IsDivider {
				continue
			}
			key := sanitizeLabel(items[i].Label)
			if key == "" {
				continue
			}
			if count := seen[key]; count > 0 {
				items[i].Label = fmt.Sprintf("%s (%d)", key, count+1)
			}
			seen[key]++
		}
	}

	for i, item := range items {
		line := b.formatItem(item, i)
		lines = append(lines, line)

		if !item.IsHeader && !item.IsDivider {
			if firstSelectable == -1 {
				firstSelectable = i
			}
			if item.IsActive && firstActiveSelectable == -1 {
				firstActiveSelectable = i
			}
		}

		if b.caps.RowStates {
			if item.IsActive && !item.IsHeader && !item.IsDivider {
				states.active = append(states.active, i)
			}
			if item.IsUrgent && !item.IsHeader && !item.IsDivider {
				states.urgent = append(states.urgent, i)
			}
		}
	}

	if firstActiveSelectable != -1 {
		states.selectedRow = firstActiveSelectable
		states.hasSelectedRow = true
	} else if firstSelectable != -1 {
		states.selectedRow = firstSelectable
		states.hasSelectedRow = true
	}

	return strings.Join(lines, "\n"), states
}

func (b *dmenuLikeBackend) formatItem(item Item, index int) string {
	display := sanitizeLabel(item.Label)
	if b.caps.Markup {
		// -markup-rows is enabled: escape all user-controlled content, and add our own markup where desired.
		display = html.EscapeString(display)
	}
	if item.IsHeader && b.caps.Markup {
		display = fmt.Sprintf("<b>%s</b>", display)
	} else if item.IsDivider && b.caps.Markup {
		display = fmt.Sprintf("<span foreground='#666666'>%s</span>", display)
	}

	// Rofi dmenu supports entry properties via the \0key\x1fvalue protocol.
	// Important: there is a *single* NUL separator followed by key/value pairs delimited by \x1f.
	if b.kind != kindRofi {
		return display
	}

	var attrs []string

	if (item.IsHeader || item.IsDivider) && b.caps.NonSelectable {
		attrs = append(attrs, "nonselectable", "true")
	}
	if item.Icon != "" && b.caps.Icons {
		attrs = append(attrs, "icon", sanitizeRofiField(item.Icon))
	}
	if item.Info != "" {
		attrs = append(attrs, "info", sanitizeRofiField(item.Info))
	}
	if item.Meta != "" {
		attrs = append(attrs, "meta", sanitizeRofiField(item.Meta))
	}
	// Per-row active/urgent is supported in script mode; keep it as a best-effort hint even though
	// we also pass -a/-u (more reliable for dmenu mode).
	if item.IsActive {
		attrs = append(attrs, "active", "true")
	}
	if item.IsUrgent {
		attrs = append(attrs, "urgent", "true")
	}

	if len(attrs) == 0 {
		return display
	}
	return display + "\x00" + strings.Join(attrs, "\x1f")
}

func (b *dmenuLikeBackend) parseSelection(selection string, items []Item) (Item, error) {
	switch b.kind {
	case kindRofi:
		// Selection is just the index (via -format i).
		idx, err := strconv.Atoi(selection)
		if err != nil {
			return b.findByLabel(selection, items)
		}

		if idx < 0 || idx >= len(items) {
			return Item{}, fmt.Errorf("palette: index %d out of range", idx)
		}

		return items[idx], nil

	case kindFuzzel:
		// Fuzzel with --index outputs just the index
		idx, err := strconv.Atoi(selection)
		if err != nil {
			// Fallback to text matching if not a number
			return b.findByLabel(selection, items)
		}

		if idx < 0 || idx >= len(items) {
			return Item{}, fmt.Errorf("palette: index %d out of range", idx)
		}

		return items[idx], nil

	default:
		// dmenu/wofi: match by label text
		return b.findByLabel(selection, items)
	}
}

func (b *dmenuLikeBackend) findByLabel(selection string, items []Item) (Item, error) {
	for _, item := range items {
		if sanitizeLabel(item.Label) == selection {
			return item, nil
		}
	}
	return Item{}, fmt.Errorf("palette: unknown selection %q", selection)
}

func sanitizeLabel(label string) string {
	label = strings.ReplaceAll(label, "\r", " ")
	label = strings.ReplaceAll(label, "\n", " ")
	return strings.TrimSpace(label)
}

func sanitizeRofiField(value string) string {
	// Avoid breaking the \0key\x1fvalue protocol with control separators.
	value = strings.ReplaceAll(value, "\x00", " ")
	value = strings.ReplaceAll(value, "\x1f", " ")
	value = strings.ReplaceAll(value, "\r", " ")
	value = strings.ReplaceAll(value, "\n", " ")
	return strings.TrimSpace(value)
}

func formatIndices(indices []int) string {
	parts := make([]string, 0, len(indices))
	for _, i := range indices {
		parts = append(parts, strconv.Itoa(i))
	}
	return strings.Join(parts, ",")
}

func isCancelExit(err error) bool {
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) {
		return false
	}
	// Rofi/dmenu/wofi typically use 1 for "no selection" and 130 for Ctrl+C.
	switch exitErr.ExitCode() {
	case 1, 130:
		return true
	default:
		return false
	}
}
