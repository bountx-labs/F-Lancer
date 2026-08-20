package llm

import (
	"context"
	"time"
)

// OpenCodeProvider implements the Provider interface via the OpenCode Zen
// OpenAI-compatible gateway.
type OpenCodeProvider struct {
	*openAICompatClient
}

func NewOpenCode(apiKey, baseURL string, timeout time.Duration) *OpenCodeProvider {
	return &OpenCodeProvider{newOpenAICompatClient("opencode", apiKey, baseURL, timeout)}
}

func (o *OpenCodeProvider) Name() string { return "opencode" }

func (o *OpenCodeProvider) Complete(ctx context.Context, model, prompt string) (string, error) {
	return o.complete(ctx, model, prompt)
}

func (o *OpenCodeProvider) Healthy(ctx context.Context) bool {
	return o.healthy(ctx)
}
