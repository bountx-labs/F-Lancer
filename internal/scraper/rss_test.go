package scraper

import "testing"

func TestParseDateRFC1123Z(t *testing.T) {
	got, err := parseDate("Mon, 01 Sep 2026 07:36:00 -0400")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Year() != 2026 || got.Month() != 9 || got.Day() != 1 {
		t.Errorf("unexpected date: %v", got)
	}
}

func TestParseDateRFC3339(t *testing.T) {
	got, err := parseDate("2026-09-01T07:36:00Z")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.UTC().Hour() != 7 {
		t.Errorf("unexpected hour: %v", got)
	}
}

func TestParseDateInvalid(t *testing.T) {
	if _, err := parseDate("not a date"); err == nil {
		t.Fatal("expected error for invalid date")
	}
}

func TestParseDateEmpty(t *testing.T) {
	if _, err := parseDate(""); err == nil {
		t.Fatal("expected error for empty date")
	}
}
