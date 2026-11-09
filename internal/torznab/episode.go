package torznab

import (
	"aniliberty-indexer/internal/api"
	"aniliberty-indexer/internal/config"
	"aniliberty-indexer/internal/torrentfile"

	"log"
	"path"
	"regexp"
	"strconv"
	"strings"
	"sync"
)

var (
	// Episode number in AniLiberty file names: Title_[01]_[HEVC].mkv
	fileEpisodeRe = regexp.MustCompile(`\[(\d{1,4})\]`)

	episodeMediaExtensions = map[string]bool{
		".mkv": true, ".mp4": true, ".avi": true, ".mka": true,
		".ass": true, ".ssa": true, ".srt": true,
	}
)

// AniLiberty episode number of a media file (`..._[03]_...mkv` -> 3)
func fileEpisode(name string) (int, bool) {
	base := path.Base(name)
	ext := strings.ToLower(path.Ext(base))
	if !episodeMediaExtensions[ext] {
		return 0, false
	}
	m := fileEpisodeRe.FindStringSubmatch(strings.TrimSuffix(base, path.Ext(base)))
	if m == nil {
		return 0, false
	}
	n, err := strconv.Atoi(m[1])
	if err != nil {
		return 0, false
	}
	return n, true
}

// Files of the requested TVDB episode; nil when the torrent has nothing else to skip
func pickEpisode(files []torrentfile.File, episode, offset int) *api.EpisodePick {
	want := episode - offset
	pick := &api.EpisodePick{Episode: episode}
	others := false
	for _, f := range files {
		n, ok := fileEpisode(f.Path)
		if ok && n == want {
			pick.Files = append(pick.Files, f.Index)
			pick.Size += f.Length
		} else if ok {
			others = true
		}
	}
	if len(pick.Files) == 0 || !others {
		return nil
	}
	return pick
}

// For an episode search, narrows multi-episode TV torrents down to the requested
// episode: the item is announced as a single episode with that file's size, and the
// download link carries the file indices so that the other files can be skipped.
// Items whose file list is unavailable or has no such episode are returned as is.
func SplitEpisode(items []api.CombinedItem, episode int, fetchFiles func(hash string) ([]torrentfile.File, error)) []api.CombinedItem {
	if episode <= 0 {
		return items
	}
	out := make([]api.CombinedItem, len(items))
	copy(out, items)

	concurrency := config.SEARCH_CONCURRENCY
	if concurrency < 1 {
		concurrency = 1
	}
	sem := make(chan struct{}, concurrency)
	var wg sync.WaitGroup
	for i := range out {
		item := out[i]
		if item.Release.Type.Value == "MOVIE" || item.Torrent.Hash == "" {
			continue
		}
		tv := resolveTV(item.Release)
		if eps := tvEpisodes(item.Torrent, tv); eps != "" && !strings.Contains(eps, "-") {
			continue // already a single episode
		}
		offset := 0
		if tv.Mapped {
			offset = tv.EpisodeOffset
		}

		wg.Add(1)
		go func(i int, hash string, offset int) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			files, err := fetchFiles(hash)
			if err != nil {
				log.Printf("[episode] file list of %s: %v\n", hash, err)
				return
			}
			out[i].Episode = pickEpisode(files, episode, offset)
		}(i, item.Torrent.Hash, offset)
	}
	wg.Wait()
	return out
}

// Formats file indices for the download link: 0,3
func FormatFileIndices(files []int) string {
	parts := make([]string, len(files))
	for i, f := range files {
		parts[i] = strconv.Itoa(f)
	}
	return strings.Join(parts, ",")
}

// Parses the files= value of a download link
func ParseFileIndices(s string) []int {
	var files []int
	for _, p := range strings.Split(s, ",") {
		if n, err := strconv.Atoi(strings.TrimSpace(p)); err == nil && n >= 0 {
			files = append(files, n)
		}
	}
	return files
}
