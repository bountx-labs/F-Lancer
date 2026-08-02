package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
)

type ModelsConfig struct {
	Providers     map[string]ProviderConfig `json:"providers"`
	FallbackOrder []string                  `json:"fallback_order"`
}

type ProviderConfig struct {
	Models map[string]string `json:"models"`
}

type Pool struct {
	providers map[string]Provider
	models    *ModelsConfig
}

func NewPool(cfg *ModelsConfig, geminiKey, opencodeKey, opencodeURL, kiloKey, kiloURL string) *Pool {
	p := &Pool{
		providers: make(map[string]Provider),
		models:    cfg,
	}

	if geminiKey != "" {
		p.providers["gemini"] = NewGemini(geminiKey)
	}
	if opencodeKey != "" && opencodeURL != "" {
		p.providers["opencode"] = NewOpenCode(opencodeKey, opencodeURL)
	}
	if kiloKey != "" && kiloURL != "" {
		p.providers["kilo"] = NewKilo(kiloKey, kiloURL)
	}

	return p
}

func LoadModelsConfig(path string) (*ModelsConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read models config: %w", err)
	}

	var cfg ModelsConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse models config: %w", err)
	}

	return &cfg, nil
}

func (p *Pool) Complete(ctx context.Context, taskProfile string, prompt string) (string, error) {
	for _, name := range p.models.FallbackOrder {
		provider, ok := p.providers[name]
		if !ok {
			continue
		}

		model := p.pickModel(name, taskProfile)
		if model == "" {
			continue
		}

		result, err := provider.Complete(ctx, model, prompt)
		if err != nil {
			continue
		}

		return result, nil
	}

	return "", fmt.Errorf("all LLM providers failed")
}

func (p *Pool) IsHealthy() bool {
	for _, name := range p.models.FallbackOrder {
		provider, ok := p.providers[name]
		if !ok {
			continue
		}
		if provider.Healthy(context.Background()) {
			return true
		}
	}
	return false
}

func (p *Pool) pickModel(providerName, taskProfile string) string {
	provCfg, ok := p.models.Providers[providerName]
	if !ok {
		return ""
	}

	if model, ok := provCfg.Models[taskProfile]; ok {
		return model
	}

	if model, ok := provCfg.Models["default"]; ok {
		return model
	}

	return ""
}