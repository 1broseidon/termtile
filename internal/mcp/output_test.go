package mcp

import (
	"strings"
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
		{"", false},             // blank line is not chrome
		{"   ", false},          // whitespace only is not chrome
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

func TestExtractFencedResponse(t *testing.T) {
	tests := []struct {
		name    string
		output  string
		want    string
		wantOK  bool
	}{
		{
			name:   "no fence tags",
			output: "just some output\nno tags here",
			want:   "",
			wantOK: false,
		},
		{
			name:   "opening tag only (still writing)",
			output: "banner\n[termtile-response]\npartial response...",
			want:   "",
			wantOK: false,
		},
		{
			name:   "complete fenced response",
			output: "banner\nstartup noise\n[termtile-response]\nThe answer is 42.\n[/termtile-response]\n❯ ",
			want:   "The answer is 42.",
			wantOK: true,
		},
		{
			name:   "multiple opens uses last one",
			output: "[termtile-response]\nold\n[/termtile-response]\nnew prompt\n[termtile-response]\nfresh answer\n[/termtile-response]",
			want:   "fresh answer",
			wantOK: true,
		},
		{
			name:   "multi-line fenced content",
			output: "[termtile-response]\nline 1\nline 2\nline 3\n[/termtile-response]",
			want:   "line 1\nline 2\nline 3",
			wantOK: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := extractFencedResponse(tt.output)
			if ok != tt.wantOK {
				t.Errorf("extractFencedResponse() ok = %v, want %v", ok, tt.wantOK)
			}
			if got != tt.want {
				t.Errorf("extractFencedResponse() =\n%q\nwant:\n%q", got, tt.want)
			}
		})
	}
}

func TestGenerateMarker(t *testing.T) {
	m1 := generateMarker()
	m2 := generateMarker()

	// Should have the expected format.
	if !strings.HasPrefix(m1, "[agent:") || !strings.HasSuffix(m1, "]") {
		t.Errorf("generateMarker() = %q, want [agent:xxxxxxxx] format", m1)
	}
	// Should be exactly 16 chars: [agent: (7) + 8 hex + ] (1).
	if len(m1) != 16 {
		t.Errorf("generateMarker() length = %d, want 16", len(m1))
	}
	// Two markers should be different.
	if m1 == m2 {
		t.Errorf("two generateMarker() calls returned same value: %q", m1)
	}
}

func TestAppendMarker(t *testing.T) {
	got := appendMarker("fix the bug", "[agent:deadbeef]")
	want := "fix the bug\n\n[agent:deadbeef]"
	if got != want {
		t.Errorf("appendMarker() = %q, want %q", got, want)
	}
}

func TestTrimOutput(t *testing.T) {
	tests := []struct {
		name          string
		output        string
		marker        string
		responseFence bool
		want          string
	}{
		{
			name:          "marker strips prompt then fence extracts response",
			output:        "banner\nfence instruction...\nfix the bug\n[agent:abc12345]\nI'm fixing it\n[termtile-response]\nFixed!\n[/termtile-response]\n❯ ",
			marker:        "[agent:abc12345]",
			responseFence: true,
			want:          "Fixed!",
		},
		{
			name:          "falls back to marker delimiter when no fence found",
			output:        "banner\nfix the bug\n[agent:abc12345]\nI fixed the bug.\nDone.",
			marker:        "[agent:abc12345]",
			responseFence: true,
			want:          "I fixed the bug.\nDone.",
		},
		{
			name:          "fence disabled uses marker delimiter only",
			output:        "banner\n[termtile-response]\nFenced!\n[/termtile-response]\nfix it\n[agent:abc12345]\nresult",
			marker:        "[agent:abc12345]",
			responseFence: false,
			want:          "result",
		},
		{
			name:          "fence tags in instruction are stripped by marker first",
			output:        "wrap inside [termtile-response] and [/termtile-response] tags\nWhat is 10 + 20\n[agent:abc12345]\n[termtile-response]\n30\n[/termtile-response]\n❯ ",
			marker:        "[agent:abc12345]",
			responseFence: true,
			want:          "30",
		},
		{
			name:          "empty marker returns full output",
			output:        "banner\nresult",
			marker:        "",
			responseFence: false,
			want:          "banner\nresult",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := trimOutput(tt.output, tt.marker, tt.responseFence)
			if got != tt.want {
				t.Errorf("trimOutput() =\n%q\nwant:\n%q", got, tt.want)
			}
		})
	}
}

func TestWrapTaskWithFence(t *testing.T) {
	task := "fix the auth bug"
	got := wrapTaskWithFence(task)
	if !strings.Contains(got, task) {
		t.Error("wrapped task should contain original task")
	}
	if !strings.Contains(got, "[termtile-response]") {
		t.Error("wrapped task should mention fence tags")
	}
}

func TestTrimToAfterMarker(t *testing.T) {
	tests := []struct {
		name   string
		output string
		marker string
		want   string
	}{
		{
			name:   "empty marker returns full output",
			output: "banner\nstuff\nresult",
			marker: "",
			want:   "banner\nstuff\nresult",
		},
		{
			name:   "no match returns full output",
			output: "banner\nstuff\nresult",
			marker: "[agent:ffffffff]",
			want:   "banner\nstuff\nresult",
		},
		{
			name:   "trims everything before and including marker line",
			output: "Welcome to Claude!\nTips: ...\nfix the auth bug\n[agent:abc12345]\nI'll fix the auth bug now.\nDone.",
			marker: "[agent:abc12345]",
			want:   "I'll fix the auth bug now.\nDone.",
		},
		{
			name:   "marker at end leaves empty output",
			output: "banner\n[agent:abc12345]",
			marker: "[agent:abc12345]",
			want:   "",
		},
		{
			name:   "strips leading newlines from result",
			output: "banner\n[agent:abc12345]\n\n\nresponse",
			marker: "[agent:abc12345]",
			want:   "response",
		},
		{
			name:   "marker embedded in line with other text",
			output: "banner\n> [agent:abc12345]\nresponse here",
			marker: "[agent:abc12345]",
			want:   "response here",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := trimToAfterMarker(tt.output, tt.marker)
			if got != tt.want {
				t.Errorf("trimToAfterMarker() =\n%q\nwant:\n%q", got, tt.want)
			}
		})
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
