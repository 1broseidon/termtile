package mcp

import "testing"

func TestRenderSpawnTemplate(t *testing.T) {
	tests := []struct {
		name     string
		template string
		dir      string
		cmd      string
		want     []string
		wantErr  bool
	}{
		{
			name:     "ghostty with dir and cmd",
			template: "ghostty --working-directory={{dir}} -e {{cmd}}",
			dir:      "/home/user/project",
			cmd:      "tmux new-session -s test",
			want:     []string{"ghostty", "--working-directory=/home/user/project", "-e", "tmux", "new-session", "-s", "test"},
		},
		{
			name:     "alacritty with dir and cmd",
			template: "alacritty --working-directory {{dir}} -e {{cmd}}",
			dir:      "/tmp",
			cmd:      "bash",
			want:     []string{"alacritty", "--working-directory", "/tmp", "-e", "bash"},
		},
		{
			name:     "empty cmd removes flag",
			template: "ghostty --working-directory={{dir}} -e {{cmd}}",
			dir:      "/home/user",
			cmd:      "",
			want:     []string{"ghostty", "--working-directory=/home/user"},
		},
		{
			name:     "multi-word cmd",
			template: "kitty --directory {{dir}} {{cmd}}",
			dir:      "/tmp",
			cmd:      "tmux new-session -s my-session -c /tmp 'claude --dangerously-skip-permissions'",
			want:     []string{"kitty", "--directory", "/tmp", "tmux", "new-session", "-s", "my-session", "-c", "/tmp", "claude --dangerously-skip-permissions"},
		},
		{
			name:     "unterminated quote",
			template: "ghostty -e '{{cmd}",
			dir:      "/tmp",
			cmd:      "test",
			wantErr:  true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := renderSpawnTemplate(tt.template, tt.dir, tt.cmd)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(got) != len(tt.want) {
				t.Fatalf("len mismatch: got %v, want %v", got, tt.want)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("argv[%d] = %q, want %q", i, got[i], tt.want[i])
				}
			}
		})
	}
}

func TestSplitCommand(t *testing.T) {
	tests := []struct {
		input   string
		want    []string
		wantErr bool
	}{
		{"hello world", []string{"hello", "world"}, false},
		{"'hello world'", []string{"hello world"}, false},
		{`"hello world"`, []string{"hello world"}, false},
		{`it\'s`, []string{"it's"}, false},
		{"  spaces  between  ", []string{"spaces", "between"}, false},
		{"", nil, false},
		{"single", []string{"single"}, false},
		{`mix 'single quoted' "double quoted"`, []string{"mix", "single quoted", "double quoted"}, false},
		{"'unterminated", nil, true},
		{`trail\`, nil, true},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got, err := splitCommand(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(got) != len(tt.want) {
				t.Fatalf("got %v, want %v", got, tt.want)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("[%d] = %q, want %q", i, got[i], tt.want[i])
				}
			}
		})
	}
}

func TestLookupSpawnTemplate(t *testing.T) {
	templates := map[string]string{
		"com.mitchellh.ghostty": "ghostty --working-directory={{dir}} -e {{cmd}}",
		"Alacritty":             "alacritty --working-directory {{dir}} -e {{cmd}}",
		"kitty":                 "kitty --directory {{dir}} {{cmd}}",
	}

	tests := []struct {
		class string
		want  string
		found bool
	}{
		{"com.mitchellh.ghostty", "ghostty --working-directory={{dir}} -e {{cmd}}", true},
		{"Alacritty", "alacritty --working-directory {{dir}} -e {{cmd}}", true},
		{"alacritty", "alacritty --working-directory {{dir}} -e {{cmd}}", true}, // case-insensitive
		{"kitty", "kitty --directory {{dir}} {{cmd}}", true},
		{"KITTY", "kitty --directory {{dir}} {{cmd}}", true}, // case-insensitive
		{"nonexistent", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.class, func(t *testing.T) {
			got, ok := lookupSpawnTemplate(templates, tt.class)
			if ok != tt.found {
				t.Fatalf("found = %v, want %v", ok, tt.found)
			}
			if got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}

	// Nil map should return not found.
	if _, ok := lookupSpawnTemplate(nil, "test"); ok {
		t.Error("expected not found for nil map")
	}
}
