package api

import (
	"aniliberty-indexer/internal/config"

	"fmt"
	"log"
	"sort"
	"sync"
	"time"
)

// Index of MAL ID -> AniLiberty release IDs, built from the release catalog
var (
	catalogMu    sync.RWMutex
	catalogByMAL = map[int][]int{}
	catalogReady bool
)

// Records a release seen in any API response so new releases are searchable by ID
// before the next full catalog refresh
func rememberRelease(r Release) {
	mal := r.MALID()
	if mal <= 0 || r.ID <= 0 {
		return
	}
	catalogMu.Lock()
	defer catalogMu.Unlock()
	for _, id := range catalogByMAL[mal] {
		if id == r.ID {
			return
		}
	}
	catalogByMAL[mal] = append(catalogByMAL[mal], r.ID)
}

// Returns AniLiberty release IDs for the given MAL IDs
func ReleaseIDsForMAL(malIDs []int) []int {
	catalogMu.RLock()
	defer catalogMu.RUnlock()
	seen := map[int]bool{}
	var out []int
	for _, mal := range malIDs {
		for _, id := range catalogByMAL[mal] {
			if !seen[id] {
				seen[id] = true
				out = append(out, id)
			}
		}
	}
	sort.Ints(out)
	return out
}

// Reports whether the full catalog index has been built at least once
func CatalogReady() bool {
	catalogMu.RLock()
	defer catalogMu.RUnlock()
	return catalogReady
}

// Walks the whole AniLiberty catalog (ids and MAL ids only) and rebuilds the index
func refreshCatalog() (int, error) {
	index := map[int][]int{}
	releases := 0
	for page, totalPages := 1, 1; page <= totalPages; page++ {
		pageUrl := fmt.Sprintf("%s/anime/catalog/releases?page=%d&limit=50&include=id,mal.id,shikimori.id", config.ANILIBERTY_API, page)
		var data catalogPage
		if err := FetchJSON(pageUrl, &data); err != nil {
			return 0, fmt.Errorf("catalog page %d: %w", page, err)
		}
		totalPages = data.Meta.Pagination.TotalPages
		for _, r := range data.Data {
			releases++
			mal := r.Mal.ID
			if mal <= 0 {
				mal = r.Shikimori.ID
			}
			if mal > 0 {
				index[mal] = append(index[mal], r.ID)
			}
		}
	}

	catalogMu.Lock()
	// Keep IDs remembered from live responses that the catalog does not list
	for mal, ids := range catalogByMAL {
		for _, id := range ids {
			found := false
			for _, x := range index[mal] {
				if x == id {
					found = true
					break
				}
			}
			if !found {
				index[mal] = append(index[mal], id)
			}
		}
	}
	catalogByMAL = index
	catalogReady = true
	catalogMu.Unlock()
	return releases, nil
}

// Builds the catalog index in the background and refreshes it periodically
func StartCatalogIndexer() {
	go func() {
		for {
			start := time.Now()
			if n, err := refreshCatalog(); err != nil {
				log.Printf("[catalog] index refresh failed: %v\n", err)
			} else {
				log.Printf("[catalog] indexed %d releases in %s\n", n, time.Since(start).Round(time.Millisecond))
			}
			time.Sleep(config.CATALOG_REFRESH_INTERVAL)
		}
	}()
}

// Fetches releases with their torrents by AniLiberty release ID
func FetchReleasesByIDs(ids []int, limit int) []CombinedItem {
	type result struct {
		rel ReleaseWithTorrents
		ok  bool
	}
	results := make([]result, len(ids))

	sem := make(chan struct{}, config.SEARCH_CONCURRENCY)
	var wg sync.WaitGroup
	for i, id := range ids {
		wg.Add(1)
		go func(i, id int) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			var rel ReleaseWithTorrents
			if err := FetchJSON(fmt.Sprintf("%s/anime/releases/%d", config.ANILIBERTY_API, id), &rel); err != nil {
				log.Printf("Failed to fetch release %d: %v\n", id, err)
				return
			}
			results[i] = result{rel: rel, ok: true}
		}(i, id)
	}
	wg.Wait()

	var items []CombinedItem
	for _, r := range results {
		if !r.ok {
			continue
		}
		rememberRelease(r.rel.Release)
		for _, t := range r.rel.Torrents {
			items = append(items, CombinedItem{Release: r.rel.Release, Torrent: t})
		}
	}
	if limit > 0 && len(items) > limit {
		items = items[:limit]
	}
	return items
}
