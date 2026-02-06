package x11

import (
	"fmt"

	"github.com/BurntSushi/xgb/randr"
	"github.com/BurntSushi/xgb/xproto"
	"github.com/BurntSushi/xgbutil/ewmh"
)

// Monitor represents a physical display
type Monitor struct {
	ID     int
	Name   string
	X      int
	Y      int
	Width  int
	Height int
}

// GetMonitors retrieves all active monitors using XRandR
func (c *Connection) GetMonitors() ([]Monitor, error) {
	// Initialize RandR if not already done
	if err := randr.Init(c.XUtil.Conn()); err != nil {
		return nil, fmt.Errorf("randr init failed: %w", err)
	}

	// Get screen resources
	resources, err := randr.GetScreenResources(c.XUtil.Conn(), c.Root).Reply()
	if err != nil {
		return nil, fmt.Errorf("failed to get screen resources: %w", err)
	}

	var monitors []Monitor

	// Query each CRTC for active monitors
	for i, crtc := range resources.Crtcs {
		crtcInfo, err := randr.GetCrtcInfo(c.XUtil.Conn(), crtc, resources.ConfigTimestamp).Reply()
		if err != nil {
			continue
		}

		// Skip disabled CRTCs
		if crtcInfo.Width == 0 || crtcInfo.Height == 0 || len(crtcInfo.Outputs) == 0 {
			continue
		}

		// Get output name
		outputName := fmt.Sprintf("Monitor%d", i)
		if len(crtcInfo.Outputs) > 0 {
			outputInfo, err := randr.GetOutputInfo(c.XUtil.Conn(), crtcInfo.Outputs[0], resources.ConfigTimestamp).Reply()
			if err == nil {
				outputName = string(outputInfo.Name)
			}
		}

		monitors = append(monitors, Monitor{
			ID:     i,
			Name:   outputName,
			X:      int(crtcInfo.X),
			Y:      int(crtcInfo.Y),
			Width:  int(crtcInfo.Width),
			Height: int(crtcInfo.Height),
		})
	}

	return monitors, nil
}

// GetActiveMonitor returns the monitor containing the currently focused window
// The returned monitor geometry is adjusted to respect the work area (excluding panels/docks)
func (c *Connection) GetActiveMonitor() (*Monitor, error) {
	// Get all monitors
	monitors, err := c.GetMonitors()
	if err != nil {
		return nil, err
	}
	if len(monitors) == 0 {
		return nil, fmt.Errorf("no monitors found")
	}

	var activeMonitor *Monitor

	// Prefer active window when available.
	if activeWin, err := ewmh.ActiveWindowGet(c.XUtil); err == nil && activeWin != 0 {
		if mon := findMonitorForWindow(c, monitors, activeWin); mon != nil {
			activeMonitor = mon
		}
	}

	// Fallback to the monitor under the mouse cursor.
	if activeMonitor == nil {
		if mon := findMonitorForPointer(c, monitors); mon != nil {
			activeMonitor = mon
		}
	}

	// Final fallback to first monitor.
	if activeMonitor == nil {
		activeMonitor = &monitors[0]
	}

	if applied := applyDockStruts(c, activeMonitor); !applied {
		// Fallback: Adjust monitor geometry to respect work area (excludes panels, docks, etc.)
		workArea, err := ewmh.WorkareaGet(c.XUtil)
		if err == nil && len(workArea) > 0 {
			desktopIndex := 0
			if currentDesktop, err := ewmh.CurrentDesktopGet(c.XUtil); err == nil {
				if int(currentDesktop) >= 0 && int(currentDesktop) < len(workArea) {
					desktopIndex = int(currentDesktop)
				}
			}

			wa := workArea[desktopIndex]

			// Only adjust if work area intersects with our monitor
			waX := int(wa.X)
			waY := int(wa.Y)
			waW := int(wa.Width)
			waH := int(wa.Height)

			// Calculate intersection of monitor and work area
			x1 := max(activeMonitor.X, waX)
			y1 := max(activeMonitor.Y, waY)
			x2 := min(activeMonitor.X+activeMonitor.Width, waX+waW)
			y2 := min(activeMonitor.Y+activeMonitor.Height, waY+waH)

			if x2 > x1 && y2 > y1 {
				activeMonitor.X = x1
				activeMonitor.Y = y1
				activeMonitor.Width = x2 - x1
				activeMonitor.Height = y2 - y1
			}
		}
	}

	return activeMonitor, nil
}

type dockStruts struct {
	left   int
	right  int
	top    int
	bottom int
}

func applyDockStruts(c *Connection, monitor *Monitor) bool {
	rootGeom, err := xproto.GetGeometry(c.XUtil.Conn(), xproto.Drawable(c.Root)).Reply()
	if err != nil {
		return false
	}
	rootWidth := int(rootGeom.Width)
	rootHeight := int(rootGeom.Height)

	clients, err := ewmh.ClientListGet(c.XUtil)
	if err != nil {
		return false
	}

	var struts dockStruts
	for _, windowID := range clients {
		types, err := ewmh.WmWindowTypeGet(c.XUtil, windowID)
		if err != nil {
			continue
		}

		isDock := false
		for _, t := range types {
			if t == "_NET_WM_WINDOW_TYPE_DOCK" {
				isDock = true
				break
			}
		}
		if !isDock {
			continue
		}

		if sp, err := ewmh.WmStrutPartialGet(c.XUtil, windowID); err == nil {
			updateStrutsForMonitor(monitor, rootWidth, rootHeight, sp, &struts)
			continue
		}

		// Some docks only set _NET_WM_STRUT (no partial ranges).
		if s, err := ewmh.WmStrutGet(c.XUtil, windowID); err == nil {
			sp := &ewmh.WmStrutPartial{
				Left:         s.Left,
				Right:        s.Right,
				Top:          s.Top,
				Bottom:       s.Bottom,
				LeftStartY:   0,
				LeftEndY:     uint(rootHeight - 1),
				RightStartY:  0,
				RightEndY:    uint(rootHeight - 1),
				TopStartX:    0,
				TopEndX:      uint(rootWidth - 1),
				BottomStartX: 0,
				BottomEndX:   uint(rootWidth - 1),
			}
			updateStrutsForMonitor(monitor, rootWidth, rootHeight, sp, &struts)
		}
	}

	if struts.left == 0 && struts.right == 0 && struts.top == 0 && struts.bottom == 0 {
		return false
	}

	monitor.X += struts.left
	monitor.Y += struts.top
	monitor.Width -= (struts.left + struts.right)
	monitor.Height -= (struts.top + struts.bottom)

	if monitor.Width < 1 {
		monitor.Width = 1
	}
	if monitor.Height < 1 {
		monitor.Height = 1
	}

	return true
}

func updateStrutsForMonitor(monitor *Monitor, rootWidth, rootHeight int, sp *ewmh.WmStrutPartial, acc *dockStruts) {
	monX1 := monitor.X
	monY1 := monitor.Y
	monX2 := monitor.X + monitor.Width
	monY2 := monitor.Y + monitor.Height

	// Top strut: y=[0,Top), x=[TopStartX,TopEndX]
	if sp.Top > 0 {
		x1 := int(sp.TopStartX)
		x2 := int(sp.TopEndX) + 1
		y1 := 0
		y2 := int(sp.Top)
		if intersects(monX1, monY1, monX2, monY2, x1, y1, x2, y2) {
			acc.top = max(acc.top, intersectionSize(monX1, monY1, monX2, monY2, x1, y1, x2, y2).h)
		}
	}

	// Bottom strut: y=[rootHeight-Bottom,rootHeight), x=[BottomStartX,BottomEndX]
	if sp.Bottom > 0 {
		x1 := int(sp.BottomStartX)
		x2 := int(sp.BottomEndX) + 1
		y2 := rootHeight
		y1 := rootHeight - int(sp.Bottom)
		if intersects(monX1, monY1, monX2, monY2, x1, y1, x2, y2) {
			acc.bottom = max(acc.bottom, intersectionSize(monX1, monY1, monX2, monY2, x1, y1, x2, y2).h)
		}
	}

	// Left strut: x=[0,Left), y=[LeftStartY,LeftEndY]
	if sp.Left > 0 {
		x1 := 0
		x2 := int(sp.Left)
		y1 := int(sp.LeftStartY)
		y2 := int(sp.LeftEndY) + 1
		if intersects(monX1, monY1, monX2, monY2, x1, y1, x2, y2) {
			acc.left = max(acc.left, intersectionSize(monX1, monY1, monX2, monY2, x1, y1, x2, y2).w)
		}
	}

	// Right strut: x=[rootWidth-Right,rootWidth), y=[RightStartY,RightEndY]
	if sp.Right > 0 {
		x2 := rootWidth
		x1 := rootWidth - int(sp.Right)
		y1 := int(sp.RightStartY)
		y2 := int(sp.RightEndY) + 1
		if intersects(monX1, monY1, monX2, monY2, x1, y1, x2, y2) {
			acc.right = max(acc.right, intersectionSize(monX1, monY1, monX2, monY2, x1, y1, x2, y2).w)
		}
	}
}

type intersection struct {
	w int
	h int
}

func intersectionSize(ax1, ay1, ax2, ay2, bx1, by1, bx2, by2 int) intersection {
	x1 := max(ax1, bx1)
	y1 := max(ay1, by1)
	x2 := min(ax2, bx2)
	y2 := min(ay2, by2)

	if x2 <= x1 || y2 <= y1 {
		return intersection{}
	}
	return intersection{w: x2 - x1, h: y2 - y1}
}

func intersects(ax1, ay1, ax2, ay2, bx1, by1, bx2, by2 int) bool {
	isect := intersectionSize(ax1, ay1, ax2, ay2, bx1, by1, bx2, by2)
	return isect.w > 0 && isect.h > 0
}

func findMonitorForWindow(c *Connection, monitors []Monitor, windowID xproto.Window) *Monitor {
	geom, err := xproto.GetGeometry(c.XUtil.Conn(), xproto.Drawable(windowID)).Reply()
	if err != nil {
		return nil
	}

	translate, err := xproto.TranslateCoordinates(
		c.XUtil.Conn(),
		windowID,
		c.Root,
		0, 0,
	).Reply()
	if err != nil {
		return nil
	}

	winCenterX := int(translate.DstX) + int(geom.Width)/2
	winCenterY := int(translate.DstY) + int(geom.Height)/2

	for i := range monitors {
		mon := &monitors[i]
		if winCenterX >= mon.X && winCenterX < mon.X+mon.Width &&
			winCenterY >= mon.Y && winCenterY < mon.Y+mon.Height {
			return mon
		}
	}
	return nil
}

func findMonitorForPointer(c *Connection, monitors []Monitor) *Monitor {
	pointer, err := xproto.QueryPointer(c.XUtil.Conn(), c.Root).Reply()
	if err != nil {
		return nil
	}

	x := int(pointer.RootX)
	y := int(pointer.RootY)

	for i := range monitors {
		mon := &monitors[i]
		if x >= mon.X && x < mon.X+mon.Width && y >= mon.Y && y < mon.Y+mon.Height {
			return mon
		}
	}
	return nil
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
