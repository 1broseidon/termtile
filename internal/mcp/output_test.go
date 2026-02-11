package mcp

import (
	"testing"
)

func TestCleanOutput(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "empty string",
			input: "",
			want:  "",
		},
		{
			name:  "plain text unchanged",
			input: "hello world\n",
			want:  "hello world",
		},
		{
			name:  "strips box-drawing lines",
			input: "───────────\nactual content\n───────────\n",
			want:  "actual content",
		},
		{
			name:  "strips mixed box-drawing",
			input: "┌──────────┐\n│  header  │\n├──────────┤\ndata row\n└──────────┘\n",
			want:  "│  header  │\ndata row",
		},
		{
			name:  "collapses 3+ blank lines to 2",
			input: "line1\n\n\n\n\nline2\n",
			want:  "line1\n\n\nline2",
		},
		{
			name:  "trims leading and trailing blank lines",
			input: "\n\n\nhello\n\n\n",
			want:  "hello",
		},
		{
			name:  "strips control chars but keeps tabs",
			input: "hello\x01\x02world\there\n",
			want:  "helloworld\there",
		},
		{
			name:  "preserves content between chrome",
			input: "════════\nResult: 42\n════════\n",
			want:  "Result: 42",
		},
		{
			name:  "handles ASCII borders",
			input: "+--------+\n| output |\n+--------+\n",
			want:  "| output |",
		},
		{
			name:  "real-world mixed content",
			input: "\n\n\n───\nAgent output:\nDone.\n\n\n\n\n❯ \n",
			want:  "Agent output:\nDone.\n\n\n❯ ",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := cleanOutput(tt.input)
			if got != tt.want {
				t.Errorf("cleanOutput() =\n%q\nwant:\n%q", got, tt.want)
			}
		})
	}
}

func TestIsChromeLine(t *testing.T) {
	tests := []struct {
		line string
		want bool
	}{
		{"───────────", true},
		{"═══════════", true},
		{"+---------+", true},
		{"|||", true},
		{"---", true},
		{"===", true},
		{"┌──────────┐", true},
		{"└──────────┘", true},
		{"├──────────┤", true},
		{"  ─────  ", true},
		{"", false},            // blank line is not chrome
		{"   ", false},         // whitespace only is not chrome
		{"hello", false},       // text
		{"│ data │", false},    // mixed: has text
		{"── hello ──", false}, // mixed: has text
		{"42", false},          // numbers
	}

	for _, tt := range tests {
		t.Run(tt.line, func(t *testing.T) {
			got := isChromeLine(tt.line)
			if got != tt.want {
				t.Errorf("isChromeLine(%q) = %v, want %v", tt.line, got, tt.want)
			}
		})
	}
}

func TestStripControlChars(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"no control chars", "hello world", "hello world"},
		{"preserves tabs", "col1\tcol2", "col1\tcol2"},
		{"strips NUL and BEL", "he\x00ll\x07o", "hello"},
		{"strips ESC", "he\x1bllo", "hello"},
		{"preserves unicode", "hello 世界 ❯", "hello 世界 ❯"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := stripControlChars(tt.input)
			if got != tt.want {
				t.Errorf("stripControlChars(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestLastNonEmptyLine(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"empty string", "", ""},
		{"all blank", "\n\n\n", ""},
		{"single line", "hello", "hello"},
		{"trailing blanks", "hello\n\n\n", "hello"},
		{"multiple lines", "first\nsecond\nthird\n\n", "third"},
		{"whitespace-only lines", "hello\n   \n  \n", "hello"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := lastNonEmptyLine(tt.input)
			if got != tt.want {
				t.Errorf("lastNonEmptyLine(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestScanFencePairs(t *testing.T) {
	tests := []struct {
		name   string
		output string
		want   []string
	}{
		{
			name:   "no fence tags",
			output: "just some output\nno tags here",
			want:   nil,
		},
		{
			name:   "standalone opening tag only (still writing)",
			output: "banner\n[termtile-response]\npartial response...",
			want:   nil,
		},
		{
			name:   "inline instruction tags are ignored",
			output: "wrap inside [termtile-response] and [/termtile-response] tags",
			want:   nil,
		},
		{
			name:   "standalone response pair (inline instruction ignored)",
			output: "wrap inside [termtile-response] and [/termtile-response] tags\ntask\n[termtile-response]\nThe answer is 42.\n[/termtile-response]\n❯ ",
			want:   []string{"The answer is 42."},
		},
		{
			name:   "cursor-agent pretty box inline + standalone response",
			output: "shell> [termtile-response] and [/termtile-response] tags'\n│ [termtile-response] and [/termtile-response] tags │\n[termtile-response]\nReal answer\n[/termtile-response]",
			want:   []string{"Real answer"},
		},
		{
			name:   "multi-turn: two standalone responses",
			output: "[termtile-response] and [/termtile-response] tags\n[termtile-response]\nFirst\n[/termtile-response]\n[termtile-response] and [/termtile-response] tags\n[termtile-response]\nSecond\n[/termtile-response]",
			want:   []string{"First", "Second"},
		},
		{
			name:   "agent discusses fence tags inline — not matched",
			output: "[termtile-response]\nThe function looks for matched [termtile-response] / [/termtile-response] tag pairs.\n[/termtile-response]",
			want:   []string{"The function looks for matched [termtile-response] / [/termtile-response] tag pairs."},
		},
		{
			name:   "indented standalone tags",
			output: "  [termtile-response]\n  The answer is 42.\n  [/termtile-response]",
			want:   []string{"The answer is 42."},
		},
		{
			name:   "codex inline tags — text on same line as tags",
			output: "echo [termtile-response] and [/termtile-response] tags\n• [termtile-response]The answer is 42.[/termtile-response]\n› ",
			want:   []string{"The answer is 42."},
		},
		{
			name:   "codex inline tags — multi-line response",
			output: "echo [termtile-response] and [/termtile-response] tags\n• [termtile-response]First line.\nSecond line.\nThird line.[/termtile-response]\n› ",
			want:   []string{"First line.\nSecond line.\nThird line."},
		},
		{
			name:   "codex inline open, standalone close",
			output: "• [termtile-response]The answer\nis 42.\n[/termtile-response]\n› ",
			want:   []string{"The answer\nis 42."},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := scanFencePairs(tt.output)
			if len(got) != len(tt.want) {
				t.Fatalf("scanFencePairs() returned %d pairs, want %d\ngot: %v", len(got), len(tt.want), got)
			}
			for i, g := range got {
				if g != tt.want[i] {
					t.Errorf("pair[%d] = %q, want %q", i, g, tt.want[i])
				}
			}
		})
	}
}

func TestIsInstructionPair(t *testing.T) {
	tests := []struct {
		content string
		want    bool
	}{
		{"and", true},
		{" and ", true},
		{"  and  ", true},
		{"The answer is 42.", false},
		{"", false},
		{"and more", false},
	}

	for _, tt := range tests {
		t.Run(tt.content, func(t *testing.T) {
			got := isInstructionPair(tt.content)
			if got != tt.want {
				t.Errorf("isInstructionPair(%q) = %v, want %v", tt.content, got, tt.want)
			}
		})
	}
}

func TestCountResponsePairs(t *testing.T) {
	tests := []struct {
		name   string
		output string
		want   int
	}{
		{
			name:   "no pairs",
			output: "just output",
			want:   0,
		},
		{
			name:   "inline instruction only",
			output: "wrap inside [termtile-response] and [/termtile-response] tags",
			want:   0,
		},
		{
			name:   "inline instruction + standalone response",
			output: "[termtile-response] and [/termtile-response] tags\n[termtile-response]\nAnswer\n[/termtile-response]",
			want:   1,
		},
		{
			name:   "multiple inline echoes + standalone response",
			output: "shell [termtile-response] and [/termtile-response]\nbox [termtile-response] and [/termtile-response]\n[termtile-response]\nAnswer\n[/termtile-response]",
			want:   1,
		},
		{
			name:   "multi-turn: two standalone responses",
			output: "[termtile-response] and [/termtile-response]\n[termtile-response]\nFirst\n[/termtile-response]\n[termtile-response] and [/termtile-response]\n[termtile-response]\nSecond\n[/termtile-response]",
			want:   2,
		},
		{
			name:   "agent still writing (open tag, no close)",
			output: "[termtile-response] and [/termtile-response] tags\n[termtile-response]\nPartial answer...",
			want:   0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := countResponsePairs(tt.output)
			if got != tt.want {
				t.Errorf("countResponsePairs() = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestLastResponseContent(t *testing.T) {
	tests := []struct {
		name   string
		output string
		want   string
		wantOK bool
	}{
		{
			name:   "no pairs",
			output: "just output",
			want:   "",
			wantOK: false,
		},
		{
			name:   "inline instruction only",
			output: "[termtile-response] and [/termtile-response] tags",
			want:   "",
			wantOK: false,
		},
		{
			name:   "standalone response",
			output: "[termtile-response] and [/termtile-response]\n[termtile-response]\nThe answer is 42.\n[/termtile-response]",
			want:   "The answer is 42.",
			wantOK: true,
		},
		{
			name:   "multi-turn returns last response",
			output: "[termtile-response]\nFirst\n[/termtile-response]\n[termtile-response]\nSecond\n[/termtile-response]",
			want:   "Second",
			wantOK: true,
		},
		{
			name:   "multi-line response",
			output: "[termtile-response]\nline 1\nline 2\nline 3\n[/termtile-response]",
			want:   "line 1\nline 2\nline 3",
			wantOK: true,
		},
		{
			name:   "codex inline tags",
			output: "echo [termtile-response] and [/termtile-response] tags\n• [termtile-response]The answer is 42.[/termtile-response]\n› ",
			want:   "The answer is 42.",
			wantOK: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := lastResponseContent(tt.output)
			if ok != tt.wantOK {
				t.Errorf("lastResponseContent() ok = %v, want %v", ok, tt.wantOK)
			}
			if got != tt.want {
				t.Errorf("lastResponseContent() =\n%q\nwant:\n%q", got, tt.want)
			}
		})
	}
}

func TestTrimOutput(t *testing.T) {
	tests := []struct {
		name          string
		output        string
		responseFence bool
		want          string
	}{
		{
			name:          "no fence returns output as-is",
			output:        "banner\nresult",
			responseFence: false,
			want:          "banner\nresult",
		},
		{
			name:          "fence extracts last response",
			output:        "banner\n[termtile-response] and [/termtile-response] tags\ntask\n[termtile-response]\nFixed!\n[/termtile-response]\n❯ ",
			responseFence: true,
			want:          "Fixed!",
		},
		{
			name:          "fence with no response returns full output",
			output:        "banner\n[termtile-response] and [/termtile-response] tags\ntask text",
			responseFence: true,
			want:          "banner\n[termtile-response] and [/termtile-response] tags\ntask text",
		},
		{
			name:          "fence ignores instruction 'and' pair",
			output:        "wrap inside [termtile-response] and [/termtile-response] tags\nstill working...",
			responseFence: true,
			want:          "wrap inside [termtile-response] and [/termtile-response] tags\nstill working...",
		},
		{
			name:          "cursor-agent double instruction echo + response",
			output:        "shell> [termtile-response] and [/termtile-response]'\n│ [termtile-response] and [/termtile-response] │\n[termtile-response]\nReal answer here\n[/termtile-response]\n→ Add a follow-up",
			responseFence: true,
			want:          "Real answer here",
		},
		{
			name:          "codex inline tags extracted",
			output:        "echo [termtile-response] and [/termtile-response] tags\n• [termtile-response]The answer is 42.[/termtile-response]\n› ",
			responseFence: true,
			want:          "The answer is 42.",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := trimOutput(tt.output, tt.responseFence)
			if got != tt.want {
				t.Errorf("trimOutput() =\n%q\nwant:\n%q", got, tt.want)
			}
		})
	}
}

func TestCountCloseTags(t *testing.T) {
	tests := []struct {
		name   string
		output string
		want   int
	}{
		{
			name:   "no close tags",
			output: "just output\nno tags here",
			want:   0,
		},
		{
			name:   "instruction echo not counted (both tags on line)",
			output: "wrap inside [termtile-response] and [/termtile-response] tags",
			want:   0,
		},
		{
			name:   "standalone close tag counted",
			output: "[termtile-response]\nThe answer.\n[/termtile-response]\n❯ ",
			want:   1,
		},
		{
			name:   "close tag without open tag still counted",
			output: "...long response scrolled off...\nfinal line.\n[/termtile-response]\n❯ ",
			want:   1,
		},
		{
			name:   "two close tags",
			output: "[termtile-response]\nFirst\n[/termtile-response]\n[termtile-response]\nSecond\n[/termtile-response]",
			want:   2,
		},
		{
			name:   "codex inline close tag counted",
			output: "• [termtile-response]The answer is 42.[/termtile-response]\n› ",
			want:   1,
		},
		{
			name:   "codex inline close — end of line",
			output: "Third line.[/termtile-response]\n› ",
			want:   1,
		},
		{
			name:   "instruction echo wrapping — line ends with close tag but content is 'and'",
			output: "d, wrap ONLY your final answer inside [termtile-response] and [/termtile-response]\n tags.",
			want:   0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := countCloseTags(tt.output)
			if got != tt.want {
				t.Errorf("countCloseTags() = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestWrapTaskWithFence(t *testing.T) {
	task := "fix the auth bug"
	got := wrapTaskWithFence(task)
	if got == task {
		t.Error("wrapped task should differ from original task")
	}
	// Should contain the fence instruction and the original task.
	if len(got) <= len(task) {
		t.Error("wrapped task should be longer than original")
	}
}

func TestContainsIdlePattern(t *testing.T) {
	tests := []struct {
		name    string
		text    string
		pattern string
		want    bool
	}{
		{
			name:    "pattern on last non-empty line",
			text:    "output\n❯ \n\n\n",
			pattern: "❯",
			want:    true,
		},
		{
			name:    "pattern behind status bar (Claude Code)",
			text:    "● 4\n────\n❯ \n────\n  ⏵⏵ bypass permissions on\n\n\n",
			pattern: "❯",
			want:    true,
		},
		{
			name:    "pattern not present",
			text:    "Processing...\nStill working\n\n",
			pattern: "❯",
			want:    false,
		},
		{
			name:    "pattern too far up (beyond 5 non-empty lines)",
			text:    "❯ \nline1\nline2\nline3\nline4\nline5\n",
			pattern: "❯",
			want:    false,
		},
		{
			name:    "pattern exactly at 5th non-empty line from bottom",
			text:    "❯ \nline1\nline2\nline3\nline4\n",
			pattern: "❯",
			want:    true,
		},
		{
			name:    "empty text",
			text:    "",
			pattern: "❯",
			want:    false,
		},
		{
			name:    "codex prompt",
			text:    "Done.\n› \n",
			pattern: "\u203a",
			want:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := containsIdlePattern(tt.text, tt.pattern)
			if got != tt.want {
				t.Errorf("containsIdlePattern() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestNormalizeReadLines(t *testing.T) {
	tests := []struct {
		name  string
		input int
		want  int
	}{
		{name: "default when unset", input: 0, want: 50},
		{name: "default when negative", input: -10, want: 50},
		{name: "keeps valid range", input: 72, want: 72},
		{name: "caps large values", input: 999, want: 100},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := normalizeReadLines(tt.input)
			if got != tt.want {
				t.Fatalf("normalizeReadLines(%d) = %d, want %d", tt.input, got, tt.want)
			}
		})
	}
}

func TestTailOutputLines(t *testing.T) {
	input := "l1\nl2\nl3\nl4\nl5"
	got := tailOutputLines(input, 3)
	want := "l3\nl4\nl5"
	if got != want {
		t.Fatalf("tailOutputLines() = %q, want %q", got, want)
	}
}

func TestOutputDelta(t *testing.T) {
	tests := []struct {
		name     string
		previous string
		current  string
		want     string
	}{
		{
			name:     "empty previous returns current",
			previous: "",
			current:  "a\nb",
			want:     "a\nb",
		},
		{
			name:     "identical returns empty",
			previous: "a\nb",
			current:  "a\nb",
			want:     "",
		},
		{
			name:     "overlap returns suffix",
			previous: "a\nb\nc",
			current:  "b\nc\nd\ne",
			want:     "d\ne",
		},
		{
			name:     "no overlap returns current",
			previous: "x\ny",
			current:  "a\nb",
			want:     "a\nb",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := outputDelta(tt.previous, tt.current)
			if got != tt.want {
				t.Fatalf("outputDelta() = %q, want %q", got, tt.want)
			}
		})
	}
}
