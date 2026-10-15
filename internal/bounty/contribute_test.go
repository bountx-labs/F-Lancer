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

func newContributorForTest(t *testing.T, handler http.HandlerFunc) (*Contributor, *httptest.Server) {
	t.Helper()
	srv := httptest.NewServer(handler)
	c := NewContributor("test-token", 5*time.Second)
	c.base = srv.URL
	c.client = srv.Client()
	return c, srv
}

func TestContributorAuthenticated(t *testing.T) {
	if NewContributor("", time.Second).Authenticated() {
		t.Error("empty token must not count as authenticated")
	}
	if NewContributor("   ", time.Second).Authenticated() {
		t.Error("whitespace token must not count as authenticated")
	}
	if !NewContributor("ghp_x", time.Second).Authenticated() {
		t.Error("a real token must count as authenticated")
	}
}

func TestCurrentLogin(t *testing.T) {
	c, srv := newContributorForTest(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test-token" {
			t.Errorf("missing auth header")
		}
		json.NewEncoder(w).Encode(map[string]any{"login": "airdropia"})
	})
	defer srv.Close()

	login, err := c.CurrentLogin(context.Background())
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	if login != "airdropia" {
		t.Errorf("unexpected login: %q", login)
	}
}

func TestCommentOnIssuePostsBody(t *testing.T) {
	var gotPath string
	var gotBody map[string]any
	c, srv := newContributorForTest(t, func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		json.NewDecoder(r.Body).Decode(&gotBody)
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(map[string]any{"id": 1})
	})
	defer srv.Close()

	err := c.CommentOnIssue(context.Background(), "owner/repo", 42, "claiming this")
	if err != nil {
		t.Fatalf("comment: %v", err)
	}
	if gotPath != "/repos/owner/repo/issues/42/comments" {
		t.Errorf("unexpected path: %q", gotPath)
	}
	if gotBody["body"] != "claiming this" {
		t.Errorf("unexpected body: %v", gotBody)
	}
}

func TestEnsureForkReusesExistingFork(t *testing.T) {
	forkCalls := 0
	c, srv := newContributorForTest(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/repos/me/target" {
			json.NewEncoder(w).Encode(map[string]any{"full_name": "me/target"})
			return
		}
		if strings.HasSuffix(r.URL.Path, "/forks") {
			forkCalls++
			w.WriteHeader(http.StatusAccepted)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	})
	defer srv.Close()

	got, err := c.EnsureFork(context.Background(), "upstream/target", "me")
	if err != nil {
		t.Fatalf("fork: %v", err)
	}
	if got != "me/target" {
		t.Errorf("unexpected fork: %q", got)
	}
	if forkCalls != 0 {
		t.Errorf("an existing fork must not be recreated, got %d fork calls", forkCalls)
	}
}

func TestEnsureForkCreatesWhenMissing(t *testing.T) {
	created := false
	forks := 0
	c, srv := newContributorForTest(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/repos/me/target":
			if !created {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			json.NewEncoder(w).Encode(map[string]any{"full_name": "me/target"})
		case strings.HasSuffix(r.URL.Path, "/forks"):
			forks++
			created = true
			w.WriteHeader(http.StatusAccepted)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	})
	defer srv.Close()

	got, err := c.EnsureFork(context.Background(), "upstream/target", "me")
	if err != nil {
		t.Fatalf("fork: %v", err)
	}
	if got != "me/target" {
		t.Errorf("unexpected fork: %q", got)
	}
	if forks != 1 {
		t.Errorf("expected exactly one fork creation, got %d", forks)
	}
}

func TestEnsureForkRejectsBadRepoName(t *testing.T) {
	c := NewContributor("t", time.Second)
	if _, err := c.EnsureFork(context.Background(), "noslash", "me"); err == nil {
		t.Fatal("expected error for a repo without an owner")
	}
}

func TestEnsureForkRejectsOwnRepo(t *testing.T) {
	c, srv := newContributorForTest(t, func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{"full_name": "me/target"})
	})
	defer srv.Close()

	// The account already owns the repo, so no fork is needed.
	got, err := c.EnsureFork(context.Background(), "me/target", "me")
	if err != nil {
		t.Fatalf("fork: %v", err)
	}
	if got != "me/target" {
		t.Errorf("unexpected fork: %q", got)
	}
}

func TestAPIErrorCarriesBody(t *testing.T) {
	c, srv := newContributorForTest(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		json.NewEncoder(w).Encode(map[string]any{"message": "Resource not accessible"})
	})
	defer srv.Close()

	_, err := c.CurrentLogin(context.Background())
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "Resource not accessible") {
		t.Errorf("error should carry the API message, got %v", err)
	}
}
