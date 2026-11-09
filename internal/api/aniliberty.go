package api

import (
	"aniliberty-indexer/internal/config"
	"aniliberty-indexer/internal/torrentfile"

	"errors"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"strings"
	"sync"
)

// Fetches the latest torrents from AniLiberty with pagination
func FetchLatestTorrents(limit int, page int) ([]CombinedItem, error) {
	apiUrl := fmt.Sprintf("%s/anime/torrents?page=%d&limit=%d", config.ANILIBERTY_API, page, limit)
	var data LatestTorrentsResponse
	if err := FetchJSON(apiUrl, &data); err != nil {
		return nil, err
	}

	items := make([]CombinedItem, 0, len(data.Data))
	for _, item := range data.Data {
		rememberRelease(item.Release)
		items = append(items, CombinedItem{
			Release: item.Release,
			Torrent: item.Torrent,
		})
	}
	return items, nil
}

// Searches releases by query, then fetches their torrents concurrently
func SearchTorrents(query string, limit int) ([]CombinedItem, error) {
	searchUrl := fmt.Sprintf("%s/app/search/releases?query=%s", config.ANILIBERTY_API, url.QueryEscape(query))
	var releases []Release
	if err := FetchJSON(searchUrl, &releases); err != nil {
		return nil, err
	}

	if len(releases) == 0 {
		return []CombinedItem{}, nil
	}
	for _, r := range releases {
		rememberRelease(r)
	}

	var results []CombinedItem
	concurrency := config.SEARCH_CONCURRENCY

	for i := 0; i < len(releases) && len(results) < limit; i += concurrency {
		end := i + concurrency
		if end > len(releases) {
			end = len(releases)
		}
		batch := releases[i:end]

		var wg sync.WaitGroup
		var mu sync.Mutex
		torrentsMap := make(map[int][]Torrent)

		for idx, release := range batch {
			wg.Add(1)
			go func(index int, rel Release) {
				defer wg.Done()
				torrentsUrl := fmt.Sprintf("%s/anime/torrents/release/%d", config.ANILIBERTY_API, rel.ID)
				var torrents []Torrent
				if err := FetchJSON(torrentsUrl, &torrents); err != nil {
					log.Printf("Failed to fetch torrents for release %d: %v\n", rel.ID, err)
					return
				}
				mu.Lock()
				torrentsMap[index] = torrents
				mu.Unlock()
			}(idx, release)
		}
		wg.Wait()

		for idx, release := range batch {
			if torrents, ok := torrentsMap[idx]; ok {
				for _, t := range torrents {
					results = append(results, CombinedItem{
						Release: release,
						Torrent: t,
					})
				}
			}
		}
	}

	if len(results) > limit {
		results = results[:limit]
	}
	return results, nil
}

// Looks up a torrent by infohash; found is false when AniLiberty does not know the hash
func FetchTorrentByHash(hash string) (item CombinedItem, found bool, err error) {
	apiUrl := fmt.Sprintf("%s/anime/torrents/%s", config.ANILIBERTY_API, url.PathEscape(hash))
	var data struct {
		Torrent
		Release Release `json:"release"`
	}
	if err := FetchJSON(apiUrl, &data); err != nil {
		var se *StatusError
		if errors.As(err, &se) && se.Code == http.StatusNotFound {
			return CombinedItem{}, false, nil
		}
		return CombinedItem{}, false, err
	}
	if data.Release.ID == 0 {
		return CombinedItem{}, false, nil
	}
	return CombinedItem{Release: data.Release, Torrent: data.Torrent}, true, nil
}

var (
	torrentFilesMu    sync.Mutex
	torrentFilesCache = map[string][]torrentfile.File{}
)

// Downloads a .torrent from AniLiberty and returns its file list; cached by infohash
func FetchTorrentFiles(hash string) ([]torrentfile.File, error) {
	key := strings.ToLower(hash)
	torrentFilesMu.Lock()
	files, ok := torrentFilesCache[key]
	torrentFilesMu.Unlock()
	if ok {
		return files, nil
	}

	apiUrl := fmt.Sprintf("%s/anime/torrents/%s/file", config.ANILIBERTY_API, url.PathEscape(hash))
	data, err := FetchBytes(apiUrl)
	if err != nil {
		return nil, err
	}
	files, err = torrentfile.Files(data)
	if err != nil {
		return nil, err
	}

	torrentFilesMu.Lock()
	torrentFilesCache[key] = files
	torrentFilesMu.Unlock()
	return files, nil
}
