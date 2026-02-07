package mcp

import (
	"strings"
	"testing"
)

func TestArtifactStoreSetGetClear(t *testing.T) {
	store := NewArtifactStore(1024)
	store.Set("ws", 1, "hello")

	art, ok := store.Get("ws", 1)
	if !ok {
		t.Fatalf("expected artifact to exist")
	}
	if art.Output != "hello" {
		t.Fatalf("expected output %q, got %q", "hello", art.Output)
	}
	if art.Truncated {
		t.Fatalf("expected not truncated")
	}

	store.Clear("ws", 1)
	_, ok = store.Get("ws", 1)
	if ok {
		t.Fatalf("expected artifact to be cleared")
	}
}

func TestArtifactStoreTruncatesWithWarning(t *testing.T) {
	capBytes := 80
	store := NewArtifactStore(capBytes)
	long := strings.Repeat("a", 500)

	art := store.Set("ws", 2, long)
	if !art.Truncated {
		t.Fatalf("expected truncated artifact")
	}
	if art.Warning == "" {
		t.Fatalf("expected warning to be set")
	}
	if art.OriginalBytes != len(long) {
		t.Fatalf("expected original_bytes %d, got %d", len(long), art.OriginalBytes)
	}
	if art.StoredBytes > capBytes {
		t.Fatalf("expected stored_bytes <= %d, got %d", capBytes, art.StoredBytes)
	}
	if len(art.Output) != art.StoredBytes {
		t.Fatalf("expected output length %d, got %d", art.StoredBytes, len(art.Output))
	}
}

func TestArtifactTemplateSubstitutionReplacesDependencyOnly(t *testing.T) {
	store := NewArtifactStore(1024)
	store.Set("ws", 1, "ONE")
	store.Set("ws", 2, "TWO")

	in := "a {{slot_1.output}} b {{slot_2.output}} c"
	out, missing := substituteSlotOutputTemplates(in, "ws", []int{1}, store)

	if len(missing) != 0 {
		t.Fatalf("expected no missing, got %v", missing)
	}
	if out != "a ONE b {{slot_2.output}} c" {
		t.Fatalf("unexpected substitution output: %q", out)
	}
}

func TestArtifactTemplateSubstitutionMissingLeavesPlaceholder(t *testing.T) {
	store := NewArtifactStore(1024)

	in := "x {{slot_3.output}} y"
	out, missing := substituteSlotOutputTemplates(in, "ws", []int{3}, store)

	if out != in {
		t.Fatalf("expected placeholder unchanged, got %q", out)
	}
	if len(missing) != 1 || missing[0] != 3 {
		t.Fatalf("expected missing [3], got %v", missing)
	}
}

