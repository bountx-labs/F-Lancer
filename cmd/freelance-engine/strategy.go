package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"text/template"
	"time"

	"github.com/bountx-labs/autonomous-freelance-engine/internal/config"
	"github.com/bountx-labs/autonomous-freelance-engine/internal/llm"
	"github.com/bountx-labs/autonomous-freelance-engine/internal/matcher"
	"github.com/bountx-labs/autonomous-freelance-engine/internal/notify"
	"github.com/bountx-labs/autonomous-freelance-engine/internal/scraper"
)

// runSetup renders prompts/setup-gigs.tmpl through the LLM and writes the
// generated gig/profile copy to profiles/gig-profiles.md for the workflow to
// commit. It runs instead of the monitor pipeline when MODE=setup.
func runSetup(pool *llm.Pool, skillsReg *matcher.SkillsRegistry, baseDir string, tg *notify.Telegram) {
	profilesDir := filepath.Join(baseDir, "profiles")
	if err := os.MkdirAll(profilesDir, 0755); err != nil {
		log.Fatalf("create profiles dir: %v", err)
	}

	tmpl, err := template.ParseFiles(filepath.Join(baseDir, "prompts", "setup-gigs.tmpl"))
	if err != nil {
		log.Fatalf("load setup template: %v", err)
	}

	var buf strings.Builder
	if err := tmpl.Execute(&buf, struct{ Skills []matcher.Skill }{Skills: skillsReg.Skills}); err != nil {
		log.Fatalf("render setup template: %v", err)
	}

	result, err := pool.Complete(context.Background(), "setup", buf.String())
	if err != nil {
		log.Fatalf("generate profiles: %v", err)
	}

	path := filepath.Join(profilesDir, "gig-profiles.md")
	if err := os.WriteFile(path, []byte(result), 0644); err != nil {
		log.Fatalf("write profiles: %v", err)
	}

	log.Printf("setup complete: wrote %s", path)
	if err := tg.SendAlert("Setup mode complete - gig profiles generated and committed to profiles/."); err != nil {
		log.Printf("telegram setup alert failed: %v", err)
	}
}

// runStrategy scans live feeds, measures how many jobs the current registry
// captures, asks the LLM for conservative keyword additions, applies them to
// skills-registry.json, regenerates gig profiles, and writes a strategy
// report. It runs instead of the monitor pipeline when MODE=strategy.
func runStrategy(cfg *config.Config, pool *llm.Pool, skillsReg *matcher.SkillsRegistry, baseDir string, tg *notify.Telegram) {
	ctx := context.Background()
	var matched, unmatched int
	matchCount := make(map[string]int)
	var missedTitles []string
	for _, feedURL := range skillsReg.GetFeeds() {
		jobs, err := scraper.FetchFeed(feedURL, time.Duration(cfg.RSSTimeoutSeconds)*time.Second)
		if err != nil {
			log.Printf("fetch feed %s: %v", feedURL, err)
			continue
		}
		for _, job := range jobs {
			matches := skillsReg.Match(job.Title, job.Description)
			if len(matches) == 0 {
				unmatched++
				if len(missedTitles) < 40 {
					missedTitles = append(missedTitles, truncateRunes(job.Title, 120))
				}
				continue
			}
			matched++
			for _, m := range matches {
				matchCount[m.Skill.ID]++
			}
		}
	}

	tmpl, err := template.ParseFiles(filepath.Join(baseDir, "prompts", "strategy-analysis.tmpl"))
	if err != nil {
		log.Fatalf("load strategy template: %v", err)
	}
	var buf strings.Builder
	if err := tmpl.Execute(&buf, struct {
		Skills       []matcher.Skill
		Matched      int
		Unmatched    int
		MatchCount   map[string]int
		MissedTitles []string
	}{skillsReg.Skills, matched, unmatched, matchCount, missedTitles}); err != nil {
		log.Fatalf("render strategy template: %v", err)
	}

	result, err := pool.Complete(ctx, "strategy", buf.String())
	if err != nil {
		log.Fatalf("generate strategy: %v", err)
	}

	applied := applyKeywordAdditions(skillsReg, extractJSONBlock(result))
	if applied > 0 {
		if err := skillsReg.Save(filepath.Join(baseDir, "skills-registry.json")); err != nil {
			log.Printf("save skills registry: %v", err)
		}
		runSetup(pool, skillsReg, baseDir, tg)
	}

	profilesDir := filepath.Join(baseDir, "profiles")
	os.MkdirAll(profilesDir, 0755)
	reportPath := filepath.Join(profilesDir, "strategy-report.md")
	if err := os.WriteFile(reportPath, []byte(result), 0644); err != nil {
		log.Fatalf("write strategy report: %v", err)
	}

	summary := fmt.Sprintf("Strategy Mode Complete. Matched: %d, Missed: %d, Keywords added: %d.", matched, unmatched, applied)
	if err := tg.SendAlert(summary + "\n\n" + truncateRunes(result, 900)); err != nil {
		log.Printf("telegram strategy alert failed: %v", err)
	}
	log.Printf("strategy complete: matched %d, unmatched %d, keywords added %d", matched, unmatched, applied)
}

type keywordAddition struct {
	SkillID  string   `json:"skill_id"`
	Keywords []string `json:"keywords"`
}

// applyKeywordAdditions merges deduplicated, lowercased keywords from the
// LLM JSON block into existing skills only. It never creates new skills.
func applyKeywordAdditions(reg *matcher.SkillsRegistry, jsonBlock string) int {
	if jsonBlock == "" {
		return 0
	}
	var payload struct {
		Additions []keywordAddition `json:"keyword_additions"`
	}
	if err := json.Unmarshal([]byte(jsonBlock), &payload); err != nil {
		log.Printf("strategy: parse keyword JSON failed: %v", err)
		return 0
	}
	applied := 0
	for _, add := range payload.Additions {
		for i := range reg.Skills {
			if reg.Skills[i].ID != add.SkillID {
				continue
			}
			existing := make(map[string]bool)
			for _, kw := range reg.Skills[i].Keywords {
				existing[strings.ToLower(strings.TrimSpace(kw))] = true
			}
			for _, kw := range add.Keywords {
				kw = strings.ToLower(strings.TrimSpace(kw))
				if kw == "" || existing[kw] {
					continue
				}
				reg.Skills[i].Keywords = append(reg.Skills[i].Keywords, kw)
				existing[kw] = true
				applied++
			}
		}
	}
	return applied
}

// extractJSONBlock returns the content of the LAST fenced json block in s,
// or "" when absent. The fence is built from \x60 escapes so this source
// file never contains literal triple backticks.
func extractJSONBlock(s string) string {
	fence := "\x60\x60\x60"
	marker := fence + "json"
	start := strings.LastIndex(s, marker)
	if start == -1 {
		return ""
	}
	start += len(marker)
	end := strings.Index(s[start:], fence)
	if end == -1 {
		return ""
	}
	return strings.TrimSpace(s[start : start+end])
}

func truncateRunes(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "..."
}
