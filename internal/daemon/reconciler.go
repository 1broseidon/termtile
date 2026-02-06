package daemon

import (
	"context"
	"log/slog"
	"time"

	"github.com/1broseidon/termtile/internal/workspace"
)

// WindowLister is a function that returns current terminal window IDs.
type WindowLister func() ([]uint32, error)

// ReconcilerConfig holds configuration for the reconciler.
type ReconcilerConfig struct {
	Interval        time.Duration
	CleanupOrphaned bool
	Logger          *slog.Logger
}

// Reconciler periodically checks for state drift and corrects it.
type Reconciler struct {
	interval        time.Duration
	cleanupOrphaned bool
	sync            *StateSynchronizer
	listWindows     WindowLister
	logger          *slog.Logger
}

// NewReconciler creates a new reconciler with the given configuration.
// The listWindows function should return current terminal window IDs.
func NewReconciler(cfg ReconcilerConfig, sync *StateSynchronizer, listWindows WindowLister) *Reconciler {
	interval := cfg.Interval
	if interval <= 0 {
		interval = 10 * time.Second
	}

	return &Reconciler{
		interval:        interval,
		cleanupOrphaned: cfg.CleanupOrphaned,
		sync:            sync,
		listWindows:     listWindows,
		logger:          cfg.Logger,
	}
}

// Run starts the reconciliation loop. Blocks until context is cancelled.
func (r *Reconciler) Run(ctx context.Context) {
	ticker := time.NewTicker(r.interval)
	defer ticker.Stop()

	r.logger.Info("reconciler started", "interval", r.interval)

	for {
		select {
		case <-ctx.Done():
			r.logger.Info("reconciler stopped")
			return
		case <-ticker.C:
			r.reconcile()
		}
	}
}

// reconcile performs a single reconciliation pass.
func (r *Reconciler) reconcile() {
	// Recover from panics to prevent crashing the daemon
	defer func() {
		if err := recover(); err != nil {
			r.logger.Error("reconciler panic recovered", "error", err)
		}
	}()

	// Get expected slots from registry
	expected, err := workspace.GetAllSlots()
	if err != nil {
		r.logger.Error("reconciler: failed to get slots", "error", err)
		return
	}

	if len(expected) == 0 {
		// No slots tracked, check for orphaned sessions
		if r.cleanupOrphaned {
			r.sync.CleanupOrphanedSessions()
		}
		return
	}

	// Get actual terminal window IDs
	actualWindowIDs, err := r.listWindows()
	if err != nil {
		r.logger.Error("reconciler: failed to list windows", "error", err)
		return
	}

	// Build set of actual window IDs
	actualIDs := make(map[uint32]bool)
	for _, wid := range actualWindowIDs {
		actualIDs[wid] = true
	}

	// Find orphaned slots (in registry but window doesn't exist)
	var orphanedWindows []uint32
	for windowID := range expected {
		if !actualIDs[windowID] {
			orphanedWindows = append(orphanedWindows, windowID)
		}
	}

	// Clean up orphaned slots
	for _, windowID := range orphanedWindows {
		slot := expected[windowID]
		r.logger.Info("reconciler: orphaned slot detected",
			"window_id", windowID,
			"slot", slot.SlotIndex,
			"session", slot.SessionName)
		r.sync.HandleWindowClosed(windowID)
	}

	// Clean up orphaned tmux sessions
	if r.cleanupOrphaned {
		if err := r.sync.CleanupOrphanedSessions(); err != nil {
			r.logger.Warn("reconciler: failed to cleanup orphaned sessions", "error", err)
		}
	}
}

// ReconcileNow triggers an immediate reconciliation pass.
func (r *Reconciler) ReconcileNow() {
	r.reconcile()
}
