package bounty

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
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

// newScannerForTest serves both the search endpoint and the repo detail
// endpoint so the gates can run. Issues returned are given a healthy age.
func newScannerForTest(t *testing.T, status int, body string) (*GitHubScanner, *httptest.Server) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Accept") != "application/vnd.github+json" {
			t.Errorf("missing github accept header")
		}
		if strings.HasPrefix(r.URL.Path, "/repos/") {
			json.NewEncoder(w).Encode(map[string]any{
				"full_name":        "good/repo",
				"stargazers_count": 500,
				"pushed_at":        time.Now().UTC().Add(-24 * time.Hour).Format(time.RFC3339),
				"archived":         false,
				"fork":             false,
			})
			return
		}
		w.WriteHeader(status)
		w.Write([]byte(body))
	}))
	s := NewGitHubScanner("", 5*time.Second)
	s.client = srv.Client()
	return s, srv
}

func freshIssueJSON(number int, url string) map[string]any {
	return map[string]any{
		"number":     number,
		"title":      "Fix the parser",
		"html_url":   url,
		"body":       "We pay in USDC on merge.",
		"created_at": time.Now().UTC().Add(-1 * time.Hour).Format(time.RFC3339),
		"comments":   0,
		"repository": map[string]any{"full_name": "good/repo", "stargazers_count": 500},
	}
}

func TestScanBountiesSkipsPullRequests(t *testing.T) {
	pr := freshIssueJSON(2, "https://github.com/a/b/pull/2")
	pr["pull_request"] = map[string]any{"url": "x"}
	payload, _ := json.Marshal(map[string]any{
		"items": []map[string]any{
			freshIssueJSON(1, "https://github.com/a/b/issues/1"),
			pr,
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
	assigned := freshIssueJSON(3, "https://github.com/a/b/issues/3")
	assigned["assignees"] = []map[string]any{{"login": "someone"}}
	payload, _ := json.Marshal(map[string]any{"items": []map[string]any{assigned}})
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
		"items": []map[string]any{freshIssueJSON(4, "https://github.com/a/b/issues/4")},
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

func TestScanBountiesRejectsLowStarRepo(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/repos/") {
			json.NewEncoder(w).Encode(map[string]any{
				"full_name":        "tiny/repo",
				"stargazers_count": 2,
				"pushed_at":        time.Now().UTC().Add(-1 * time.Hour).Format(time.RFC3339),
			})
			return
		}
		payload, _ := json.Marshal(map[string]any{
			"items": []map[string]any{freshIssueJSON(9, "https://github.com/tiny/repo/issues/9")},
		})
		w.Write(payload)
	}))
	defer srv.Close()
	s := NewGitHubScanner("", 5*time.Second)
	s.client = srv.Client()

	issues, err := s.ScanBounties(context.Background(), []string{"bounty"}, 10)
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	if len(issues) != 0 {
		t.Errorf("low-star repo bounty should be gated out, got %+v", issues)
	}
}

func TestScanBountiesRejectsUnverifiableRepo(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/repos/") {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		payload, _ := json.Marshal(map[string]any{
			"items": []map[string]any{freshIssueJSON(9, "https://github.com/x/y/issues/9")},
		})
		w.Write(payload)
	}))
	defer srv.Close()
	s := NewGitHubScanner("", 5*time.Second)
	s.client = srv.Client()

	issues, err := s.ScanBounties(context.Background(), []string{"bounty"}, 10)
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	if len(issues) != 0 {
		t.Errorf("unverifiable repo should be gated out, got %+v", issues)
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
