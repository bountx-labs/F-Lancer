package matcher

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

type Skill struct {
	ID             string   `json:"id"`
	Name           string   `json:"name"`
	Keywords       []string `json:"keywords"`
	SkillsPackages []string `json:"skills_packages"`
	Priority       int      `json:"priority"`
}

type SkillsRegistry struct {
	Skills       []Skill  `json:"skills"`
	DefaultFeeds []string `json:"default_feeds"`
}

type MatchResult struct {
	Skill    Skill
	Score    float64
	Keywords []string
}

func LoadRegistry(path string) (*SkillsRegistry, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read skills registry: %w", err)
	}

	var reg SkillsRegistry
	if err := json.Unmarshal(data, &reg); err != nil {
		return nil, fmt.Errorf("parse skills registry: %w", err)
	}

	return &reg, nil
}

// Validate verifies the registry is well-formed and usable for matching.
func (r *SkillsRegistry) Validate() error {
	if len(r.Skills) == 0 {
		return fmt.Errorf("skills registry: no skills defined")
	}
	seen := make(map[string]bool)
	for _, skill := range r.Skills {
		if skill.ID == "" || skill.Name == "" {
			return fmt.Errorf("skills registry: every skill must have id and name")
		}
		if seen[skill.ID] {
			return fmt.Errorf("skills registry: duplicate skill id %q", skill.ID)
		}
		seen[skill.ID] = true
		if len(skill.Keywords) == 0 {
			return fmt.Errorf("skills registry: skill %q has no keywords", skill.ID)
		}
		if skill.Priority < 1 || skill.Priority > 10 {
			return fmt.Errorf("skills registry: skill %q priority %d out of range 1-10", skill.ID, skill.Priority)
		}
	}
	return nil
}

func (r *SkillsRegistry) Match(title, description string) []MatchResult {
	combined := strings.ToLower(title + " " + description)
	var matches []MatchResult

	for _, skill := range r.Skills {
		var hits []string
		hitCount := 0

		for _, kw := range skill.Keywords {
			if strings.Contains(combined, strings.ToLower(kw)) {
				hits = append(hits, kw)
				hitCount++
			}
		}

		if hitCount > 0 {
			score := float64(hitCount) / float64(len(skill.Keywords))
			matches = append(matches, MatchResult{
				Skill:    skill,
				Score:    score,
				Keywords: hits,
			})
		}
	}

	return matches
}

func (r *SkillsRegistry) GetFeeds() []string {
	return r.DefaultFeeds
}