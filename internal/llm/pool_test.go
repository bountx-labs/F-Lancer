package llm

import "testing"

func testModelsConfig() *ModelsConfig {
	return &ModelsConfig{
		Providers: map[string]ProviderConfig{
			"gemini": {Models: map[string]string{"default": "m1,m2", "proposal": "m3"}},
			"kilo":   {Models: map[string]string{"default": "kilo-auto/free"}},
		},
		FallbackOrder: []string{"gemini", "kilo"},
	}
}

func TestValidateAcceptsGoodConfig(t *testing.T) {
	if err := testModelsConfig().Validate(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidateRejectsEmptyFallbackOrder(t *testing.T) {
	cfg := testModelsConfig()
	cfg.FallbackOrder = nil
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected error for empty fallback order")
	}
}

func TestValidateRejectsProviderWithoutConfig(t *testing.T) {
	cfg := testModelsConfig()
	cfg.FallbackOrder = []string{"gemini", "missing"}
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected error for provider missing config block")
	}
}

func TestValidateRejectsProviderWithoutDefault(t *testing.T) {
	cfg := &ModelsConfig{
		Providers:     map[string]ProviderConfig{"gemini": {Models: map[string]string{"proposal": "m"}}},
		FallbackOrder: []string{"gemini"},
	}
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected error for provider missing models.default")
	}
}

func TestModelsForSplitsCommaList(t *testing.T) {
	p := &Pool{models: testModelsConfig()}
	got := p.modelsFor("gemini", "default")
	if len(got) != 2 || got[0] != "m1" || got[1] != "m2" {
		t.Errorf("unexpected models: %v", got)
	}
}

func TestModelsForFallsBackToDefaultProfile(t *testing.T) {
	p := &Pool{models: testModelsConfig()}
	got := p.modelsFor("gemini", "unknown-task")
	if len(got) != 2 || got[0] != "m1" {
		t.Errorf("expected default chain, got %v", got)
	}
}

func TestModelsForTaskProfileOverride(t *testing.T) {
	p := &Pool{models: testModelsConfig()}
	got := p.modelsFor("gemini", "proposal")
	if len(got) != 1 || got[0] != "m3" {
		t.Errorf("expected proposal override, got %v", got)
	}
}

func TestModelsForUnknownProvider(t *testing.T) {
	p := &Pool{models: testModelsConfig()}
	if got := p.modelsFor("nope", "default"); got != nil {
		t.Errorf("expected nil for unknown provider, got %v", got)
	}
}

func TestModelsForTrimsWhitespace(t *testing.T) {
	cfg := &ModelsConfig{
		Providers:     map[string]ProviderConfig{"x": {Models: map[string]string{"default": " a , b ,c "}}},
		FallbackOrder: []string{"x"},
	}
	p := &Pool{models: cfg}
	got := p.modelsFor("x", "default")
	if len(got) != 3 || got[0] != "a" || got[2] != "c" {
		t.Errorf("whitespace not trimmed: %v", got)
	}
}

func TestIsHealthyWithNoProviders(t *testing.T) {
	p := &Pool{providers: map[string]Provider{}, models: testModelsConfig()}
	if p.IsHealthy() {
		t.Error("expected unhealthy with no providers configured")
	}
}
