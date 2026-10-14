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

func newTestClient(t *testing.T, handler http.HandlerFunc) (*Client, *httptest.Server) {
	t.Helper()
	srv := httptest.NewServer(handler)
	c := NewClient("test-key", 5*time.Second)
	c.baseURL = srv.URL
	return c, srv
}

func TestDiscoverJobsParsesResponse(t *testing.T) {
	c, srv := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("x-api-key") != "test-key" {
			t.Errorf("missing api key header")
		}
		if r.URL.Query().Get("status") != "OPEN" {
			t.Errorf("expected status=OPEN")
		}
		json.NewEncoder(w).Encode(map[string]any{
			"data": []map[string]any{{"id": "j1", "title": "Job", "proposedUsdc": "10.00"}},
		})
	})
	defer srv.Close()

	jobs, err := c.DiscoverJobs(context.Background(), "CODING", 10)
	if err != nil {
		t.Fatalf("discover: %v", err)
	}
	if len(jobs) != 1 || jobs[0].ID != "j1" {
		t.Errorf("unexpected jobs: %+v", jobs)
	}
}

func TestPlaceBidSendsStringAmount(t *testing.T) {
	var captured map[string]any
	c, srv := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		json.NewDecoder(r.Body).Decode(&captured)
		json.NewEncoder(w).Encode(map[string]any{"id": "b1"})
	})
	defer srv.Close()

	_, err := c.PlaceBid(context.Background(), "j1", "50.00", "msg")
	if err != nil {
		t.Fatalf("bid: %v", err)
	}
	if captured["proposedUsdc"] != "50.00" {
		t.Errorf("amount must be a string, got %T", captured["proposedUsdc"])
	}
}

func TestWithdrawRejectsEmptyAddress(t *testing.T) {
	c, srv := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		t.Error("no request should be made with an empty address")
	})
	defer srv.Close()

	if err := c.Withdraw(context.Background(), "a1", "  ", "10"); err == nil {
		t.Fatal("expected error for empty destination")
	}
}

func TestAPIErrorSurfacesMessage(t *testing.T) {
	c, srv := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		json.NewEncoder(w).Encode(map[string]any{"message": "certification required"})
	})
	defer srv.Close()

	_, err := c.DiscoverJobs(context.Background(), "", 5)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "certification required") {
		t.Errorf("error should carry API message, got %v", err)
	}
}

func TestWalletParsesBalance(t *testing.T) {
	c, srv := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{"address": "0xabc", "usdcBalance": "42.50"})
	})
	defer srv.Close()

	w, err := c.Wallet(context.Background(), "a1")
	if err != nil {
		t.Fatalf("wallet: %v", err)
	}
	if w.Address != "0xabc" || w.UsdcBalance != "42.50" {
		t.Errorf("unexpected wallet: %+v", w)
	}
}

func TestAssignedJobsUsesAgentFilter(t *testing.T) {
	var gotQuery string
	c, srv := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.RawQuery
		json.NewEncoder(w).Encode(map[string]any{"data": []map[string]any{}})
	})
	defer srv.Close()

	if _, err := c.AssignedJobs(context.Background(), "agent-9"); err != nil {
		t.Fatalf("assigned: %v", err)
	}
	if !strings.Contains(gotQuery, "agentId=agent-9") || !strings.Contains(gotQuery, "status=ASSIGNED") {
		t.Errorf("unexpected query: %q", gotQuery)
	}
}

func TestResolveAgentIDByHandle(t *testing.T) {
	c, srv := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{
			"data": []map[string]any{
				{"id": "a1", "handle": "one", "name": "One"},
				{"id": "a2", "handle": "flancer-worker", "name": "F-Lancer"},
			},
		})
	})
	defer srv.Close()

	got, err := c.ResolveAgentID(context.Background(), "flancer-worker")
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if got != "a2" {
		t.Errorf("expected a2, got %q", got)
	}
}

func TestResolveAgentIDAcceptsAtPrefix(t *testing.T) {
	c, srv := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{
			"data": []map[string]any{{"id": "a1", "handle": "flancer-worker"}},
		})
	})
	defer srv.Close()

	got, err := c.ResolveAgentID(context.Background(), "@flancer-worker")
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if got != "a1" {
		t.Errorf("expected a1, got %q", got)
	}
}

func TestResolveAgentIDEmptyHandleReturnsFirst(t *testing.T) {
	c, srv := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{
			"data": []map[string]any{{"id": "first"}, {"id": "second"}},
		})
	})
	defer srv.Close()

	got, err := c.ResolveAgentID(context.Background(), "")
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if got != "first" {
		t.Errorf("expected first agent, got %q", got)
	}
}

func TestResolveAgentIDNoMatch(t *testing.T) {
	c, srv := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{
			"data": []map[string]any{{"id": "a1", "handle": "other"}},
		})
	})
	defer srv.Close()

	if _, err := c.ResolveAgentID(context.Background(), "flancer-worker"); err == nil {
		t.Fatal("expected error when no agent matches the handle")
	}
}

func TestResolveAgentIDEmptyList(t *testing.T) {
	c, srv := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{"data": []map[string]any{}})
	})
	defer srv.Close()

	if _, err := c.ResolveAgentID(context.Background(), ""); err == nil {
		t.Fatal("expected error for an empty agent list")
	}
}
