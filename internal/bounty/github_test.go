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
	s.baseURL = srv.URL
	s.client = srv.Client()
	s.pace = 0
	s.attempts = 1
	return s, srv
}

func freshIssueJSON(number int, url string) map[string]any {
	return map[string]any{
		"number":         number,
		"title":          "Fix the parser",
		"html_url":       url,
		"body":           "We pay in USDC on merge.",
		"labels":         []map[string]any{{"name": "bounty"}},
		"created_at":     time.Now().UTC().Add(-1 * time.Hour).Format(time.RFC3339),
		"comments":       0,
		"repository_url": "https://api.github.com/repos/good/repo",
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
	s.baseURL = srv.URL
	s.client = srv.Client()
	s.pace = 0
	s.attempts = 1

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
	s.baseURL = srv.URL
	s.client = srv.Client()
	s.pace = 0
	s.attempts = 1

	issues, err := s.ScanBounties(context.Background(), []string{"bounty"}, 10)
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	if len(issues) != 0 {
		t.Errorf("unverifiable repo should be gated out, got %+v", issues)
	}
}

func TestRepoFullNameFromURL(t *testing.T) {
	issue := Issue{RepositoryURL: "https://api.github.com/repos/owner/name"}
	if got := issue.RepoFullName(); got != "owner/name" {
		t.Errorf("expected owner/name, got %q", got)
	}
}

func TestRepoFullNamePrefersEmbedded(t *testing.T) {
	issue := Issue{
		RepositoryURL: "https://api.github.com/repos/from/url",
		Repository:    RepoRef{FullName: "from/object"},
	}
	if got := issue.RepoFullName(); got != "from/object" {
		t.Errorf("expected the embedded name to win, got %q", got)
	}
}

func TestRepoFullNameEmpty(t *testing.T) {
	if got := (Issue{}).RepoFullName(); got != "" {
		t.Errorf("expected empty, got %q", got)
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

// The live market put current funded work in unlabelled programs while every
// label query returned only stale leftovers, so text discovery must be able to
// surface an issue that no label query returns.
func TestScanBountiesIncludesTextDiscovery(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/repos/") {
			json.NewEncoder(w).Encode(map[string]any{
				"full_name":        "funded/program",
				"stargazers_count": 50,
				"pushed_at":        time.Now().UTC().Add(-24 * time.Hour).Format(time.RFC3339),
			})
			return
		}
		if strings.Contains(r.URL.Query().Get("q"), "label:") {
			json.NewEncoder(w).Encode(map[string]any{"items": []map[string]any{}})
			return
		}
		issue := map[string]any{
			"number":         7,
			"title":          "[BOUNTY] Medium: truncated leaf index",
			"html_url":       "https://github.com/funded/program/issues/7",
			"body":           "Severity: Medium ($150 USDC). Payout addresses below.",
			"created_at":     time.Now().UTC().Add(-1 * time.Hour).Format(time.RFC3339),
			"comments":       0,
			"repository_url": "https://api.github.com/repos/funded/program",
		}
		json.NewEncoder(w).Encode(map[string]any{"items": []map[string]any{issue}})
	}))
	defer srv.Close()

	s := NewGitHubScanner("", 5*time.Second)
	s.baseURL = srv.URL
	s.client = srv.Client()
	s.pace = 0
	s.attempts = 1

	issues, err := s.ScanBounties(context.Background(), []string{"bounty"}, 10)
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	if len(issues) != 1 || issues[0].Number != 7 {
		t.Fatalf("text discovery should surface the unlabelled funded issue, got %+v", issues)
	}
}
