package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// openAICompatClient implements the OpenAI-compatible chat completions API
// used by the Kilo provider.
type openAICompatClient struct {
	name    string
	apiKey  string
	baseURL string
	client  *http.Client
}

type openaiRequest struct {
	Model    string          `json:"model"`
	Messages []openaiMessage `json:"messages"`
}

type openaiMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type openaiResponse struct {
	Choices []struct {
		Message openaiMessage `json:"message"`
	} `json:"choices"`
}

func newOpenAICompatClient(name, apiKey, baseURL string, timeout time.Duration) *openAICompatClient {
	return &openAICompatClient{
		name:    name,
		apiKey:  apiKey,
		baseURL: baseURL,
		client:  &http.Client{Timeout: timeout},
	}
}

func (c *openAICompatClient) complete(ctx context.Context, model, prompt string) (string, error) {
	reqBody := openaiRequest{
		Model:    model,
		Messages: []openaiMessage{{Role: "user", Content: prompt}},
	}
	data, _ := json.Marshal(reqBody)

	req, err := http.NewRequestWithContext(ctx, "POST", c.baseURL+"/v1/chat/completions", bytes.NewReader(data))
	if err != nil {
		return "", fmt.Errorf("%s request: %w", c.name, err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.apiKey)

	resp, err := c.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("%s call: %w", c.name, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("%s HTTP %d", c.name, resp.StatusCode)
	}

	var result openaiResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("%s decode: %w", c.name, err)
	}

	if len(result.Choices) == 0 {
		return "", fmt.Errorf("%s empty response", c.name)
	}

	return result.Choices[0].Message.Content, nil
}

// healthy probes /v1/models without consuming generation quota.
// Returns false if the endpoint is unavailable; the pool then falls through
// to Complete(), which will still attempt the provider.
func (c *openAICompatClient) healthy(ctx context.Context) bool {
	req, err := http.NewRequestWithContext(ctx, "GET", c.baseURL+"/v1/models", nil)
	if err != nil {
		return false
	}
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	resp, err := c.client.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	return resp.StatusCode == http.StatusOK
}
