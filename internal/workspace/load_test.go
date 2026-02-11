package workspace

import "testing"

func TestWMClassesMatch(t *testing.T) {
	tests := []struct {
		a, b string
		want bool
	}{
		// Exact match
		{"ghostty", "ghostty", true},
		// Reverse-domain vs short name
		{"com.mitchellh.ghostty", "ghostty", true},
		{"ghostty", "com.mitchellh.ghostty", true},
		// Full reverse-domain match
		{"com.mitchellh.ghostty", "com.mitchellh.ghostty", true},
		// Different terminals should not match
		{"ghostty", "kitty", false},
		{"com.mitchellh.ghostty", "kitty", false},
		// Other reverse-domain formats
		{"com.gexperts.tilix", "tilix", true},
		{"dev.warp.warp", "warp", true},
		// Hyphenated names
		{"gnome-terminal", "gnome-terminal", true},
		// No false positives: short name is not a suffix segment
		{"alacritty", "kitty", false},
		// Empty strings
		{"", "", true},
		{"ghostty", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.a+"_vs_"+tt.b, func(t *testing.T) {
			got := wmClassesMatch(tt.a, tt.b)
			if got != tt.want {
				t.Errorf("wmClassesMatch(%q, %q) = %v, want %v", tt.a, tt.b, got, tt.want)
			}
		})
	}
}
