package bounty

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestParseAmount(t *testing.T) {
	tests := []struct {
		in   string
		want float64
	}{
		{"$500 bounty", 500},
		{"reward: $1,200", 1200},
		{"$2.5k prize", 2500},
		{"100 USD", 100},
		{"no amount here", 0},
		{"", 0},
	}
	for _, tc := range tests {
		if got := parseAmount(tc.in); got != tc.want {
			t.Errorf("parseAmount(%q) = %v, want %v", tc.in, got, tc.want)
		}
	}
}

func TestParseAmountPicksLargest(t *testing.T) {
	if got := parseAmount("$100 or maybe $900"); got != 900 {
		t.Errorf("expected largest amount 900, got %v", got)
	}
}

func TestParseAmountIgnoresAbsurdValues(t *testing.T) {
	if got := parseAmount("$999999999"); got != 0 {
		t.Errorf("expected implausible amount to be ignored, got %v", got)
	}
}

func TestBountyHintFromLabel(t *testing.T) {
	issue := Issue{Labels: []Label{{Name: "$250"}}, Body: "nothing"}
	if got := BountyHint(issue); got != 250 {
		t.Errorf("expected 250 from label, got %v", got)
	}
}

func TestBountyHintFromBody(t *testing.T) {
	issue := Issue{Body: "This issue has a $75 reward attached."}
	if got := BountyHint(issue); got != 75 {
		t.Errorf("expected 75 from body, got %v", got)
	}
}

func TestFirstLinesTruncates(t *testing.T) {
	long := ""
	for i := 0; i < 500; i++ {
		long += "a"
	}
	if got := firstLines(long, 100); len(got) != 100 {
		t.Errorf("expected 100 bytes, got %d", len(got))
	}
}

func TestFirstLinesKeepsShort(t *testing.T) {
	if got := firstLines("short", 100); got != "short" {
		t.Errorf("unexpected: %q", got)
	}
}

func newScannerForTest(t *testing.T, status int, body string) (*GitHubScanner, *httptest.Server) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Accept") != "application/vnd.github+json" {
			t.Errorf("missing github accept header")
		}
		w.WriteHeader(status)
		w.Write([]byte(body))
	}))
	s := NewGitHubScanner("", 5*time.Second)
	s.client = srv.Client()
	return s, srv
}

func TestScanBountiesSkipsPullRequests(t *testing.T) {
	payload, _ := json.Marshal(map[string]any{
		"items": []map[string]any{
			{"number": 1, "title": "Real issue", "html_url": "https://github.com/a/b/issues/1"},
			{"number": 2, "title": "Is a PR", "html_url": "https://github.com/a/b/pull/2", "pull_request": map[string]any{"url": "x"}},
		},
	})
	s, srv := newScannerForTest(t, http.StatusOK, string(payload))
	defer srv.Close()

	issues, err := s.ScanBounties(context.Background(), []string{"bounty"}, 10)
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	if len(issues) != 1 || issues[0].Number != 1 {
		t.Errorf("expected only the issue, got %+v", issues)
	}
}

func TestScanBountiesSkipsAssigned(t *testing.T) {
	payload, _ := json.Marshal(map[string]any{
		"items": []map[string]any{
			{"number": 3, "title": "Taken", "html_url": "https://github.com/a/b/issues/3", "assignees": []map[string]any{{"login": "someone"}}},
		},
	})
	s, srv := newScannerForTest(t, http.StatusOK, string(payload))
	defer srv.Close()

	issues, err := s.ScanBounties(context.Background(), []string{"bounty"}, 10)
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	if len(issues) != 0 {
		t.Errorf("assigned issues should be skipped, got %+v", issues)
	}
}

func TestScanBountiesDeduplicates(t *testing.T) {
	payload, _ := json.Marshal(map[string]any{
		"items": []map[string]any{
			{"number": 4, "title": "Dup", "html_url": "https://github.com/a/b/issues/4"},
		},
	})
	s, srv := newScannerForTest(t, http.StatusOK, string(payload))
	defer srv.Close()

	issues, err := s.ScanBounties(context.Background(), []string{"bounty", "help wanted", "good first issue"}, 10)
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	if len(issues) != 1 {
		t.Errorf("expected dedup to 1, got %d", len(issues))
	}
}

func TestScanBountiesSurvivesErrorStatus(t *testing.T) {
	s, srv := newScannerForTest(t, http.StatusForbidden, `{"message":"rate limited"}`)
	defer srv.Close()

	issues, err := s.ScanBounties(context.Background(), []string{"bounty"}, 10)
	if err != nil {
		t.Fatalf("a bad label should not fail the whole scan: %v", err)
	}
	if len(issues) != 0 {
		t.Errorf("expected no issues, got %d", len(issues))
	}
}
