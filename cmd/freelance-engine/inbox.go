package main

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/bountx-labs/autonomous-freelance-engine/internal/config"
	"github.com/bountx-labs/autonomous-freelance-engine/internal/matcher"
	"github.com/bountx-labs/autonomous-freelance-engine/internal/scraper"
	"github.com/bountx-labs/autonomous-freelance-engine/internal/state"
)

// runInbox implements the folder-only delivery mode: matched jobs are written
// as markdown briefs under proposals/inbox/<date>/ and committed to the repo
// by CI. No LLM and no Telegram is involved; a local agent later reads the
// briefs, drafts the proposals, and the user submits them manually.
func runInbox(cfg *config.Config, skillsReg *matcher.SkillsRegistry, baseDir string) {
	stateDir := filepath.Join(baseDir, "state")
	if err := os.MkdirAll(stateDir, 0755); err != nil {
		log.Fatalf("create state dir: %v", err)
	}

	seenPath := filepath.Join(stateDir, "seen_jobs.json")
	seen, err := state.LoadWithLimits(seenPath, cfg.StatePruneDays, cfg.StateMaxEntries)
	if err != nil {
		log.Fatalf("load state: %v", err)
	}
	log.Printf("loaded state from %s with %d hashes", seenPath, len(seen.Hashes))

	inboxDir := filepath.Join(baseDir, "proposals", "inbox", time.Now().UTC().Format("2006-01-02"))
	rss := scraper.New(time.Duration(cfg.RSSTimeoutSeconds) * time.Second)

	jobCount := 0
	for _, feedURL := range skillsReg.GetFeeds() {
		if jobCount >= cfg.MaxJobsPerRun {
			break
		}

		jobs, err := rss.FetchFeed(feedURL)
		if err != nil {
			log.Printf("fetch feed %s: %v", feedURL, err)
			continue
		}

		for _, job := range jobs {
			if jobCount >= cfg.MaxJobsPerRun {
				break
			}

			if seen.IsSeen(job.Link) {
				continue
			}

			matches := skillsReg.Match(job.Title, job.Description)
			if len(matches) == 0 {
				continue
			}

			path, err := writeBrief(inboxDir, job, matches)
			if err != nil {
				log.Printf("brief for %s: %v", job.Link, err)
				continue
			}

			// Mark seen only after the brief is durably written so a failed
			// write retries on the next run without duplicating files.
			seen.MarkSeen(job.Link)
			jobCount++
			log.Printf("brief saved: %s", path)
		}
	}

	if err := seen.Save(); err != nil {
		log.Fatalf("save state FAILED: %v", err)
	}

	log.Printf("run complete. %d briefs written to %s", jobCount, inboxDir)
}

// writeBrief renders one job brief as markdown and returns its path.
func writeBrief(dir string, job scraper.Job, matches []matcher.MatchResult) (string, error) {
	sum := sha256.Sum256([]byte(job.Link))
	name := fmt.Sprintf("%s-%s.md", slugify(job.Title), hex.EncodeToString(sum[:4]))
	path := filepath.Join(dir, name)

	var b strings.Builder
	fmt.Fprintf(&b, "# %s\n\n", job.Title)
	b.WriteString("| Field | Value |\n|---|---|\n")
	fmt.Fprintf(&b, "| Link | %s |\n", job.Link)
	fmt.Fprintf(&b, "| Budget | %s |\n", job.Budget)
	fmt.Fprintf(&b, "| Category | %s |\n", job.Category)
	fmt.Fprintf(&b, "| Published | %s |\n", job.PubDate.UTC().Format(time.RFC3339))

	sorted := make([]matcher.MatchResult, len(matches))
	copy(sorted, matches)
	sort.SliceStable(sorted, func(i, j int) bool { return sorted[i].Score > sorted[j].Score })
	names := make([]string, 0, len(sorted))
	for _, m := range sorted {
		names = append(names, m.Skill.Name)
	}
	fmt.Fprintf(&b, "| Matched Skills | %s |\n", strings.Join(names, ", "))

	fmt.Fprintf(&b, "\n## Description\n\n%s\n", strings.TrimSpace(job.Description))

	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", err
	}
	if err := os.WriteFile(path, []byte(b.String()), 0644); err != nil {
		return "", err
	}
	return path, nil
}

// slugify reduces a title to a filesystem-safe lowercase kebab string.
func slugify(title string) string {
	var b strings.Builder
	lastDash := true // avoids a leading dash
	for _, r := range strings.ToLower(title) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
			lastDash = false
		default:
			if !lastDash {
				b.WriteByte('-')
				lastDash = true
			}
		}
	}
	out := strings.Trim(b.String(), "-")
	if out == "" {
		return "job"
	}
	if len(out) > 60 {
		out = out[:60]
	}
	return out
}
