package state

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestMarkAndIsSeen(t *testing.T) {
	path := filepath.Join(t.TempDir(), "seen.json")
	s, err := LoadWithLimits(path, 30, 500)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if s.IsSeen("https://example.com/job/1") {
		t.Fatal("unseen url reported as seen")
	}
	s.MarkSeen("https://example.com/job/1")
	if !s.IsSeen("https://example.com/job/1") {
		t.Fatal("marked url not reported as seen")
	}
}

func TestSaveLoadRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "seen.json")
	s, _ := LoadWithLimits(path, 30, 500)
	s.MarkSeen("https://example.com/a")
	s.MarkSeen("https://example.com/b")
	if err := s.Save(); err != nil {
		t.Fatalf("save: %v", err)
	}

	reloaded, err := LoadWithLimits(path, 30, 500)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if len(reloaded.Hashes) != 2 {
		t.Fatalf("expected 2 hashes, got %d", len(reloaded.Hashes))
	}
	if !reloaded.IsSeen("https://example.com/a") {
		t.Fatal("persisted url not seen after reload")
	}
}

func TestLoadMissingFileIsEmpty(t *testing.T) {
	path := filepath.Join(t.TempDir(), "does-not-exist.json")
	s, err := LoadWithLimits(path, 30, 500)
	if err != nil {
		t.Fatalf("expected no error for missing file, got %v", err)
	}
	if len(s.Hashes) != 0 {
		t.Fatalf("expected empty state, got %d hashes", len(s.Hashes))
	}
}

func TestPruneDropsOldEntries(t *testing.T) {
	path := filepath.Join(t.TempDir(), "seen.json")
	s, _ := LoadWithLimits(path, 1, 500)
	s.Hashes["old"] = time.Now().UTC().AddDate(0, 0, -10)
	s.Hashes["fresh"] = time.Now().UTC()
	s.prune()

	if _, ok := s.Hashes["old"]; ok {
		t.Error("old entry should have been pruned")
	}
	if _, ok := s.Hashes["fresh"]; !ok {
		t.Error("fresh entry should have survived prune")
	}
}

func TestPruneEnforcesMaxEntries(t *testing.T) {
	path := filepath.Join(t.TempDir(), "seen.json")
	s, _ := LoadWithLimits(path, 30, 2)
	now := time.Now().UTC()
	s.Hashes["a"] = now.Add(-3 * time.Hour)
	s.Hashes["b"] = now.Add(-2 * time.Hour)
	s.Hashes["c"] = now.Add(-1 * time.Hour)
	s.prune()

	if len(s.Hashes) != 2 {
		t.Fatalf("expected 2 entries after cap, got %d", len(s.Hashes))
	}
	if _, ok := s.Hashes["c"]; !ok {
		t.Error("newest entry should survive the cap")
	}
	if _, ok := s.Hashes["a"]; ok {
		t.Error("oldest entry should be dropped first")
	}
}

func TestCorruptStateReturnsError(t *testing.T) {
	path := filepath.Join(t.TempDir(), "seen.json")
	if err := os.WriteFile(path, []byte("{not json"), 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadWithLimits(path, 30, 500); err == nil {
		t.Fatal("expected error for corrupt state file")
	}
}

func TestHashURLIsStable(t *testing.T) {
	if hashURL("https://a") != hashURL("https://a") {
		t.Error("hash should be deterministic")
	}
	if hashURL("https://a") == hashURL("https://b") {
		t.Error("different urls should hash differently")
	}
}
