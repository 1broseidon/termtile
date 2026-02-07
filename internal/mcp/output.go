package mcp

import (
	"crypto/rand"
	"encoding/hex"
	"strings"
	"unicode"
)

// cleanOutput processes raw tmux capture-pane text by removing TUI chrome,
// collapsing excessive blank lines, and trimming leading/trailing whitespace.
// tmux capture-pane -p already strips ANSI escapes, so no regex stripping needed.
func cleanOutput(raw string) string {
	lines := strings.Split(raw, "\n")
	var out []string
	blankCount := 0

	for _, line := range lines {
		if isChromeLine(line) {
			continue
		}
		if strings.TrimSpace(line) == "" {
			blankCount++
			if blankCount <= 2 {
				out = append(out, "")
			}
			continue
		}
		blankCount = 0
		out = append(out, stripControlChars(line))
	}

	// Trim leading blank lines.
	for len(out) > 0 && strings.TrimSpace(out[0]) == "" {
		out = out[1:]
	}
	// Trim trailing blank lines.
	for len(out) > 0 && strings.TrimSpace(out[len(out)-1]) == "" {
		out = out[:len(out)-1]
	}

	return strings.Join(out, "\n")
}

// isChromeLine returns true if a line consists entirely of box-drawing,
// table border, or other TUI chrome characters (plus whitespace).
func isChromeLine(line string) bool {
	trimmed := strings.TrimSpace(line)
	if trimmed == "" {
		return false // blank lines are not chrome
	}
	for _, r := range trimmed {
		if !isChromeRune(r) {
			return false
		}
	}
	return true
}

// isChromeRune returns true for box-drawing, block elements, and common
// TUI border characters.
func isChromeRune(r rune) bool {
	// Box Drawing: U+2500–U+257F
	if r >= 0x2500 && r <= 0x257F {
		return true
	}
	// Block Elements: U+2580–U+259F
	if r >= 0x2580 && r <= 0x259F {
		return true
	}
	// Common ASCII borders
	switch r {
	case '+', '-', '|', '=':
		return true
	}
	return false
}

const (
	fenceOpen  = "[termtile-response]"
	fenceClose = "[/termtile-response]"

	// fenceInstruction is prepended to the task when response_fence is enabled.
	fenceInstruction = "IMPORTANT: When you are completely finished, wrap ONLY your final answer inside " +
		fenceOpen + " and " + fenceClose + " tags. Do not include any other text outside these tags in your final response.\n\n"
)

// wrapTaskWithFence prepends the fence instruction to the task text.
func wrapTaskWithFence(task string) string {
	return fenceInstruction + task
}

// extractFencedResponse extracts content between <termtile-response> tags.
// Returns the extracted content and true if found, or empty string and false.
func extractFencedResponse(output string) (string, bool) {
	openIdx := strings.LastIndex(output, fenceOpen)
	if openIdx < 0 {
		return "", false
	}
	start := openIdx + len(fenceOpen)

	closeIdx := strings.Index(output[start:], fenceClose)
	if closeIdx < 0 {
		// Opening tag found but no closing tag yet — agent may still be writing.
		return "", false
	}

	content := output[start : start+closeIdx]
	return strings.TrimSpace(content), true
}

// generateMarker creates a unique short marker for an agent task.
// Format: [agent:xxxxxxxx] where x is random hex (4 bytes = 8 hex chars).
// At 17 chars total, this never wraps in any TUI and is reliably matchable.
func generateMarker() string {
	b := make([]byte, 4)
	_, _ = rand.Read(b)
	return "[agent:" + hex.EncodeToString(b) + "]"
}

// appendMarker appends the marker to the task text as a trailing line.
func appendMarker(task, marker string) string {
	return task + "\n\n" + marker
}

// trimOutput applies the best available output trimming strategy:
//  1. Marker delimiter first (strips startup noise, fence instruction, and task text)
//  2. Fenced response tags on the remaining text (if response_fence was enabled)
//  3. Trimmed output (fallback)
//
// The marker runs first because the fence instruction text itself contains
// the fence tags as literal text ("...inside [termtile-response] and
// [/termtile-response] tags..."). If the marker isn't found (e.g. scrolled
// off-screen), we must NOT attempt fence extraction — it would match the
// instruction's tags and extract the word "and".
func trimOutput(output, marker string, responseFence bool) string {
	trimmed, markerFound := trimToAfterMarker(output, marker)
	if responseFence && markerFound {
		if fenced, ok := extractFencedResponse(trimmed); ok {
			return fenced
		}
	}
	return trimmed
}

// trimToAfterMarker scans the output for the LAST occurrence of the agent
// marker and returns only the content that follows it, along with whether
// the marker was found. Uses the last occurrence because the marker appears
// multiple times in the terminal output: once in the shell command echo
// and once in the agent TUI's echo. The last one is closest to the actual
// response and reliably separates the instruction/task from the answer.
func trimToAfterMarker(output, marker string) (string, bool) {
	if marker == "" {
		return output, false
	}

	lines := strings.Split(output, "\n")
	lastIdx := -1
	for i, line := range lines {
		if strings.Contains(line, marker) {
			lastIdx = i
		}
	}
	if lastIdx < 0 {
		return output, false
	}

	remaining := strings.Join(lines[lastIdx+1:], "\n")
	return strings.TrimLeft(remaining, "\n"), true
}

// stripControlChars removes control characters from a line,
// preserving tabs and newlines.
func stripControlChars(line string) string {
	var b strings.Builder
	b.Grow(len(line))
	for _, r := range line {
		if r == '\t' || r == '\n' || !unicode.IsControl(r) {
			b.WriteRune(r)
		}
	}
	return b.String()
}
