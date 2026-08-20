package state

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"
)

const (
	DefaultPruneDays  = 30
	DefaultMaxEntries = 500
)

type SeenJobs struct {
	mu         sync.Mutex
	path       string
	pruneDays  int
	maxEntries int
	Hashes     map[string]time.Time `json:"hashes"`
}

func LoadWithLimits(path string, pruneDays, maxEntries int) (*SeenJobs, error) {
	s := &SeenJobs{
		path:       path,
		pruneDays:  pruneDays,
		maxEntries: maxEntries,
		Hashes:     make(map[string]time.Time),
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return s, nil
		}
		return nil, fmt.Errorf("read state: %w", err)
	}

	var disk struct {
		Hashes map[string]time.Time `json:"hashes"`
	}
	if err := json.Unmarshal(data, &disk); err != nil {
		return nil, fmt.Errorf("parse state: %w", err)
	}
	if disk.Hashes != nil {
		s.Hashes = disk.Hashes
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

	if err := atomicWrite(s.path, data); err != nil {
		return fmt.Errorf("write state: %w", err)
	}

	log.Printf("state saved: %s (%d hashes)", s.path, len(s.Hashes))
	return nil
}

func (s *SeenJobs) prune() {
	if s.pruneDays <= 0 {
		s.pruneDays = DefaultPruneDays
	}
	if s.maxEntries <= 0 {
		s.maxEntries = DefaultMaxEntries
	}

	cutoff := time.Now().UTC().AddDate(0, 0, -s.pruneDays)
	for hash, ts := range s.Hashes {
		if ts.Before(cutoff) {
			delete(s.Hashes, hash)
		}
	}

	if len(s.Hashes) > s.maxEntries {
		type entry struct {
			hash string
			ts   time.Time
		}
		var entries []entry
		for h, t := range s.Hashes {
			entries = append(entries, entry{h, t})
		}
		sort.Slice(entries, func(i, j int) bool {
			return entries[i].ts.Before(entries[j].ts)
		})
		for i := 0; i < len(entries)-s.maxEntries; i++ {
			delete(s.Hashes, entries[i].hash)
		}
	}
}

// hashURL returns a SHA256 hex digest of the job URL, used as the dedup key.
func hashURL(url string) string {
	h := sha256.New()
	h.Write([]byte(url))
	return fmt.Sprintf("%x", h.Sum(nil))
}

// atomicWrite writes data to path atomically via a temp file + rename, so a
// concurrent or interrupted write can never leave a corrupted state file.
func atomicWrite(path string, data []byte) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".seen_jobs-*.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer func() {
		if tmpName != "" {
			os.Remove(tmpName)
		}
	}()

	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmpName, path); err != nil {
		return err
	}
	tmpName = ""
	return nil
}
