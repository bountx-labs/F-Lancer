package proposal

import (
	"context"
	"fmt"
	"os"
	"regexp"
	"strings"
	"text/template"

	"github.com/bountx-labs/autonomous-freelance-engine/internal/llm"
	"github.com/bountx-labs/autonomous-freelance-engine/internal/matcher"
	"github.com/bountx-labs/autonomous-freelance-engine/internal/scraper"
)

var placeholderPattern = regexp.MustCompile(`\[(?:NAME|YOUR|INSERT|TODO|PLACEHOLDER|COMPANY|CLIENT|PROJECT|DATE|BUDGET|TIMELINE|EXPERIENCE|SKILL|RATE|LINK)[^\]]*\]`)

type Generator struct {
	pool       *llm.Pool
	proposalTmpl *template.Template
	guideTmpl    *template.Template
}

func New(pool *llm.Pool, promptsDir string) (*Generator, error) {
	proposalTmpl, err := template.ParseFiles(promptsDir + "/proposal.tmpl")
	if err != nil {
		return nil, fmt.Errorf("load proposal template: %w", err)
	}

	guideTmpl, err := template.ParseFiles(promptsDir + "/executive-guide.tmpl")
	if err != nil {
		return nil, fmt.Errorf("load guide template: %w", err)
	}

	return &Generator{
		pool:         pool,
		proposalTmpl: proposalTmpl,
		guideTmpl:    guideTmpl,
	}, nil
}

type proposalData struct {
	Title       string
	Description string
	Budget      string
	Category    string
	Skills      []matcher.Skill
}

type guideData struct {
	Link     string
	Budget   string
	Category string
}

func (g *Generator) GenerateProposal(ctx context.Context, job scraper.Job, matches []matcher.MatchResult) (string, error) {
	var skills []matcher.Skill
	for _, m := range matches {
		skills = append(skills, m.Skill)
	}

	var promptBuf strings.Builder
	data := proposalData{
		Title:       job.Title,
		Description: job.Description,
		Budget:      job.Budget,
		Category:    job.Category,
		Skills:      skills,
	}
	if err := g.proposalTmpl.Execute(&promptBuf, data); err != nil {
		return "", fmt.Errorf("render proposal template: %w", err)
	}

	result, err := g.pool.Complete(ctx, "proposal", promptBuf.String())
	if err != nil {
		return "", fmt.Errorf("generate proposal: %w", err)
	}

	if placeholderPattern.MatchString(result) {
		result, err = g.pool.Complete(ctx, "proposal", "Re-generate this proposal with NO placeholders like [Name] or [Your Experience]. Fill in all values:\n\n"+result)
		if err != nil {
			return result, nil
		}
	}

	return result, nil
}

func (g *Generator) GenerateGuide(ctx context.Context, job scraper.Job) (string, error) {
	var promptBuf strings.Builder
	data := guideData{
		Link:     job.Link,
		Budget:   job.Budget,
		Category: job.Category,
	}
	if err := g.guideTmpl.Execute(&promptBuf, data); err != nil {
		return "", fmt.Errorf("render guide template: %w", err)
	}

	result, err := g.pool.Complete(ctx, "default", promptBuf.String())
	if err != nil {
		return "", fmt.Errorf("generate guide: %w", err)
	}

	return result, nil
}

func ValidateNoPlaceholders(text string) bool {
	return !placeholderPattern.MatchString(text)
}

func ReadPromptFile(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return string(data), nil
}