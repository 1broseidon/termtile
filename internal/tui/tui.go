package tui

import (
	"fmt"
	"os"
	"os/exec"
	"sort"
	"strings"

	"github.com/1broseidon/termtile/internal/config"
	"golang.org/x/term"
)

// TUI represents the terminal user interface state.
type TUI struct {
	configPath string
	result     *config.LoadResult

	// UI state
	layouts       []string // sorted layout names
	selectedIndex int
	tileCount     int // 2, 4, 6, or 9
	lastError     string
	fatalErr      error

	// Terminal state
	oldState *term.State
	width    int
	height   int
}

// New creates a new TUI instance.
func New(configPath string) *TUI {
	return &TUI{
		configPath: configPath,
		tileCount:  4,
	}
}

// Run starts the TUI main loop.
func (t *TUI) Run() error {
	if !term.IsTerminal(int(os.Stdin.Fd())) || !term.IsTerminal(int(os.Stdout.Fd())) {
		return fmt.Errorf("tui requires an interactive terminal (stdin/stdout must be TTYs)")
	}

	// Enter raw mode
	oldState, err := term.MakeRaw(int(os.Stdin.Fd()))
	if err != nil {
		return fmt.Errorf("failed to enter raw mode: %w", err)
	}
	t.oldState = oldState
	defer t.restore()

	// Get terminal size
	t.updateSize()

	// Load initial config (non-fatal; show error inline so user can press 'e' to fix)
	_ = t.loadConfig()

	// Initial render
	t.render()

	// Main event loop
	buf := make([]byte, 32)
	for {
		n, err := os.Stdin.Read(buf)
		if err != nil {
			return err
		}

		if t.handleInput(buf[:n]) {
			break
		}

		t.render()
	}

	if t.fatalErr != nil {
		return t.fatalErr
	}
	return nil
}

func (t *TUI) restore() {
	if t.oldState != nil {
		term.Restore(int(os.Stdin.Fd()), t.oldState)
	}
	// Clear screen and show cursor on exit
	fmt.Print("\x1b[0m")   // reset
	fmt.Print("\x1b[?25h") // show cursor
	fmt.Print("\x1b[2J")   // clear screen
	fmt.Print("\x1b[H")    // home cursor
}

func (t *TUI) updateSize() {
	w, h, err := term.GetSize(int(os.Stdout.Fd()))
	if err != nil {
		t.width = 80
		t.height = 24
		return
	}
	t.width = w
	t.height = h
}

func (t *TUI) loadConfig() error {
	prevSelected := t.selectedLayoutName()

	var res *config.LoadResult
	var err error

	if t.configPath == "" {
		res, err = config.LoadWithSources()
	} else {
		res, err = config.LoadFromPath(t.configPath)
	}

	if err != nil {
		t.lastError = err.Error()
		// Keep old config if we have one
		if t.result != nil {
			return nil
		}
		return err
	}

	t.result = res
	t.lastError = ""

	// Build sorted layout list
	t.layouts = make([]string, 0, len(res.Config.Layouts))
	for name := range res.Config.Layouts {
		t.layouts = append(t.layouts, name)
	}
	sort.Strings(t.layouts)

	if len(t.layouts) == 0 {
		t.selectedIndex = 0
		return nil
	}

	// Preserve current selection if it still exists; otherwise fall back to default layout.
	if prevSelected != "" {
		for i, name := range t.layouts {
			if name == prevSelected {
				t.selectedIndex = i
				return nil
			}
		}
	}

	for i, name := range t.layouts {
		if name == res.Config.DefaultLayout {
			t.selectedIndex = i
			return nil
		}
	}
	t.selectedIndex = 0

	return nil
}

func (t *TUI) handleInput(input []byte) bool {
	if len(input) == 0 {
		return false
	}

	for len(input) > 0 {
		// Check for escape sequences
		if len(input) >= 3 && input[0] == 0x1b && input[1] == '[' {
			switch input[2] {
			case 'A': // Up arrow
				t.moveSelection(-1)
			case 'B': // Down arrow
				t.moveSelection(1)
			}
			input = input[3:]
			continue
		}

		// Single character commands
		switch input[0] {
		case 'q', 0x1b: // q or Escape
			return true
		case 0x03: // Ctrl+C
			return true
		case 'j': // vim down
			t.moveSelection(1)
		case 'k': // vim up
			t.moveSelection(-1)
		case '2':
			t.tileCount = 2
		case '4':
			t.tileCount = 4
		case '6':
			t.tileCount = 6
		case '9':
			t.tileCount = 9
		case 'e': // edit
			if err := t.editConfig(); err != nil {
				t.fatalErr = err
				return true
			}
		case 'r': // reload
			_ = t.loadConfig()
		}

		input = input[1:]
	}

	return false
}

func (t *TUI) moveSelection(delta int) {
	if len(t.layouts) == 0 {
		return
	}
	t.selectedIndex += delta
	if t.selectedIndex < 0 {
		t.selectedIndex = len(t.layouts) - 1
	} else if t.selectedIndex >= len(t.layouts) {
		t.selectedIndex = 0
	}
}

func (t *TUI) editConfig() (err error) {
	// Restore terminal state before launching editor
	t.restore()

	editor := os.Getenv("EDITOR")
	if editor == "" {
		editor = os.Getenv("VISUAL")
	}
	if editor == "" {
		editor = "vi"
	}

	configPath := t.configPath
	if configPath == "" {
		path, err := config.DefaultConfigPath()
		if err != nil {
			t.lastError = err.Error()
			return t.reenterRawMode()
		}
		configPath = path
	}

	editorParts := strings.Fields(editor)
	if len(editorParts) == 0 {
		editorParts = []string{"vi"}
	}

	cmd := exec.Command(editorParts[0], append(editorParts[1:], configPath)...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		t.lastError = fmt.Sprintf("editor failed: %v", err)
	}

	if err := t.reenterRawMode(); err != nil {
		return err
	}

	// Reload config after editing
	_ = t.loadConfig()
	t.updateSize()

	return nil
}

func (t *TUI) reenterRawMode() error {
	oldState, err := term.MakeRaw(int(os.Stdin.Fd()))
	if err != nil {
		return fmt.Errorf("failed to re-enter raw mode: %w", err)
	}
	t.oldState = oldState
	return nil
}

func (t *TUI) selectedLayout() *config.Layout {
	if t.result == nil || len(t.layouts) == 0 {
		return nil
	}
	name := t.layouts[t.selectedIndex]
	layout, ok := t.result.Config.Layouts[name]
	if !ok {
		return nil
	}
	return &layout
}

func (t *TUI) selectedLayoutName() string {
	if len(t.layouts) == 0 {
		return ""
	}
	return t.layouts[t.selectedIndex]
}
