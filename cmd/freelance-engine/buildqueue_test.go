package main

import (
	"os"
	"path/filepath"
	"testing"
)

func writeBriefFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
}

func TestParseBriefExtractsTitleAndLink(t *testing.T) {
	path := filepath.Join(t.TempDir(), "brief.md")
	content := "# Build Next.js Website\n\n| Field | Value |\n|---|---|\n| Link | https://www.freelancer.com/projects/web/foo/123/ |\n| Budget | $500 |\n"
	writeBriefFile(t, path, content)

	b, err := parseBrief(path)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if b.Title != "Build Next.js Website" {
		t.Errorf("unexpected title: %q", b.Title)
	}
	if b.Link != "https://www.freelancer.com/projects/web/foo/123/" {
		t.Errorf("unexpected link: %q", b.Link)
	}
}

func TestParseBriefMissingLink(t *testing.T) {
	path := filepath.Join(t.TempDir(), "brief.md")
	writeBriefFile(t, path, "# Title Only\n\nno table here\n")

	b, err := parseBrief(path)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if b.Title != "Title Only" {
		t.Errorf("unexpected title: %q", b.Title)
	}
	if b.Link != "" {
		t.Errorf("expected empty link, got %q", b.Link)
	}
}

func TestListBriefsNewestFirst(t *testing.T) {
	root := t.TempDir()
	writeBriefFile(t, filepath.Join(root, "2026-09-01", "a.md"), "# A\n| Link | https://x/a |\n")
	writeBriefFile(t, filepath.Join(root, "2026-09-05", "b.md"), "# B\n| Link | https://x/b |\n")

	briefs, err := listBriefs(root)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(briefs) != 2 {
		t.Fatalf("expected 2 briefs, got %d", len(briefs))
	}
	if briefs[0].Title != "B" {
		t.Errorf("expected newest brief first, got %q", briefs[0].Title)
	}
}

func TestListBriefsSkipsFilesWithoutLink(t *testing.T) {
	root := t.TempDir()
	writeBriefFile(t, filepath.Join(root, "2026-09-01", "good.md"), "# Good\n| Link | https://x/g |\n")
	writeBriefFile(t, filepath.Join(root, "2026-09-01", "bad.md"), "# Bad\nno link\n")

	briefs, err := listBriefs(root)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(briefs) != 1 {
		t.Fatalf("expected only 1 brief with a link, got %d", len(briefs))
	}
}

func TestListBriefsMissingRootIsEmpty(t *testing.T) {
	briefs, err := listBriefs(filepath.Join(t.TempDir(), "nope"))
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if len(briefs) != 0 {
		t.Fatalf("expected 0 briefs, got %d", len(briefs))
	}
}

func TestLoadQueueMissingFile(t *testing.T) {
	items, err := loadQueue(filepath.Join(t.TempDir(), "none.json"))
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if items != nil {
		t.Fatalf("expected nil slice, got %v", items)
	}
}

func TestSaveLoadQueueRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "queue.json")
	original := []submitQueueItem{
		{ProjectID: 123, Title: "Job", BidAmount: 500, PeriodDays: 7, MilestonePercentage: 50, Description: "text"},
	}
	if err := saveQueue(path, original); err != nil {
		t.Fatalf("save: %v", err)
	}
	got, err := loadQueue(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(got) != 1 || got[0].ProjectID != 123 || got[0].BidAmount != 500 {
		t.Errorf("round trip mismatch: %+v", got)
	}
}

func TestSaveQueueEmptyIsNoOp(t *testing.T) {
	path := filepath.Join(t.TempDir(), "queue.json")
	if err := saveQueue(path, nil); err != nil {
		t.Fatalf("save empty: %v", err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Error("expected no file written for empty queue")
	}
}
