package bounty

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
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

	// Resolve the branch's current head on the fork, if the branch exists.
	headSHA, _ := c.branchSHA(ctx, fork, baseBranch)

	for _, f := range files {
		payload := map[string]any{
			"message": "Apply the change described by the linked issue",
			"content": base64Encode(f.Content),
		}
		if headSHA != "" {
			payload["sha"] = headSHA
		}

		path := "/repos/" + fork + "/contents/" + f.Path
		var out struct {
			Content struct {
				SHA string `json:"sha"`
			} `json:"content"`
			Commit struct {
				SHA string `json:"sha"`
			} `json:"commit"`
		}
		err := c.api(ctx, http.MethodPut, path+"?ref="+branch, payload, &out)
		if err != nil && headSHA == "" {
			// The branch does not exist yet; create it from the base branch.
			if createErr := c.createBranch(ctx, fork, branch, baseBranch); createErr != nil {
				return "", fmt.Errorf("create branch: %w", createErr)
			}
			if retryErr := c.api(ctx, http.MethodPut, path+"?ref="+branch, payload, &out); retryErr != nil {
				return "", retryErr
			}
		} else if err != nil {
			return "", err
		}
		headSHA = out.Commit.SHA
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
