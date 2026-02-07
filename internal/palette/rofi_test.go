package palette

import (
	"strings"
	"testing"
)

func TestRofiFormatItem_UsesSingleNullSeparator(t *testing.T) {
	b := NewRofiBackend().(*dmenuLikeBackend)

	out := b.formatItem(Item{
		Label:    "Header",
		IsHeader: true,
		Icon:     "folder",
		Info:     "info",
		Meta:     "meta",
		IsActive: true,
		IsUrgent: true,
	}, 0)

	if got := strings.Count(out, "\x00"); got != 1 {
		t.Fatalf("expected exactly 1 NUL separator, got %d (%q)", got, out)
	}
	if !strings.Contains(out, "\x00nonselectable\x1ftrue") {
		t.Fatalf("expected nonselectable property, got %q", out)
	}
	if strings.Contains(out, "\x00icon\x1f") {
		t.Fatalf("expected icon attribute to be after the first NUL and delimited by \\x1f, got %q", out)
	}
	if !strings.Contains(out, "icon\x1ffolder") || !strings.Contains(out, "info\x1finfo") || !strings.Contains(out, "meta\x1fmeta") {
		t.Fatalf("expected icon/info/meta attributes, got %q", out)
	}
}

func TestRofiFormatItem_DimDivider(t *testing.T) {
	b := NewRofiBackend().(*dmenuLikeBackend)

	out := b.formatItem(Item{
		Label:     "────────",
		IsDivider: true,
	}, 0)

	if !strings.Contains(out, "<span foreground='#666666'>") {
		t.Fatalf("expected dim span for divider, got %q", out)
	}
	if !strings.Contains(out, "\x00nonselectable\x1ftrue") {
		t.Fatalf("expected nonselectable property for divider, got %q", out)
	}
}

func TestRofiFormatItem_BoldHeader(t *testing.T) {
	b := NewRofiBackend().(*dmenuLikeBackend)

	out := b.formatItem(Item{
		Label:    "Section",
		IsHeader: true,
	}, 0)

	if !strings.Contains(out, "<b>Section</b>") {
		t.Fatalf("expected bold markup for header, got %q", out)
	}
	if !strings.Contains(out, "\x00nonselectable\x1ftrue") {
		t.Fatalf("expected nonselectable property for header, got %q", out)
	}
}

func TestRofiBuildArgs_UsesIndexFormatAndNoCustom(t *testing.T) {
	b := NewRofiBackend().(*dmenuLikeBackend)

	_, states := b.formatInput([]Item{
		{Label: "a", IsActive: true},
		{Label: "b", IsUrgent: true},
	})
	args := b.buildArgs("prompt", "message", states)

	if !containsArgs(args, "-format", "i") {
		t.Fatalf("expected -format i in args, got %v", args)
	}
	if !containsArg(args, "-no-custom") {
		t.Fatalf("expected -no-custom in args, got %v", args)
	}
	if !containsArgs(args, "-a", "0") {
		t.Fatalf("expected -a 0 in args, got %v", args)
	}
	if !containsArgs(args, "-u", "1") {
		t.Fatalf("expected -u 1 in args, got %v", args)
	}
	if !containsArgs(args, "-selected-row", "0") {
		t.Fatalf("expected -selected-row 0 in args, got %v", args)
	}
}

func TestRofiBuildArgs_FuzzyMatching(t *testing.T) {
	b := NewRofiBackend().(*dmenuLikeBackend)
	b.SetFuzzyMatching(true)

	_, states := b.formatInput([]Item{{Label: "a"}})
	args := b.buildArgs("prompt", "message", states)

	if !containsArgs(args, "-matching", "fuzzy") {
		t.Fatalf("expected -matching fuzzy in args, got %v", args)
	}
}

func TestRofiParseSelection_Index(t *testing.T) {
	b := NewRofiBackend().(*dmenuLikeBackend)
	items := []Item{
		{Label: "a", Action: "a"},
		{Label: "b", Action: "b"},
	}
	got, err := b.parseSelection("1", items)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Action != "b" {
		t.Fatalf("expected action b, got %q", got.Action)
	}
}

func TestMenu_IgnoresHeaderSelection(t *testing.T) {
	m := NewMenu(&fakeBackend{
		results: []SelectResult{
			{Item: Item{Label: "Header", IsHeader: true}, ExitCode: 0},
			{Item: Item{Label: "Do", Action: "do"}, ExitCode: 0},
		},
	}, []MenuItem{
		{Label: "Header", IsHeader: true},
		{Label: "Do", Action: "do"},
	})

	res, err := m.Show()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Action != "do" {
		t.Fatalf("expected action do, got %q", res.Action)
	}
}

func TestFormatInput_DisambiguatesDuplicateLabels(t *testing.T) {
	b := NewDmenuBackend().(*dmenuLikeBackend)
	items := []Item{
		{Label: "Dup", Action: "a"},
		{Label: "Dup", Action: "b"},
	}

	_, _ = b.formatInput(items)
	if items[0].Label != "Dup" {
		t.Fatalf("expected first label unchanged, got %q", items[0].Label)
	}
	if items[1].Label != "Dup (2)" {
		t.Fatalf("expected second label disambiguated, got %q", items[1].Label)
	}
}

func TestFormatInput_IndexBackendsDoNotDisambiguateDuplicateLabels(t *testing.T) {
	b := NewRofiBackend().(*dmenuLikeBackend)
	items := []Item{
		{Label: "Dup", Action: "a"},
		{Label: "Dup", Action: "b"},
	}

	_, _ = b.formatInput(items)
	if items[0].Label != "Dup" || items[1].Label != "Dup" {
		t.Fatalf("expected labels unchanged for index backend, got %#v", items)
	}
}

type fakeBackend struct {
	results []SelectResult
	i       int
}

func (f *fakeBackend) Show(prompt string, items []Item, message string) (SelectResult, error) {
	if f.i >= len(f.results) {
		return SelectResult{}, ErrCancelled
	}
	res := f.results[f.i]
	f.i++
	return res, nil
}

func (f *fakeBackend) Capabilities() Capabilities {
	return Capabilities{}
}

func containsArg(args []string, want string) bool {
	for _, a := range args {
		if a == want {
			return true
		}
	}
	return false
}

func containsArgs(args []string, a string, b string) bool {
	for i := 0; i+1 < len(args); i++ {
		if args[i] == a && args[i+1] == b {
			return true
		}
	}
	return false
}
