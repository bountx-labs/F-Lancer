package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

type OpenCodeProvider struct {
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

func NewOpenCode(apiKey, baseURL string) *OpenCodeProvider {
	return &OpenCodeProvider{
		apiKey:  apiKey,
		baseURL: baseURL,
		client:  &http.Client{Timeout: 30 * time.Second},
	}
}

func (o *OpenCodeProvider) Name() string { return "opencode" }

func (o *OpenCodeProvider) Complete(ctx context.Context, model string, prompt string) (string, error) {
	url := o.baseURL + "/v1/chat/completions"

	reqBody := openaiRequest{
		Model: model,
		Messages: []openaiMessage{
			{Role: "user", Content: prompt},
		},
	}

	data, _ := json.Marshal(reqBody)
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(data))
	if err != nil {
		return "", fmt.Errorf("opencode request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+o.apiKey)

	resp, err := o.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("opencode call: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("opencode HTTP %d", resp.StatusCode)
	}

	var result openaiResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("opencode decode: %w", err)
	}

	if len(result.Choices) == 0 {
		return "", fmt.Errorf("opencode empty response")
	}

	return result.Choices[0].Message.Content, nil
}

// Healthy checks the models endpoint without consuming generation quota.
func (o *OpenCodeProvider) Healthy(ctx context.Context) bool {
	req, err := http.NewRequestWithContext(ctx, "GET", o.baseURL+"/v1/models", nil)
	if err != nil {
		return false
	}
	req.Header.Set("Authorization", "Bearer "+o.apiKey)
	resp, err := o.client.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	return resp.StatusCode == http.StatusOK
}