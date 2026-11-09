package qbit

import (
	"aniliberty-indexer/internal/config"

	"encoding/json"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

const (
	selectionsFileName = "file_selections.json"
	// A grab that never reached qBittorrent is forgotten after this time
	selectionPendingTTL = 24 * time.Hour
	// Applied selections are kept to merge later grabs from the same torrent
	selectionKeepTTL = 90 * 24 * time.Hour
	// Right after a grab qBittorrent is polled often, so that the skipped files barely start downloading
	selectionFastWindow = 3 * time.Minute
	selectionFastPoll   = 5 * time.Second
)

// Files of a torrent that were requested through single-episode downloads
type Selection struct {
	Files   []int     `json:"files"`
	Pending bool      `json:"pending"` // not applied in qBittorrent yet
	Updated time.Time `json:"updated"`
}

// Store of file selections, persisted in DATA_DIR
type Selections struct {
	mu    sync.Mutex
	path  string
	items map[string]*Selection
	wake  chan struct{}
}

func NewSelections(path string) *Selections {
	s := &Selections{path: path, items: map[string]*Selection{}, wake: make(chan struct{}, 1)}
	s.load()
	return s
}

var (
	defaultSelectionsOnce sync.Once
	defaultSelections     *Selections
)

// Process-wide store shared by the download endpoint and the renamer
func DefaultSelections() *Selections {
	defaultSelectionsOnce.Do(func() {
		p := ""
		if config.DATA_DIR != "" {
			p = filepath.Join(config.DATA_DIR, selectionsFileName)
		}
		defaultSelections = NewSelections(p)
	})
	return defaultSelections
}

// Remembers that only these files of the torrent are wanted (merged with earlier grabs).
// Does nothing when the qBittorrent integration is disabled.
func SelectFiles(hash string, files []int) {
	if config.QBIT_URL == "" || len(files) == 0 {
		return
	}
	DefaultSelections().Add(hash, files)
}

func (s *Selections) Add(hash string, files []int) {
	hash = strings.ToLower(hash)
	s.mu.Lock()
	sel, ok := s.items[hash]
	if !ok {
		sel = &Selection{}
		s.items[hash] = sel
	}
	sel.Files = mergeIndices(sel.Files, files)
	sel.Pending = true
	sel.Updated = time.Now()
	s.saveLocked()
	s.mu.Unlock()
	log.Printf("[qbit] %s: only files %v will be downloaded\n", hash, sel.Files)

	select {
	case s.wake <- struct{}{}:
	default:
	}
}

// Pending selections, keyed by lowercase infohash
func (s *Selections) pending() map[string][]int {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := map[string][]int{}
	for h, sel := range s.items {
		if sel.Pending {
			out[h] = append([]int(nil), sel.Files...)
		}
	}
	return out
}

// Marks a selection as applied; files must be what was applied, a newer grab keeps it pending
func (s *Selections) applied(hash string, files []int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if sel, ok := s.items[hash]; ok && len(sel.Files) == len(files) {
		sel.Pending = false
		s.saveLocked()
	}
}

// Drops pending selections whose torrent never showed up in qBittorrent
func (s *Selections) expire(now time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	changed := false
	for h, sel := range s.items {
		age := now.Sub(sel.Updated)
		if (sel.Pending && age > selectionPendingTTL) || age > selectionKeepTTL {
			delete(s.items, h)
			changed = true
		}
	}
	if changed {
		s.saveLocked()
	}
}

// True while a fresh grab is waiting for its torrent to appear in qBittorrent
func (s *Selections) hasFresh(now time.Time) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, sel := range s.items {
		if sel.Pending && now.Sub(sel.Updated) < selectionFastWindow {
			return true
		}
	}
	return false
}

func (s *Selections) load() {
	if s.path == "" {
		return
	}
	data, err := os.ReadFile(s.path)
	if err != nil {
		if !os.IsNotExist(err) {
			log.Printf("[qbit] read %s: %v\n", s.path, err)
		}
		return
	}
	if err := json.Unmarshal(data, &s.items); err != nil {
		log.Printf("[qbit] parse %s: %v\n", s.path, err)
		s.items = map[string]*Selection{}
	}
}

func (s *Selections) saveLocked() {
	if s.path == "" {
		return
	}
	data, err := json.MarshalIndent(s.items, "", "  ")
	if err != nil {
		return
	}
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		log.Printf("[qbit] save selections: %v\n", err)
		return
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		log.Printf("[qbit] save selections: %v\n", err)
		return
	}
	if err := os.Rename(tmp, s.path); err != nil {
		log.Printf("[qbit] save selections: %v\n", err)
	}
}

func mergeIndices(a, b []int) []int {
	seen := map[int]bool{}
	var out []int
	for _, list := range [][]int{a, b} {
		for _, n := range list {
			if !seen[n] {
				seen[n] = true
				out = append(out, n)
			}
		}
	}
	sort.Ints(out)
	return out
}

// Priority changes that leave only the wanted files: ids to skip and ids to re-enable
func PlanPriorities(files []File, wanted []int) (skip, enable []int) {
	want := map[int]bool{}
	for _, n := range wanted {
		want[n] = true
	}
	for i, f := range files {
		switch {
		case want[i] && f.Priority == 0:
			enable = append(enable, i)
		case !want[i] && f.Priority != 0:
			skip = append(skip, i)
		}
	}
	return skip, enable
}
