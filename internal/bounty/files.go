package bounty

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// FileRef is a file read from a repository, with everything needed to write a
// revised version back through the contents API.
type FileRef struct {
	Path    string
	Content string
	SHA     string
}

// ReadFile fetches a file's text content and blob SHA from a repository.
func (c *Contributor) ReadFile(ctx context.Context, repo, path, ref string) (*FileRef, error) {
	if ref == "" {
		ref = "main"
	}
	endpoint := fmt.Sprintf("/repos/%s/contents/%s?ref=%s", repo, path, ref)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.base+endpoint, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("Authorization", "Bearer "+c.token)

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("read %s: HTTP %d: %s", path, resp.StatusCode, truncateBody(raw))
	}

	var out struct {
		Path     string `json:"path"`
		SHA      string `json:"sha"`
		Content  string `json:"content"`
		Encoding string `json:"encoding"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, err
	}
	if out.Encoding != "base64" {
		return nil, fmt.Errorf("read %s: unexpected encoding %q", path, out.Encoding)
	}

	decoded, err := base64.StdEncoding.DecodeString(strings.ReplaceAll(out.Content, "\n", ""))
	if err != nil {
		return nil, fmt.Errorf("read %s: decode: %w", path, err)
	}

	return &FileRef{Path: out.Path, Content: string(decoded), SHA: out.SHA}, nil
}

// WriteFile commits a new version of a file to a branch on the given repo.
// The branch must already exist.
func (c *Contributor) WriteFile(ctx context.Context, repo, branch, path, content, message, existingSHA string) (string, error) {
	payload := map[string]any{
		"message": message,
		"content": base64Encode(content),
		"branch":  branch,
	}
	if existingSHA != "" {
		payload["sha"] = existingSHA
	}

	var out struct {
		Commit struct {
			SHA string `json:"sha"`
		} `json:"commit"`
	}
	path2 := fmt.Sprintf("/repos/%s/contents/%s", repo, path)
	if err := c.api(ctx, http.MethodPut, path2, payload, &out); err != nil {
		return "", err
	}
	return out.Commit.SHA, nil
}

// ReadFileAtFork reads a file from the fork so the agent edits what the PR
// will actually contain.
func (c *Contributor) ReadFileAtFork(ctx context.Context, fork, branch, path string) (*FileRef, error) {
	return c.ReadFile(ctx, fork, path, branch)
}
