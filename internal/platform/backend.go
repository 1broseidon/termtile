package platform

// WindowID is a platform-neutral window identifier.
type WindowID uint32

// Rect describes a rectangular region in screen coordinates.
type Rect struct {
	X      int
	Y      int
	Width  int
	Height int
}

// Display describes a physical display and its usable work area.
type Display struct {
	ID     int
	Name   string
	Bounds Rect
	Usable Rect
}

// Window contains metadata and geometry for a top-level window.
type Window struct {
	ID     WindowID
	PID    int
	AppID  string
	Title  string
	Bounds Rect
}

// Backend abstracts window-system operations across platforms.
type Backend interface {
	Displays() ([]Display, error)
	ActiveDisplay() (Display, error)
	ActiveWindow() (WindowID, error)
	ListWindowsOnDisplay(displayID int) ([]Window, error)
	MoveResize(windowID WindowID, bounds Rect) error
	Minimize(windowID WindowID) error
	Close(windowID WindowID) error
}
