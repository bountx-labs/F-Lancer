package bounty

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestSeenStoreMarkAndHas(t *testing.T) {
	s, err := NewSeenStore(filepath.Join(t.TempDir(), "seen.json"))
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	key := IssueKey("o/r", 7)
	if s.Has(key) {
		t.Fatal("unseen key reported as seen")
	}
	s.Mark(key)
	if !s.Has(key) {
		t.Fatal("marked key not reported as seen")
	}
}

func TestSeenStoreRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "seen.json")
	s, _ := NewSeenStore(path)
	s.Mark(IssueKey("o/r", 1))
	s.Mark(IssueKey("o/r", 2))
	if err := s.Save(); err != nil {
		t.Fatalf("save: %v", err)
	}

	loaded, err := NewSeenStore(path)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if loaded.Count() != 2 {
		t.Errorf("expected 2 entries, got %d", loaded.Count())
	}
	if !loaded.Has(IssueKey("o/r", 1)) {
		t.Error("persisted key missing after reload")
	}
}

func TestSeenStoreMissingFileIsEmpty(t *testing.T) {
	s, err := NewSeenStore(filepath.Join(t.TempDir(), "absent.json"))
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if s.Count() != 0 {
		t.Errorf("expected empty store, got %d", s.Count())
	}
}

func TestSeenStoreCorruptFileStartsFresh(t *testing.T) {
	path := filepath.Join(t.TempDir(), "seen.json")
	if err := os.WriteFile(path, []byte("{corrupt"), 0644); err != nil {
		t.Fatal(err)
	}
	s, err := NewSeenStore(path)
	if err != nil {
		t.Fatalf("a corrupt store must not stop the run: %v", err)
	}
	if s.Count() != 0 {
		t.Errorf("expected a fresh store, got %d entries", s.Count())
	}
}

func TestSeenStorePruneDropsOldEntries(t *testing.T) {
	s, _ := NewSeenStore(filepath.Join(t.TempDir(), "seen.json"))
	s.Seen["old"] = time.Now().UTC().Add(-30 * 24 * time.Hour)
	s.Seen["fresh"] = time.Now().UTC()

	removed := s.Prune(7 * 24 * time.Hour)
	if removed != 1 {
		t.Errorf("expected 1 removal, got %d", removed)
	}
	if s.Has("old") {
		t.Error("old entry should have been pruned")
	}
	if !s.Has("fresh") {
		t.Error("fresh entry should survive")
	}
}

func TestIssueKey(t *testing.T) {
	if got := IssueKey("o/r", 42); got != "o/r#42" {
		t.Errorf("unexpected key: %q", got)
	}
}

func TestWithinRaceWindow(t *testing.T) {
	now := time.Now().UTC()

	fresh := now.Add(-1 * time.Hour).Format(time.RFC3339)
	if within, _ := WithinRaceWindow(fresh, now); !within {
		t.Error("a one-hour-old bounty is still winnable")
	}

	stale := now.Add(-48 * time.Hour).Format(time.RFC3339)
	if within, _ := WithinRaceWindow(stale, now); within {
		t.Error("a two-day-old bounty is past the window")
	}
}

func TestWithinRaceWindowRejectsBadTimestamp(t *testing.T) {
	if within, _ := WithinRaceWindow("not-a-date", time.Now().UTC()); within {
		t.Error("an unparseable timestamp must not be treated as fresh")
	}
}

func TestRaceWindowIsTight(t *testing.T) {
	// The whole point of the tighter window is to stop wasting the budget on
	// leftovers. A window of a day or more defeats it.
	if RaceWindow > 12*time.Hour {
		t.Errorf("race window of %s is too wide to win anything", RaceWindow)
	}
}

func TestSortByFreshnessOrdersNewestFirst(t *testing.T) {
	now := time.Now().UTC()
	issues := []Issue{
		{Number: 1, CreatedAt: now.Add(-10 * time.Hour).Format(time.RFC3339)},
		{Number: 2, CreatedAt: now.Add(-1 * time.Hour).Format(time.RFC3339)},
		{Number: 3, CreatedAt: now.Add(-5 * time.Hour).Format(time.RFC3339)},
	}
	SortByFreshness(issues)
	if issues[0].Number != 2 || issues[2].Number != 1 {
		t.Errorf("expected newest first, got %d,%d,%d", issues[0].Number, issues[1].Number, issues[2].Number)
	}
}

func TestItoa(t *testing.T) {
	cases := map[int]string{0: "0", 7: "7", 42: "42", 1234: "1234", -5: "-5"}
	for in, want := range cases {
		if got := itoa(in); got != want {
			t.Errorf("itoa(%d) = %q, want %q", in, got, want)
		}
	}
}
