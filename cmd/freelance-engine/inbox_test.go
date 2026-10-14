package main

import "testing"

func TestSlugifyBasic(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{"Modern Corporate Business Website", "modern-corporate-business-website"},
		{"PDF Document  Data Entry", "pdf-document-data-entry"},
		{"  Leading and trailing  ", "leading-and-trailing"},
		{"UPPERCASE", "uppercase"},
	}
	for _, tc := range tests {
		if got := slugify(tc.in); got != tc.want {
			t.Errorf("slugify(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestSlugifyStripsSymbols(t *testing.T) {
	got := slugify("Node.js & React: Full-Stack!")
	for _, ch := range got {
		if !((ch >= 'a' && ch <= 'z') || (ch >= '0' && ch <= '9') || ch == '-') {
			t.Errorf("slug contains invalid char %q in %q", ch, got)
		}
	}
}

func TestSlugifyEmptyFallsBack(t *testing.T) {
	if got := slugify("!!!"); got != "job" {
		t.Errorf("expected fallback 'job', got %q", got)
	}
	if got := slugify(""); got != "job" {
		t.Errorf("expected fallback 'job', got %q", got)
	}
}

func TestSlugifyTruncatesLongTitles(t *testing.T) {
	long := "a very long title that keeps going and going and going without any end in sight at all"
	got := slugify(long)
	if len(got) > 60 {
		t.Errorf("slug too long: %d chars", len(got))
	}
}

func TestSlugifyNoLeadingOrTrailingDash(t *testing.T) {
	for _, in := range []string{"---test---", "  hello  ", "!a!"} {
		got := slugify(in)
		if len(got) > 0 && (got[0] == '-' || got[len(got)-1] == '-') {
			t.Errorf("slugify(%q) = %q has edge dash", in, got)
		}
	}
}

func TestSuggestBidZeroBudget(t *testing.T) {
	if got := suggestBid(0); got != 50 {
		t.Errorf("expected default 50, got %v", got)
	}
}

func TestSuggestBidAppliesFloor(t *testing.T) {
	if got := suggestBid(5); got != 10 {
		t.Errorf("expected floor 10, got %v", got)
	}
}

func TestSuggestBidRoundsToTwoDecimals(t *testing.T) {
	got := suggestBid(1000)
	if got != 900 {
		t.Errorf("expected 900, got %v", got)
	}
}
