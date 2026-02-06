package tui

import (
	"fmt"
	"strings"
)

// ANSI escape codes
const (
	escClear      = "\x1b[2J"
	escHome       = "\x1b[H"
	escHideCursor = "\x1b[?25l"
	escShowCursor = "\x1b[?25h"
	escBold       = "\x1b[1m"
	escDim        = "\x1b[2m"
	escReset      = "\x1b[0m"
	escReverse    = "\x1b[7m"
	escCyan       = "\x1b[36m"
	escYellow     = "\x1b[33m"
	escRed        = "\x1b[31m"
	escGreen      = "\x1b[32m"
)

func moveTo(row, col int) string {
	return fmt.Sprintf("\x1b[%d;%dH", row, col)
}

func (t *TUI) render() {
	t.updateSize()

	var sb strings.Builder

	// Hide cursor during render
	sb.WriteString(escHideCursor)
	sb.WriteString(escReset)
	sb.WriteString(escClear)
	sb.WriteString(escHome)

	// Calculate layout
	const (
		sepWidth        = 3 // " │ "
		maxListWidth    = 30
		minListWidth    = 10
		minPreviewWidth = 12
		headerLines     = 2 // title + divider
		footerLines     = 3 // divider + status + footer
	)

	width := t.width
	height := t.height
	if width < 1 {
		width = 1
	}
	if height < 1 {
		height = 1
	}

	listWidth := width / 3
	if listWidth > maxListWidth {
		listWidth = maxListWidth
	}
	if listWidth < minListWidth {
		listWidth = minListWidth
	}

	previewWidth := width - listWidth - sepWidth
	if previewWidth < minPreviewWidth {
		previewWidth = minPreviewWidth
		listWidth = width - sepWidth - previewWidth
		if listWidth < minListWidth {
			listWidth = minListWidth
			previewWidth = width - listWidth - sepWidth
			if previewWidth < 1 {
				previewWidth = 1
			}
		}
	}

	previewHeight := height - headerLines - footerLines
	if previewHeight < 1 {
		previewHeight = 1
	}

	// Header
	sb.WriteString(escBold)
	sb.WriteString(escCyan)
	title := "termtile TUI"
	sb.WriteString(centerText(title, width))
	sb.WriteString(escReset)
	sb.WriteString("\r\n")

	// Divider
	sb.WriteString(strings.Repeat("─", width))
	sb.WriteString("\r\n")

	// Main content area
	layoutLines := t.renderLayoutList(listWidth, previewHeight)
	previewLines := t.renderPreview(previewWidth, previewHeight)

	for i := 0; i < previewHeight; i++ {
		// Layout list column
		if i < len(layoutLines) {
			sb.WriteString(layoutLines[i])
		} else {
			sb.WriteString(strings.Repeat(" ", listWidth))
		}

		// Separator
		sb.WriteString(" │ ")

		// Preview column
		if i < len(previewLines) {
			sb.WriteString(previewLines[i])
		}

		sb.WriteString("\r\n")
	}

	// Divider
	sb.WriteString(strings.Repeat("─", width))
	sb.WriteString("\r\n")

	// Status line
	sb.WriteString(truncateANSI(t.renderStatus(), width))
	sb.WriteString("\r\n")

	// Footer with keybindings
	sb.WriteString(truncateANSI(t.renderFooter(), width))

	fmt.Print(sb.String())
}

func (t *TUI) renderLayoutList(width, height int) []string {
	lines := make([]string, 0, height)

	// Title
	title := escBold + "Layouts" + escReset
	lines = append(lines, padRight(title, width))

	if t.result == nil || len(t.layouts) == 0 {
		lines = append(lines, padRight(escDim+"(no layouts)"+escReset, width))
		return lines
	}

	// Layouts list
	for i, name := range t.layouts {
		if len(lines) >= height {
			break
		}

		var line string
		isDefault := name == t.result.Config.DefaultLayout
		isSelected := i == t.selectedIndex

		// Build layout entry
		prefix := "  "
		if isDefault {
			prefix = escGreen + "* " + escReset
		}

		displayName := name
		if len(displayName) > width-4 {
			displayName = displayName[:width-7] + "..."
		}

		if isSelected {
			line = escReverse + prefix + displayName + escReset
		} else {
			line = prefix + displayName
		}

		lines = append(lines, padRight(line, width))
	}

	return lines
}

func (t *TUI) renderPreview(width, height int) []string {
	layout := t.selectedLayout()
	if layout == nil {
		lines := make([]string, height)
		for i := range lines {
			lines[i] = strings.Repeat(" ", width)
		}
		return lines
	}

	// Preview title
	title := fmt.Sprintf("%sPreview: %s (%d tiles)%s",
		escBold, t.selectedLayoutName(), t.tileCount, escReset)

	lines := make([]string, 0, height)
	lines = append(lines, padRight(truncateANSI(title, width), width))
	lines = append(lines, "") // blank line

	// Render ASCII preview
	canvasHeight := height - 3 // title + blank + summary
	canvasWidth := width - 2   // some padding
	if canvasHeight < 1 {
		canvasHeight = 1
	}
	if canvasWidth < 1 {
		canvasWidth = 1
	}

	previewLines := renderASCIIPreview(layout, t.tileCount, canvasWidth, canvasHeight)
	lines = append(lines, previewLines...)

	gapSize := 8
	if t.result != nil && t.result.Config != nil {
		gapSize = t.result.Config.GapSize
	}
	summary := summarizeLayout(layout, t.tileCount, gapSize)
	if summary != "" {
		lines = append(lines, padRight(truncateANSI(escDim+summary+escReset, width), width))
	}

	// Pad to height
	for len(lines) < height {
		lines = append(lines, strings.Repeat(" ", width))
	}

	return lines
}

func (t *TUI) renderStatus() string {
	if t.lastError != "" {
		return fmt.Sprintf("%sError: %s%s", escRed, t.lastError, escReset)
	}

	if t.result == nil {
		return escDim + "No config loaded" + escReset
	}

	layout := t.selectedLayout()
	if layout == nil {
		return ""
	}

	// Show layout details
	return fmt.Sprintf("Mode: %s%s%s  |  Region: %s%s%s  |  Tiles: %s%d%s",
		escCyan, layout.Mode, escReset,
		escCyan, layout.TileRegion.Type, escReset,
		escYellow, t.tileCount, escReset)
}

func (t *TUI) renderFooter() string {
	keys := []string{
		"j/k/↑/↓:nav", "2/4/6/9:tiles", "e:edit", "r:reload", "q/esc/^C:quit",
	}
	return escDim + strings.Join(keys, "  ") + escReset
}

func centerText(text string, width int) string {
	visibleLen := visibleLength(text)
	if visibleLen >= width {
		return text
	}
	padding := (width - visibleLen) / 2
	return strings.Repeat(" ", padding) + text
}

func padRight(text string, width int) string {
	visibleLen := visibleLength(text)
	if visibleLen >= width {
		return text
	}
	return text + strings.Repeat(" ", width-visibleLen)
}

// visibleLength returns the visible length of a string, ignoring ANSI codes.
func visibleLength(s string) int {
	inEscape := false
	length := 0
	for _, r := range s {
		if r == '\x1b' {
			inEscape = true
			continue
		}
		if inEscape {
			if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') {
				inEscape = false
			}
			continue
		}
		length++
	}
	return length
}

func truncateANSI(text string, width int) string {
	if width < 1 {
		return ""
	}
	if visibleLength(text) <= width {
		return text
	}

	var sb strings.Builder
	inEscape := false
	visible := 0
	for _, r := range text {
		if r == '\x1b' {
			inEscape = true
			sb.WriteRune(r)
			continue
		}
		if inEscape {
			sb.WriteRune(r)
			if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') {
				inEscape = false
			}
			continue
		}

		if visible >= width-1 {
			break
		}
		sb.WriteRune(r)
		visible++
	}

	sb.WriteString("…")
	sb.WriteString(escReset)
	return sb.String()
}
