package hotkeys

import (
	"fmt"
	"log"
	"sync"

	"github.com/1broseidon/termtile/internal/movemode"
	"github.com/1broseidon/termtile/internal/platform"
	"github.com/BurntSushi/xgb/xproto"
	"github.com/BurntSushi/xgbutil"
	"github.com/BurntSushi/xgbutil/keybind"
	"github.com/BurntSushi/xgbutil/xevent"
)

// Tiler interface for tiling operations
type Tiler interface {
	TileCurrentMonitor() error
}

// x11Accessor is an optional interface for backends that expose X11 internals.
type x11Accessor interface {
	XUtil() *xgbutil.XUtil
	RootWindow() xproto.Window
}

// Handler manages global keyboard shortcuts
type Handler struct {
	xu       *xgbutil.XUtil
	root     xproto.Window
	tiler    Tiler
	moveMode *movemode.Mode
}

var ignoreModsOnce sync.Once

// NewHandler creates a new hotkey handler.
func NewHandler(backend platform.Backend, tiler Tiler) *Handler {
	var xu *xgbutil.XUtil
	var root xproto.Window
	if accessor, ok := backend.(x11Accessor); ok {
		xu = accessor.XUtil()
		root = accessor.RootWindow()
	}

	ignoreModsOnce.Do(func() {
		configureIgnoreMods(xu)
	})

	return &Handler{
		xu:    xu,
		root:  root,
		tiler: tiler,
	}
}

// Register registers the tiling hotkey.
func (h *Handler) Register(keySequence string) error {
	return h.RegisterFunc(keySequence, func() {
		log.Println("Tiling hotkey triggered!")
		if err := h.tiler.TileCurrentMonitor(); err != nil {
			log.Printf("Tiling failed: %v", err)
		}
	})
}

// SetMoveMode sets the move mode controller reference.
func (h *Handler) SetMoveMode(mm *movemode.Mode) {
	h.moveMode = mm
}

// RegisterMoveMode registers the move mode toggle hotkey.
// Navigation keys (arrows, Enter, Escape) are handled via keyboard grab
// when move mode is active, not via global hotkey registration.
func (h *Handler) RegisterMoveMode(keySequence string) error {
	if h.moveMode == nil {
		return fmt.Errorf("move mode not set")
	}

	// Register only the toggle hotkey - navigation keys are handled
	// via keyboard grab when move mode is active
	if err := h.RegisterFunc(keySequence, func() {
		if h.moveMode.IsActive() {
			h.moveMode.Exit()
		} else {
			if err := h.moveMode.Enter(); err != nil {
				log.Printf("Failed to enter move mode: %v", err)
			}
		}
	}); err != nil {
		return fmt.Errorf("failed to register move mode hotkey: %w", err)
	}

	return nil
}

// RegisterFunc registers an arbitrary hotkey callback.
func (h *Handler) RegisterFunc(keySequence string, callback func()) error {
	return keybind.KeyPressFun(func(xu *xgbutil.XUtil, ev xevent.KeyPressEvent) {
		callback()
	}).Connect(h.xu, h.root, keySequence, true)
}

func configureIgnoreMods(xu *xgbutil.XUtil) {
	// Always ignore CapsLock.
	caps := uint16(xproto.ModMaskLock)

	numLock := modMaskForKeysym(xu, "Num_Lock")
	scrollLock := modMaskForKeysym(xu, "Scroll_Lock")

	unique := make(map[uint16]struct{})
	add := func(mask uint16) {
		unique[mask] = struct{}{}
	}

	add(0)
	base := []uint16{caps}
	if numLock != 0 && numLock != caps {
		base = append(base, numLock)
	}
	if scrollLock != 0 && scrollLock != caps && scrollLock != numLock {
		base = append(base, scrollLock)
	}

	for subset := 1; subset < (1 << len(base)); subset++ {
		var mask uint16
		for bit := range base {
			if subset&(1<<bit) != 0 {
				mask |= base[bit]
			}
		}
		add(mask)
	}

	ignore := make([]uint16, 0, len(unique))
	for mask := range unique {
		ignore = append(ignore, mask)
	}

	xevent.IgnoreMods = ignore
}

func modMaskForKeysym(xu *xgbutil.XUtil, keysym string) uint16 {
	for _, keycode := range keybind.StrToKeycodes(xu, keysym) {
		if mask := keybind.ModGet(xu, keycode); mask != 0 {
			return mask
		}
	}
	return 0
}
