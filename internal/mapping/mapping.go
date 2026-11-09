// Package mapping resolves MyAnimeList IDs to TheTVDB series, season and
// episode offset using the Fribb/anime-lists dataset
// (https://github.com/Fribb/anime-lists).
package mapping

import (
	"aniliberty-indexer/internal/config"

	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"sync/atomic"
	"time"
)

// TVDB placement of a single MAL entry
type Entry struct {
	MALID         int
	TVDBID        int
	Season        int  // TVDB season number (0 = specials)
	HasSeason     bool // false when the list knows the series but not the season
	EpisodeOffset int  // added to AniLiberty episode numbers to get TVDB numbering
}

type store struct {
	byMAL  map[int]Entry
	byTVDB map[int][]Entry
}

var current atomic.Pointer[store]

// Raw entry of anime-list-full.json (only the fields we use)
type rawEntry struct {
	MALID  int `json:"mal_id"`
	TVDBID int `json:"tvdb_id"`
	Season *struct {
		TVDB *int `json:"tvdb"`
	} `json:"season"`
	EpisodeOffset *struct {
		TVDB *int `json:"tvdb"`
	} `json:"episode_offset"`
}

const cacheFileName = "anime-list-full.json"

// Parses the Fribb JSON list and builds lookup indexes
func parse(data []byte) (*store, error) {
	var raw []rawEntry
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("parse mapping list: %w", err)
	}

	s := &store{
		byMAL:  make(map[int]Entry, len(raw)/4),
		byTVDB: make(map[int][]Entry),
	}
	for _, r := range raw {
		if r.MALID <= 0 || r.TVDBID <= 0 {
			continue
		}
		e := Entry{MALID: r.MALID, TVDBID: r.TVDBID}
		if r.Season != nil && r.Season.TVDB != nil {
			e.Season = *r.Season.TVDB
			e.HasSeason = true
		}
		if r.EpisodeOffset != nil && r.EpisodeOffset.TVDB != nil {
			e.EpisodeOffset = *r.EpisodeOffset.TVDB
		}
		s.byMAL[e.MALID] = e
		s.byTVDB[e.TVDBID] = append(s.byTVDB[e.TVDBID], e)
	}
	if len(s.byMAL) == 0 {
		return nil, fmt.Errorf("mapping list contains no usable entries")
	}
	return s, nil
}

// Replaces the active mapping with one parsed from raw JSON (used by tests and loaders)
func LoadFromBytes(data []byte) error {
	s, err := parse(data)
	if err != nil {
		return err
	}
	current.Store(s)
	return nil
}

// Drops the active mapping (used by tests)
func Reset() {
	current.Store(nil)
}

// Returns the TVDB placement for a MAL ID
func Lookup(malID int) (Entry, bool) {
	s := current.Load()
	if s == nil || malID <= 0 {
		return Entry{}, false
	}
	e, ok := s.byMAL[malID]
	return e, ok
}

// Returns MAL entries that belong to a TVDB series; season < 0 means any season
func EntriesForTVDB(tvdbID int, season int) []Entry {
	s := current.Load()
	if s == nil || tvdbID <= 0 {
		return nil
	}
	var out []Entry
	for _, e := range s.byTVDB[tvdbID] {
		if season >= 0 && (!e.HasSeason || e.Season != season) {
			continue
		}
		out = append(out, e)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].MALID < out[j].MALID })
	return out
}

// Reports whether a mapping list is loaded
func Loaded() bool {
	return current.Load() != nil
}

// Number of MAL entries in the active list
func Size() int {
	s := current.Load()
	if s == nil {
		return 0
	}
	return len(s.byMAL)
}

func cachePath() string {
	if config.DATA_DIR == "" {
		return ""
	}
	return filepath.Join(config.DATA_DIR, cacheFileName)
}

// Downloads the list, activates it and writes it to the cache file
func refresh() error {
	client := &http.Client{Timeout: 2 * time.Minute, Transport: config.HTTPClient.Transport}
	req, err := http.NewRequest("GET", config.MAPPING_URL, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "AniLiberty-Indexer/1.0")
	res, err := client.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP %d from %s", res.StatusCode, config.MAPPING_URL)
	}
	data, err := io.ReadAll(res.Body)
	if err != nil {
		return err
	}
	if err := LoadFromBytes(data); err != nil {
		return err
	}

	if p := cachePath(); p != "" {
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err == nil {
			tmp := p + ".tmp"
			if err := os.WriteFile(tmp, data, 0o644); err == nil {
				os.Rename(tmp, p)
			} else {
				log.Printf("[mapping] cannot write cache %s: %v\n", p, err)
			}
		}
	}
	return nil
}

// Loads the list from cache (or network) and keeps it refreshed in the background.
// Returns a short status string for the startup banner.
func Init() string {
	if !config.SEASON_MAPPING {
		return "disabled"
	}

	var cacheAge time.Duration = -1
	if p := cachePath(); p != "" {
		if info, err := os.Stat(p); err == nil {
			if data, err := os.ReadFile(p); err == nil {
				if err := LoadFromBytes(data); err == nil {
					cacheAge = time.Since(info.ModTime())
				} else {
					log.Printf("[mapping] ignoring broken cache %s: %v\n", p, err)
				}
			}
		}
	}

	status := ""
	if cacheAge < 0 || cacheAge > config.MAPPING_REFRESH_INTERVAL {
		if err := refresh(); err != nil {
			log.Printf("[mapping] download failed: %v\n", err)
			if !Loaded() {
				status = fmt.Sprintf("unavailable (%v), falling back to title parsing", err)
			}
		}
	}
	if status == "" {
		status = fmt.Sprintf("%d MAL entries", Size())
	}

	go func() {
		ticker := time.NewTicker(config.MAPPING_REFRESH_INTERVAL)
		defer ticker.Stop()
		for range ticker.C {
			if err := refresh(); err != nil {
				log.Printf("[mapping] refresh failed: %v\n", err)
			} else {
				log.Printf("[mapping] refreshed: %d MAL entries\n", Size())
			}
		}
	}()

	return status
}
