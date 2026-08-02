package state

import (
	"crypto/sha1"
	"encoding/json"
	"fmt"
	"os"
	"sync"
	"time"
)

type SeenJobs struct {
	mu       sync.Mutex
	path     string
	Hashes   map[string]time.Time `json:"hashes"`
}

func Load(path string) (*SeenJobs, error) {
	s := &SeenJobs{
		path:   path,
		Hashes: make(map[string]time.Time),
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return s, nil
		}
		return nil, fmt.Errorf("read state: %w", err)
	}

	if err := json.Unmarshal(data, &s); err != nil {
		return nil, fmt.Errorf("parse state: %w", err)
	}

	s.prune()
	return s, nil
}

func (s *SeenJobs) IsSeen(url string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	hash := hashURL(url)
	_, ok := s.Hashes[hash]
	return ok
}

func (s *SeenJobs) MarkSeen(url string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	hash := hashURL(url)
	s.Hashes[hash] = time.Now().UTC()
}

func (s *SeenJobs) Save() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.prune()

	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal state: %w", err)
	}

	if err := os.WriteFile(s.path, data, 0644); err != nil {
		return fmt.Errorf("write state: %w", err)
	}

	return nil
}

func (s *SeenJobs) prune() {
	cutoff := time.Now().UTC().AddDate(0, 0, -30)
	for hash, ts := range s.Hashes {
		if ts.Before(cutoff) {
			delete(s.Hashes, hash)
		}
	}

	if len(s.Hashes) > 500 {
		type entry struct {
			hash string
			ts   time.Time
		}
		var entries []entry
		for h, t := range s.Hashes {
			entries = append(entries, entry{h, t})
		}
		for i := 0; i < len(entries)-500; i++ {
			delete(s.Hashes, entries[i].hash)
		}
	}
}

func hashURL(url string) string {
	h := sha1.New()
	h.Write([]byte(url))
	return fmt.Sprintf("%x", h.Sum(nil))
}