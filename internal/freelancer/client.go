package freelancer

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// Client is a minimal client for the official Freelancer.com API.
// It only implements bid placement, which is all the submit mode needs.
type Client struct {
	oauthToken string
	client     *http.Client
}

// BidRequest is the payload for placing a bid on a project.
type BidRequest struct {
	ProjectID           int64   `json:"project_id"`
	Amount              float64 `json:"amount"`
	Period              int     `json:"period"`
	MilestonePercentage int     `json:"milestone_percentage"`
	Description         string  `json:"description"`
}

type bidResponse struct {
	Status  string `json:"status"`
	Message string `json:"message"`
	Result  struct {
		ID int64 `json:"id"`
	} `json:"result"`
}

// NewClient returns a client that authenticates with a pre-obtained OAuth2
// bearer token. Tokens are one-time user input; the agent never sees the
// secret value itself, only the environment variable name.
func NewClient(oauthToken string, timeout time.Duration) *Client {
	return &Client{
		oauthToken: oauthToken,
		client:     &http.Client{Timeout: timeout},
	}
}

// PlaceBid submits one bid via POST /projects/0.1/bids/ and returns the
// created bid ID.
func (c *Client) PlaceBid(ctx context.Context, req BidRequest) (int64, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return 0, fmt.Errorf("freelancer encode request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost,
		"https://www.freelancer.com/api/projects/0.1/bids/", bytes.NewReader(body))
	if err != nil {
		return 0, fmt.Errorf("freelancer build request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+c.oauthToken)

	resp, err := c.client.Do(httpReq)
	if err != nil {
		return 0, fmt.Errorf("freelancer call: %w", err)
	}
	defer resp.Body.Close()

	var parsed bidResponse
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return 0, fmt.Errorf("freelancer decode (HTTP %d): %w", resp.StatusCode, err)
	}

	if resp.StatusCode != http.StatusOK || parsed.Status != "success" {
		msg := parsed.Message
		if msg == "" {
			msg = parsed.Status
		}
		return 0, fmt.Errorf("freelancer HTTP %d: %s", resp.StatusCode, msg)
	}

	return parsed.Result.ID, nil
}
