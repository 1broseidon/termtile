package workspace

import (
	"time"

	"github.com/1broseidon/termtile/internal/config"
)

// WorkspaceConfig is a persisted snapshot of a set of terminal sessions.
type WorkspaceConfig struct {
	Name      string           `json:"name"`
	Layout    string           `json:"layout"`
	AgentMode bool             `json:"agent_mode,omitempty"`
	Terminals []TerminalConfig `json:"terminals"`
}

type TerminalConfig struct {
	WMClass     string   `json:"wm_class"`
	Cwd         string   `json:"cwd,omitempty"`
	Cmd         []string `json:"cmd,omitempty"`
	SlotIndex   int      `json:"slot_index"`
	SessionName string   `json:"session_name,omitempty"`
}

// TerminalWindow is a lightweight snapshot of a currently-open terminal window.
type TerminalWindow struct {
	WindowID uint32
	WMClass  string
	X        int
	Y        int
	PID      int
}

type TerminalLister interface {
	ListTerminals() ([]TerminalWindow, error)
	ActiveWindowID() (uint32, error)
}

type LayoutApplier interface {
	ApplyLayout(layoutName string, tileNow bool) error
	ApplyLayoutWithOrder(layoutName string, windowOrder []uint32) error
}

type WindowMinimizer interface {
	MinimizeWindow(windowID uint32) error
}

type LoadOptions struct {
	Timeout              time.Duration
	RerunCommand         bool
	NoReplace            bool
	AutoSaveLayout       string
	AutoSaveTerminalSort string
	AppConfig            *config.Config // Application config for agent mode multiplexer settings
}
