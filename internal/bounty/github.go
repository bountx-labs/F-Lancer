package bounty

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
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
	// pace is the delay inserted between requests. It exists as a field so
	// tests can set it to zero instead of waiting out the real throttle.
	pace time.Duration
	// attempts overrides the retry count; zero means use the default.
	attempts int
	// seenStore remembers evaluated issues so a fast poll skips repeats.
	seenStore *SeenStore
}

// SetSeenStore attaches the dedupe store that lets the scanner run frequently
// without re-processing the same issues on every cycle.
func (g *GitHubScanner) SetSeenStore(s *SeenStore) {
	g.seenStore = s
}

// MarkHandled records an issue as handled so a later run does not select it
// again. The scanner chooses candidates before delivery runs, so without this
// a second run could post the same public claim comment on an issue the first
// run had already claimed.
func (g *GitHubScanner) MarkHandled(repo string, number int) {
	if g.seenStore == nil {
		return
	}
	g.seenStore.Mark(IssueKey(repo, number))
}

// DeferRetry pushes an issue out of the next few scans without retiring it.
// Used when an attempt failed for a transient reason: the issue deserves
// another try, but not on the very next cycle.
func (g *GitHubScanner) DeferRetry(repo string, number int, d time.Duration) {
	if g.seenStore == nil {
		return
	}
	g.seenStore.SkipFor(IssueKey(repo, number), d)
}

func NewGitHubScanner(token string, timeout time.Duration) *GitHubScanner {
	return &GitHubScanner{
		token:   token,
		baseURL: "https://api.github.com",
		client:  &http.Client{Timeout: timeout},
		pace:    requestGap,
	}
}

// Issue is the subset of a GitHub issue the agent needs to evaluate a bounty.
// The search API returns repository_url rather than an embedded repository
// object, so the repo is derived from that URL.
type Issue struct {
	Number        int      `json:"number"`
	Title         string   `json:"title"`
	HTMLURL       string   `json:"html_url"`
	Body          string   `json:"body"`
	Labels        []Label  `json:"labels"`
	RepositoryURL string   `json:"repository_url"`
	Repository    RepoRef  `json:"repository"`
	PullRequest   *pullRef `json:"pull_request,omitempty"`
	Comments      int      `json:"comments"`
	CreatedAt     string   `json:"created_at"`
	State         string   `json:"state"`
	Assignees     []Person `json:"assignees"`
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
//
// MainPushedAt is when the default branch itself last moved. PushedAt is the
// newest push to any branch, so a repository drowning in unmerged bot branches
// still looks busy; MainPushedAt is the signal that says whether the maintainer
// is actually landing work.
type RepoMeta struct {
	FullName     string
	Stars        int
	PushedAt     time.Time
	MainPushedAt time.Time
	Archived     bool
	Fork         bool
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

// requestGap throttles calls so GitHub's secondary rate limiter is not
// tripped. GitHub rejects short bursts even when the hourly quota is fine, so
// the scanner paces itself between every request.
const requestGap = 2500 * time.Millisecond

// maxResponseBytes bounds how much of a GitHub response is read. A search page
// of 100 results carries each issue's full body, which easily exceeds a
// megabyte; truncating at that size cut the JSON off mid-object and the decode
// failed, losing the whole page. The limit is high enough for a full page.
const maxResponseBytes = 8 << 20

// maxRetries bounds how many times a rate-limited request is retried. Each
// retry waits longer, so a few attempts cover a short throttling window.
const maxRetries = 4

// retries returns how many attempts a request gets. Tests set attempts to one
// so a rejected request fails fast instead of waiting out the real backoff.
func (g *GitHubScanner) retries() int {
	if g.attempts > 0 {
		return g.attempts
	}
	return maxRetries
}

// get performs a GET with retry/backoff on rate-limit responses and paces
// every attempt so the scanner never bursts the API.
func (g *GitHubScanner) get(ctx context.Context, url string) ([]byte, error) {
	var lastErr error
	for attempt := 0; attempt < g.retries(); attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(time.Duration(attempt*attempt) * 5 * time.Second):
			}
		}

		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("Accept", "application/vnd.github+json")
		if g.token != "" {
			req.Header.Set("Authorization", "Bearer "+g.token)
		}

		resp, err := g.client.Do(req)
		if err != nil {
			lastErr = err
			continue
		}

		body, readErr := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes))
		resp.Body.Close()
		if readErr != nil {
			lastErr = readErr
			continue
		}

		// Pace the next request regardless of outcome.
		if g.pace > 0 {
			select {
			case <-ctx.Done():
			case <-time.After(g.pace):
			}
		}

		if resp.StatusCode == http.StatusOK {
			return body, nil
		}
		if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode == http.StatusForbidden {
			lastErr = fmt.Errorf("github HTTP %d (rate limited): %s", resp.StatusCode, truncateBody(body))
			continue
		}
		return nil, fmt.Errorf("github HTTP %d", resp.StatusCode)
	}
	return nil, lastErr
}

// truncateBody keeps an error message short enough for a log line.
func truncateBody(b []byte) string {
	s := strings.TrimSpace(string(b))
	if len(s) > 160 {
		return s[:160]
	}
	return s
}

// PaidBountyLabels mark work that carries a real reward. Only these labels can
// lead to a payout. The list is deliberately short: a live count showed
// `bounty` alone covers thousands of open issues while `bounty-available`,
// `algora`, and `opire` return almost nothing, so querying them only burns the
// search rate limit.
var PaidBountyLabels = []string{
	"bounty",
	"💎 Bounty",
}

// ContributionLabels are GitHub's standard "please help" labels. They are not
// paid work: no reward is attached, so an autonomous earning agent must never
// treat them as a bounty. Kept for reference and future opt-in use.
var ContributionLabels = []string{
	"good first issue",
	"help wanted",
}

// DefaultBountyLabels is what the scanner searches. It is the paid set only.
var DefaultBountyLabels = PaidBountyLabels

// DiscoveryQueries are free-text searches that surface funded work which
// carries no bounty label. A live count showed the labelled market is almost
// entirely stale: the newest open `label:bounty` issue was months old, while
// current, genuinely open work sat in unlabelled funded programs that only
// appear through their wording. These queries close that gap.
var DiscoveryQueries = []string{
	`"USDC" bounty`,
	`"bounty" "severity"`,
}

// searchPageSize is how many issues one label query fetches. GitHub caps this
// at 100. It is far larger than the number of bounties acted on, because the
// pages are ordered newest first and a small page would stop before reaching
// genuinely fresh work.
const searchPageSize = 100

// ScanBounties returns open issues that carry funded work, newest first,
// de-duplicated by issue URL. It searches both bounty labels and free-text
// paid markers, because a live scan found the labelled market almost entirely
// stale while current funded work sat in unlabelled programs. Pull requests,
// already-assigned work, and anything already evaluated are skipped, then every
// hard gate runs so only credible, freshly posted work is returned.
//
// The freshness ordering matters more than the filtering: funded bounties are
// claimed within hours, so the newest issue is the only one with a real chance
// of still being open. Older survivors are usually the leftover bait.
func (g *GitHubScanner) ScanBounties(ctx context.Context, labels []string, limit int) ([]Issue, error) {
	if len(labels) == 0 {
		labels = DefaultBountyLabels
	}
	if limit <= 0 {
		limit = 20
	}

	seen := make(map[string]bool)
	repoCache := make(map[string]RepoMeta)
	var candidates []Issue
	var searched, searchFailed, alreadySeen int

	for _, label := range labels {
		// The page size is deliberately decoupled from the result limit. The
		// result limit caps how many bounties the agent acts on, while the
		// page size decides how far back it looks. Sizing the page to the
		// result limit meant a fresh bounty sitting past the twentieth
		// result was never even fetched, which is how a genuinely live bounty
		// was missed while the agent saw nothing but stale leftovers.
		issues, err := g.searchIssues(ctx, fmt.Sprintf(`label:"%s"`, label), searchPageSize)
		if err != nil {
			searchFailed++
			log.Printf("bounty: search label %q failed (likely rate limit): %v", label, err)
			continue
		}
		searched += len(issues)
		for _, issue := range issues {
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

			// Skip anything already evaluated on an earlier cycle so a fast
			// poll does not spend its budget re-reading known issues.
			if g.seenStore != nil {
				key := IssueKey(issue.RepoFullName(), issue.Number)
				if g.seenStore.ShouldSkip(key) {
					alreadySeen++
					continue
				}
			}

			candidates = append(candidates, issue)
		}
	}

	// Text discovery runs after the label sweep so unlabelled funded work is
	// considered even when no label query returns anything current. The same
	// de-duplication and seen-store filtering apply.
	for _, qualifier := range DiscoveryQueries {
		issues, err := g.searchIssues(ctx, qualifier, searchPageSize)
		if err != nil {
			searchFailed++
			log.Printf("bounty: search %q failed (likely rate limit): %v", qualifier, err)
			continue
		}
		searched += len(issues)
		for _, issue := range issues {
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

			if g.seenStore != nil {
				key := IssueKey(issue.RepoFullName(), issue.Number)
				if g.seenStore.ShouldSkip(key) {
					alreadySeen++
					continue
				}
			}

			candidates = append(candidates, issue)
		}
	}

	// Newest first: attempt the freshest bounties before anything older.
	SortByFreshness(candidates)

	now := time.Now().UTC()
	var out []Issue
	var gated, unverifiable, outsideWindow int

	for _, issue := range candidates {
		if len(out) >= limit {
			break
		}

		fullName := issue.RepoFullName()
		key := IssueKey(fullName, issue.Number)

		// A chance-of-winning check runs before the expensive repo lookup:
		// anything past the race window is already lost.
		if within, age := WithinRaceWindow(issue.CreatedAt, now); !within {
			outsideWindow++
			log.Printf("bounty: skip %s#%d -- posted %s ago, past the %s race window",
				fullName, issue.Number, age.Round(time.Minute), RaceWindow)
			if g.seenStore != nil {
				g.seenStore.Mark(key)
			}
			continue
		}

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
			if g.seenStore != nil {
				g.seenStore.Mark(key)
			}
			continue
		}

		out = append(out, issue)
	}

	log.Printf("bounty: scan summary -- searched %d, fresh %d, passed %d, gated %d, outside window %d, already seen %d, unverifiable %d, search errors %d",
		searched, len(candidates), len(out), gated, outsideWindow, alreadySeen, unverifiable, searchFailed)
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

	body, err := g.get(ctx, g.baseURL+"/repos/"+fullName)
	if err != nil {
		return RepoMeta{}, fmt.Errorf("repo %s: %w", fullName, err)
	}

	var raw struct {
		FullName      string `json:"full_name"`
		Stars         int    `json:"stargazers_count"`
		PushedAt      string `json:"pushed_at"`
		Archived      bool   `json:"archived"`
		Fork          bool   `json:"fork"`
		DefaultBranch string `json:"default_branch"`
	}
	if err := json.Unmarshal(body, &raw); err != nil {
		return RepoMeta{}, err
	}

	pushed, _ := time.Parse(time.RFC3339, raw.PushedAt)
	meta := RepoMeta{
		FullName:     raw.FullName,
		Stars:        raw.Stars,
		PushedAt:     pushed,
		MainPushedAt: g.mainBranchPushedAt(ctx, fullName, raw.DefaultBranch),
		Archived:     raw.Archived,
		Fork:         raw.Fork,
	}
	cache[fullName] = meta
	return meta, nil
}

// mainBranchPushedAt returns the commit date of the newest commit on the
// repository's default branch. A repo can look freshly pushed while nothing has
// landed on the default branch for weeks, and only the latter says whether the
// maintainer still reviews and merges. A failed lookup returns the zero time,
// which the activity gate treats as unverifiable, so the candidate is skipped
// rather than assumed good.
func (g *GitHubScanner) mainBranchPushedAt(ctx context.Context, fullName, branch string) time.Time {
	if branch == "" {
		branch = "main"
	}
	endpoint := fmt.Sprintf("%s/repos/%s/commits?sha=%s&per_page=1", g.baseURL, fullName, url.QueryEscape(branch))
	body, err := g.get(ctx, endpoint)
	if err != nil {
		return time.Time{}
	}

	var commits []struct {
		Commit struct {
			Committer struct {
				Date string `json:"date"`
			} `json:"committer"`
		} `json:"commit"`
	}
	if err := json.Unmarshal(body, &commits); err != nil || len(commits) == 0 {
		return time.Time{}
	}
	t, err := time.Parse(time.RFC3339, commits[0].Commit.Committer.Date)
	if err != nil {
		return time.Time{}
	}
	return t
}

// searchIssues queries the search API for open, unassigned issues matching the
// given qualifiers. The caller supplies the full filter, so both label and
// free-text discovery share one implementation.
func (g *GitHubScanner) searchIssues(ctx context.Context, qualifier string, perPage int) ([]Issue, error) {
	q := fmt.Sprintf(`%s state:open is:issue no:assignee`, qualifier)
	params := url.Values{}
	params.Set("q", q)
	params.Set("sort", "created")
	params.Set("order", "desc")
	params.Set("per_page", strconv.Itoa(perPage))

	endpoint := g.baseURL + "/search/issues?" + params.Encode()
	body, err := g.get(ctx, endpoint)
	if err != nil {
		return nil, fmt.Errorf("github search: %w", err)
	}

	var parsed searchResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
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
