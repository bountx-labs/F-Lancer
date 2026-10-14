package matcher

import "testing"

func testRegistry() *SkillsRegistry {
	return &SkillsRegistry{
		Skills: []Skill{
			{ID: "python-dev", Name: "Python Development", Keywords: []string{"python", "django", "flask"}, Priority: 9},
			{ID: "seo-marketing", Name: "SEO", Keywords: []string{"seo", "backlink"}, Priority: 6},
		},
		DefaultFeeds: []string{"https://example.com/rss.xml"},
	}
}

func TestMatchFindsKeywords(t *testing.T) {
	reg := testRegistry()
	matches := reg.Match("Need a Python Flask API", "Build a small backend service")
	if len(matches) != 1 {
		t.Fatalf("expected 1 match, got %d", len(matches))
	}
	if matches[0].Skill.ID != "python-dev" {
		t.Errorf("expected python-dev, got %s", matches[0].Skill.ID)
	}
	if len(matches[0].Keywords) != 2 {
		t.Errorf("expected 2 keyword hits, got %d", len(matches[0].Keywords))
	}
}

func TestMatchIsCaseInsensitive(t *testing.T) {
	reg := testRegistry()
	matches := reg.Match("PYTHON DJANGO developer", "")
	if len(matches) != 1 {
		t.Fatalf("expected 1 match, got %d", len(matches))
	}
}

func TestMatchNoHitsReturnsEmpty(t *testing.T) {
	reg := testRegistry()
	if matches := reg.Match("Logo design needed", "Minimalist vector logo"); len(matches) != 0 {
		t.Fatalf("expected 0 matches, got %d", len(matches))
	}
}

func TestMatchOrdersByPriority(t *testing.T) {
	reg := &SkillsRegistry{
		Skills: []Skill{
			{ID: "low", Name: "Low", Keywords: []string{"word"}, Priority: 2},
			{ID: "high", Name: "High", Keywords: []string{"word"}, Priority: 9},
		},
	}
	matches := reg.Match("word", "")
	if len(matches) != 2 {
		t.Fatalf("expected 2 matches, got %d", len(matches))
	}
	if matches[0].Skill.ID != "high" {
		t.Errorf("expected high priority first, got %s", matches[0].Skill.ID)
	}
}

func TestValidateRejectsEmptyRegistry(t *testing.T) {
	reg := &SkillsRegistry{}
	if err := reg.Validate(); err == nil {
		t.Fatal("expected error for empty registry")
	}
}

func TestValidateRejectsDuplicateID(t *testing.T) {
	reg := &SkillsRegistry{
		Skills: []Skill{
			{ID: "dup", Name: "A", Keywords: []string{"a"}, Priority: 5},
			{ID: "dup", Name: "B", Keywords: []string{"b"}, Priority: 5},
		},
	}
	if err := reg.Validate(); err == nil {
		t.Fatal("expected error for duplicate skill id")
	}
}

func TestValidateRejectsMissingKeywords(t *testing.T) {
	reg := &SkillsRegistry{
		Skills: []Skill{{ID: "x", Name: "X", Priority: 5}},
	}
	if err := reg.Validate(); err == nil {
		t.Fatal("expected error for skill without keywords")
	}
}

func TestValidateRejectsPriorityOutOfRange(t *testing.T) {
	reg := &SkillsRegistry{
		Skills: []Skill{{ID: "x", Name: "X", Keywords: []string{"a"}, Priority: 11}},
	}
	if err := reg.Validate(); err == nil {
		t.Fatal("expected error for out-of-range priority")
	}
}

func TestValidateAcceptsGoodRegistry(t *testing.T) {
	if err := testRegistry().Validate(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestMatchScore(t *testing.T) {
	reg := testRegistry()
	matches := reg.Match("python django flask", "")
	if len(matches) != 1 {
		t.Fatalf("expected 1 match, got %d", len(matches))
	}
	if matches[0].Score != 1.0 {
		t.Errorf("expected score 1.0, got %f", matches[0].Score)
	}
}
