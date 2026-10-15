package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/bountx-labs/F-Lancer/internal/config"
	"github.com/bountx-labs/F-Lancer/internal/freelancer"
	"github.com/bountx-labs/F-Lancer/internal/llm"
	"github.com/bountx-labs/F-Lancer/internal/notify"
	"github.com/bountx-labs/F-Lancer/internal/proposal"
	"github.com/bountx-labs/F-Lancer/internal/scraper"
)

// runBuildQueue implements MODE=build-queue: it reads the unread briefs in
// proposals/inbox/, resolves each to a live Freelancer.com project_id through
// the search API, drafts a proposal with the LLM, and appends ready bids to
// proposals/drafts/submit-queue.json. CI then places them in MODE=submit.
//
// The build is idempotent: briefs already present in the queue are skipped, so
// repeated runs never duplicate a bid.
func runBuildQueue(cfg *config.Config, pool *llm.Pool, baseDir string, tg *notify.Telegram) {
	ctx := context.Background()

	inboxRoot := filepath.Join(baseDir, "proposals", "inbox")
	queuePath := filepath.Join(baseDir, "proposals", "drafts", "submit-queue.json")

	briefs, err := listBriefs(inboxRoot)
	if err != nil {
		log.Fatalf("build-queue: read inbox: %v", err)
	}
	if len(briefs) == 0 {
		log.Println("build-queue: no briefs found")
		return
	}

	existing, err := loadQueue(queuePath)
	if err != nil {
		log.Fatalf("build-queue: load queue: %v", err)
	}
	queued := make(map[int64]bool, len(existing))
	for _, item := range existing {
		queued[item.ProjectID] = true
	}

	token := os.Getenv("FREELANCER_OAUTH_TOKEN")
	search := freelancer.NewSearchClient(token, time.Duration(cfg.LLMTimeoutSeconds)*time.Second)
	gen, err := newQueueGenerator(pool, filepath.Join(baseDir, "prompts"))
	if err != nil {
		log.Fatalf("build-queue: init generator: %v", err)
	}

	maxItems := cfg.MaxJobsPerRun
	added, skipped, failed := 0, 0, 0

	for _, b := range briefs {
		if added >= maxItems {
			break
		}

		project, err := search.FindProjectByLink(ctx, b.Link)
		if err != nil {
			log.Printf("build-queue: resolve %s: %v", b.Link, err)
			failed++
			continue
		}
		if queued[project.ID] {
			skipped++
			continue
		}

		bidAmount := suggestBid(project.ParsedBudget())
		description, err := gen.Generate(ctx, *project, bidAmount)
		if err != nil {
			log.Printf("build-queue: draft for project %d: %v", project.ID, err)
			failed++
			continue
		}

		existing = append(existing, submitQueueItem{
			ProjectID:           project.ID,
			Title:               project.Title,
			BidAmount:           bidAmount,
			PeriodDays:          7,
			MilestonePercentage: 50,
			Description:         description,
		})
		queued[project.ID] = true
		added++
		log.Printf("build-queue: queued project %d (%s) bid %v", project.ID, project.Title, bidAmount)
	}

	if err := saveQueue(queuePath, existing); err != nil {
		log.Fatalf("build-queue: save queue: %v", err)
	}

	summary := fmt.Sprintf("Build-queue complete. Queued: %d, already queued: %d, unresolved: %d.", added, skipped, failed)
	log.Println(summary)
	if err := tg.SendAlert(summary); err != nil {
		log.Printf("build-queue: telegram alert failed: %v", err)
	}
}

// brief is the minimal front-matter of an inbox brief the builder needs.
type brief struct {
	Title string
	Link  string
}

// listBriefs walks proposals/inbox/<date>/ and returns every brief newest
// first, so the most recent opportunities are queued before older ones.
func listBriefs(inboxRoot string) ([]brief, error) {
	dateDirs, err := os.ReadDir(inboxRoot)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var dirs []string
	for _, d := range dateDirs {
		if d.IsDir() {
			dirs = append(dirs, d.Name())
		}
	}
	sort.Sort(sort.Reverse(sort.StringSlice(dirs)))

	var briefs []brief
	for _, dir := range dirs {
		entries, err := os.ReadDir(filepath.Join(inboxRoot, dir))
		if err != nil {
			continue
		}
		for _, e := range entries {
			if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
				continue
			}
			b, err := parseBrief(filepath.Join(inboxRoot, dir, e.Name()))
			if err != nil {
				log.Printf("build-queue: parse brief %s: %v", e.Name(), err)
				continue
			}
			if b.Link != "" {
				briefs = append(briefs, b)
			}
		}
	}
	return briefs, nil
}

// parseBrief pulls the title heading and the Link table row from a brief.
func parseBrief(path string) (brief, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return brief{}, err
	}
	var b brief
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if b.Title == "" && strings.HasPrefix(line, "# ") {
			b.Title = strings.TrimSpace(strings.TrimPrefix(line, "# "))
			continue
		}
		if b.Link == "" && strings.HasPrefix(line, "| Link |") {
			parts := strings.Split(line, "|")
			if len(parts) >= 3 {
				b.Link = strings.TrimSpace(parts[2])
			}
		}
	}
	return b, nil
}

// loadQueue reads the existing queue, tolerating a missing file.
func loadQueue(path string) ([]submitQueueItem, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	if len(strings.TrimSpace(string(data))) == 0 {
		return nil, nil
	}
	var items []submitQueueItem
	if err := json.Unmarshal(data, &items); err != nil {
		return nil, err
	}
	return items, nil
}

// saveQueue writes the queue atomically so a crash mid-write cannot corrupt
// the file the submit mode depends on.
func saveQueue(path string, items []submitQueueItem) error {
	if len(items) == 0 {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(items, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')

	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// suggestBid derives a bid from the project's maximum budget. Below 10 an
// absurdly small bid would be rejected by the platform, so a floor is applied.
func suggestBid(maxBudget float64) float64 {
	if maxBudget <= 0 {
		return 50
	}
	bid := maxBudget * 0.9
	if bid < 10 {
		bid = 10
	}
	return float64(int(bid*100)) / 100
}

// queueGenerator renders the proposal template and calls the LLM for one job.
// It reuses the shared proposal generator so bid copy follows the same
// placeholder handling as every other path.
type queueGenerator struct {
	gen *proposal.Generator
}

func newQueueGenerator(pool *llm.Pool, promptsDir string) (*queueGenerator, error) {
	gen, err := proposal.New(pool, promptsDir)
	if err != nil {
		return nil, err
	}
	return &queueGenerator{gen: gen}, nil
}

// Generate produces the bid description for a project.
func (g *queueGenerator) Generate(ctx context.Context, project freelancer.Project, bidAmount float64) (string, error) {
	job := scraper.Job{
		Title:       project.Title,
		Description: project.Description,
		Budget:      strconv.FormatFloat(bidAmount, 'f', 2, 64),
		Category:    project.Category,
	}
	return g.gen.GenerateProposal(ctx, job, nil)
}
