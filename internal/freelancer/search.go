package freelancer

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// SearchClient resolves a job brief back to a live Freelancer.com project so
// the submit queue can carry the numeric project_id the bid endpoint requires.
// The RSS feed gives us a title and link but not the project id, so the agent
// looks the project up by exact link, then falls back to title search.
type SearchClient struct {
	oauthToken string
	client     *http.Client
}

func NewSearchClient(oauthToken string, timeout time.Duration) *SearchClient {
	return &SearchClient{
		oauthToken: oauthToken,
		client:     &http.Client{Timeout: timeout},
	}
}

// Project is the subset of the Freelancer project payload the queue builder
// needs. Only stable, documented fields are decoded.
type Project struct {
	ID       int64  `json:"id"`
	Title    string `json:"title"`
	Currency struct {
		Code string `json:"code"`
	} `json:"currency"`
	Budget struct {
		Minimum float64 `json:"minimum"`
		Maximum float64 `json:"maximum"`
	} `json:"budget"`
	SeoURL string `json:"seo_url"`
}

type activeProjectsResponse struct {
	Status  string `json:"status"`
	Message string `json:"message"`
	Result  struct {
		Projects []Project `json:"projects"`
	} `json:"result"`
}

// FindProjectByLink resolves a project from its public URL. The exact link is
// tried first through a full-text query built from the URL's slug; the first
// result whose SeoURL matches wins, otherwise the closest title match is
// returned when found.
func (c *SearchClient) FindProjectByLink(ctx context.Context, link string) (*Project, error) {
	query := linkToQuery(link)
	if query == "" {
		return nil, fmt.Errorf("cannot derive search query from link %q", link)
	}

	projects, err := c.searchActive(ctx, query)
	if err != nil {
		return nil, err
	}
	if len(projects) == 0 {
		return nil, fmt.Errorf("no project found for query %q", query)
	}

	want := normalizeURL(link)
	for i := range projects {
		if normalizeURL(projects[i].SeoURL) == want {
			return &projects[i], nil
		}
	}

	return &projects[0], nil
}

// searchActive calls the active-projects search endpoint, which is public and
// read-only but still accepts the OAuth bearer token for consistent quota.
func (c *SearchClient) searchActive(ctx context.Context, query string) ([]Project, error) {
	params := url.Values{}
	params.Set("query", query)
	params.Set("limit", "10")
	params.Set("offset", "0")
	params.Set("job_details", "true")
	params.Set("full_description", "true")

	endpoint := "https://www.freelancer.com/api/projects/0.1/projects/active/?" + params.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("freelancer search build request: %w", err)
	}
	if c.oauthToken != "" {
		req.Header.Set("Authorization", "Bearer "+c.oauthToken)
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("freelancer search call: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("freelancer search HTTP %d", resp.StatusCode)
	}

	var parsed activeProjectsResponse
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return nil, fmt.Errorf("freelancer search decode: %w", err)
	}
	if parsed.Status != "success" {
		msg := parsed.Message
		if msg == "" {
			msg = parsed.Status
		}
		return nil, fmt.Errorf("freelancer search failed: %s", msg)
	}

	return parsed.Result.Projects, nil
}

// linkToQuery turns a project URL into a human search phrase. Freelancer URLs
// look like /projects/<category>/<slug>/, so the slug is the best query term.
func linkToQuery(link string) string {
	trimmed := strings.TrimSpace(link)
	if trimmed == "" {
		return ""
	}

	if u, err := url.Parse(trimmed); err == nil && u.Path != "" {
		trimmed = strings.Trim(u.Path, "/")
	}

	parts := strings.Split(trimmed, "/")
	slug := parts[len(parts)-1]
	slug = strings.TrimSuffix(slug, ".html")
	slug = strings.TrimSuffix(slug, ".htm")
	slug = strings.NewReplacer("-", " ", "_", " ").Replace(slug)
	return strings.TrimSpace(slug)
}

// normalizeURL compares two project links ignoring scheme, host, trailing
// slash, and query string so a feed link and an API seo_url match reliably.
func normalizeURL(raw string) string {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return strings.ToLower(strings.TrimSuffix(raw, "/"))
	}
	u.Scheme = ""
	u.Host = ""
	u.RawQuery = ""
	u.Fragment = ""
	return strings.ToLower(strings.Trim(u.Path, "/"))
}

// ParsedBudget returns the maximum budget as a float when present, so the
// queue builder can suggest a bid without the agent guessing.
func (p *Project) ParsedBudget() float64 {
	if p.Budget.Maximum > 0 {
		return p.Budget.Maximum
	}
	return p.Budget.Minimum
}

// ParseProjectID extracts a numeric project id from a project URL of the form
// /projects/<category>/<slug>/<id>/ or from a bare integer string.
func ParseProjectID(raw string) (int64, error) {
	s := strings.TrimSpace(raw)
	if s == "" {
		return 0, fmt.Errorf("empty project reference")
	}
	if id, err := strconv.ParseInt(s, 10, 64); err == nil {
		return id, nil
	}

	u, err := url.Parse(s)
	if err != nil {
		return 0, fmt.Errorf("parse project reference %q: %w", s, err)
	}
	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	for i := len(parts) - 1; i >= 0; i-- {
		if id, err := strconv.ParseInt(parts[i], 10, 64); err == nil {
			return id, nil
		}
	}
	return 0, fmt.Errorf("no project id in %q", s)
}
