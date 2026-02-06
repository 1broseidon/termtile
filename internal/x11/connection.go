package x11

import (
	"github.com/BurntSushi/xgb/xproto"
	"github.com/BurntSushi/xgbutil"
	"github.com/BurntSushi/xgbutil/keybind"
	"github.com/BurntSushi/xgbutil/xevent"
)

// Connection manages the X11 connection and core X resources
type Connection struct {
	XUtil *xgbutil.XUtil
	Root  xproto.Window
}

// NewConnection establishes a connection to the X11 server and initializes required extensions
func NewConnection() (*Connection, error) {
	xu, err := xgbutil.NewConn()
	if err != nil {
		return nil, err
	}

	// Initialize keybind module (required for global hotkeys)
	keybind.Initialize(xu)
	// EWMH and RandR extensions are initialized automatically by xgbutil

	return &Connection{
		XUtil: xu,
		Root:  xu.RootWin(),
	}, nil
}

// EventLoop starts the main X11 event loop (blocking)
func (c *Connection) EventLoop() {
	xevent.Main(c.XUtil)
}

// Close cleanly disconnects from the X11 server
func (c *Connection) Close() {
	c.XUtil.Conn().Close()
}
