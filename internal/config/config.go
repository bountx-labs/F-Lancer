package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

const (
	DefaultOpenCodeZenURL  = "https://opencode.ai/zen/v1"
	DefaultKiloGatewayURL  = "https://api.kilo.ai/api/gateway"
	DefaultMaxJobsPerRun   = 5
	DefaultStatePruneDays  = 30
	DefaultStateMaxEntries = 500
	DefaultRSSTimeoutSec   = 10
	DefaultLLMTimeoutSec   = 30
)

type Config struct {
	TelegramBotToken  string
	TelegramChatID    string
	GeminiAPIKey      string
	OpenCodeZenKey    string
	OpenCodeZenURL    string
	KiloGatewayKey    string
	KiloGatewayURL    string
	Mode              string
	DryRun            bool
	MaxJobsPerRun     int
	StatePruneDays    int
	StateMaxEntries   int
	RSSTimeoutSeconds int
	LLMTimeoutSeconds int
}

func Load() (*Config, error) {
	cfg := &Config{
		TelegramBotToken: os.Getenv("TELEGRAM_BOT_TOKEN"),
		TelegramChatID:   os.Getenv("TELEGRAM_CHAT_ID"),
		GeminiAPIKey:     os.Getenv("GEMINI_API_KEY"),
		OpenCodeZenKey:   os.Getenv("OPENCODE_ZEN_API_KEY"),
		OpenCodeZenURL:   envOrDefault("OPENCODE_ZEN_BASE_URL", DefaultOpenCodeZenURL),
		KiloGatewayKey:   envOrDefaultFirst("KILO_GATEWAY_API_KEY", "KILO_API_KEY"),
		KiloGatewayURL:   envOrDefault("KILO_GATEWAY_BASE_URL", DefaultKiloGatewayURL),
		Mode:             os.Getenv("MODE"),
		DryRun:           strings.ToLower(os.Getenv("DRY_RUN")) == "true",
	}

	if cfg.Mode == "" {
		cfg.Mode = "monitor"
	}

	var err error
	if cfg.MaxJobsPerRun, err = envPositiveInt("MAX_JOBS_PER_RUN", DefaultMaxJobsPerRun); err != nil {
		return nil, err
	}
	if cfg.StatePruneDays, err = envPositiveInt("STATE_PRUNE_DAYS", DefaultStatePruneDays); err != nil {
		return nil, err
	}
	if cfg.StateMaxEntries, err = envPositiveInt("STATE_MAX_ENTRIES", DefaultStateMaxEntries); err != nil {
		return nil, err
	}
	if cfg.RSSTimeoutSeconds, err = envPositiveInt("RSS_TIMEOUT_SECONDS", DefaultRSSTimeoutSec); err != nil {
		return nil, err
	}
	if cfg.LLMTimeoutSeconds, err = envPositiveInt("LLM_TIMEOUT_SECONDS", DefaultLLMTimeoutSec); err != nil {
		return nil, err
	}

	if cfg.TelegramBotToken == "" {
		return nil, fmt.Errorf("TELEGRAM_BOT_TOKEN is required")
	}
	if cfg.TelegramChatID == "" {
		return nil, fmt.Errorf("TELEGRAM_CHAT_ID is required")
	}

	return cfg, nil
}

func envPositiveInt(key string, def int) (int, error) {
	v := os.Getenv(key)
	if v == "" {
		return def, nil
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return 0, fmt.Errorf("%s must be an integer, got %q", key, v)
	}
	if n < 1 {
		return 0, fmt.Errorf("%s must be >= 1, got %d", key, n)
	}
	return n, nil
}

func envOrDefault(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func envOrDefaultFirst(keys ...string) string {
	for _, key := range keys {
		if v := os.Getenv(key); v != "" {
			return v
		}
	}
	return ""
}

func (c *Config) HasGemini() bool {
	return c.GeminiAPIKey != ""
}

func (c *Config) HasOpenCodeZen() bool {
	return c.OpenCodeZenKey != "" && c.OpenCodeZenURL != ""
}

func (c *Config) HasKiloGateway() bool {
	return c.KiloGatewayKey != "" && c.KiloGatewayURL != ""
}

func (c *Config) AvailableProviders() []string {
	var providers []string
	if c.HasGemini() {
		providers = append(providers, "gemini")
	}
	if c.HasOpenCodeZen() {
		providers = append(providers, "opencode")
	}
	if c.HasKiloGateway() {
		providers = append(providers, "kilo")
	}
	return providers
}
