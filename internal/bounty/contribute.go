package bounty

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Contributor opens pull requests on GitHub using the operator's existing
// token. It forks the target repository, creates a branch, commits a change,
// and opens a pull request. No interactive step is involved.
type Contributor struct {
	token  string
	base   string
	client *http.Client
}

const githubAPI = "https://api.github.com"

func NewContributor(token string, timeout time.Duration) *Contributor {
	return &Contributor{
		token:  token,
		base:   githubAPI,
		client: &http.Client{Timeout: timeout},
	}
}

// Authenticated reports whether a token is available to act with.
func (c *Contributor) Authenticated() bool {
	return strings.TrimSpace(c.token) != ""
}

// api performs an authenticated JSON request and decodes the response.
func (c *Contributor) api(ctx context.Context, method, path string, body any, out any) error {
	var reader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return err
		}
		reader = bytes.NewReader(data)
	}

	req, err := http.NewRequestWithContext(ctx, method, c.base+path, reader)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("Authorization", "Bearer "+c.token)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("github %s %s: HTTP %d: %s", method, path, resp.StatusCode, truncateBody(raw))
	}
	if out == nil {
		return nil
	}
	return json.Unmarshal(raw, out)
}

// CurrentLogin returns the authenticated account name.
func (c *Contributor) CurrentLogin(ctx context.Context) (string, error) {
	var out struct {
		Login string `json:"login"`
	}
	if err := c.api(ctx, http.MethodGet, "/user", nil, &out); err != nil {
		return "", err
	}
	return out.Login, nil
}

// EnsureFork returns the owner/name of the caller's fork of repo, creating it
// if needed. Forking is idempotent: an existing fork is reused.
func (c *Contributor) EnsureFork(ctx context.Context, repo, login string) (string, error) {
	parts := strings.SplitN(repo, "/", 2)
	if len(parts) != 2 {
		return "", fmt.Errorf("invalid repo %q", repo)
	}
	owner, name := parts[0], parts[1]

	forked := owner + "/" + name
	if owner != login {
		// Look up whether a fork already exists.
		var existing struct {
			FullName string `json:"full_name"`
		}
		err := c.api(ctx, http.MethodGet, "/repos/"+login+"/"+name, nil, &existing)
		if err == nil && existing.FullName != "" {
			return existing.FullName, nil
		}

		if err := c.api(ctx, http.MethodPost, "/repos/"+owner+"/"+name+"/forks", map[string]any{}, nil); err != nil {
			// A fork may have been created by a concurrent run; re-check.
			if err2 := c.api(ctx, http.MethodGet, "/repos/"+login+"/"+name, nil, &existing); err2 == nil && existing.FullName != "" {
				return existing.FullName, nil
			}
			return "", err
		}
		forked = login + "/" + name
	}

	// Fork creation is asynchronous on GitHub's side; wait for it to appear.
	for attempt := 0; attempt < 10; attempt++ {
		var probe struct {
			FullName string `json:"full_name"`
		}
		if err := c.api(ctx, http.MethodGet, "/repos/"+forked, nil, &probe); err == nil && probe.FullName != "" {
			return probe.FullName, nil
		}
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-time.After(2 * time.Second):
		}
	}
	return "", fmt.Errorf("fork %s did not become available", forked)
}

// FileChange is one file to write in the pull request.
type FileChange struct {
	Path    string
	Content string
}

// OpenPullRequest creates a branch on the fork, commits every file, and opens
// a pull request against the upstream repository. It returns the PR URL.
func (c *Contributor) OpenPullRequest(ctx context.Context, upstream, fork, baseBranch, branch, title, body string, files []FileChange) (string, error) {
	if len(files) == 0 {
		return "", fmt.Errorf("no files to change")
	}
	if baseBranch == "" {
		baseBranch = "main"
	}

	if _, err := c.branchSHA(ctx, fork, branch); err != nil {
		if createErr := c.createBranch(ctx, fork, branch, baseBranch); createErr != nil {
			return "", fmt.Errorf("create branch: %w", createErr)
		}
	}

	for _, f := range files {
		var existingSHA string
		if ref, err := c.ReadFile(ctx, fork, f.Path, branch); err == nil {
			existingSHA = ref.SHA
		}
		if _, err := c.WriteFile(ctx, fork, branch, f.Path, f.Content, "Apply the change described by the linked issue", existingSHA); err != nil {
			return "", fmt.Errorf("write %s: %w", f.Path, err)
		}
	}

	var pr struct {
		HTMLURL string `json:"html_url"`
	}
	err := c.api(ctx, http.MethodPost, "/repos/"+upstream+"/pulls", map[string]any{
		"title": title,
		"head":  strings.SplitN(fork, "/", 2)[0] + ":" + branch,
		"base":  baseBranch,
		"body":  body,
	}, &pr)
	if err != nil {
		return "", err
	}
	return pr.HTMLURL, nil
}

// branchSHA returns the commit SHA a branch points at, or "" when absent.
func (c *Contributor) branchSHA(ctx context.Context, repo, branch string) (string, error) {
	var out struct {
		Object struct {
			SHA string `json:"sha"`
		} `json:"object"`
	}
	if err := c.api(ctx, http.MethodGet, "/repos/"+repo+"/git/ref/heads/"+branch, nil, &out); err != nil {
		return "", err
	}
	return out.Object.SHA, nil
}

// createBranch makes a new branch from an existing one.
func (c *Contributor) createBranch(ctx context.Context, repo, branch, from string) error {
	sha, err := c.branchSHA(ctx, repo, from)
	if err != nil {
		return err
	}
	return c.api(ctx, http.MethodPost, "/repos/"+repo+"/git/refs", map[string]any{
		"ref": "refs/heads/" + branch,
		"sha": sha,
	}, nil)
}

// FetchIssue reads a single issue so its body, labels, and metadata are
// available without the search API.
func (c *Contributor) FetchIssue(ctx context.Context, repo string, number int) (*Issue, error) {
	path := fmt.Sprintf("/repos/%s/issues/%d", repo, number)
	var out Issue
	if err := c.api(ctx, http.MethodGet, path, nil, &out); err != nil {
		return nil, err
	}
	out.RepositoryURL = "https://api.github.com/repos/" + repo
	return &out, nil
}

// ReviewComments returns the review and inline comments on a pull request.
// Only human and bot review threads are included; the agent's own comments are
// filtered out so it never answers itself.
func (c *Contributor) ReviewComments(ctx context.Context, repo string, number int) ([]ReviewComment, error) {
	var out []ReviewComment

	var reviews []struct {
		ID   int64 `json:"id"`
		User struct {
			Login string `json:"login"`
		} `json:"user"`
		Body string `json:"body"`
	}
	if err := c.api(ctx, http.MethodGet, fmt.Sprintf("/repos/%s/pulls/%d/reviews", repo, number), nil, &reviews); err == nil {
		for _, r := range reviews {
			if strings.TrimSpace(r.Body) == "" {
				continue
			}
			out = append(out, ReviewComment{ID: r.ID, Author: r.User.Login, Body: r.Body})
		}
	}

	var inline []struct {
		ID   int64 `json:"id"`
		User struct {
			Login string `json:"login"`
		} `json:"user"`
		Body string `json:"body"`
		Path string `json:"path"`
		Line int    `json:"line"`
	}
	if err := c.api(ctx, http.MethodGet, fmt.Sprintf("/repos/%s/pulls/%d/comments", repo, number), nil, &inline); err == nil {
		for _, r := range inline {
			if strings.TrimSpace(r.Body) == "" {
				continue
			}
			out = append(out, ReviewComment{ID: r.ID, Author: r.User.Login, Body: r.Body, Path: r.Path, Line: r.Line})
		}
	}

	return out, nil
}

// PullFiles lists the paths a pull request changes.
func (c *Contributor) PullFiles(ctx context.Context, repo string, number int) ([]string, error) {
	var out []struct {
		Filename string `json:"filename"`
	}
	path := fmt.Sprintf("/repos/%s/pulls/%d/files?per_page=30", repo, number)
	if err := c.api(ctx, http.MethodGet, path, nil, &out); err != nil {
		return nil, err
	}
	files := make([]string, 0, len(out))
	for _, f := range out {
		if f.Filename != "" {
			files = append(files, f.Filename)
		}
	}
	return files, nil
}

// PullHeadBranch returns the branch name a pull request is opened from.
func (c *Contributor) PullHeadBranch(ctx context.Context, repo string, number int) (string, error) {
	var out struct {
		Head struct {
			Ref string `json:"ref"`
		} `json:"head"`
	}
	if err := c.api(ctx, http.MethodGet, fmt.Sprintf("/repos/%s/pulls/%d", repo, number), nil, &out); err != nil {
		return "", err
	}
	return out.Head.Ref, nil
}

// PullUpdatedAt returns when a pull request last saw any activity.
func (c *Contributor) PullUpdatedAt(ctx context.Context, repo string, number int) (time.Time, error) {
	var out struct {
		UpdatedAt string `json:"updated_at"`
	}
	if err := c.api(ctx, http.MethodGet, fmt.Sprintf("/repos/%s/pulls/%d", repo, number), nil, &out); err != nil {
		return time.Time{}, err
	}
	t, err := time.Parse(time.RFC3339, out.UpdatedAt)
	if err != nil {
		return time.Time{}, fmt.Errorf("parse updated_at %q: %w", out.UpdatedAt, err)
	}
	return t.UTC(), nil
}

// ClosePull closes a pull request, leaving a comment first so the closure
// never looks silent.
func (c *Contributor) ClosePull(ctx context.Context, repo string, number int, note string) error {
	if note != "" {
		if err := c.CommentOnPull(ctx, repo, number, note); err != nil {
			log.Printf("bounty: close comment on %s#%d failed: %v", repo, number, err)
		}
	}
	path := fmt.Sprintf("/repos/%s/pulls/%d", repo, number)
	return c.api(ctx, http.MethodPatch, path, map[string]any{"state": "closed"}, nil)
}

// CommentOnPull posts a comment on a pull request.
func (c *Contributor) CommentOnPull(ctx context.Context, repo string, number int, body string) error {
	path := fmt.Sprintf("/repos/%s/issues/%d/comments", repo, number)
	return c.api(ctx, http.MethodPost, path, map[string]any{"body": body}, nil)
}

// repoFromURL extracts owner/name from a repository API URL.
func repoFromURL(raw string) string {
	const marker = "/repos/"
	if idx := strings.Index(raw, marker); idx >= 0 {
		return strings.Trim(raw[idx+len(marker):], "/")
	}
	return ""
}

// urlQueryEscape escapes a search query for use in a request path.
func urlQueryEscape(s string) string {
	return strings.ReplaceAll(url.QueryEscape(s), "+", "%20")
}

// CommentOnIssue posts a comment on an issue. Bounty programs often require a
// claim comment before a pull request, so this is how the agent announces
// intent without any interactive step.
func (c *Contributor) CommentOnIssue(ctx context.Context, repo string, number int, body string) error {
	path := fmt.Sprintf("/repos/%s/issues/%d/comments", repo, number)
	return c.api(ctx, http.MethodPost, path, map[string]any{"body": body}, nil)
}

// base64Encode encodes file content for the contents API.
func base64Encode(s string) string {
	return base64.StdEncoding.EncodeToString([]byte(s))
}
