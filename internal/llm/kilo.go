package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

type KiloProvider struct {
	apiKey  string
	baseURL string
	client  *http.Client
}

func NewKilo(apiKey, baseURL string, timeout time.Duration) *KiloProvider {
	return &KiloProvider{
		apiKey:  apiKey,
		baseURL: baseURL,
		client:  &http.Client{Timeout: timeout},
	}
}

func (k *KiloProvider) Name() string { return "kilo" }

func (k *KiloProvider) Complete(ctx context.Context, model string, prompt string) (string, error) {
	url := k.baseURL + "/v1/chat/completions"

	reqBody := openaiRequest{
		Model: model,
		Messages: []openaiMessage{
			{Role: "user", Content: prompt},
		},
	}

	data, _ := json.Marshal(reqBody)
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(data))
	if err != nil {
		return "", fmt.Errorf("kilo request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+k.apiKey)

	resp, err := k.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("kilo call: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("kilo HTTP %d", resp.StatusCode)
	}

	var result openaiResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("kilo decode: %w", err)
	}

	if len(result.Choices) == 0 {
		return "", fmt.Errorf("kilo empty response")
	}

	return result.Choices[0].Message.Content, nil
}

// Healthy checks the models endpoint without consuming generation quota.
// If the gateway does not expose /v1/models it reports unhealthy, and the
// pool simply falls through to Complete(), which still attempts the provider.
func (k *KiloProvider) Healthy(ctx context.Context) bool {
	req, err := http.NewRequestWithContext(ctx, "GET", k.baseURL+"/v1/models", nil)
	if err != nil {
		return false
	}
	req.Header.Set("Authorization", "Bearer "+k.apiKey)
	resp, err := k.client.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	return resp.StatusCode == http.StatusOK
}