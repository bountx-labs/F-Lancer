package bounty

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// GitHubScanner finds open GitHub issues that carry a bounty label, using only
// the public search API. No account or token is required, which keeps the agent
// fully autonomous. An optional token raises the rate limit when one is set.
type GitHubScanner struct {
	token   string
	baseURL string
	client  *http.Client
}

func NewGitHubScanner(token string, timeout time.Duration) *GitHubScanner {
	return &GitHubScanner{
		token:   token,
		baseURL: "https://api.github.com",
		client:  &http.Client{Timeout: timeout},
	}
}

// Issue is the subset of a GitHub issue the agent needs to evaluate a bounty.
// The search API returns repository_url rather than an embedded repository
// object, so the repo is derived from that URL.
type Issue struct {
	Number          int      `json:"number"`
	Title           string   `json:"title"`
	HTMLURL         string   `json:"html_url"`
	Body            string   `json:"body"`
	Labels          []Label  `json:"labels"`
	RepositoryURL   string   `json:"repository_url"`
	Repository      RepoRef  `json:"repository"`
	PullRequest     *pullRef `json:"pull_request,omitempty"`
	Comments        int      `json:"comments"`
	CreatedAt       string   `json:"created_at"`
	State           string   `json:"state"`
	Assignees       []Person `json:"assignees"`
}

// RepoFullName returns the owner/name for the issue, preferring the embedded
// repository object and falling back to parsing repository_url, which is what
// the search API actually returns.
func (i Issue) RepoFullName() string {
	if i.Repository.FullName != "" {
		return i.Repository.FullName
	}
	const marker = "/repos/"
	if idx := strings.Index(i.RepositoryURL, marker); idx >= 0 {
		return strings.Trim(i.RepositoryURL[idx+len(marker):], "/")
	}
	return ""
}

type pullRef struct {
	URL string `json:"url"`
}

type RepoRef struct {
	FullName    string `json:"full_name"`
	HTMLURL     string `json:"html_url"`
	Stars       int    `json:"stargazers_count"`
	Language    string `json:"language"`
	Description string `json:"description"`
	PushedAt    string `json:"pushed_at"`
	Archived    bool   `json:"archived"`
	Fork        bool   `json:"fork"`
}

// RepoMeta is the repository detail the gates need. The search API returns an
// abbreviated repo object, so the scanner fetches the full record per repo and
// caches it to stay within the unauthenticated rate limit.
type RepoMeta struct {
	FullName string
	Stars    int
	PushedAt time.Time
	Archived bool
	Fork     bool
}

type Label struct {
	Name string `json:"name"`
}

type Person struct {
	Login string `json:"login"`
}

type searchResponse struct {
	TotalCount int     `json:"total_count"`
	Items      []Issue `json:"items"`
}

// DefaultBountyLabels are the label names bounty platforms and projects use to
// mark paid, solvable work. The scanner unions these queries.
var DefaultBountyLabels = []string{
	"bounty",
	"💎 Bounty",
	"bounty-available",
	"good first issue",
	"help wanted",
}

// ScanBounties returns open issues matching any bounty label, newest first,
// de-duplicated by issue URL. It skips pull requests and already-assigned work,
// then runs every hard gate so only credible, funded-looking work is returned.
func (g *GitHubScanner) ScanBounties(ctx context.Context, labels []string, limit int) ([]Issue, error) {
	if len(labels) == 0 {
		labels = DefaultBountyLabels
	}
	if limit <= 0 {
		limit = 20
	}

	seen := make(map[string]bool)
	repoCache := make(map[string]RepoMeta)
	var out []Issue
	var searched, searchFailed, unverifiable, gated int

	for _, label := range labels {
		if len(out) >= limit {
			break
		}
		issues, err := g.searchLabel(ctx, label, limit)
		if err != nil {
			searchFailed++
			log.Printf("bounty: search label %q failed (likely rate limit): %v", label, err)
			continue
		}
		searched += len(issues)
		for _, issue := range issues {
			if len(out) >= limit {
				break
			}
			if issue.PullRequest != nil {
				continue
			}
			if len(issue.Assignees) > 0 {
				continue
			}
			if seen[issue.HTMLURL] {
				continue
			}
			seen[issue.HTMLURL] = true

			fullName := issue.RepoFullName()
			meta, err := g.repoMeta(ctx, fullName, repoCache)
			if err != nil {
				unverifiable++
				log.Printf("bounty: cannot verify repo %q: %v", fullName, err)
				continue
			}

			verdict := EvaluateGates(GateInput{
				Issue:         issue,
				Repo:          meta,
				PriorAttempts: issue.Comments,
			})
			if !verdict.Allowed {
				gated++
				log.Printf("bounty: skip %s -- %s", issue.HTMLURL, verdict)
				continue
			}

			out = append(out, issue)
		}
	}

	log.Printf("bounty: scan summary -- searched %d, passed %d, gated %d, unverifiable %d, search errors %d",
		searched, len(out), gated, unverifiable, searchFailed)
	return out, nil
}

// repoMeta fetches and caches repository detail. The search API's repo object
// omits pushed_at and archived, which the gates require.
func (g *GitHubScanner) repoMeta(ctx context.Context, fullName string, cache map[string]RepoMeta) (RepoMeta, error) {
	if m, ok := cache[fullName]; ok {
		return m, nil
	}
	if fullName == "" {
		return RepoMeta{}, fmt.Errorf("empty repository name")
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, g.baseURL+"/repos/"+fullName, nil)
	if err != nil {
		return RepoMeta{}, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	if g.token != "" {
		req.Header.Set("Authorization", "Bearer "+g.token)
	}

	resp, err := g.client.Do(req)
	if err != nil {
		return RepoMeta{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return RepoMeta{}, fmt.Errorf("repo %s: HTTP %d", fullName, resp.StatusCode)
	}

	var raw struct {
		FullName string `json:"full_name"`
		Stars    int    `json:"stargazers_count"`
		PushedAt string `json:"pushed_at"`
		Archived bool   `json:"archived"`
		Fork     bool   `json:"fork"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return RepoMeta{}, err
	}

	pushed, _ := time.Parse(time.RFC3339, raw.PushedAt)
	meta := RepoMeta{
		FullName: raw.FullName,
		Stars:    raw.Stars,
		PushedAt: pushed,
		Archived: raw.Archived,
		Fork:     raw.Fork,
	}
	cache[fullName] = meta
	return meta, nil
}

// searchLabel queries the search API for open, unassigned issues with a label.
func (g *GitHubScanner) searchLabel(ctx context.Context, label string, perPage int) ([]Issue, error) {
	q := fmt.Sprintf(`label:"%s" state:open is:issue no:assignee`, label)
	params := url.Values{}
	params.Set("q", q)
	params.Set("sort", "created")
	params.Set("order", "desc")
	params.Set("per_page", strconv.Itoa(perPage))

	endpoint := g.baseURL + "/search/issues?" + params.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("github search build request: %w", err)
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	if g.token != "" {
		req.Header.Set("Authorization", "Bearer "+g.token)
	}

	resp, err := g.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("github search call: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("github search HTTP %d", resp.StatusCode)
	}

	var parsed searchResponse
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return nil, fmt.Errorf("github search decode: %w", err)
	}
	return parsed.Items, nil
}

// BountyHint extracts a dollar amount from an issue's labels or body, so the
// agent can rank work by expected value. It returns 0 when no amount is found.
func BountyHint(issue Issue) float64 {
	for _, l := range issue.Labels {
		if v := parseAmount(l.Name); v > 0 {
			return v
		}
	}
	return parseAmount(firstLines(issue.Body, 400))
}

// parseAmount finds the largest currency-looking number in a string.
func parseAmount(s string) float64 {
	var best float64
	fields := strings.FieldsFunc(s, func(r rune) bool {
		return !(r >= '0' && r <= '9' || r == '.' || r == '$' || r == ',' || r == 'k' || r == 'K')
	})
	for _, f := range fields {
		f = strings.TrimPrefix(f, "$")
		f = strings.ReplaceAll(f, ",", "")
		mult := 1.0
		if strings.HasSuffix(f, "k") || strings.HasSuffix(f, "K") {
			mult = 1000
			f = strings.TrimRight(f, "kK")
		}
		v, err := strconv.ParseFloat(f, 64)
		if err != nil {
			continue
		}
		v *= mult
		if v > best && v <= 100000 {
			best = v
		}
	}
	return best
}

// firstLines returns at most n bytes of the body, so a huge issue cannot slow
// the scan down.
func firstLines(body string, n int) string {
	if len(body) <= n {
		return body
	}
	return body[:n]
}
