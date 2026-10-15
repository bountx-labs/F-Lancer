package bounty

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"
)

// orphanServer answers the endpoints the orphan scanner uses.
func orphanServer(t *testing.T, updatedAt string, comments []map[string]any) func(http.ResponseWriter, *http.Request) {
	t.Helper()
	return func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/reviews"):
			json.NewEncoder(w).Encode([]map[string]any{})
		case strings.HasSuffix(r.URL.Path, "/comments"):
			json.NewEncoder(w).Encode(comments)
		case strings.Contains(r.URL.Path, "/pulls/"):
			json.NewEncoder(w).Encode(map[string]any{"updated_at": updatedAt})
		case strings.Contains(r.URL.Path, "/search/issues"):
			json.NewEncoder(w).Encode(map[string]any{
				"items": []map[string]any{
					{
						"number":         9,
						"title":          "Old work",
						"html_url":       "https://github.com/o/r/pull/9",
						"repository_url": "https://api.github.com/repos/o/r",
					},
				},
			})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}
}

func TestFindOrphansFlagsStalePull(t *testing.T) {
	stale := time.Now().UTC().Add(-30 * 24 * time.Hour).Format(time.RFC3339)
	c, srv := newContributorForTest(t, orphanServer(t, stale, []map[string]any{}))
	defer srv.Close()

	h := NewReviewHandler(nil, c)
	orphans, err := h.FindOrphans(context.Background(), "me")
	if err != nil {
		t.Fatalf("find: %v", err)
	}
	if len(orphans) != 1 {
		t.Fatalf("expected 1 orphan, got %d", len(orphans))
	}
	if orphans[0].IdleDays < 29 {
		t.Errorf("unexpected idle days: %d", orphans[0].IdleDays)
	}
}

func TestFindOrphansIgnoresFreshPull(t *testing.T) {
	fresh := time.Now().UTC().Add(-2 * time.Hour).Format(time.RFC3339)
	c, srv := newContributorForTest(t, orphanServer(t, fresh, []map[string]any{}))
	defer srv.Close()

	h := NewReviewHandler(nil, c)
	orphans, err := h.FindOrphans(context.Background(), "me")
	if err != nil {
		t.Fatalf("find: %v", err)
	}
	if len(orphans) != 0 {
		t.Errorf("a fresh pull request must not be an orphan, got %+v", orphans)
	}
}

func TestCleanOrphansSkipsPullWithFeedback(t *testing.T) {
	stale := time.Now().UTC().Add(-30 * 24 * time.Hour).Format(time.RFC3339)
	closedPath := false
	c, srv := newContributorForTest(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPatch && strings.Contains(r.URL.Path, "/pulls/") {
			closedPath = true
		}
		orphanServer(t, stale, []map[string]any{
			{"id": 1, "user": map[string]any{"login": "reviewer"}, "body": "please fix"},
		})(w, r)
	})
	defer srv.Close()

	h := NewReviewHandler(nil, c)
	closed, err := h.CleanOrphans(context.Background(), "me")
	if err != nil {
		t.Fatalf("clean: %v", err)
	}
	if closed != 0 {
		t.Errorf("a pull with feedback must be left for review handling, closed %d", closed)
	}
	if closedPath {
		t.Error("a pull with feedback must not be closed")
	}
}

func TestCleanOrphansClosesSilentPull(t *testing.T) {
	stale := time.Now().UTC().Add(-30 * 24 * time.Hour).Format(time.RFC3339)
	patched := false
	commented := false
	c, srv := newContributorForTest(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPatch && strings.Contains(r.URL.Path, "/pulls/") {
			patched = true
			json.NewEncoder(w).Encode(map[string]any{})
			return
		}
		if r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/comments") {
			commented = true
			w.WriteHeader(http.StatusCreated)
			return
		}
		orphanServer(t, stale, []map[string]any{})(w, r)
	})
	defer srv.Close()

	h := NewReviewHandler(nil, c)
	closed, err := h.CleanOrphans(context.Background(), "me")
	if err != nil {
		t.Fatalf("clean: %v", err)
	}
	if closed != 1 {
		t.Fatalf("expected 1 closure, got %d", closed)
	}
	if !patched {
		t.Error("the pull request should have been closed")
	}
	if !commented {
		t.Error("a closing note should have been posted")
	}
}

func TestOrphanAgeIsSevenDays(t *testing.T) {
	if OrphanAge != 7*24*time.Hour {
		t.Errorf("unexpected orphan age threshold: %v", OrphanAge)
	}
}
