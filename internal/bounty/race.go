package bounty

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"
)

// RaceWindow is how long a bounty stays realistically winnable. Real hunters
// and their scripts claim funded work within hours, so a bounty that is still
// open hours later has almost certainly been solved, abandoned, or is bait.
//
// This is deliberately much tighter than a general "freshness" rule. A slow
// agent that watches a wide window only ever sees the leftovers, which is why
// the previous threshold produced nothing but fraud and stale work.
const RaceWindow = 6 * time.Hour

// SeenStore remembers which bounties have been evaluated so a fast polling
// cycle does not re-process the same issues and burn its budget on repeats.
// It is the mechanism that makes a fast loop affordable.
type SeenStore struct {
	mu   sync.Mutex
	path string
	// Seen maps an issue key (repo#number) to when it was first evaluated.
	Seen map[string]time.Time `json:"seen"`
	// SkipUntil maps an issue key to a time before which it must not be
	// reconsidered. Used for issues that failed a gate for a fixable reason.
	SkipUntil map[string]time.Time `json:"skip_until,omitempty"`
}

// NewSeenStore loads the seen set, tolerating a missing file.
func NewSeenStore(path string) (*SeenStore, error) {
	s := &SeenStore{
		path:      path,
		Seen:      make(map[string]time.Time),
		SkipUntil: make(map[string]time.Time),
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return s, nil
		}
		return nil, err
	}
	if len(data) == 0 {
		return s, nil
	}
	if err := json.Unmarshal(data, s); err != nil {
		// A corrupt seen file must not stop the run; start fresh.
		return s, nil
	}
	if s.Seen == nil {
		s.Seen = make(map[string]time.Time)
	}
	if s.SkipUntil == nil {
		s.SkipUntil = make(map[string]time.Time)
	}
	return s, nil
}

// IssueKey builds the dedupe key for an issue.
func IssueKey(repo string, number int) string {
	return repo + "#" + itoa(number)
}

// Has reports whether the issue was already evaluated.
func (s *SeenStore) Has(key string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, ok := s.Seen[key]
	return ok
}

// Mark records that the issue has been evaluated.
func (s *SeenStore) Mark(key string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Seen[key] = time.Now().UTC()
}

// SkipFor defers an issue without retiring it. An issue that could not be
// attempted yet, but may become attemptable, should be reconsidered later
// rather than dropped for good or re-examined on every cycle.
func (s *SeenStore) SkipFor(key string, d time.Duration) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.SkipUntil[key] = time.Now().UTC().Add(d)
}

// ShouldSkip reports whether the issue is already handled or is still in its
// retry cooldown.
func (s *SeenStore) ShouldSkip(key string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.Seen[key]; ok {
		return true
	}
	if t, ok := s.SkipUntil[key]; ok && time.Now().UTC().Before(t) {
		return true
	}
	return false
}

// Prune drops entries older than the retention period so the file stays small.
func (s *SeenStore) Prune(retention time.Duration) int {
	s.mu.Lock()
	defer s.mu.Unlock()

	cutoff := time.Now().UTC().Add(-retention)
	removed := 0
	for k, t := range s.Seen {
		if t.Before(cutoff) {
			delete(s.Seen, k)
			removed++
		}
	}
	for k, t := range s.SkipUntil {
		if t.Before(cutoff) {
			delete(s.SkipUntil, k)
		}
	}
	return removed
}

// Save writes the store atomically so a crash cannot corrupt it.
func (s *SeenStore) Save() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := os.MkdirAll(filepath.Dir(s.path), 0755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, append(data, '\n'), 0644); err != nil {
		return err
	}
	return os.Rename(tmp, s.path)
}

// Count returns how many issues have been recorded.
func (s *SeenStore) Count() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.Seen)
}

// WithinRaceWindow reports whether an issue is recent enough to still be
// realistically winnable.
func WithinRaceWindow(createdAt string, now time.Time) (bool, time.Duration) {
	t, err := parseGitHubTime(createdAt)
	if err != nil {
		return false, 0
	}
	age := now.Sub(t)
	return age <= RaceWindow, age
}

// SortByFreshness orders issues newest first, so the freshest are attempted
// before the loop spends its budget on anything older.
func SortByFreshness(issues []Issue) {
	sort.SliceStable(issues, func(i, j int) bool {
		return issues[i].CreatedAt > issues[j].CreatedAt
	})
}

// itoa avoids importing strconv for a single call site.
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}
