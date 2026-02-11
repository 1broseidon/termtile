package terminals

import (
	"strings"
	"sync"

	"github.com/1broseidon/termtile/internal/platform"
)

// TerminalWindow represents a detected terminal window
type TerminalWindow struct {
	WindowID platform.WindowID
	Class    string
	Title    string
	X        int
	Y        int
	Width    int
	Height   int
}

// Detector identifies terminal windows on the display
type Detector struct {
	mu              sync.RWMutex
	terminalClasses map[string]bool
}

// NewDetector creates a new terminal detector with the given terminal class list
func NewDetector(terminalClasses []string) *Detector {
	classMap := make(map[string]bool)
	for _, class := range terminalClasses {
		// Store both original and lowercase for case-insensitive matching
		classMap[class] = true
		classMap[strings.ToLower(class)] = true
	}

	return &Detector{
		terminalClasses: classMap,
	}
}

// UpdateTerminalClasses updates the terminal classes for detection
func (d *Detector) UpdateTerminalClasses(terminalClasses []string) {
	classMap := make(map[string]bool)
	for _, class := range terminalClasses {
		classMap[class] = true
		classMap[strings.ToLower(class)] = true
	}

	d.mu.Lock()
	defer d.mu.Unlock()
	d.terminalClasses = classMap
}

// FindTerminals finds all terminal windows on the specified display within the given bounds.
// The bounds parameter is used to filter windows whose center falls inside that rectangle
// (typically the padded monitor area).
func (d *Detector) FindTerminals(backend platform.Backend, displayID int, bounds platform.Rect) ([]TerminalWindow, error) {
	windows, err := backend.ListWindowsOnDisplay(displayID)
	if err != nil {
		return nil, err
	}

	var terminals []TerminalWindow
	for _, w := range windows {
		// Check if this is a terminal
		if !d.isTerminalClass(w.AppID) {
			continue
		}

		// Check if window center is within bounds
		centerX := w.Bounds.X + w.Bounds.Width/2
		centerY := w.Bounds.Y + w.Bounds.Height/2
		if centerX < bounds.X || centerX >= bounds.X+bounds.Width ||
			centerY < bounds.Y || centerY >= bounds.Y+bounds.Height {
			continue
		}

		terminals = append(terminals, TerminalWindow{
			WindowID: w.ID,
			Class:    w.AppID,
			Title:    w.Title,
			X:        w.Bounds.X,
			Y:        w.Bounds.Y,
			Width:    w.Bounds.Width,
			Height:   w.Bounds.Height,
		})
	}

	return terminals, nil
}

// AllDesktopsWindowLister is an optional interface for backends that can list
// windows across all virtual desktops.
type AllDesktopsWindowLister interface {
	ListWindowsOnDisplayAllDesktops(displayID int) ([]platform.Window, error)
}

// FindTerminalsAllDesktops is like FindTerminals but lists windows across all
// virtual desktops. The backend must implement AllDesktopsWindowLister.
func (d *Detector) FindTerminalsAllDesktops(backend platform.Backend, displayID int, bounds platform.Rect) ([]TerminalWindow, error) {
	adl, ok := backend.(AllDesktopsWindowLister)
	if !ok {
		// Fall back to current-desktop listing.
		return d.FindTerminals(backend, displayID, bounds)
	}

	windows, err := adl.ListWindowsOnDisplayAllDesktops(displayID)
	if err != nil {
		return nil, err
	}

	var terminals []TerminalWindow
	for _, w := range windows {
		if !d.isTerminalClass(w.AppID) {
			continue
		}

		centerX := w.Bounds.X + w.Bounds.Width/2
		centerY := w.Bounds.Y + w.Bounds.Height/2
		if centerX < bounds.X || centerX >= bounds.X+bounds.Width ||
			centerY < bounds.Y || centerY >= bounds.Y+bounds.Height {
			continue
		}

		terminals = append(terminals, TerminalWindow{
			WindowID: w.ID,
			Class:    w.AppID,
			Title:    w.Title,
			X:        w.Bounds.X,
			Y:        w.Bounds.Y,
			Width:    w.Bounds.Width,
			Height:   w.Bounds.Height,
		})
	}

	return terminals, nil
}

// isTerminalClass checks if the given WM_CLASS matches a known terminal
func (d *Detector) isTerminalClass(class string) bool {
	d.mu.RLock()
	defer d.mu.RUnlock()

	// Check exact match first
	if d.terminalClasses[class] {
		return true
	}
	// Check lowercase match
	return d.terminalClasses[strings.ToLower(class)]
}
