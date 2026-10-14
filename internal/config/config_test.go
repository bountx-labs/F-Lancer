package config

import "testing"

func setRequiredEnv(t *testing.T) {
	t.Helper()
	t.Setenv("TELEGRAM_BOT_TOKEN", "tok")
	t.Setenv("TELEGRAM_CHAT_ID", "chat")
}

func TestLoadDefaults(t *testing.T) {
	setRequiredEnv(t)
	cfg, err := Load()
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if cfg.Mode != "monitor" {
		t.Errorf("expected default mode monitor, got %q", cfg.Mode)
	}
	if cfg.MaxJobsPerRun != DefaultMaxJobsPerRun {
		t.Errorf("expected default max jobs %d, got %d", DefaultMaxJobsPerRun, cfg.MaxJobsPerRun)
	}
	if cfg.KiloGatewayURL != DefaultKiloGatewayURL {
		t.Errorf("expected default kilo url, got %q", cfg.KiloGatewayURL)
	}
}

func TestLoadRequiresTelegramToken(t *testing.T) {
	t.Setenv("TELEGRAM_BOT_TOKEN", "")
	t.Setenv("TELEGRAM_CHAT_ID", "chat")
	if _, err := Load(); err == nil {
		t.Fatal("expected error when token missing")
	}
}

func TestLoadRequiresChatID(t *testing.T) {
	t.Setenv("TELEGRAM_BOT_TOKEN", "tok")
	t.Setenv("TELEGRAM_CHAT_ID", "")
	if _, err := Load(); err == nil {
		t.Fatal("expected error when chat id missing")
	}
}

func TestLoadRejectsInvalidInt(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("MAX_JOBS_PER_RUN", "not-a-number")
	if _, err := Load(); err == nil {
		t.Fatal("expected error for invalid integer")
	}
}

func TestLoadRejectsZeroInt(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("MAX_JOBS_PER_RUN", "0")
	if _, err := Load(); err == nil {
		t.Fatal("expected error for zero value")
	}
}

func TestKiloKeyFallback(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("KILO_GATEWAY_API_KEY", "")
	t.Setenv("KILO_API_KEY", "fallback-key")
	cfg, err := Load()
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if cfg.KiloGatewayKey != "fallback-key" {
		t.Errorf("expected fallback key, got %q", cfg.KiloGatewayKey)
	}
}

func TestAvailableProviders(t *testing.T) {
	cfg := &Config{GeminiAPIKey: "g", KiloGatewayKey: "k", KiloGatewayURL: "u"}
	got := cfg.AvailableProviders()
	if len(got) != 2 || got[0] != "gemini" || got[1] != "kilo" {
		t.Errorf("unexpected providers: %v", got)
	}

	onlyGemini := &Config{GeminiAPIKey: "g"}
	if p := onlyGemini.AvailableProviders(); len(p) != 1 || p[0] != "gemini" {
		t.Errorf("unexpected providers: %v", p)
	}
}

func TestHasKiloGatewayRequiresBothKeyAndURL(t *testing.T) {
	if (&Config{KiloGatewayKey: "k"}).HasKiloGateway() {
		t.Error("should be false without url")
	}
	if (&Config{KiloGatewayURL: "u"}).HasKiloGateway() {
		t.Error("should be false without key")
	}
	if !(&Config{KiloGatewayKey: "k", KiloGatewayURL: "u"}).HasKiloGateway() {
		t.Error("should be true with key and url")
	}
}
