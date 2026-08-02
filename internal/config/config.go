package config

import (
	"fmt"
	"os"
	"strings"
)

const (
	DefaultOpenCodeZenURL = "https://opencode.ai/zen/v1"
	DefaultKiloGatewayURL = "https://api.kilo.ai/api/gateway"
)

type Config struct {
	TelegramBotToken string
	TelegramChatID   string
	GeminiAPIKey     string
	OpenCodeZenKey   string
	OpenCodeZenURL   string
	KiloGatewayKey   string
	KiloGatewayURL   string
	Mode             string
	DryRun           bool
}

func Load() (*Config, error) {
	cfg := &Config{
		TelegramBotToken: os.Getenv("TELEGRAM_BOT_TOKEN"),
		TelegramChatID:   os.Getenv("TELEGRAM_CHAT_ID"),
		GeminiAPIKey:     os.Getenv("GEMINI_API_KEY"),
		OpenCodeZenKey:   os.Getenv("OPENCODE_ZEN_API_KEY"),
		OpenCodeZenURL:   envOrDefault("OPENCODE_ZEN_BASE_URL", DefaultOpenCodeZenURL),
		KiloGatewayKey:   os.Getenv("KILO_GATEWAY_API_KEY"),
		KiloGatewayURL:   envOrDefault("KILO_GATEWAY_BASE_URL", DefaultKiloGatewayURL),
		Mode:             os.Getenv("MODE"),
		DryRun:           strings.ToLower(os.Getenv("DRY_RUN")) == "true",
	}

	if cfg.Mode == "" {
		cfg.Mode = "monitor"
	}

	if cfg.TelegramBotToken == "" {
		return nil, fmt.Errorf("TELEGRAM_BOT_TOKEN is required")
	}
	if cfg.TelegramChatID == "" {
		return nil, fmt.Errorf("TELEGRAM_CHAT_ID is required")
	}

	return cfg, nil
}

func envOrDefault(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
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