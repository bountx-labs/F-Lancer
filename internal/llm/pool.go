package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strings"
	"time"
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

func NewPool(cfg *ModelsConfig, geminiKey, kiloKey, kiloURL string, timeout time.Duration) *Pool {
	p := &Pool{
		providers: make(map[string]Provider),
		models:    cfg,
	}

	if geminiKey != "" {
		p.providers["gemini"] = NewGemini(geminiKey, timeout)
	}
	if kiloKey != "" && kiloURL != "" {
		p.providers["kilo"] = NewKilo(kiloKey, kiloURL, timeout)
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

// Validate verifies that every provider in fallback_order has a config block
// and a models.default entry so pool.Complete never silently skips providers.
func (c *ModelsConfig) Validate() error {
	if len(c.FallbackOrder) == 0 {
		return fmt.Errorf("models config: fallback_order must not be empty")
	}
	for _, name := range c.FallbackOrder {
		prov, ok := c.Providers[name]
		if !ok {
			return fmt.Errorf("models config: provider %q in fallback_order has no config", name)
		}
		if len(prov.Models) == 0 {
			return fmt.Errorf("models config: provider %q has no models configured", name)
		}
		if _, ok := prov.Models["default"]; !ok {
			return fmt.Errorf("models config: provider %q is missing the required models.default entry", name)
		}
	}
	return nil
}

func (p *Pool) Complete(ctx context.Context, taskProfile string, prompt string) (string, error) {
	for _, name := range p.models.FallbackOrder {
		provider, ok := p.providers[name]
		if !ok {
			continue
		}

		for _, model := range p.modelsFor(name, taskProfile) {
			result, err := provider.Complete(ctx, model, prompt)
			if err == nil {
				return result, nil
			}
			log.Printf("llm provider %s (model %s) failed: %v", name, model, err)
			time.Sleep(time.Second)
		}
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


// modelsFor returns the ordered model chain for a provider and task profile.
// Models may be listed comma-separated; they are tried in order before the
// pool falls back to the next provider.
func (p *Pool) modelsFor(providerName, taskProfile string) []string {
	provCfg, ok := p.models.Providers[providerName]
	if !ok {
		return nil
	}

	model, ok := provCfg.Models[taskProfile]
	if !ok {
		model = provCfg.Models["default"]
	}
	if model == "" {
		return nil
	}

	var models []string
	for _, m := range strings.Split(model, ",") {
		if m = strings.TrimSpace(m); m != "" {
			models = append(models, m)
		}
	}
	return models
}
