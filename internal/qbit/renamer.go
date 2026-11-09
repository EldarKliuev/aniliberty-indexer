// Package qbit renames AniLiberty episode files inside qBittorrent torrents.
//
// AniLiberty numbers files within a release (`..._[01]_...mkv`), while the
// indexer announces season packs in TVDB numbering (`S02E01-E07`). Sonarr
// parses `[01]` as absolute episode 1 (S01E01) and refuses to import it.
// The renamer rewrites `[01]` to `S02E01` using the same season and offset
// as the release title. Renaming does not change the infohash, so seeding
// continues.
package qbit

import (
	"aniliberty-indexer/internal/api"
	"aniliberty-indexer/internal/config"
	"aniliberty-indexer/internal/torznab"

	"fmt"
	"log"
	"path"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

var (
	// Cheap pre-filter before asking AniLiberty: torrent name or tracker must mention it
	anilibertyRe = regexp.MustCompile(`(?i)anili(bria|berty)|libria\.fun`)
	// Episode number in AniLiberty file names: Title_[01]_[HEVC].mkv
	episodeRe = regexp.MustCompile(`\[(\d{1,4})\]`)
	// File already carries SxxEyy (renamed earlier or by hand)
	seasonEpisodeRe = regexp.MustCompile(`(?i)S\d{1,2}E\d{1,4}`)

	mediaExtensions = map[string]bool{
		".mkv": true, ".mp4": true, ".avi": true, ".mka": true,
		".ass": true, ".ssa": true, ".srt": true,
	}
)

// Resolves an infohash to an AniLiberty release; found=false for foreign torrents
type LookupFunc func(hash string) (item api.CombinedItem, found bool, err error)

type Renamer struct {
	client   *Client
	category string
	lookup   LookupFunc

	mu   sync.Mutex
	done map[string]bool // hashes that need no further work
}

func NewRenamer(client *Client, category string, lookup LookupFunc) *Renamer {
	return &Renamer{client: client, category: category, lookup: lookup, done: map[string]bool{}}
}

// Starts the background renamer if QBIT_URL is set; returns a state string for the banner
func Start() string {
	if config.QBIT_URL == "" {
		return "disabled"
	}
	client := NewClient(config.QBIT_URL, config.QBIT_USERNAME, config.QBIT_PASSWORD)
	r := NewRenamer(client, config.QBIT_CATEGORY, api.FetchTorrentByHash)
	go func() {
		if err := client.Login(); err != nil {
			log.Printf("[qbit] %v\n", err)
		}
		for {
			r.RunOnce()
			time.Sleep(config.QBIT_RENAME_INTERVAL)
		}
	}()
	state := config.QBIT_URL
	if config.QBIT_CATEGORY != "" {
		state += " (category " + config.QBIT_CATEGORY + ")"
	}
	return state
}

// Checks all torrents once and renames files where needed
func (r *Renamer) RunOnce() {
	torrents, err := r.client.Torrents(r.category)
	if err != nil {
		log.Printf("[qbit] list torrents: %v\n", err)
		return
	}
	for _, t := range torrents {
		hash := strings.ToLower(t.Hash)
		r.mu.Lock()
		done := r.done[hash]
		r.mu.Unlock()
		if done {
			continue
		}
		finished, err := r.process(hash, t)
		if err != nil {
			log.Printf("[qbit] %s (%s): %v\n", t.Name, hash, err)
		}
		if finished {
			r.mu.Lock()
			r.done[hash] = true
			r.mu.Unlock()
		}
	}
}

// Handles one torrent; finished=false means it should be retried on the next run
func (r *Renamer) process(hash string, t Torrent) (finished bool, err error) {
	if !anilibertyRe.MatchString(t.Name) && !anilibertyRe.MatchString(t.Tracker) {
		return true, nil
	}

	files, err := r.client.Files(hash)
	if err != nil {
		return false, err
	}
	if len(files) == 0 {
		return false, nil // metadata of a magnet link is not downloaded yet
	}

	// The authoritative check: the hash must belong to an AniLiberty release
	item, found, err := r.lookup(hash)
	if err != nil {
		return false, fmt.Errorf("AniLiberty lookup: %w", err)
	}
	if !found {
		return true, nil
	}
	if item.Release.Type.Value == "MOVIE" {
		return true, nil
	}

	season, offset := torznab.TVNumbering(item.Release)
	names := make([]string, len(files))
	for i, f := range files {
		names[i] = f.Name
	}
	renames := PlanRenames(names, season, offset)
	for _, rn := range renames {
		if err := r.client.RenameFile(hash, rn.Old, rn.New); err != nil {
			return false, err
		}
		log.Printf("[qbit] %s -> %s\n", rn.Old, path.Base(rn.New))
	}
	if len(renames) > 0 {
		log.Printf("[qbit] renamed %d file(s) in %q to S%02d\n", len(renames), t.Name, season)
	}
	return true, nil
}

type Rename struct {
	Old string
	New string
}

// Maps `[NN]` in episode file names to SxxEyy; nothing to do for S01 without offset,
// because Sonarr resolves plain absolute numbers of the first season correctly
func PlanRenames(files []string, season, offset int) []Rename {
	if season < 1 || (season == 1 && offset == 0) {
		return nil
	}
	var renames []Rename
	for _, p := range files {
		dir, base := path.Split(p)
		ext := strings.ToLower(path.Ext(base))
		stem := strings.TrimSuffix(base, path.Ext(base))
		if !mediaExtensions[ext] || seasonEpisodeRe.MatchString(stem) {
			continue
		}
		loc := episodeRe.FindStringSubmatchIndex(stem)
		if loc == nil {
			continue
		}
		num, err := strconv.Atoi(stem[loc[2]:loc[3]])
		if err != nil {
			continue
		}
		tag := fmt.Sprintf("S%02dE%02d", season, num+offset)
		renames = append(renames, Rename{Old: p, New: dir + stem[:loc[0]] + tag + stem[loc[1]:] + path.Ext(base)})
	}
	return renames
}
