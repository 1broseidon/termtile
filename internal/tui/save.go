package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"gopkg.in/yaml.v3"

	"github.com/1broseidon/termtile/internal/config"
	"github.com/1broseidon/termtile/internal/ipc"
)

type savePhase int

const (
	saveHidden  savePhase = iota
	savePreview           // showing diff, awaiting confirm
	saveResult            // showing outcome message
)

type diffKind int

const (
	diffContext diffKind = iota
	diffRemoved
	diffAdded
)

type diffLine struct {
	kind diffKind
	text string
}

// SaveOverlay manages the config save diff preview and confirmation workflow.
type SaveOverlay struct {
	phase        savePhase
	diffLines    []diffLine
	err          error
	reloaded     bool
	scrollOffset int
}

// Active reports whether the overlay is visible.
func (s SaveOverlay) Active() bool {
	return s.phase != saveHidden
}

// Show computes the diff and opens the preview overlay.
func (s *SaveOverlay) Show(original, current *config.Config) {
	s.err = nil
	s.reloaded = false
	s.scrollOffset = 0

	lines := computeDiffLines(original, current)
	if len(lines) == 0 {
		s.phase = saveResult
		s.err = fmt.Errorf("no changes to save")
		return
	}
	s.diffLines = lines
	s.phase = savePreview
}

// SaveSucceeded reports whether the last save completed without error.
func (s SaveOverlay) SaveSucceeded() bool {
	return s.phase == saveResult && s.err == nil
}

// Update handles input while the overlay is active.
func (s SaveOverlay) Update(msg tea.Msg, cfg *config.Config, client *ipc.Client, connected bool) SaveOverlay {
	switch s.phase {
	case savePreview:
		if km, ok := msg.(tea.KeyMsg); ok {
			switch km.String() {
			case "esc":
				s.phase = saveHidden
			case "enter", "y":
				s.err = cfg.Save()
				if s.err == nil && connected && client != nil {
					s.reloaded = client.Reload() == nil
				}
				s.phase = saveResult
			case "up", "k":
				if s.scrollOffset > 0 {
					s.scrollOffset--
				}
			case "down", "j":
				s.scrollOffset++
			}
		}
	case saveResult:
		if _, ok := msg.(tea.KeyMsg); ok {
			s.phase = saveHidden
		}
	}
	return s
}

// View renders the overlay for the given content area dimensions.
func (s SaveOverlay) View(width, height int) string {
	switch s.phase {
	case savePreview:
		return s.viewPreview(width, height)
	case saveResult:
		return s.viewResult(width, height)
	}
	return ""
}

func (s SaveOverlay) viewPreview(areaW, areaH int) string {
	boxW := areaW - 8
	if boxW > 80 {
		boxW = 80
	}
	if boxW < 30 {
		boxW = 30
	}

	titleStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("15"))
	addStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("42"))
	rmStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("196"))
	ctxStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("245"))
	footStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("241"))

	title := titleStyle.Render("Save Config — Pending Changes")

	// Visible diff area height: total minus title, blank lines, footer, border, padding
	diffH := areaH - 10
	if diffH < 3 {
		diffH = 3
	}

	// Clamp scroll offset
	maxScroll := len(s.diffLines) - diffH
	if maxScroll < 0 {
		maxScroll = 0
	}
	off := s.scrollOffset
	if off > maxScroll {
		off = maxScroll
	}

	innerW := boxW - 6 // account for border + padding
	if innerW < 10 {
		innerW = 10
	}

	end := off + diffH
	if end > len(s.diffLines) {
		end = len(s.diffLines)
	}

	var lines []string
	for _, dl := range s.diffLines[off:end] {
		t := dl.text
		if len(t) > innerW-2 {
			t = t[:innerW-2]
		}
		switch dl.kind {
		case diffAdded:
			lines = append(lines, addStyle.Render("+ "+t))
		case diffRemoved:
			lines = append(lines, rmStyle.Render("- "+t))
		default:
			lines = append(lines, ctxStyle.Render("  "+t))
		}
	}

	diff := strings.Join(lines, "\n")
	footer := footStyle.Render("enter: save  esc: cancel  j/k: scroll")
	content := title + "\n\n" + diff + "\n\n" + footer

	box := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("62")).
		Padding(1, 2).
		Width(boxW).
		Render(content)

	return lipgloss.Place(areaW, areaH, lipgloss.Center, lipgloss.Center, box)
}

func (s SaveOverlay) viewResult(areaW, areaH int) string {
	boxW := areaW - 8
	if boxW > 60 {
		boxW = 60
	}
	if boxW < 30 {
		boxW = 30
	}

	var msg string
	if s.err != nil {
		errStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("196")).Bold(true)
		msg = errStyle.Render("Error: " + s.err.Error())
	} else {
		okStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("42")).Bold(true)
		msg = okStyle.Render("Config saved successfully")
		if s.reloaded {
			msg += "\n" + lipgloss.NewStyle().Foreground(lipgloss.Color("42")).Render("Daemon reloaded")
		}
	}

	footer := lipgloss.NewStyle().Foreground(lipgloss.Color("241")).Render("press any key to dismiss")
	content := msg + "\n\n" + footer

	box := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("62")).
		Padding(1, 2).
		Width(boxW).
		Render(content)

	return lipgloss.Place(areaW, areaH, lipgloss.Center, lipgloss.Center, box)
}

// --- diff computation ---

func computeDiffLines(original, current *config.Config) []diffLine {
	if original == nil || current == nil {
		return nil
	}

	origBytes, err := yaml.Marshal(original)
	if err != nil {
		return nil
	}
	currBytes, err := yaml.Marshal(current)
	if err != nil {
		return nil
	}

	origStr := strings.TrimSpace(string(origBytes))
	currStr := strings.TrimSpace(string(currBytes))
	if origStr == currStr {
		return nil
	}

	origLines := strings.Split(origStr, "\n")
	currLines := strings.Split(currStr, "\n")

	return lcsDiff(origLines, currLines)
}

// lcsDiff computes a diff using longest common subsequence.
func lcsDiff(a, b []string) []diffLine {
	m, n := len(a), len(b)

	// For very large configs, fall back to simple parallel comparison
	if m*n > 500000 {
		return parallelDiff(a, b)
	}

	// Build LCS table
	tbl := make([][]int, m+1)
	for i := range tbl {
		tbl[i] = make([]int, n+1)
	}
	for i := m - 1; i >= 0; i-- {
		for j := n - 1; j >= 0; j-- {
			if a[i] == b[j] {
				tbl[i][j] = tbl[i+1][j+1] + 1
			} else if tbl[i+1][j] >= tbl[i][j+1] {
				tbl[i][j] = tbl[i+1][j]
			} else {
				tbl[i][j] = tbl[i][j+1]
			}
		}
	}

	// Backtrack to produce diff
	var all []diffLine
	i, j := 0, 0
	for i < m && j < n {
		if a[i] == b[j] {
			all = append(all, diffLine{kind: diffContext, text: a[i]})
			i++
			j++
		} else if tbl[i+1][j] >= tbl[i][j+1] {
			all = append(all, diffLine{kind: diffRemoved, text: a[i]})
			i++
		} else {
			all = append(all, diffLine{kind: diffAdded, text: b[j]})
			j++
		}
	}
	for ; i < m; i++ {
		all = append(all, diffLine{kind: diffRemoved, text: a[i]})
	}
	for ; j < n; j++ {
		all = append(all, diffLine{kind: diffAdded, text: b[j]})
	}

	return filterDiffContext(all, 2)
}

// parallelDiff does simple line-by-line comparison for very large configs.
func parallelDiff(a, b []string) []diffLine {
	var result []diffLine
	maxLen := len(a)
	if len(b) > maxLen {
		maxLen = len(b)
	}
	for i := 0; i < maxLen; i++ {
		var al, bl string
		if i < len(a) {
			al = a[i]
		}
		if i < len(b) {
			bl = b[i]
		}
		if al == bl {
			continue
		}
		if al != "" {
			result = append(result, diffLine{kind: diffRemoved, text: al})
		}
		if bl != "" {
			result = append(result, diffLine{kind: diffAdded, text: bl})
		}
	}
	return result
}

// filterDiffContext keeps changed lines and ctx surrounding context lines.
func filterDiffContext(lines []diffLine, ctx int) []diffLine {
	if len(lines) == 0 {
		return nil
	}

	keep := make([]bool, len(lines))
	for i, l := range lines {
		if l.kind != diffContext {
			lo := i - ctx
			if lo < 0 {
				lo = 0
			}
			hi := i + ctx
			if hi >= len(lines) {
				hi = len(lines) - 1
			}
			for j := lo; j <= hi; j++ {
				keep[j] = true
			}
		}
	}

	var result []diffLine
	prevKept := true
	hasChange := false
	for i, l := range lines {
		if keep[i] {
			if !prevKept {
				result = append(result, diffLine{kind: diffContext, text: "..."})
			}
			result = append(result, l)
			if l.kind != diffContext {
				hasChange = true
			}
			prevKept = true
		} else {
			prevKept = false
		}
	}

	if !hasChange {
		return nil
	}
	return result
}

// cloneConfig creates a deep copy of a Config via YAML round-trip.
func cloneConfig(cfg *config.Config) *config.Config {
	if cfg == nil {
		return nil
	}
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return nil
	}
	var clone config.Config
	if err := yaml.Unmarshal(data, &clone); err != nil {
		return nil
	}
	return &clone
}
