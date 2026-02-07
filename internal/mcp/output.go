package mcp

import (
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

// hasOpenTag returns true if a line contains the open fence tag but NOT the
// close tag. This filters out instruction echoes where both tags appear on
// the same line ("...inside [termtile-response] and [/termtile-response] tags...").
func hasOpenTag(line string) bool {
	return strings.Contains(line, fenceOpen) && !strings.Contains(line, fenceClose)
}

// hasCloseTag returns true if a line contains the close fence tag but NOT the
// open tag. This filters out instruction echoes where both tags appear on
// the same line.
func hasCloseTag(line string) bool {
	return strings.Contains(line, fenceClose) && !strings.Contains(line, fenceOpen)
}

// scanFencePairs finds matched open/close fence tag pairs in the output.
// Tags can be standalone (on their own line) or inline (with response text
// on the same line, as codex does). Instruction echoes are filtered out
// because they contain BOTH tags on a single line with text after the close
// tag (e.g. "...inside [termtile-response] and [/termtile-response] tags...").
//
// For inline tags, content after the open tag and before the close tag on
// their respective lines is included in the extracted content.
func scanFencePairs(output string) []string {
	lines := strings.Split(output, "\n")
	var pairs []string
	for i := 0; i < len(lines); i++ {
		// Case 1: single-line response — both tags on same line and the
		// line ends with fenceClose (instruction echoes have text after
		// the close tag like "tags..." so they don't match).
		if content, ok := extractSingleLine(lines[i]); ok {
			if !isInstructionPair(content) {
				pairs = append(pairs, content)
			}
			continue
		}

		// Case 2: multi-line response — open tag on one line, close on another.
		if !hasOpenTag(lines[i]) {
			continue
		}
		found := false
		for j := i + 1; j < len(lines); j++ {
			if !hasCloseTag(lines[j]) {
				continue
			}
			content := extractBetweenTags(lines, i, j)
			pairs = append(pairs, content)
			i = j // outer loop will i++ past the close tag
			found = true
			break
		}
		if !found {
			break // unclosed pair — agent still writing
		}
	}
	return pairs
}

// extractSingleLine checks if a line contains both fence tags with the close
// tag at the end of the line (after trimming). Returns the content between
// the tags and true if matched, or empty string and false otherwise.
func extractSingleLine(line string) (string, bool) {
	trimmed := strings.TrimSpace(line)
	if !strings.Contains(trimmed, fenceOpen) || !strings.HasSuffix(trimmed, fenceClose) {
		return "", false
	}
	openIdx := strings.Index(line, fenceOpen)
	closeIdx := strings.Index(line, fenceClose)
	if openIdx >= closeIdx {
		return "", false
	}
	content := strings.TrimSpace(line[openIdx+len(fenceOpen) : closeIdx])
	return content, true
}

// extractBetweenTags extracts response content from between open and close
// tag lines, including any text after the open tag and before the close tag
// on their respective lines (handles both standalone and inline tags).
func extractBetweenTags(lines []string, openLine, closeLine int) string {
	var contentLines []string

	// Text after the open tag on its line.
	if idx := strings.Index(lines[openLine], fenceOpen); idx >= 0 {
		after := lines[openLine][idx+len(fenceOpen):]
		if strings.TrimSpace(after) != "" {
			contentLines = append(contentLines, after)
		}
	}

	// Lines between open and close.
	for k := openLine + 1; k < closeLine; k++ {
		contentLines = append(contentLines, lines[k])
	}

	// Text before the close tag on its line.
	if idx := strings.Index(lines[closeLine], fenceClose); idx >= 0 {
		before := lines[closeLine][:idx]
		if strings.TrimSpace(before) != "" {
			contentLines = append(contentLines, before)
		}
	}

	return strings.TrimSpace(strings.Join(contentLines, "\n"))
}

// isInstructionPair returns true if the content between fence tags came from
// the fence instruction text wrapping across lines rather than an actual agent
// response. This happens on very narrow terminals where the instruction
// "...inside [termtile-response] and [/termtile-response] tags..." wraps so
// the tags end up on different lines, producing content "and".
func isInstructionPair(content string) bool {
	return strings.TrimSpace(content) == "and"
}

// countCloseTags counts response close tags in the output. A close tag is
// counted if either: (1) the line contains fenceClose but not fenceOpen
// (multi-line response), or (2) both tags are on the same line and the line
// ends with fenceClose (single-line response, as codex does). Instruction
// echoes are excluded because they have text after the close tag.
func countCloseTags(output string) int {
	lines := strings.Split(output, "\n")
	count := 0
	for _, line := range lines {
		if hasCloseTag(line) {
			count++
		} else if content, ok := extractSingleLine(line); ok && !isInstructionPair(content) {
			count++
		}
	}
	return count
}

// countResponsePairs counts the number of real (non-instruction) fence pairs
// in the output.
func countResponsePairs(output string) int {
	pairs := scanFencePairs(output)
	count := 0
	for _, content := range pairs {
		if !isInstructionPair(content) {
			count++
		}
	}
	return count
}

// lastResponseContent returns the content of the last non-instruction fence
// pair, or empty string and false if no real response exists.
func lastResponseContent(output string) (string, bool) {
	pairs := scanFencePairs(output)
	for i := len(pairs) - 1; i >= 0; i-- {
		if !isInstructionPair(pairs[i]) {
			return pairs[i], true
		}
	}
	return "", false
}

// trimOutput extracts the agent's response from raw terminal output.
// For fence-enabled agents, it returns the last real response pair's content.
// For non-fence agents, it returns the output as-is.
func trimOutput(output string, responseFence bool) string {
	if !responseFence {
		return output
	}
	if content, ok := lastResponseContent(output); ok {
		return content
	}
	return output
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
