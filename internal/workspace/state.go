package workspace

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/1broseidon/termtile/internal/x11"
)

// WorkspaceInfo holds information about an active workspace on a specific desktop.
type WorkspaceInfo struct {
	Name          string    `json:"name"`
	Desktop       int       `json:"desktop"`
	TerminalCount int       `json:"terminal_count"`
	AgentMode     bool      `json:"agent_mode"`
	AgentSlots    []int     `json:"agent_slots,omitempty"`
	OpenedAt      time.Time `json:"opened_at"`
}

// SlotInfo tracks a single terminal slot with its X11 window ID and tmux session.
type SlotInfo struct {
	WindowID    uint32 `json:"window_id"`
	SessionName string `json:"session_name,omitempty"`
	SlotIndex   int    `json:"slot_index"`
	Desktop     int    `json:"desktop"`
}

// workspaceRegistry tracks all active workspaces keyed by desktop number.
type workspaceRegistry struct {
	Workspaces map[int]WorkspaceInfo  `json:"workspaces"`
	Slots      map[uint32]SlotInfo    `json:"slots,omitempty"` // WindowID -> SlotInfo
}

// statePath returns the path to the workspace registry state file.
func statePath() (string, error) {
	runtimeDir := os.Getenv("XDG_RUNTIME_DIR")
	if runtimeDir == "" {
		runtimeDir = fmt.Sprintf("/tmp/termtile-runtime-%d", os.Getuid())
	}
	if err := os.MkdirAll(runtimeDir, 0700); err != nil {
		return "", fmt.Errorf("failed to create runtime dir: %w", err)
	}
	return filepath.Join(runtimeDir, "termtile-workspace.json"), nil
}

// loadRegistry loads the workspace registry from disk.
func loadRegistry() (*workspaceRegistry, error) {
	path, err := statePath()
	if err != nil {
		return nil, err
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return &workspaceRegistry{Workspaces: make(map[int]WorkspaceInfo)}, nil
		}
		return nil, fmt.Errorf("failed to read workspace registry: %w", err)
	}

	var registry workspaceRegistry
	if err := json.Unmarshal(data, &registry); err != nil {
		// Try to migrate from old single-workspace format
		var oldState struct {
			Name          string    `json:"name"`
			TerminalCount int       `json:"terminal_count"`
			LoadedAt      time.Time `json:"loaded_at"`
		}
		if err2 := json.Unmarshal(data, &oldState); err2 == nil && oldState.Name != "" {
			// Successfully parsed old format, migrate to new format
			registry = workspaceRegistry{
				Workspaces: map[int]WorkspaceInfo{
					0: {
						Name:          oldState.Name,
						Desktop:       0,
						TerminalCount: oldState.TerminalCount,
						AgentMode:     false,
						OpenedAt:      oldState.LoadedAt,
					},
				},
			}
			return &registry, nil
		}
		return nil, fmt.Errorf("failed to parse workspace registry: %w", err)
	}

	if registry.Workspaces == nil {
		registry.Workspaces = make(map[int]WorkspaceInfo)
	}
	if registry.Slots == nil {
		registry.Slots = make(map[uint32]SlotInfo)
	}

	return &registry, nil
}

// saveRegistry writes the workspace registry to disk.
func saveRegistry(registry *workspaceRegistry) error {
	path, err := statePath()
	if err != nil {
		return err
	}

	data, err := json.MarshalIndent(registry, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to encode workspace registry: %w", err)
	}

	if err := os.WriteFile(path, append(data, '\n'), 0600); err != nil {
		return fmt.Errorf("failed to write workspace registry: %w", err)
	}

	return nil
}

// SetActiveWorkspace registers a workspace on a specific desktop.
// If desktop is -1, auto-detect current desktop.
// If agentSlots is provided and agentMode is true, the slots are recorded.
func SetActiveWorkspace(name string, terminalCount int, agentMode bool, desktop int, agentSlots []int) error {
	if desktop == -1 {
		d, err := x11.GetCurrentDesktopStandalone()
		if err != nil {
			// Fallback to desktop 0 with warning
			fmt.Fprintf(os.Stderr, "warning: failed to detect current desktop, using 0: %v\n", err)
			desktop = 0
		} else {
			desktop = d
		}
	}

	registry, err := loadRegistry()
	if err != nil {
		return err
	}

	// Dedupe and sort agent slots
	var slots []int
	if agentMode && len(agentSlots) > 0 {
		seen := make(map[int]struct{})
		for _, s := range agentSlots {
			if s >= 0 {
				if _, ok := seen[s]; !ok {
					seen[s] = struct{}{}
					slots = append(slots, s)
				}
			}
		}
		sort.Ints(slots)
	}

	registry.Workspaces[desktop] = WorkspaceInfo{
		Name:          name,
		Desktop:       desktop,
		TerminalCount: terminalCount,
		AgentMode:     agentMode,
		AgentSlots:    slots,
		OpenedAt:      time.Now(),
	}

	return saveRegistry(registry)
}

// GetActiveWorkspace returns the workspace on the current desktop (auto-detected).
// Returns empty WorkspaceInfo if no workspace on current desktop.
func GetActiveWorkspace() (WorkspaceInfo, error) {
	desktop, err := x11.GetCurrentDesktopStandalone()
	if err != nil {
		// Fallback to desktop 0 with warning
		fmt.Fprintf(os.Stderr, "warning: failed to detect current desktop, using 0: %v\n", err)
		desktop = 0
	}

	info, _ := GetWorkspaceByDesktop(desktop)
	return info, nil
}

// GetWorkspaceByDesktop returns workspace on a specific desktop.
// Returns empty WorkspaceInfo and false if no workspace on that desktop.
func GetWorkspaceByDesktop(desktop int) (WorkspaceInfo, bool) {
	registry, err := loadRegistry()
	if err != nil {
		return WorkspaceInfo{}, false
	}

	info, ok := registry.Workspaces[desktop]
	return info, ok
}

// GetWorkspaceByName finds a workspace by name across all desktops.
// Returns error if workspace not found.
func GetWorkspaceByName(name string) (WorkspaceInfo, error) {
	registry, err := loadRegistry()
	if err != nil {
		return WorkspaceInfo{}, err
	}
	for _, ws := range registry.Workspaces {
		if ws.Name == name {
			return ws, nil
		}
	}
	return WorkspaceInfo{}, fmt.Errorf("workspace %q not found", name)
}

// GetAllWorkspaces returns all registered workspaces keyed by desktop number.
func GetAllWorkspaces() (map[int]WorkspaceInfo, error) {
	registry, err := loadRegistry()
	if err != nil {
		return nil, err
	}

	return registry.Workspaces, nil
}

// ClearWorkspace removes the workspace on a specific desktop.
// If desktop is -1, clear workspace on current desktop.
func ClearWorkspace(desktop int) error {
	if desktop == -1 {
		d, err := x11.GetCurrentDesktopStandalone()
		if err != nil {
			// Fallback to desktop 0 with warning
			fmt.Fprintf(os.Stderr, "warning: failed to detect current desktop, using 0: %v\n", err)
			desktop = 0
		} else {
			desktop = d
		}
	}

	registry, err := loadRegistry()
	if err != nil {
		return err
	}

	delete(registry.Workspaces, desktop)

	return saveRegistry(registry)
}

// ClearActiveWorkspace removes the workspace on the current desktop.
// This is a convenience wrapper for ClearWorkspace(-1).
func ClearActiveWorkspace() error {
	return ClearWorkspace(-1)
}

// RemoveTerminalFromWorkspace removes a slot from the workspace.
// Returns error if slot doesn't exist.
// If desktop is -1, auto-detect current desktop.
func RemoveTerminalFromWorkspace(desktop int, slot int) error {
	if desktop == -1 {
		d, err := x11.GetCurrentDesktopStandalone()
		if err != nil {
			return fmt.Errorf("failed to detect current desktop: %w", err)
		}
		desktop = d
	}

	registry, err := loadRegistry()
	if err != nil {
		return err
	}

	ws, ok := registry.Workspaces[desktop]
	if !ok {
		return fmt.Errorf("no workspace on desktop %d", desktop)
	}

	if slot < 0 || slot >= ws.TerminalCount {
		return fmt.Errorf("slot %d out of range (workspace has %d terminals)", slot, ws.TerminalCount)
	}

	ws.TerminalCount--

	// Remove from agent slots and shift remaining slots down
	// If removing slot 2 from [0,1,2,3,4], result is [0,1,2,3] (3→2, 4→3)
	newSlots := make([]int, 0, len(ws.AgentSlots))
	for _, s := range ws.AgentSlots {
		if s < slot {
			newSlots = append(newSlots, s)
		} else if s > slot {
			newSlots = append(newSlots, s-1)
		}
		// s == slot is removed (not added to newSlots)
	}
	ws.AgentSlots = newSlots

	registry.Workspaces[desktop] = ws
	return saveRegistry(registry)
}

// InsertTerminalAtSlot inserts a terminal at a specific slot position, shifting
// existing slots up. If agentSlot is true, the new slot is added to AgentSlots.
// Returns error if slot is out of range.
// If desktop is -1, auto-detect current desktop.
func InsertTerminalAtSlot(desktop int, insertSlot int, agentSlot bool) error {
	if desktop == -1 {
		d, err := x11.GetCurrentDesktopStandalone()
		if err != nil {
			return fmt.Errorf("failed to detect current desktop: %w", err)
		}
		desktop = d
	}

	registry, err := loadRegistry()
	if err != nil {
		return err
	}

	ws, ok := registry.Workspaces[desktop]
	if !ok {
		return fmt.Errorf("no workspace on desktop %d", desktop)
	}

	if insertSlot < 0 || insertSlot > ws.TerminalCount {
		return fmt.Errorf("slot %d out of range (0-%d)", insertSlot, ws.TerminalCount)
	}

	// Shift existing agent slots up
	newSlots := make([]int, 0, len(ws.AgentSlots)+1)
	for _, s := range ws.AgentSlots {
		if s >= insertSlot {
			newSlots = append(newSlots, s+1)
		} else {
			newSlots = append(newSlots, s)
		}
	}

	// Add the new slot if it's an agent slot
	if agentSlot {
		newSlots = append(newSlots, insertSlot)
		sort.Ints(newSlots)
	}

	ws.AgentSlots = newSlots
	ws.TerminalCount++

	registry.Workspaces[desktop] = ws
	return saveRegistry(registry)
}

// AddTerminalToWorkspace increments terminal count and optionally adds agent slot.
// Returns the new slot index.
// If desktop is -1, auto-detect current desktop.
func AddTerminalToWorkspace(desktop int, agentSlot bool) (int, error) {
	if desktop == -1 {
		d, err := x11.GetCurrentDesktopStandalone()
		if err != nil {
			return -1, fmt.Errorf("failed to detect current desktop: %w", err)
		}
		desktop = d
	}

	registry, err := loadRegistry()
	if err != nil {
		return -1, err
	}

	ws, ok := registry.Workspaces[desktop]
	if !ok {
		return -1, fmt.Errorf("no workspace on desktop %d", desktop)
	}

	newSlot := ws.TerminalCount
	ws.TerminalCount++

	if agentSlot {
		ws.AgentSlots = append(ws.AgentSlots, newSlot)
		sort.Ints(ws.AgentSlots)
	}

	registry.Workspaces[desktop] = ws
	if err := saveRegistry(registry); err != nil {
		return -1, err
	}

	return newSlot, nil
}

// SwapSlotsInRegistry swaps two slot indices in the workspace's AgentSlots.
// This is called after a move/swap operation to keep runtime state in sync.
// If desktop is -1, auto-detect current desktop.
func SwapSlotsInRegistry(desktop, slotA, slotB int) error {
	if desktop == -1 {
		d, err := x11.GetCurrentDesktopStandalone()
		if err != nil {
			return fmt.Errorf("failed to detect current desktop: %w", err)
		}
		desktop = d
	}

	registry, err := loadRegistry()
	if err != nil {
		return err
	}

	ws, ok := registry.Workspaces[desktop]
	if !ok {
		return fmt.Errorf("no workspace on desktop %d", desktop)
	}

	if !ws.AgentMode {
		// Not agent mode, nothing to swap
		return nil
	}

	// The AgentSlots list contains which slots have tmux sessions.
	// After a swap, we need to update the indices if they're in the list.
	// Note: we don't add/remove slots, just update the slot numbers.
	newSlots := make([]int, len(ws.AgentSlots))
	for i, s := range ws.AgentSlots {
		if s == slotA {
			newSlots[i] = slotB
		} else if s == slotB {
			newSlots[i] = slotA
		} else {
			newSlots[i] = s
		}
	}
	sort.Ints(newSlots)
	ws.AgentSlots = newSlots

	registry.Workspaces[desktop] = ws
	return saveRegistry(registry)
}

// UpdateSessionNameInConfig updates the session_name field for a specific slot
// in the saved workspace config file. This is called after renaming tmux sessions.
func UpdateSessionNameInConfig(workspaceName string, slot int, newSessionName string) error {
	cfg, err := Read(workspaceName)
	if err != nil {
		return err
	}

	// Find the terminal with this slot and update its session_name
	for i := range cfg.Terminals {
		if cfg.Terminals[i].SlotIndex == slot {
			cfg.Terminals[i].SessionName = newSessionName
			break
		}
	}

	return Write(cfg)
}

// SwapSessionNamesInConfig swaps the session_name fields for two slots in the config.
func SwapSessionNamesInConfig(workspaceName string, slotA, slotB int) error {
	cfg, err := Read(workspaceName)
	if err != nil {
		return err
	}

	// Find both terminals
	var termA, termB *TerminalConfig
	for i := range cfg.Terminals {
		if cfg.Terminals[i].SlotIndex == slotA {
			termA = &cfg.Terminals[i]
		} else if cfg.Terminals[i].SlotIndex == slotB {
			termB = &cfg.Terminals[i]
		}
	}

	// Swap session names
	if termA != nil && termB != nil {
		termA.SessionName, termB.SessionName = termB.SessionName, termA.SessionName
	}

	return Write(cfg)
}

// HasUnsavedChanges checks if the current terminal state differs from the saved workspace.
// This compares the terminal count - if different, there are unsaved changes.
func HasUnsavedChanges(workspaceName string) bool {
	// Get the active workspace info
	wsInfo, err := GetActiveWorkspace()
	if err != nil || wsInfo.Name == "" {
		return false
	}

	// Only check changes for the requested workspace
	if wsInfo.Name != workspaceName {
		return false
	}

	// Load the saved workspace config
	saved, err := Read(workspaceName)
	if err != nil {
		// If we can't read the saved config, assume changes exist
		return true
	}

	// Compare terminal counts
	// If different, there are unsaved changes
	return wsInfo.TerminalCount != len(saved.Terminals)
}

// SetSlotInfo records a terminal slot with its window ID and session name.
func SetSlotInfo(windowID uint32, slotIndex int, sessionName string, desktop int) error {
	if desktop == -1 {
		d, err := x11.GetCurrentDesktopStandalone()
		if err != nil {
			return fmt.Errorf("failed to detect current desktop: %w", err)
		}
		desktop = d
	}

	registry, err := loadRegistry()
	if err != nil {
		return err
	}

	registry.Slots[windowID] = SlotInfo{
		WindowID:    windowID,
		SessionName: sessionName,
		SlotIndex:   slotIndex,
		Desktop:     desktop,
	}

	return saveRegistry(registry)
}

// GetSlotByWindowID returns slot info for a given window ID.
// Returns the slot and true if found, empty SlotInfo and false otherwise.
func GetSlotByWindowID(windowID uint32) (SlotInfo, bool) {
	registry, err := loadRegistry()
	if err != nil {
		return SlotInfo{}, false
	}

	slot, ok := registry.Slots[windowID]
	return slot, ok
}

// RemoveSlotByWindowID removes a slot from the registry by its window ID.
func RemoveSlotByWindowID(windowID uint32) error {
	registry, err := loadRegistry()
	if err != nil {
		return err
	}

	delete(registry.Slots, windowID)
	return saveRegistry(registry)
}

// GetAllSlots returns all tracked slots.
func GetAllSlots() (map[uint32]SlotInfo, error) {
	registry, err := loadRegistry()
	if err != nil {
		return nil, err
	}
	return registry.Slots, nil
}

// GetSlotsByDesktop returns all slots for a specific desktop, sorted by slot index.
func GetSlotsByDesktop(desktop int) ([]SlotInfo, error) {
	registry, err := loadRegistry()
	if err != nil {
		return nil, err
	}

	var slots []SlotInfo
	for _, slot := range registry.Slots {
		if slot.Desktop == desktop {
			slots = append(slots, slot)
		}
	}

	sort.Slice(slots, func(i, j int) bool {
		return slots[i].SlotIndex < slots[j].SlotIndex
	})

	return slots, nil
}

// UpdateSlotIndex updates the slot index and session name for a window.
func UpdateSlotIndex(windowID uint32, newIndex int, newSessionName string) error {
	registry, err := loadRegistry()
	if err != nil {
		return err
	}

	slot, ok := registry.Slots[windowID]
	if !ok {
		return fmt.Errorf("window %d not found in registry", windowID)
	}

	slot.SlotIndex = newIndex
	slot.SessionName = newSessionName
	registry.Slots[windowID] = slot

	return saveRegistry(registry)
}

// HasSessionInRegistry checks if a session name exists in the registry.
func HasSessionInRegistry(sessionName string) bool {
	registry, err := loadRegistry()
	if err != nil {
		return false
	}

	for _, slot := range registry.Slots {
		if slot.SessionName == sessionName {
			return true
		}
	}
	return false
}

// ClearSlotsByDesktop removes all slot entries for a specific desktop.
func ClearSlotsByDesktop(desktop int) error {
	registry, err := loadRegistry()
	if err != nil {
		return err
	}

	for windowID, slot := range registry.Slots {
		if slot.Desktop == desktop {
			delete(registry.Slots, windowID)
		}
	}

	return saveRegistry(registry)
}

// GetActiveState returns the full active workspace state for backwards compatibility.
// Deprecated: Use GetActiveWorkspace() instead.
func GetActiveState() (*ActiveState, error) {
	wsInfo, err := GetActiveWorkspace()
	if err != nil {
		return nil, err
	}
	if wsInfo.Name == "" {
		return nil, nil
	}

	return &ActiveState{
		Name:          wsInfo.Name,
		TerminalCount: wsInfo.TerminalCount,
		LoadedAt:      wsInfo.OpenedAt,
	}, nil
}

// ActiveState is kept for backwards compatibility during migration.
// Deprecated: Use WorkspaceInfo instead.
type ActiveState struct {
	Name          string    `json:"name"`
	TerminalCount int       `json:"terminal_count"`
	LoadedAt      time.Time `json:"loaded_at"`
}
