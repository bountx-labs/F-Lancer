package bounty

import (
	"context"
	"fmt"
	"log"
	"time"
)

// OrphanAge is how long the agent's own pull request may sit without any
// activity before it is treated as abandoned and cleaned up. A stale pull
// request is a burden on the account: it makes the profile look careless and
// invites maintainers to ignore future contributions.
const OrphanAge = 7 * 24 * time.Hour

// Orphan is the agent's own pull request that has gone quiet.
type Orphan struct {
	Repo     string
	Number   int
	HTMLURL  string
	Title    string
	IdleDays int
	Comments int
}

// FindOrphans lists the agent's open pull requests that have had no activity
// for longer than OrphanAge. These are the ones that should be closed rather
// than left to rot.
func (h *ReviewHandler) FindOrphans(ctx context.Context, login string) ([]Orphan, error) {
	pulls, err := h.PendingReviews(ctx, login)
	if err != nil {
		return nil, err
	}

	now := time.Now().UTC()
	var orphans []Orphan

	for _, pull := range pulls {
		updated, err := h.contributor.PullUpdatedAt(ctx, pull.Repo, pull.Number)
		if err != nil {
			log.Printf("bounty: orphan: %s#%d updated-at failed: %v", pull.Repo, pull.Number, err)
			continue
		}
		idle := now.Sub(updated)
		if idle < OrphanAge {
			continue
		}

		comments, _ := h.contributor.ReviewComments(ctx, pull.Repo, pull.Number)
		orphans = append(orphans, Orphan{
			Repo:     pull.Repo,
			Number:   pull.Number,
			HTMLURL:  pull.HTMLURL,
			Title:    pull.Title,
			IdleDays: int(idle.Hours() / 24),
			Comments: len(comments),
		})
	}
	return orphans, nil
}

// CleanOrphans closes every stale pull request with an honest note, so the
// account carries no abandoned work. It returns how many were closed.
func (h *ReviewHandler) CleanOrphans(ctx context.Context, login string) (int, error) {
	orphans, err := h.FindOrphans(ctx, login)
	if err != nil {
		return 0, err
	}

	closed := 0
	for _, o := range orphans {
		// A pull request with reviewer feedback is addressable; try to answer
		// it instead of discarding the work.
		if o.Comments > 0 {
			log.Printf("bounty: orphan: %s#%d has %d comments; leaving for review handling", o.Repo, o.Number, o.Comments)
			continue
		}

		note := fmt.Sprintf(
			"Closing this pull request as stale. It has been open for %d days with no review activity, and leaving it pending adds noise for maintainers. I will reopen with a fresh change if this is still wanted.",
			o.IdleDays)

		if err := h.contributor.ClosePull(ctx, o.Repo, o.Number, note); err != nil {
			log.Printf("bounty: orphan: close %s#%d failed: %v", o.Repo, o.Number, err)
			continue
		}
		closed++
		log.Printf("bounty: orphan: closed %s#%d after %d idle days", o.Repo, o.Number, o.IdleDays)
	}

	return closed, nil
}
