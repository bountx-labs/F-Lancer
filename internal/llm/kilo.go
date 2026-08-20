package llm

import (
	"context"
	"time"
)

// KiloProvider implements the Provider interface via the Kilo AI gateway,
// which exposes an OpenAI-compatible API.
type KiloProvider struct {
	*openAICompatClient
}

func NewKilo(apiKey, baseURL string, timeout time.Duration) *KiloProvider {
	return &KiloProvider{newOpenAICompatClient("kilo", apiKey, baseURL, timeout)}
}

func (k *KiloProvider) Name() string { return "kilo" }

func (k *KiloProvider) Complete(ctx context.Context, model, prompt string) (string, error) {
	return k.complete(ctx, model, prompt)
}

func (k *KiloProvider) Healthy(ctx context.Context) bool {
	return k.healthy(ctx)
}
