package bounty

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// Client is a minimal client for the MoltJobs agent API. MoltJobs provisions
// a Turnkey-managed USDC wallet for each registered agent, so this client never
// handles raw private keys: it authenticates with an API key, discovers jobs,
// bids, submits work, and withdraws earnings to an external address.
type Client struct {
	apiKey  string
	baseURL string
	client  *http.Client
}

const defaultBaseURL = "https://api.moltjobs.io"

func NewClient(apiKey string, timeout time.Duration) *Client {
	return &Client{
		apiKey:  apiKey,
		baseURL: defaultBaseURL,
		client:  &http.Client{Timeout: timeout},
	}
}

// Job is the subset of the MoltJobs job payload the agent needs to evaluate
// and bid. Fields are kept as-is from the API.
type Job struct {
	ID                 string   `json:"id"`
	Title              string   `json:"title"`
	Description        string   `json:"description"`
	Vertical           string   `json:"vertical"`
	AcceptanceCriteria string   `json:"acceptanceCriteria"`
	ProposedUsdc       string   `json:"proposedUsdc"`
	Status             string   `json:"status"`
	Tags               []string `json:"tags"`
}

// Wallet is the agent's USDC wallet as reported by MoltJobs.
type Wallet struct {
	Address     string `json:"address"`
	UsdcBalance string `json:"usdcBalance"`
}

// Bid is the result of placing a bid.
type Bid struct {
	ID           string `json:"id"`
	ProposedUsdc string `json:"proposedUsdc"`
}

type apiError struct {
	Status  string `json:"status"`
	Message string `json:"message"`
	Code    string `json:"code"`
}

// do performs an authenticated request and decodes a successful JSON body into
// out. Non-2xx responses are surfaced with the API message when present.
func (c *Client) do(ctx context.Context, method, path string, body any, out any) error {
	var reader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("moltjobs encode request: %w", err)
		}
		reader = strings.NewReader(string(data))
	}

	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, reader)
	if err != nil {
		return fmt.Errorf("moltjobs build request: %w", err)
	}
	req.Header.Set("x-api-key", c.apiKey)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return fmt.Errorf("moltjobs call: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		var e apiError
		_ = json.NewDecoder(resp.Body).Decode(&e)
		msg := e.Message
		if msg == "" {
			msg = e.Code
		}
		if msg == "" {
			msg = resp.Status
		}
		return fmt.Errorf("moltjobs HTTP %d: %s", resp.StatusCode, msg)
	}

	if out == nil {
		return nil
	}
	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return fmt.Errorf("moltjobs decode: %w", err)
	}
	return nil
}

// DiscoverJobs returns open jobs for a vertical, newest first.
func (c *Client) DiscoverJobs(ctx context.Context, vertical string, limit int) ([]Job, error) {
	if limit <= 0 {
		limit = 20
	}
	path := fmt.Sprintf("/v1/jobs?status=OPEN&limit=%d&sort=NEWEST", limit)
	if vertical != "" {
		path += "&vertical=" + vertical
	}

	var out struct {
		Data []Job `json:"data"`
	}
	if err := c.do(ctx, http.MethodGet, path, nil, &out); err != nil {
		return nil, err
	}
	return out.Data, nil
}

// Agent is one registered agent as reported by the MoltJobs agent list.
type Agent struct {
	ID     string `json:"id"`
	Handle string `json:"handle"`
	Name   string `json:"name"`
	Status string `json:"status"`
}

// ResolveAgentID finds the agent id for a handle so setup only needs the API
// key. An empty handle returns the first agent, because a solo operator runs a
// single agent and should not have to hunt for an internal id.
func (c *Client) ResolveAgentID(ctx context.Context, handle string) (string, error) {
	agents, err := c.ListAgents(ctx)
	if err != nil {
		return "", err
	}
	if len(agents) == 0 {
		return "", fmt.Errorf("moltjobs: no agents found for this API key")
	}

	want := strings.TrimPrefix(strings.TrimSpace(handle), "@")
	for _, a := range agents {
		if want != "" && (a.ID == want || strings.TrimPrefix(a.Handle, "@") == want) {
			return a.ID, nil
		}
	}
	if want == "" {
		return agents[0].ID, nil
	}
	return "", fmt.Errorf("moltjobs: no agent matched handle %q", handle)
}

// ListAgents returns every agent the API key can access.
func (c *Client) ListAgents(ctx context.Context) ([]Agent, error) {
	var out struct {
		Data []Agent `json:"data"`
	}
	if err := c.do(ctx, http.MethodGet, "/v1/agents", nil, &out); err != nil {
		return nil, err
	}
	return out.Data, nil
}

// PlaceBid bids on a job. proposedUsdc must be a string per the API contract.
func (c *Client) PlaceBid(ctx context.Context, jobID, proposedUsdc, message string) (*Bid, error) {
	var out Bid
	err := c.do(ctx, http.MethodPost, "/v1/jobs/"+jobID+"/bid", map[string]any{
		"proposedUsdc": proposedUsdc,
		"message":      message,
	}, &out)
	if err != nil {
		return nil, err
	}
	return &out, nil
}

// Heartbeat marks the agent online for a job within the 30-minute window.
func (c *Client) Heartbeat(ctx context.Context, agentID, jobID, progress string) error {
	return c.do(ctx, http.MethodPost, "/v1/agents/"+agentID+"/heartbeat", map[string]any{
		"jobId":    jobID,
		"progress": progress,
	}, nil)
}

// SubmitWork delivers a completed artifact for a job.
func (c *Client) SubmitWork(ctx context.Context, jobID, output, notes string) error {
	return c.do(ctx, http.MethodPost, "/v1/jobs/"+jobID+"/submit", map[string]any{
		"output":       output,
		"notes":        notes,
		"outputFormat": "text",
	}, nil)
}

// Wallet returns the agent's USDC wallet address and balance.
func (c *Client) Wallet(ctx context.Context, agentID string) (*Wallet, error) {
	var out Wallet
	if err := c.do(ctx, http.MethodGet, "/v1/agents/"+agentID+"/wallet", nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// Withdraw sends USDC to an external Base-compatible address. An empty
// destination is rejected before the request so a misconfiguration can never
// burn funds to a zero address.
func (c *Client) Withdraw(ctx context.Context, agentID, toAddress, amountUsdc string) error {
	if strings.TrimSpace(toAddress) == "" {
		return fmt.Errorf("moltjobs withdraw: destination address is empty")
	}
	return c.do(ctx, http.MethodPost, "/v1/agents/"+agentID+"/wallet/withdraw", map[string]any{
		"toAddress": toAddress,
		"amount":    amountUsdc,
	}, nil)
}

// AssignedJobs returns jobs currently assigned to the agent.
func (c *Client) AssignedJobs(ctx context.Context, agentID string) ([]Job, error) {
	path := "/v1/jobs?status=ASSIGNED&agentId=" + agentID
	var out struct {
		Data []Job `json:"data"`
	}
	if err := c.do(ctx, http.MethodGet, path, nil, &out); err != nil {
		return nil, err
	}
	return out.Data, nil
}
