package mcp

import (
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	// DefaultArtifactCapBytes is the maximum size of a stored artifact per slot.
	// Artifacts are kept in memory only.
	DefaultArtifactCapBytes = 1 << 20 // 1MB
)

type artifactKey struct {
	workspace string
	slot      int
}

// Artifact is a captured output blob for a given workspace slot.
type Artifact struct {
	Workspace      string    `json:"workspace"`
	Slot           int       `json:"slot"`
	Output         string    `json:"output"`
	Truncated      bool      `json:"truncated"`
	Warning        string    `json:"warning,omitempty"`
	OriginalBytes  int       `json:"original_bytes"`
	StoredBytes    int       `json:"stored_bytes"`
	LastUpdatedUTC time.Time `json:"last_updated_utc"`
}

// ArtifactStore holds slot artifacts in memory with a per-artifact size cap.
// It is safe for concurrent access.
type ArtifactStore struct {
	mu       sync.RWMutex
	capBytes int
	items    map[artifactKey]Artifact
}

func NewArtifactStore(capBytes int) *ArtifactStore {
	if capBytes <= 0 {
		capBytes = DefaultArtifactCapBytes
	}
	return &ArtifactStore{
		capBytes: capBytes,
		items:    make(map[artifactKey]Artifact),
	}
}

func (s *ArtifactStore) Set(workspace string, slot int, output string) Artifact {
	if s == nil {
		return Artifact{}
	}
	workspace = strings.TrimSpace(workspace)
	if workspace == "" {
		workspace = DefaultWorkspace
	}

	capped, truncated, warning := truncateWithWarning(output, s.capBytes)
	now := time.Now().UTC()

	art := Artifact{
		Workspace:      workspace,
		Slot:           slot,
		Output:         capped,
		Truncated:      truncated,
		Warning:        warning,
		OriginalBytes:  len(output),
		StoredBytes:    len(capped),
		LastUpdatedUTC: now,
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	s.items[artifactKey{workspace: workspace, slot: slot}] = art
	return art
}

func (s *ArtifactStore) Get(workspace string, slot int) (Artifact, bool) {
	if s == nil {
		return Artifact{}, false
	}
	workspace = strings.TrimSpace(workspace)
	if workspace == "" {
		workspace = DefaultWorkspace
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	art, ok := s.items[artifactKey{workspace: workspace, slot: slot}]
	return art, ok
}

func (s *ArtifactStore) Clear(workspace string, slot int) {
	if s == nil {
		return
	}
	workspace = strings.TrimSpace(workspace)
	if workspace == "" {
		workspace = DefaultWorkspace
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.items, artifactKey{workspace: workspace, slot: slot})
}

func truncateWithWarning(s string, capBytes int) (out string, truncated bool, warning string) {
	if capBytes <= 0 {
		capBytes = DefaultArtifactCapBytes
	}
	if len(s) <= capBytes {
		return s, false, ""
	}

	warning = fmt.Sprintf("artifact truncated from %d bytes to %d bytes", len(s), capBytes)
	suffix := "\n\n[termtile-warning] " + warning + "\n"
	if len(suffix) >= capBytes {
		// Cap is too small to fit the warning; store a hard-cut prefix.
		return s[:capBytes], true, warning
	}
	prefixLen := capBytes - len(suffix)
	return s[:prefixLen] + suffix, true, warning
}

var slotOutputTemplateRE = regexp.MustCompile(`\{\{\s*slot_(\d+)\.output\s*\}\}`)

// substituteSlotOutputTemplates replaces {{slot_N.output}} placeholders using
// artifacts from dependency slots only. Placeholders for non-dependency slots
// or missing artifacts are left unchanged.
func substituteSlotOutputTemplates(task, workspace string, dependsOn []int, store *ArtifactStore) (string, []int) {
	if strings.TrimSpace(task) == "" || store == nil || len(dependsOn) == 0 {
		return task, nil
	}

	depSet := make(map[int]struct{}, len(dependsOn))
	for _, s := range dependsOn {
		depSet[s] = struct{}{}
	}

	missingSet := make(map[int]struct{})
	out := slotOutputTemplateRE.ReplaceAllStringFunc(task, func(m string) string {
		sub := slotOutputTemplateRE.FindStringSubmatch(m)
		if len(sub) != 2 {
			return m
		}
		n, err := strconv.Atoi(sub[1])
		if err != nil {
			return m
		}
		if _, ok := depSet[n]; !ok {
			return m
		}
		art, ok := store.Get(workspace, n)
		if !ok {
			missingSet[n] = struct{}{}
			return m
		}
		return art.Output
	})

	missing := make([]int, 0, len(missingSet))
	for slot := range missingSet {
		missing = append(missing, slot)
	}
	sort.Ints(missing)
	return out, missing
}

