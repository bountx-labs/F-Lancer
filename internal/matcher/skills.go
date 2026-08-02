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

func (r *SkillsRegistry) GetFeeds(envFeeds []string) []string {
	if len(envFeeds) > 0 {
		return envFeeds
	}
	return r.DefaultFeeds
}